// internal/mediaproxy/proxy_test.go
package mediaproxy

import (
	"bytes"
	"image"
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

func TestProxyUpstreamErrorReturns502(t *testing.T) {
	cfg := config.MediaConfig{Allowlist: []string{"cdn.test"}, MaxBytes: 8 << 20, CacheEntries: 8}
	h, err := NewHandler(cfg, func(u string) (*http.Response, error) {
		return &http.Response{
			StatusCode: 503,
			Header:     http.Header{},
			Body:       io.NopCloser(bytes.NewReader(nil)),
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/media?u="+url.QueryEscape("https://cdn.test/x.jpg"), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}

func TestProxyCacheHitReusesResult(t *testing.T) {
	calls := 0
	cfg := config.MediaConfig{Allowlist: []string{"cdn.test"}, MaxBytes: 8 << 20, CacheEntries: 8}
	img := jpegBytes(t)
	h, err := NewHandler(cfg, func(u string) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": {"image/jpeg"}},
			Body:       io.NopCloser(bytes.NewReader(img)),
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	target := "/media?u=" + url.QueryEscape("https://cdn.test/cached.jpg")
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", target, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("request %d: status = %d, want 200", i+1, rec.Code)
		}
	}
	if calls != 1 {
		t.Errorf("fetch called %d times, want 1 (second should be cache hit)", calls)
	}
}

func TestRedirectGuard(t *testing.T) {
	allowlist := []string{"cdn.test"}
	guard := redirectGuard(allowlist)

	reqTo := func(rawURL string) *http.Request {
		r := httptest.NewRequest("GET", rawURL, nil)
		return r
	}

	// Allowed host within the hop cap: nil.
	if err := guard(reqTo("https://cdn.test/a.jpg"), nil); err != nil {
		t.Errorf("allowed host should pass, got %v", err)
	}
	// Subdomain of an allowlisted suffix is allowed too.
	if err := guard(reqTo("https://img.cdn.test/a.jpg"), nil); err != nil {
		t.Errorf("allowed subdomain should pass, got %v", err)
	}

	// Disallowed host (classic SSRF target): error.
	if err := guard(reqTo("https://169.254.169.254/latest/meta-data"), nil); err == nil {
		t.Error("redirect to disallowed host must error")
	}
	// Non-allowlisted public host: error.
	if err := guard(reqTo("https://evil.com/x.jpg"), nil); err == nil {
		t.Error("redirect to non-allowlisted host must error")
	}

	// Exceeding the hop cap errors even for an allowlisted target.
	via := make([]*http.Request, maxRedirects)
	if err := guard(reqTo("https://cdn.test/a.jpg"), via); err == nil {
		t.Errorf("exceeding %d hops must error", maxRedirects)
	}
}
