// Package r2 is a small client for Cloudflare R2 through its S3 compatible
// API: put, get, head, delete, presigned GET URLs and directory uploads.
// It signs requests with AWS Signature Version 4 using only the standard
// library.
package r2

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ErrNotFound is returned when an object does not exist.
var ErrNotFound = errors.New("r2: object not found")

// Client talks to one bucket.
type Client struct {
	Endpoint  string // https://<account id>.r2.cloudflarestorage.com, or a test server
	Bucket    string
	Region    string // "auto" for R2
	AccessKey string
	SecretKey string
	HTTP      *http.Client
	Now       func() time.Time // for tests
}

// New returns a client for an R2 bucket in the given Cloudflare account.
func New(accountID, bucket, accessKey, secretKey string) *Client {
	return &Client{
		Endpoint:  "https://" + accountID + ".r2.cloudflarestorage.com",
		Bucket:    bucket,
		Region:    "auto",
		AccessKey: accessKey,
		SecretKey: secretKey,
	}
}

// Object is a stored object's body and headers.
type Object struct {
	Body        []byte
	ContentType string
	ETag        string
}

// Put stores body under key.
func (c *Client) Put(ctx context.Context, key string, body []byte, contentType, cacheControl string) error {
	req, err := c.request(ctx, http.MethodPut, key, body)
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if cacheControl != "" {
		req.Header.Set("Cache-Control", cacheControl)
	}
	_, err = c.do(req, body)
	return err
}

// Get fetches an object.
func (c *Client) Get(ctx context.Context, key string) (*Object, error) {
	req, err := c.request(ctx, http.MethodGet, key, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req, nil)
	if err != nil {
		return nil, err
	}
	return &Object{Body: resp.body, ContentType: resp.header.Get("Content-Type"), ETag: resp.header.Get("ETag")}, nil
}

// Exists reports whether an object exists.
func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	req, err := c.request(ctx, http.MethodHead, key, nil)
	if err != nil {
		return false, err
	}
	_, err = c.do(req, nil)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// Delete removes an object. Deleting a missing object is not an error.
func (c *Client) Delete(ctx context.Context, key string) error {
	req, err := c.request(ctx, http.MethodDelete, key, nil)
	if err != nil {
		return err
	}
	_, err = c.do(req, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// Copy copies an object within the bucket by reading and writing it. Site
// pages are small, so this keeps the client simple.
func (c *Client) Copy(ctx context.Context, from, to, cacheControl string) error {
	obj, err := c.Get(ctx, from)
	if err != nil {
		return err
	}
	return c.Put(ctx, to, obj.Body, obj.ContentType, cacheControl)
}

// PutDir uploads every file under dir to prefix/<relative path>, skipping
// names that start with a dot. It returns the keys written.
func (c *Client) PutDir(ctx context.Context, prefix, dir string) ([]string, error) {
	var keys []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), ".") && path != dir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		key := strings.TrimSuffix(prefix, "/") + "/" + filepath.ToSlash(rel)
		if err := c.Put(ctx, key, body, ContentType(path), ""); err != nil {
			return fmt.Errorf("upload %s: %w", rel, err)
		}
		keys = append(keys, key)
		return nil
	})
	return keys, err
}

// ContentType guesses a content type from a file name.
func ContentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".json":
		return "application/json"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	}
	if t := mime.TypeByExtension(filepath.Ext(name)); t != "" {
		return t
	}
	return "application/octet-stream"
}

// PresignGet returns a URL that fetches key without credentials until ttl
// passes. Use it to show previews from R2's own origin, never the app's.
func (c *Client) PresignGet(key string, ttl time.Duration) (string, error) {
	u, err := c.objectURL(key)
	if err != nil {
		return "", err
	}
	now := c.now()
	amzDate := now.Format("20060102T150405Z")
	scope := c.scope(now)
	q := url.Values{}
	q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	q.Set("X-Amz-Credential", c.AccessKey+"/"+scope)
	q.Set("X-Amz-Date", amzDate)
	q.Set("X-Amz-Expires", strconv.Itoa(int(ttl.Seconds())))
	q.Set("X-Amz-SignedHeaders", "host")
	canonical := strings.Join([]string{
		http.MethodGet,
		u.EscapedPath(),
		canonicalQuery(q),
		"host:" + u.Host + "\n",
		"host",
		"UNSIGNED-PAYLOAD",
	}, "\n")
	sig := c.signature(now, amzDate, scope, canonical)
	u.RawQuery = canonicalQuery(q) + "&X-Amz-Signature=" + sig
	return u.String(), nil
}

