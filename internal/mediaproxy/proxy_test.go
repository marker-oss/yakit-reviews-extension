// internal/mediaproxy/proxy_test.go
package mediaproxy

import (
	"bytes"
	"image"
	_ "image/jpeg"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"reviews/internal/config"
)

func jpegBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newTestHandler(t *testing.T, body []byte) http.Handler {
	t.Helper()
	cfg := config.MediaConfig{Allowlist: []string{"cdn.test"}, MaxBytes: 8 << 20, CacheEntries: 8}
	h, err := NewHandler(cfg, func(u string) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": {"image/jpeg"}},
			Body:       io.NopCloser(bytes.NewReader(body)),
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestProxyRejectsDisallowedHost(t *testing.T) {
	h := newTestHandler(t, jpegBytes(t))
	req := httptest.NewRequest("GET", "/media?u="+url.QueryEscape("https://evil.com/x.jpg"), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestProxyServesAllowedImage(t *testing.T) {
	h := newTestHandler(t, jpegBytes(t))
	req := httptest.NewRequest("GET", "/media?u="+url.QueryEscape("https://cdn.test/x.jpg"), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("missing no-referrer policy")
	}
	if _, _, err := image.Decode(rec.Body); err != nil {
		t.Errorf("response is not a decodable image: %v", err)
	}
}

func TestProxyMissingURL(t *testing.T) {
	h := newTestHandler(t, jpegBytes(t))
	req := httptest.NewRequest("GET", "/media", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
