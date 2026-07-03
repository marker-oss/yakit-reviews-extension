package installer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"
)

type HTTPAdminClient struct {
	client *http.Client
	base   string
	csrf   string
	// pollInterval spaces out status polls for the background catalog
	// refresh; zero means the 3s default.
	pollInterval time.Duration
}

func NewHTTPAdminClient(baseURL string) (*HTTPAdminClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &HTTPAdminClient{
		client: &http.Client{Jar: jar, Timeout: 30 * time.Second},
		base:   strings.TrimRight(baseURL, "/"),
	}, nil
}

func (c *HTTPAdminClient) WaitHealth(ctx context.Context, baseURL string) error {
	if baseURL != "" {
		c.base = strings.TrimRight(baseURL, "/")
	}
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	var lastErr error
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/healthz", nil)
		if err != nil {
			return err
		}
		res, err := c.client.Do(req)
		if err == nil && res.Body != nil {
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
		}
		if err == nil && res.StatusCode >= 200 && res.StatusCode < 300 {
			return nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("healthz returned HTTP %d", res.StatusCode)
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return lastErr
			}
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *HTTPAdminClient) SetupAdmin(ctx context.Context, login, password string) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(map[string]string{"login": login, "password": password}); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/admin/api/setup", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusConflict {
		_, _ = io.Copy(io.Discard, res.Body)
		return nil
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return responseError(res)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	return nil
}

func (c *HTTPAdminClient) Login(ctx context.Context, login, password string) error {
	if err := c.postJSON(ctx, "/admin/api/login", map[string]string{"login": login, "password": password}, false); err != nil {
		return err
	}
	return c.fetchCSRF(ctx)
}

func (c *HTTPAdminClient) SaveMarketplaceCredentials(ctx context.Context, request CredentialRequest) error {
	body := map[string]any{
		"enabled": request.Enabled,
		"values":  request.Values,
	}
	return c.putJSON(ctx, "/admin/api/marketplaces/"+request.ID+"/credentials", body, true)
}

// RefreshSiteLinks kicks off the background catalog crawl and waits for it to
// finish: the endpoint returns 202 immediately, progress is exposed on the
// same path via GET. A large catalog takes many minutes, so completion must be
// awaited before the publish step reads the crawled links.
func (c *HTTPAdminClient) RefreshSiteLinks(ctx context.Context) error {
	if err := c.postJSON(ctx, "/admin/api/site-links/refresh", map[string]string{}, true); err != nil {
		return err
	}
	interval := c.pollInterval
	if interval <= 0 {
		interval = 3 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		status, err := c.refreshStatus(ctx)
		if err != nil {
			return err
		}
		switch status.State {
		case "done":
			return nil
		case "error":
			if status.Error != "" {
				return errors.New(status.Error)
			}
			return errors.New("catalog refresh failed")
		}
	}
}

func (c *HTTPAdminClient) refreshStatus(ctx context.Context) (refreshStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/admin/api/site-links/refresh", nil)
	if err != nil {
		return refreshStatus{}, err
	}
	res, err := c.client.Do(req)
	if err != nil {
		return refreshStatus{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return refreshStatus{}, responseError(res)
	}
	var status refreshStatus
	if err := json.NewDecoder(res.Body).Decode(&status); err != nil {
		return refreshStatus{}, err
	}
	return status, nil
}

type refreshStatus struct {
	State   string `json:"state"`
	Total   int    `json:"total"`
	Crawled int    `json:"crawled"`
	Error   string `json:"error"`
}

func (c *HTTPAdminClient) PublishReviews(ctx context.Context) error {
	return c.postJSON(ctx, "/admin/api/reviews/publish", map[string]string{}, true)
}

func (c *HTTPAdminClient) fetchCSRF(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/admin/api/csrf", nil)
	if err != nil {
		return err
	}
	res, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return responseError(res)
	}
	var payload struct {
		Token string `json:"csrf_token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return err
	}
	if payload.Token == "" {
		return errors.New("empty CSRF token")
	}
	c.csrf = payload.Token
	return nil
}

func (c *HTTPAdminClient) postJSON(ctx context.Context, path string, body any, csrf bool) error {
	return c.doJSON(ctx, http.MethodPost, path, body, csrf)
}

func (c *HTTPAdminClient) putJSON(ctx context.Context, path string, body any, csrf bool) error {
	return c.doJSON(ctx, http.MethodPut, path, body, csrf)
}

func (c *HTTPAdminClient) doJSON(ctx context.Context, method, path string, body any, csrf bool) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if csrf {
		if c.csrf == "" {
			return errors.New("missing CSRF token; login first")
		}
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	res, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return responseError(res)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	return nil
}

func responseError(res *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && payload.Error != "" {
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, payload.Error)
	}
	if len(body) > 0 {
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	return fmt.Errorf("HTTP %d", res.StatusCode)
}
