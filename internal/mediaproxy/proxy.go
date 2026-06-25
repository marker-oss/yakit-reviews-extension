// internal/mediaproxy/proxy.go
package mediaproxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"sync"

	"reviews/internal/config"

	// Decoders registered with image.Decode. WB/YM serve .webp, so it is
	// required; png/gif/jpeg cover the rest. Anything still undecodable falls
	// through to streaming the original bytes (see process).
	_ "golang.org/x/image/webp"
)

// HTTPGetter fetches a remote image. Injected for testing.
type HTTPGetter func(url string) (*http.Response, error)

type handler struct {
	cfg   config.MediaConfig
	fetch HTTPGetter
	det   *detector

	mu    sync.Mutex
	cache map[string][]byte
	order []string
}

// NewHandler builds the face-blurring image proxy. It serves
// GET /media?u=<encoded https url>: validate against the allowlist, fetch,
// decode, blur detected faces, re-encode as JPEG, and stream with an in-memory
// LRU cache. A nil fetch defaults to a plain http.Client.Get.
func NewHandler(cfg config.MediaConfig, fetch HTTPGetter) (http.Handler, error) {
	det, err := newDetector()
	if err != nil {
		return nil, err
	}
	if fetch == nil {
		client := &http.Client{}
		fetch = func(u string) (*http.Response, error) { return client.Get(u) }
	}
	return &handler{cfg: cfg, fetch: fetch, det: det, cache: map[string][]byte{}}, nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("u")
	if raw == "" {
		http.Error(w, "missing u", http.StatusBadRequest)
		return
	}
	if !HostAllowed(raw, h.cfg.Allowlist) {
		http.Error(w, "host not allowed", http.StatusForbidden)
		return
	}

	key := hashKey(raw)
	if blob, ok := h.get(key); ok {
		writeImage(w, blob)
		return
	}

	resp, err := h.fetch(raw)
	if err != nil || resp.StatusCode != 200 {
		if resp != nil {
			resp.Body.Close()
		}
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, h.cfg.MaxBytes))
	if err != nil {
		http.Error(w, "read error", http.StatusBadGateway)
		return
	}

	out := h.process(body)
	h.put(key, out)
	writeImage(w, out)
}

// process decodes, blurs detected faces, re-encodes as JPEG. On any decode
// failure it returns the original bytes unchanged (image still renders).
func (h *handler) process(body []byte) []byte {
	src, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return body
	}
	rgba := toRGBA(src)
	for _, box := range h.det.detectFaces(rgba) {
		// pad the box ~20% and blur generously
		pad := box.Dx() / 5
		region := image.Rect(box.Min.X-pad, box.Min.Y-pad, box.Max.X+pad, box.Max.Y+pad)
		blurRegion(rgba, region, 8)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, rgba, &jpeg.Options{Quality: 82}); err != nil {
		return body
	}
	return buf.Bytes()
}

func toRGBA(src image.Image) *image.RGBA {
	if r, ok := src.(*image.RGBA); ok {
		return r
	}
	b := src.Bounds()
	dst := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(x, y, src.At(x, y))
		}
	}
	return dst
}

func writeImage(w http.ResponseWriter, blob []byte) {
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(blob)
}

func hashKey(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func (h *handler) get(key string) ([]byte, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	b, ok := h.cache[key]
	return b, ok
}

func (h *handler) put(key string, blob []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, exists := h.cache[key]; !exists {
		h.order = append(h.order, key)
	}
	h.cache[key] = blob
	for len(h.order) > h.cfg.CacheEntries && h.cfg.CacheEntries > 0 {
		oldest := h.order[0]
		h.order = h.order[1:]
		delete(h.cache, oldest)
	}
}
