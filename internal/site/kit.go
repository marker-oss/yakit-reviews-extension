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

// DiscoverKitProductLinks crawls a Yandex Kit shop sitemap and maps each
// product page URL to its seller article (SKU). On context expiry the links
// crawled so far are returned together with the context error, so callers can
// persist partial progress instead of silently losing it.
func DiscoverKitProductLinks(ctx context.Context, sitemapURL string, client *http.Client) ([]ProductLink, error) {
	urls, err := FetchProductURLs(ctx, client, sitemapURL)
	if err != nil {
		return nil, err
	}
	return CrawlProductLinks(ctx, client, urls, nil)
}

// CrawlProductLinks fetches every product URL and extracts its seller article.
// progress (optional) is called from a single goroutine after each URL is
// processed with the number of URLs handled so far. When ctx ends early the
// links collected so far are returned alongside ctx.Err().
func CrawlProductLinks(ctx context.Context, client *http.Client, urls []string, progress func(done int)) ([]ProductLink, error) {
	if client == nil {
		client = http.DefaultClient
	}

	type crawlResult struct {
		link ProductLink
		err  error
	}
	jobs := make(chan string)
	results := make(chan crawlResult)
	var wg sync.WaitGroup

	workerCount := 4
	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for productURL := range jobs {
				link, err := fetchProductLink(ctx, client, productURL)
				results <- crawlResult{link: link, err: err}
				select {
				case <-ctx.Done():
				case <-time.After(120 * time.Millisecond):
				}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, productURL := range urls {
			if ctx.Err() != nil {
				return
			}
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

	// Keep every product URL (no collapsing by article): many product pages
	// (colours/sizes) share one seller article, and the embed loader must be
	// able to resolve ANY visited product path to its article.
	var links []ProductLink
	var firstErr error
	done := 0
	for result := range results {
		done++
		if result.err != nil {
			if firstErr == nil {
				firstErr = result.err
			}
		} else if result.link.SellerArticle != "" {
			links = append(links, result.link)
		}
		if progress != nil {
			progress(done)
		}
	}

	sortProductLinks(links)
	if ctx.Err() != nil {
		return links, ctx.Err()
	}
	if len(links) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return links, nil
}

// NewProductURLs returns the sitemap URLs that are not present in the known
// (already crawled) link list, preserving sitemap order. Used for incremental
// catalog refreshes: only newly added products are fetched.
func NewProductURLs(sitemapURLs []string, known []ProductLink) []string {
	seen := make(map[string]bool, len(known))
	for _, link := range known {
		if link.URL != "" {
			seen[link.URL] = true
		}
	}
	var fresh []string
	for _, url := range sitemapURLs {
		if !seen[url] {
			fresh = append(fresh, url)
		}
	}
	return fresh
}

// MergeProductLinks combines already-known links with newly crawled ones:
// known entries whose URL is still in the sitemap are kept, crawled entries
// win on URL collision, and URLs gone from the sitemap are pruned. The result
// is sorted like DiscoverKitProductLinks output.
func MergeProductLinks(known []ProductLink, sitemapURLs []string, crawled []ProductLink) []ProductLink {
	inSitemap := make(map[string]bool, len(sitemapURLs))
	for _, url := range sitemapURLs {
		inSitemap[url] = true
	}
	byURL := make(map[string]ProductLink, len(known)+len(crawled))
	for _, link := range known {
		if inSitemap[link.URL] {
			byURL[link.URL] = link
		}
	}
	for _, link := range crawled {
		if inSitemap[link.URL] {
			byURL[link.URL] = link
		}
	}
	links := make([]ProductLink, 0, len(byURL))
	for _, link := range byURL {
		links = append(links, link)
	}
	sortProductLinks(links)
	return links
}

func sortProductLinks(links []ProductLink) {
	sort.Slice(links, func(i, j int) bool {
		if links[i].URL != links[j].URL {
			return links[i].URL < links[j].URL
		}
		return links[i].SellerArticle < links[j].SellerArticle
	})
}

func EncodeProductLinks(w io.Writer, links []ProductLink) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(links)
}

// LoadProductLinks decodes the full crawled product-link list. Unlike
// LoadProductLinkMap (article→URL, one entry per article), this preserves every
// product URL so callers can build a complete path→article lookup.
func LoadProductLinks(r io.Reader) ([]ProductLink, error) {
	var links []ProductLink
	if err := json.NewDecoder(r).Decode(&links); err != nil {
		return nil, err
	}
	return links, nil
}

func LoadProductLinkMap(r io.Reader) (map[string]string, error) {
	links, err := LoadProductLinks(r)
	if err != nil {
		return nil, err
	}
	return ProductLinkMap(links), nil
}

// ProductLinkMap builds an article→URL lookup from a crawled product-link list,
// keeping one entry per article and skipping entries missing an article or URL.
func ProductLinkMap(links []ProductLink) map[string]string {
	byArticle := make(map[string]string, len(links))
	for _, link := range links {
		if link.SellerArticle != "" && link.URL != "" {
			byArticle[link.SellerArticle] = link.URL
		}
	}
	return byArticle
}

// FetchProductURLs downloads the shop sitemap and returns the product page
// URLs (the ones containing "/products/") in sitemap order.
func FetchProductURLs(ctx context.Context, client *http.Client, sitemapURL string) ([]string, error) {
	if client == nil {
		client = http.DefaultClient
	}
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
