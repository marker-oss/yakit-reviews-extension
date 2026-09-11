# UGC widget customization gaps

Audit of every visual/behavioral knob the widget exposes today, and what brands
will likely ask for next. Sources: `defaultConfig` + `normalizeConfig` in
`web/reviews-widget/reviews-widget.js`, CSS custom properties in
`web/reviews-widget/reviews-widget.css`, admin Editor in `web/admin`.

## Already configurable

- Theme colors: accent, accentInk, text, muted, panel, border, star (`theme.*`).
- Typography: fontFamily, inheritSite, scale (0.85–1.25), radius (0–24), density.
- Appearance presets: default, native-kit, minimal, editorial, compact-commerce,
  lead-summary, shoppable (+ viewAllHref header link).
- Layout: mode (list/grid/carousel/video/wall), columns (1–4), pageSize (1–24),
  video tile width (140–320), video aspect (3:4/9:16/1:1), wall minTileWidth/gap/maxTiles.
- Visibility toggles: photos, sellerAnswers, prosCons, marketplaceBadges,
  ratingDistribution, videoRail, filters, questions.
- Defaults: minRating, requireText, requirePhoto, marketplace, initialSort,
  textFirst, photoFirst, onlyWithAnswer.
- Ranking rules (pinned/hasPhoto/hasText/rating/createdAt, per-rule direction).
- Per-marketplace policy: hidden, public label, showSourceLinks.
- Custom review attributes end-to-end (2026-09-10): `customFields` schema in widget config
  (id/label/type/options/required/filterable/showInReview/showInSummary), admin editor panel,
  submission form fields, answers stored in `Review.CustomData` and served as `review.custom`,
  attribute tags in review cards, local public filter segments, and server-side
  `custom_<fieldId>=<value>` filtering on GET /api/reviews (whitelisted against the schema).
  Remaining gap: `showInSummary` is stored but not yet rendered in the widget summary.

## Cheap to add (pure config plumbing, no design work)

- Star rating color beyond `theme.star`: empty-star color, star size.
  - Closed 2026-09-10: `--rw-star` / `--rw-star-empty` / `--rw-focus-ring` are now derived in
    `applyConfig` from `theme.star` / `theme.muted` / `theme.accent`; `typography.radius` is wired
    to `--rw-radius` (was dead config).
- Label texts: "Оставить отзыв", "Отзыв появится после проверки модератором",
  consent text, tab labels, "Загрузить ещё" — all hardcoded Russian strings.
- Section order: media rail / distribution / filters / list are fixed in the shell.
- Default filter state on load (mediaFilter, rating besides minRating).
- Avatar/author display toggles (show author name chip per review card).
- Carousel autoplay speed and arrows visibility.
- Read-more truncation length for long review texts.
- Custom-field chip visual: single- vs multi-select, chip color.

## Needs a design decision

- Section drag-order in admin (needs a stable order schema in payload).
- Per-article CSS overrides / custom CSS injection (security + CSP posture).
- Custom fonts upload vs. host-page fonts only (licensing, loading).
- Multi-language labels (i18n schema: per-locale strings in config?).
- Rich review cards: verified-purchase badge, "helpful" votes (data model).
- Homepage context layout: hero aggregate vs. masonry split.
- Dark mode auto-detect from host page `prefers-color-scheme`.
- Review permalink/deep-link routing inside the widget.
- Email/PII display rules in admin vs. public per deployment (152-ФЗ posture).
