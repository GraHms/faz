package r2

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Reference values come from botocore (AWS's own Python signer) for the same
// credentials, clock and request: see sigv4_vectors.py in this folder.
func testClient() *Client {
	c := New("acct123", "faz-sites", "AKIDEXAMPLE", "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY")
	c.Now = func() time.Time { return time.Date(2026, 11, 15, 8, 30, 0, 0, time.UTC) }
	return c
}

func TestSignMatchesBotocore(t *testing.T) {
	c := testClient()
	body := []byte("<!DOCTYPE html><p>olá</p>")
	req, err := c.request(context.Background(), http.MethodPut, "sites/abc/v1/index page.html", body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "text/html; charset=utf-8")
	c.Sign(req, body)
	want := "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20261115/auto/s3/aws4_request, SignedHeaders=content-type;host;x-amz-content-sha256;x-amz-date, Signature=3783fff83ddd76a578e40e9390ea3ac24675e2fa1a632706e5cc5c809cd84f30"
	if got := req.Header.Get("Authorization"); got != want {
		t.Fatalf("authorization\n got %s\nwant %s", got, want)
	}
	if got := req.Header.Get("X-Amz-Content-Sha256"); got != "e7dbc35963c4ae44938b587b8b9950ea10d8102ae4b91dd5b9da2e0510409eb2" {
		t.Fatalf("payload hash %s", got)
	}
}

func TestPresignMatchesBotocore(t *testing.T) {
	got, err := testClient().PresignGet("sites/abc/v1/index.html", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://acct123.r2.cloudflarestorage.com/faz-sites/sites/abc/v1/index.html?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=AKIDEXAMPLE%2F20261115%2Fauto%2Fs3%2Faws4_request&X-Amz-Date=20261115T083000Z&X-Amz-Expires=900&X-Amz-SignedHeaders=host&X-Amz-Signature=e9e88cfe89de264c417c3d8c22e85429d5183afc1d50cf74fbb50e85412c84db"
	if got != want {
		t.Fatalf("presigned url\n got %s\nwant %s", got, want)
	}
}

// TestAgainstS3Server runs against any S3 compatible server named in
// FAZ_TEST_S3 (for example moto_server on http://127.0.0.1:9000).
func TestAgainstS3Server(t *testing.T) {
	endpoint := os.Getenv("FAZ_TEST_S3")
	if endpoint == "" {
		t.Skip("set FAZ_TEST_S3 to run against an S3 compatible server")
	}
	c := &Client{Endpoint: endpoint, Bucket: "faz-test", Region: "us-east-1", AccessKey: "test", SecretKey: "test"}
	ctx := context.Background()
	if err := CreateBucket(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := c.Put(ctx, "a/b.html", []byte("<p>hi</p>"), "text/html; charset=utf-8", "no-cache"); err != nil {
		t.Fatal(err)
	}
	obj, err := c.Get(ctx, "a/b.html")
	if err != nil || string(obj.Body) != "<p>hi</p>" || !strings.HasPrefix(obj.ContentType, "text/html") {
		t.Fatalf("get: %v %+v", err, obj)
	}
	if ok, err := c.Exists(ctx, "a/missing.html"); ok || err != nil {
		t.Fatalf("exists on missing key: %v %v", ok, err)
	}
	if _, err := c.Get(ctx, "a/missing.html"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("get missing: %v", err)
	}
	url, err := c.PresignGet("a/b.html", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(b) != "<p>hi</p>" {
		t.Fatalf("presigned get returned %d %q", resp.StatusCode, b)
	}
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "screenshots"), 0o755)
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "screenshots", "s.jpg"), []byte("y"), 0o644)
	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("z"), 0o644)
	keys, err := c.PutDir(ctx, "v1", dir)
	if err != nil || len(keys) != 2 {
		t.Fatalf("putdir: %v %v", keys, err)
	}
	if err := c.Delete(ctx, "a/b.html"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := c.Exists(ctx, "a/b.html"); ok {
		t.Fatal("object still exists after delete")
	}
}
