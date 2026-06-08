<claude-mem-context>
# Memory Context

# [Reviews] recent context, 2026-06-08 2:06pm GMT+3

Legend: 🎯session 🔴bugfix 🟣feature 🔄refactor ✅change 🔵discovery ⚖️decision
Format: ID TIME TYPE TITLE
Fetch details: get_observations([IDs]) | Search: mem-search skill

Stats: 40 obs (15,841t read) | 247,234t work | 94% savings

### Jun 4, 2026
3740 12:18p ⚖️ Review aggregation widget service concept for Yandex Kit sellers
3741 12:28p ⚖️ Reviews aggregation service concept for Yandex Kit sellers
3742 12:29p 🔵 Marketplace review APIs: endpoints, fields, and access restrictions confirmed
3743 12:35p 🔵 Yandex Market Partner API: goods-feedback endpoint schema confirmed
3744 " 🔵 Wildberries reviews API: auth required (HTTP 498), "Jam" subscription gates some methods
3745 " 🔵 Reviews aggregation service concept defined: marketplace APIs → backend → GTM widget
3746 12:37p 🔵 Wildberries Feedbacks API: full endpoint map, response schema, and rate limits confirmed
3747 " 🔵 Yandex Market Partner API: Api-Key header is the recommended auth method (OAuth deprecated)
3748 12:38p 🔵 Yandex Business / Yandex Kit site builder: arbitrary JS/HTML injection confirmed blocked
3749 12:39p 🔵 Wildberries Feedbacks API: complete schema, sync strategy, and gotchas confirmed
3750 12:40p 🔵 Yandex Market Partner API reviews: full schema confirmed + critical GTM injection constraint on Yandex builders
3751 " 🔵 Ozon reviews API gated behind paid "Управление отзывами" subscription launched April 2026
3752 12:44p 🔵 Ozon reviews API: full endpoint map confirmed, Premium Plus gate reconfirmed for 2025
3753 12:55p ⚖️ Review aggregation service concept: marketplace API + GTM widget for Yandex Kit sellers
3754 12:58p 🔵 Ozon Seller API review access requires paid "Управление отзывами" subscription (launched April 1, 2026)
3755 12:59p 🔵 Ozon Seller API review integration ambiguously positioned as "for large sellers" within subscription
3756 1:00p 🔵 Ozon Seller API review methods confirmed gated behind "Управление отзывами" subscription — definitive ruling
3757 1:06p 🔵 Reviews project initialized at /home/mama/DEV/Reviews alongside existing projects
3758 1:07p 🔵 analitica project has reusable marketplace API clients for WB, Ozon, YM — but no review-fetch methods
3759 1:37p ⚖️ Review aggregation service concept: marketplace APIs + GTM widget for Yandex Kit sellers
### Jun 8, 2026
3760 11:44a 🔵 Reviews project file structure at /home/mama/DEV/Reviews
3761 11:46a 🔵 Reviews project: Go module structure and test coverage confirmed
S500 Reviews project: Go module structure and test coverage confirmed (Jun 8, 11:46 AM)
3763 11:51a 🔵 shegida-product-links.json: URL and article structure confirmed
3764 " 🔵 shegida.ru product page: no GTM, SSR confirmed, Redux state present
3765 11:52a 🔵 shegida.ru product page: Yandex Kit CSS modules, zero analytics, product SKU structure
3766 11:53a 🔵 shegida.ru: __REDUX_INITIAL_STATE__ not regex-extractable; SKU only in JSON-LD
3768 11:57a 🔵 Yandex Tag Manager: custom HTML/JS injection approach confirmed
3769 " 🔵 Yandex Tag Manager: Sandboxed JS constraint, trigger list, SPA navigation gap
3770 12:09p ⚖️ Backend communication architecture: HTTP vs HTTPS and VPS sizing for embedded widget
3771 12:26p 🔵 Reviews project: full Go service architecture confirmed
S501 Reviews project: full Go service architecture confirmed (Jun 8, 12:26 PM)
3772 12:30p ⚖️ analitica: implementation scoped to Phase A only — design review before deploy
3773 12:31p 🔵 Reviews: new Go project discovered at /home/mama/DEV/Reviews
3774 " 🟣 Reviews: reviewjson package — Mapper.ToReview() and NormalizeSellerArticle() TDD tests created
3775 12:32p 🟣 Reviews: reviewjson package implemented and all tests passing
3776 " 🔄 Reviews: server.go refactored to use reviewjson package — inline DTOs and mapping logic removed
3777 12:33p 🔵 Reviews: full package structure confirmed — all tests pass after reviewjson refactor
3778 " ✅ Reviews: reviewjson extraction committed to phase1-store-skeleton branch (90c6906)
3779 12:35p ✅ Reviews Task 1: independent spec-compliance review confirmed PASS
3780 " 🔵 Reviews: store package file structure confirmed — 6 files including list.go and sync.go
3781 " 🔵 Reviews: implementation plan doc committed at 86ace83 before Phase A work began

Access 247k tokens of past work via get_observations([IDs]) or mem-search skill.
</claude-mem-context>