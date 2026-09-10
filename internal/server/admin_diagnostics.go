package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"reviews/internal/store"
)

type DiagItem struct {
	ID      string `json:"id"`
	Level   string `json:"level"` // ok | warn | fail
	Title   string `json:"title"`
	Detail  string `json:"detail"`
	FixHref string `json:"fixHref,omitempty"`
}

type ActivityItem struct {
	At      time.Time `json:"at"`
	Level   string    `json:"level"`
	Source  string    `json:"source"`
	Message string    `json:"message"`
}

func itoa(n int) string { return strconv.Itoa(n) }

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	checks := []DiagItem{}

	// CORS / shop origin
	origin, _ := s.store.GetAppSetting(ctx, store.SettingShopOrigin)
	if strings.TrimSpace(origin) == "" {
		checks = append(checks, DiagItem{
			ID: "cors", Level: "fail",
			Title:   "Адрес магазина не задан",
			Detail:  "Без адреса магазина браузер блокирует виджет (CORS): он останется без данных и стилей.",
			FixHref: "#/settings/general",
		})
	} else {
		checks = append(checks, DiagItem{
			ID: "cors", Level: "ok",
			Title: "Адрес магазина задан", Detail: origin,
			FixHref: "#/settings/general",
		})
	}

	// Active widget version (product context)
	versions, err := s.store.ListWidgetConfigVersions(ctx, "product")
	if err != nil || len(versions) == 0 {
		checks = append(checks, DiagItem{
			ID: "widget-version", Level: "warn",
			Title:   "Нет опубликованной версии виджета",
			Detail:  "Опубликуйте оформление на вкладке «Виджет».",
			FixHref: "#/widget/editor",
		})
	} else {
		checks = append(checks, DiagItem{
			ID: "widget-version", Level: "ok",
			Title:   "Версия виджета опубликована",
			Detail:  "Есть активная конфигурация карточки товара.",
			FixHref: "#/widget/editor",
		})
	}

	// Export freshness
	if _, dirty, derr := s.store.ExportDirtySince(ctx); derr == nil && dirty {
		checks = append(checks, DiagItem{
			ID: "export", Level: "warn",
			Title:   "Есть неопубликованные изменения отзывов",
			Detail:  "Нажмите «Опубликовать изменения» на странице «Отзывы», чтобы обновить данные на сайте.",
			FixHref: "#/reviews",
		})
	} else {
		checks = append(checks, DiagItem{
			ID: "export", Level: "ok",
			Title: "Экспорт отзывов актуален", Detail: "",
			FixHref: "#/reviews",
		})
	}

	// Catalog coverage
	links, _ := s.productCatalogLinks()
	if len(links) == 0 {
		checks = append(checks, DiagItem{
			ID: "catalog", Level: "warn",
			Title:   "Каталог товаров пуст",
			Detail:  "Обновите каталог на странице «Маркетплейсы», чтобы виджет находил товары по URL.",
			FixHref: "#/settings/marketplaces",
		})
	} else {
		checks = append(checks, DiagItem{
			ID: "catalog", Level: "ok",
			Title:   "Каталог заполнен",
			Detail:  "Товаров в каталоге: " + itoa(len(links)),
			FixHref: "#/settings/marketplaces",
		})
	}

	// Per-marketplace sync
	for _, mp := range s.marketplaceStatuses(r) {
		if !mp.Enabled {
			continue
		}
		if !mp.Configured {
			checks = append(checks, DiagItem{
				ID: "mp-" + mp.ID, Level: "fail",
				Title:   "«" + mp.ID + "»: доступы не заданы",
				Detail:  "Заполните доступы на странице «Маркетплейсы».",
				FixHref: "#/settings/marketplaces",
			})
			continue
		}
		checks = append(checks, DiagItem{
			ID: "mp-" + mp.ID, Level: "ok",
			Title:   "«" + mp.ID + "»: включён и настроен",
			Detail:  "",
			FixHref: "#/settings/marketplaces",
		})
	}

	// Activity log from existing sync runs.
	activity := []ActivityItem{}
	if runs, rerr := s.store.RecentSyncRuns(ctx, 10); rerr == nil {
		for _, run := range runs {
			level := "ok"
			msg := "Синхронизация «" + run.Marketplace + "» — успешно, новых " + itoa(run.ReviewsUpserted)
			if run.Status == "error" {
				level = "fail"
				text := "ошибка"
				if run.ErrorText != nil {
					text = *run.ErrorText
				}
				msg = "Синхронизация «" + run.Marketplace + "» — " + text
			}
			activity = append(activity, ActivityItem{
				At: run.StartedAt, Level: level, Source: "sync", Message: msg,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"checks": checks, "activity": activity})
}

type probeRequest struct {
	ProductURL string `json:"productUrl"`
}

func (s *Server) handleDiagnosticsProbe(w http.ResponseWriter, r *http.Request) {
	var req probeRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req) // empty body is fine
	}
	ctx := r.Context()
	checks := []DiagItem{}

	origin, _ := s.store.GetAppSetting(ctx, store.SettingShopOrigin)
	origin = strings.TrimSpace(origin)
	if origin == "" {
		checks = append(checks, DiagItem{
			ID: "site-reachable", Level: "warn",
			Title:   "Адрес магазина не задан",
			Detail:  "Укажите адрес магазина в «Настройках», чтобы проверить доступность сайта.",
			FixHref: "#/settings/general",
		})
	} else {
		client := &http.Client{Timeout: 8 * time.Second}
		resp, err := client.Get(origin)
		if err != nil {
			checks = append(checks, DiagItem{
				ID: "site-reachable", Level: "fail",
				Title:  "Сайт магазина недоступен",
				Detail: "Не удалось открыть " + origin + ": " + err.Error(),
			})
		} else {
			resp.Body.Close()
			level, detail := "ok", "Ответ "+itoa(resp.StatusCode)
			if resp.StatusCode >= 400 {
				level = "warn"
			}
			checks = append(checks, DiagItem{
				ID: "site-reachable", Level: level,
				Title: "Сайт магазина отвечает", Detail: detail,
			})
		}
	}

	if strings.TrimSpace(req.ProductURL) != "" {
		checks = append(checks, s.probeProductURL(req.ProductURL))
	}

	writeJSON(w, http.StatusOK, map[string]any{"checks": checks})
}

// probeProductURL resolves the article for a product URL against the same
// product-link map the widget uses. It never performs a "snippet present"
// check — that cannot be done reliably server-side when the snippet is
// injected by Tag Manager (documented limitation, surfaced in the UI).
func (s *Server) probeProductURL(productURL string) DiagItem {
	article := s.resolveArticleFromURL(productURL)
	if article == "" {
		return DiagItem{
			ID: "article-resolve", Level: "warn",
			Title:   "Артикул для этой страницы не найден",
			Detail:  "URL нет в каталоге — обновите каталог или проверьте адрес. Виджет не сможет подобрать отзывы.",
			FixHref: "#/settings/marketplaces",
		}
	}
	return DiagItem{
		ID: "article-resolve", Level: "ok",
		Title:  "Артикул найден",
		Detail: "URL сопоставлен с артикулом " + article,
	}
}

// resolveArticleFromURL inverts the in-memory article→URL map (s.productLinks(),
// which is keyed by seller article — verified in server.go / site.ProductLinkMap)
// to find the article whose product page matches productURL.
func (s *Server) resolveArticleFromURL(productURL string) string {
	target := strings.TrimRight(strings.TrimSpace(productURL), "/")
	for article, u := range s.productLinks() {
		if strings.TrimRight(u, "/") == target {
			return article
		}
	}
	return ""
}