// ---- signing ----

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}

func (c *Client) objectURL(key string) (*url.URL, error) {
	if key == "" || strings.HasPrefix(key, "/") {
		return nil, fmt.Errorf("r2: invalid key %q", key)
	}
	u, err := url.Parse(strings.TrimRight(c.Endpoint, "/"))
	if err != nil {
		return nil, err
	}
	raw := "/" + c.Bucket + "/" + key
	u.Path = raw
	u.RawPath = uriEncode(raw, false)
	return u, nil
}

func (c *Client) request(ctx context.Context, method, key string, body []byte) (*http.Request, error) {
	u, err := c.objectURL(key)
	if err != nil {
		return nil, err
	}
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.ContentLength = int64(len(body))
	}
	return req, nil
}

// Sign adds SigV4 headers to req. Every header already set on the request
// is signed, along with host, x-amz-date and x-amz-content-sha256.
func (c *Client) Sign(req *http.Request, body []byte) {
	now := c.now()
	amzDate := now.Format("20060102T150405Z")
	sum := sha256.Sum256(body)
	payload := hex.EncodeToString(sum[:])
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payload)

	headers := map[string]string{"host": req.URL.Host}
	for k, v := range req.Header {
		lk := strings.ToLower(k)
		if lk == "authorization" || lk == "user-agent" || lk == "expect" {
			continue
		}
		headers[lk] = strings.Join(strings.Fields(strings.Join(v, ",")), " ")
	}
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	var ch strings.Builder
	for _, k := range names {
		ch.WriteString(k + ":" + headers[k] + "\n")
	}
	signed := strings.Join(names, ";")
	canonical := strings.Join([]string{
		req.Method,
		req.URL.EscapedPath(),
		canonicalQuery(req.URL.Query()),
		ch.String(),
		signed,
		payload,
	}, "\n")
	scope := c.scope(now)
	sig := c.signature(now, amzDate, scope, canonical)
	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		c.AccessKey, scope, signed, sig))
}

func (c *Client) scope(now time.Time) string {
	return now.Format("20060102") + "/" + c.Region + "/s3/aws4_request"
}

func (c *Client) signature(now time.Time, amzDate, scope, canonical string) string {
	h := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(h[:])
	k := hmacSHA256([]byte("AWS4"+c.SecretKey), now.Format("20060102"))
	k = hmacSHA256(k, c.Region)
	k = hmacSHA256(k, "s3")
	k = hmacSHA256(k, "aws4_request")
	return hex.EncodeToString(hmacSHA256(k, toSign))
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

func canonicalQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vals := append([]string(nil), q[k]...)
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, uriEncode(k, true)+"="+uriEncode(v, true))
		}
	}
	return strings.Join(parts, "&")
}

// uriEncode percent encodes everything except unreserved characters, and
// slashes too when encodeSlash is set, as SigV4 requires.
func uriEncode(s string, encodeSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch >= 'A' && ch <= 'Z', ch >= 'a' && ch <= 'z', ch >= '0' && ch <= '9', ch == '-', ch == '_', ch == '.', ch == '~':
			b.WriteByte(ch)
		case ch == '/' && !encodeSlash:
			b.WriteByte(ch)
		default:
			fmt.Fprintf(&b, "%%%02X", ch)
		}
	}
	return b.String()
}

type response struct {
	header http.Header
	body   []byte
}

func (c *Client) do(req *http.Request, body []byte) (*response, error) {
	c.Sign(req, body)
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("%w: %s", ErrNotFound, req.URL.Path)
	case resp.StatusCode >= 300:
		return nil, fmt.Errorf("r2: %s %s: status %d: %s", req.Method, req.URL.Path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return &response{header: resp.Header, body: b}, nil
}
