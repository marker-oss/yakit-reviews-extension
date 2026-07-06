package server

import (
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
