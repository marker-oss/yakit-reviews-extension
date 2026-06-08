package site

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type ProductLink struct {
	SellerArticle string `json:"sellerArticle"`
	URL           string `json:"url"`
	Title         string `json:"title,omitempty"`
}

func DiscoverShegidaProductLinks(ctx context.Context, sitemapURL string, client *http.Client) ([]ProductLink, error) {
	if client == nil {
		client = http.DefaultClient
	}
	urls, err := fetchProductURLs(ctx, client, sitemapURL)
	if err != nil {
		return nil, err
	}

	jobs := make(chan string)
	results := make(chan ProductLink)
	errs := make(chan error, 1)
	var wg sync.WaitGroup

	workerCount := 4
	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for productURL := range jobs {
				link, err := fetchProductLink(ctx, client, productURL)
				if err != nil {
					select {
					case errs <- err:
					default:
					}
					continue
				}
				if link.SellerArticle != "" {
					results <- link
				}
				time.Sleep(120 * time.Millisecond)
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, productURL := range urls {
			select {
			case <-ctx.Done():
				return
			case jobs <- productURL:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	byArticle := make(map[string]ProductLink)
	for link := range results {
		if _, exists := byArticle[link.SellerArticle]; !exists {
			byArticle[link.SellerArticle] = link
		}
	}

	select {
	case err := <-errs:
		if len(byArticle) == 0 {
			return nil, err
		}
	default:
	}

	links := make([]ProductLink, 0, len(byArticle))
	for _, link := range byArticle {
		links = append(links, link)
	}
	sort.Slice(links, func(i, j int) bool {
		return links[i].SellerArticle < links[j].SellerArticle
	})
	return links, nil
}

func EncodeProductLinks(w io.Writer, links []ProductLink) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(links)
}

func LoadProductLinkMap(r io.Reader) (map[string]string, error) {
	var links []ProductLink
	if err := json.NewDecoder(r).Decode(&links); err != nil {
		return nil, err
	}
	byArticle := make(map[string]string, len(links))
	for _, link := range links {
		if link.SellerArticle != "" && link.URL != "" {
			byArticle[link.SellerArticle] = link.URL
		}
	}
	return byArticle, nil
}

func fetchProductURLs(ctx context.Context, client *http.Client, sitemapURL string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sitemapURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch sitemap: status %d", resp.StatusCode)
	}

	var sitemap struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&sitemap); err != nil {
		return nil, err
	}

	var urls []string
	for _, item := range sitemap.URLs {
		if strings.Contains(item.Loc, "/products/") {
			urls = append(urls, strings.TrimSpace(item.Loc))
		}
	}
	return urls, nil
}

func fetchProductLink(ctx context.Context, client *http.Client, productURL string) (ProductLink, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, productURL, nil)
	if err != nil {
		return ProductLink{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return ProductLink{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ProductLink{}, fmt.Errorf("fetch product %s: status %d", productURL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2_500_000))
	if err != nil {
		return ProductLink{}, err
	}
	html := string(body)
	return ProductLink{
		SellerArticle: extractArticle(html),
		URL:           productURL,
		Title:         extractTitle(html),
	}, nil
}

func extractArticle(html string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?is)"sku"\s*:\s*"([^"]+)"`),
		regexp.MustCompile(`(?is)Артикул\s*</[^>]+>\s*<[^>]+>\s*([^<]+)`),
		regexp.MustCompile(`(?is)Артикул\s+([0-9A-Za-zА-Яа-яЁё/_\-.]+)`),
	}
	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(html)
		if len(match) > 1 {
			return strings.TrimSpace(htmlUnescape(match[1]))
		}
	}
	return ""
}

func extractTitle(html string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?is)"name"\s*:\s*"([^"]+)"`),
		regexp.MustCompile(`(?is)<h1[^>]*>(.*?)</h1>`),
	}
	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(html)
		if len(match) > 1 {
			return strings.TrimSpace(stripTags(htmlUnescape(match[1])))
		}
	}
	return ""
}

func stripTags(value string) string {
	return regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(value, "")
}

func htmlUnescape(value string) string {
	replacer := strings.NewReplacer(
		"&amp;", "&",
		"&quot;", `"`,
		"&#039;", "'",
		"&lt;", "<",
		"&gt;", ">",
		"&nbsp;", " ",
	)
	return replacer.Replace(value)
}
