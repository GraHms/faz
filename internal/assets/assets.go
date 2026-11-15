// Package assets loads the client's files and inlines them into the page so
// the deliverable stays a single self contained file.
package assets

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Asset is one client file.
type Asset struct {
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	Size      int64  `json:"size"`
	Data      []byte `json:"-"`
}

var mediaTypes = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".webp": "image/webp",
	".gif": "image/gif", ".svg": "image/svg+xml", ".avif": "image/avif", ".ico": "image/x-icon",
	".pdf": "application/pdf", ".txt": "text/plain", ".md": "text/markdown",
}

// Load reads every regular file directly inside dir. Files over maxBytes are
// listed with no data, so the brief can still mention them.
func Load(dir string, maxBytes int64) ([]Asset, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []Asset
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		a := Asset{Name: e.Name(), Size: info.Size(), MediaType: mediaTypes[strings.ToLower(filepath.Ext(e.Name()))]}
		if a.MediaType == "" {
			a.MediaType = "application/octet-stream"
		}
		if info.Size() <= maxBytes {
			if a.Data, err = os.ReadFile(filepath.Join(dir, e.Name())); err != nil {
				return nil, err
			}
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// VisionReady reports whether the model can look at this asset directly.
func (a Asset) VisionReady() bool {
	switch a.MediaType {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		return len(a.Data) > 0 && len(a.Data) <= 3_500_000
	}
	return false
}

// Inline replaces assets/<name> references with data URIs for every asset
// at or under maxBytes. It returns the names it could not inline.
func Inline(html string, list []Asset, maxBytes int64) (string, []string) {
	var skipped []string
	for _, a := range list {
		// Longest first, so "./assets/x" is not half replaced by "assets/x".
		refs := []string{"./assets/" + a.Name, "assets/" + a.Name, "assets/" + url.PathEscape(a.Name)}
		used := false
		for _, r := range refs {
			if strings.Contains(html, r) {
				used = true
			}
		}
		if !used {
			continue
		}
		if len(a.Data) == 0 || int64(len(a.Data)) > maxBytes {
			skipped = append(skipped, a.Name)
			continue
		}
		uri := fmt.Sprintf("data:%s;base64,%s", a.MediaType, base64.StdEncoding.EncodeToString(a.Data))
		for _, r := range refs {
			html = strings.ReplaceAll(html, r, uri)
		}
	}
	return html, skipped
}

// Listing describes the assets for a prompt.
func Listing(list []Asset) string {
	if len(list) == 0 {
		return "No files were attached."
	}
	var b strings.Builder
	for _, a := range list {
		fmt.Fprintf(&b, "- %s (%s, %d KB)\n", a.Name, a.MediaType, (a.Size+1023)/1024)
	}
	return b.String()
}
