# Reviews Stage 5: удаление, ответ продавца, курирование по артикулу — спецификация

Дата: 2026-06-19
Статус: дизайн (на ревью)
Ветка: `feat/yandex-market-adapter`
Связано: `2026-06-19-reviews-admin-stage5-design.md` (аудит + роадмап, пункты 5–6).

Пункты 1–4 роадмапа уже реализованы (экспорт уважает видимость; согласованы
агрегаты дашборда/витрины; пагинация + сортировка; массовая модерация). Эта
спека покрывает оставшиеся пункты **5 (удаление + ответ продавца)** и
**6 (курирование по артикулу)**.

## Решения (согласованы с заказчиком)

- **Удаление — soft-delete с tombstone.** Оживляем мёртвое поле `Status`
  (`imported` → `deleted`). Sync не воскрешает удалённые. Удалённый отзыв можно
  восстановить из админки.
- **Ответ продавца — публичный.** Админ пишет свой ответ; он показывается в
  виджете как «Ответ SHEGIDA» (тот же блок, что и ответ маркетплейса).
- **Курирование — порядок/закрепление на product-странице.** Ручной подбор
  закрепляет конкретные отзывы наверх для конкретного артикула; влияет на
  статический `by-article` экспорт, который грузят product-страницы.

## Ключевое наблюдение об устойчивости к sync

`UpsertReview` (UPDATE-ветка) обновляет отзыв **явной map'ой колонок-источников**
(`rating`, `text`, `mp_answer_*` и т.д.). Колонки `visibility`, `pinned` и
`status` в эту map **не входят** → sync их уже сохраняет. Значит, чтобы новые
админские поля (`status='deleted'`, `admin_reply_*`) переживали ре-sync,
достаточно **не добавлять** их в `updates`. Отдельная защита в sync не нужна.

---

## Пункт 5 — удаление + ответ продавца

### Модель данных (`internal/store/models.go`)

```go
type Review struct {
    // ... существующие поля ...
    Status         string     // уже есть; default "imported", новое значение "deleted"
    AdminReplyText *string    // новое: ответ продавца, написанный из админки
    AdminReplyAt   *time.Time // новое: когда ответ сохранён/обновлён
}
```

`Status` уже в `AutoMigrate`; новые колонки добавятся автоматически. Миграция —
это только добавление полей в структуру.

### Store-слой (`internal/store/curation.go`)

- `SoftDeleteReview(ctx, id)` → `Update("status", "deleted")`.
- `RestoreReview(ctx, id)` → `Update("status", "imported")`.
- `SetReviewReply(ctx, id, text)` → пишет `admin_reply_text` + `admin_reply_at`;
  пустой/`nil` text очищает оба поля.

### Исключение удалённых из всех публичных и агрегатных выборок

Удалённый отзыв (`status='deleted'`) не должен появляться нигде, кроме
явного админ-фильтра «удалённые». Добавляем условие `status <> 'deleted'`
(или `status != 'deleted'`) в:

- `ListVisibleReviews` (экспорт) — `internal/store/export.go`
- `ShowcaseReviews` и `VisibleReviewAggregate` — `internal/store/curation.go`
- публичный `ListReviews`/`ListReviewsByWidgetRules` — через `applyReviewFilters`
- `DashboardStats`: `TotalReviews` и `VisibleReviews` считают **без** удалённых;
  опционально добавить `DeletedReviews` для информации в дашборде.

В админском `applyReviewFilters` по умолчанию тоже скрываем удалённые, но
добавляем фильтр `Status` (см. ниже), чтобы их можно было показать и восстановить.

### Админский список: фильтр статуса

- `ReviewListFilter` получает поле `Status string`.
- `applyReviewFilters`: если `filter.Status == "deleted"` → `status = 'deleted'`;
  иначе (по умолчанию) → `status <> 'deleted'`. Значение `"all"` показывает все.
- `handleAdminReviews` читает `?status=` (whitelist: ``""``, `deleted`, `all`).

### HTTP API (`internal/server/admin_reviews.go`, `server.go`)

- `DELETE /admin/api/reviews/{id}` → `SoftDeleteReview`.
- `POST   /admin/api/reviews/{id}/restore` → `RestoreReview`.
- `PUT    /admin/api/reviews/{id}/reply` body `{"text": "..."}` → `SetReviewReply`;
  пустой text очищает ответ.
- Все три — за `requireCSRF`, как остальные write-эндпоинты.
- Bulk: расширяем `bulkModerationRequest` опциональным `status` (`deleted`),
  чтобы можно было удалять выделенные. (Восстановление bulk — по необходимости.)

### Публичный JSON и виджет (`internal/reviewjson/reviewjson.go`)

Ответ продавца переиспользует существующий слот `answer` (виджет уже рендерит
`review.answer` как «Ответ SHEGIDA») — **изменений в виджете не требуется**:

```go
// admin reply имеет приоритет над ответом маркетплейса
if review.AdminReplyText != nil && *review.AdminReplyText != "" {
    answer = &Answer{Text: *review.AdminReplyText, State: "published"}
} else if review.MPAnswerText != nil || review.MPAnswerState != nil {
    answer = &Answer{...}
}
```

Так у отзыва всегда один блок ответа; админский ответ заполняет его там, где
маркетплейс молчит (большинство отзывов), и перекрывает, когда задан.

### Админ-UI (`web/admin/src/pages/Reviews.tsx`, `types.ts`)

- Тип `Review` получает `status` и `adminReply?: { text: string; at: string }`
  (или плоские `adminReplyText`/`adminReplyAt` — по факту мэппинга).
- В строке: кнопки **Удалить** (soft) и, когда `status==='deleted'` —
  **Восстановить**; визуальная пометка «удалён».
- Фильтр статуса в тулбаре: «Активные» (по умолч.) / «Удалённые» / «Все».
- Ответ: поле ввода/`textarea` на строке (или раскрывающийся блок) +
  кнопка «Ответить»/«Сохранить ответ»; показ текущего ответа.

---

## Пункт 6 — курирование по артикулу

### Модель данных (`internal/store/showcase_models.go`)

```go
// ShowcasePin закрепляет конкретный отзыв наверх для конкретного артикула.
type ShowcasePin struct {
    ID            uint   `gorm:"primaryKey"`
    TenantID      uint   `gorm:"not null;default:1;uniqueIndex:idx_pin_article_review"`
    SellerArticle string `gorm:"size:128;not null;uniqueIndex:idx_pin_article_review;index"`
    ReviewID      uint   `gorm:"not null;uniqueIndex:idx_pin_article_review"`
    Position      int    `gorm:"not null;default:0"`
    CreatedAt     time.Time
}
```

Добавляется в `AutoMigrate`. Уникальный индекс `(tenant, article, review)`
не даёт задвоить пин.

### Store-слой (`internal/store/curation.go`)

- `ListShowcasePins(ctx, article)` → пины артикула, отсортированы по `position`.
- `SetShowcasePin(ctx, article, reviewID, position)` (upsert).
- `RemoveShowcasePin(ctx, article, reviewID)`.
- `AllShowcasePins(ctx)` → `map[article][]ReviewID` для экспорта (одна выборка).

### Экспорт (`internal/export/export.go`, `cmd/reviews/main.go`)

`BuildBundles` получает карту пинов `map[string][]uint` (article → ordered
reviewIDs). Внутри каждого бандла: отзывы из пинов идут **первыми** в заданном
порядке, остальные — текущей сортировкой (newest first). Пин на удалённый/скрытый
отзыв игнорируется (его нет в выборке `ListVisibleReviews`). `cmd/reviews`
прокидывает `AllShowcasePins` в `BuildBundles`.

> Текущая публичная сортировка на product-странице — `createdAt desc` (см.
> `BuildBundles`). Пины меняют только «верх», не саму логику виджета.

### HTTP API

- `GET    /admin/api/articles/{article}/pins` → список пинов.
- `PUT    /admin/api/articles/{article}/pins` body `{"reviewIds":[...]}`
  (полный упорядоченный набор — простая семантика «сохранить порядок»).
- `DELETE /admin/api/articles/{article}/pins/{reviewId}` — снять один пин.

Артикул в пути — url-escaped. Все write — за `requireCSRF`.

### Админ-UI

Минимально: в списке отзывов при активном фильтре по артикулу показываем
кнопку **Закрепить на странице товара / Открепить** на строке. Полноценный
drag-and-drop порядок — по желанию позже; для MVP достаточно
закрепить/открепить (Position по порядку добавления).

---

## Стратегия тестирования (TDD)

Store (новые тесты в `curation_test.go`, `export_test.go`):
- soft-delete исключает из `ListVisibleReviews`, `ShowcaseReviews`,
  `VisibleReviewAggregate`, публичного `ListReviews`; restore возвращает.
- sync (`UpsertReview`) сохраняет `status='deleted'` и `admin_reply_*`.
- `SetReviewReply` пишет/очищает; маппер кладёт admin reply в `answer` с
  приоритетом над MP.
- пины: set/remove/list; `BuildBundles` ставит запинненные первыми в порядке
  позиции; пин на скрытый/удалённый игнорируется.

Server (`admin_reviews_test.go` + новый по пинам):
- `DELETE`/`restore`/`reply`/`status`-фильтр — коды и эффект на выборку.
- пин-эндпоинты — сохранение порядка, CSRF, auth.

Frontend: ручная проверка + `npm run build`; затем `build-embed.sh`.

## Вне scope

- Редактирование текста отзыва (gap 5 аудита) — отдельный «admin override» слой,
  не входит в роадмап-пункт 5.
- Пер-артикульные правила-фильтры (min rating и т.п.) для product-страниц —
  product-страница показывает все видимые отзывы; пины меняют только порядок.
  Фильтрующие пер-артикульные правила отложены.
- Drag-and-drop переупорядочивание пинов в UI — после MVP.
- Журнал модерации (кто/когда/почему) — gap 15, отдельно.

## Порядок реализации

1. Пункт 5 — модель (`Status` tombstone + `AdminReply*`), store, исключение
   удалённых во всех выборках, эндпоинты, маппер reply→answer, админ-UI.
2. Пункт 6 — `ShowcasePin`, store, экспорт (pinned-first), эндпоинты, админ-UI.
3. `build-embed.sh`, полный прогон тестов, уборка репозитория, единый коммит.
