# UGC video carousel — competitor visual research

Field notes for `feat/ugc-visuals`. All numbers measured from live DOM (computed styles) unless noted.
Doc created 2026-09-10 capturing the research below; no prior version existed in the repo.

## 1. Videowise QuickShop carousel (primary reference)

Artifacts:
- Tour screenshot (Bearaby PDP): https://cdn.prod.website-files.com/6906555deb3ff099aa8780aa/69776d7f1326f1f756a59085_Home%20page%20(1).webp
- Tour screenshot (Dr. Squatch hero): https://cdn.prod.website-files.com/6906555deb3ff099aa8780aa/69776d7f7b2f6915929bd2ee_Home%20page.webp
- Live embed measured: bearaby.com homepage "BEARABY BY YOU" section (`#vw-root`, keen-slider)
- Platform CSS: https://assets.videowise.com/style.css.gz (tokens extracted offline)
- Skullcandy PDP theme CSS: https://www.skullcandy.com/cdn/shop/t/3/assets/custom-videowise.css

Findings:
1. **Card ratio 9:16 exactly** — live Bearaby cards 309×549px (0.5625). Slider: 4-up, `flex: 0 0 calc(25% - 11.25px)` → **15px inter-card gap** at 1440 viewport.
2. **Corner radius 10px** — applied on the media wrapper (`border-radius: 10px` inline), not the slide.
3. **Play icon: white SVG circle + triangle**, centered. Default sizes small/medium/large = 48/72/100px (`vw-cmp__icon-size--*`); Bearaby PDP shot shows ~40px orange brand circle (brand-overridable), skullcandy theme forces 100px white circle. Translucent white with pulse/shine/bounce animation options.
4. **Overlay: bottom fade** `linear-gradient(0deg, rgba(0,0,0,.7), transparent)` height **70px** only (`.vw-cmp__in-video-card--fade`), text sits inside it.
5. **Text: plain white, no pill** — title 12px/700 left-aligned inside 8px inset (`.vw-cmp__in-video-card--title-overlay`); duration bottom-right 14px/700; optional brand logo top-left.
6. No arrows/scrollbar on rail; partial card as overflow cue. No card shadows. Cards borderless, background transparent.
7. Lightbox: full-viewport player, product panel **360px right** on desktop (`vw--is-desktop`), player scrims `linear-gradient(180deg, rgba(0,0,0,.4), transparent 15%, …)`, control hover = `rgba(0,0,0,.52)` + `hsla(0,0%,100%,.2)` border.

## 2. Yotpo community wall — Dr. Martens (homepage reference)

Artifacts:
- Live: https://www.drmartens.com/us/en/ section "Styled by our community" (+ dedicated page `/drmartensstyle`)
- Screenshot: /tmp/qa-shots/drmartens-community.png

Findings:
1. **Staggered heights, all 4:5 (0.8 ratio)** — center tile 480×600, neighbors 288×360 / 408×510; ~1.7× scale emphasis on center; items edge-to-edge (no gutters); vertical stepped offsets.
2. **Square corners (0px radius)** on tiles; media edge-clipped by container.
3. **Product cutout + name + price below the rail** (`.product` block: image, uppercase name, price like "$160.00"), only under the emphasized center item.
4. "VIEW ALL" text link top-right of section heading; white square arrow buttons mid-height on the outer tiles.
5. No per-tile overlays or text.

## 3. Yotpo reviews gallery — Princess Polly (PDP reference)

Artifacts:
- Live: https://us.princesspolly.com/products/hudsen-longline-cargo-shorts-khaki (Yotpo pictures widget under add-to-cart)
- Screenshots: /tmp/qa-shots/ppolly-gallery.png, /tmp/qa-shots/ppolly-lightbox.png

Findings:
1. **Square 245×245 thumbnails**, 0px radius, `calc(3.44828% - 15px)` per thumb → 4-up, thin gutters (~8-10px), edge thumbs faded with light wash.
2. **Video badge: 25×25 white camera icon, top-right at 10px inset** (`yotpo-icon-video`) — corner badge, not centered play circle.
3. Hover overlay on thumbs: centered star icon + "Buy Now" pill CTA.
4. **Lightbox anatomy: media left (~640px black stage), product panel right (381px white)** with product thumb, 4.8 rating, "Buy Now" black button, like/dislike; prev/next chevrons outside modal; close top-right; content radius `0 3px 3px 0` (square-ish), backdrop dimmed page.
5. Star rating pairs with UGC media in the product panel, "Uploaded by"-style attribution not shown in this build.

## 4. CSS custom properties (our implementation, post-restyle)

Property **names unchanged**; values updated to the Videowise-derived pattern:

| Property | Value | Source |
|---|---|---|
| `--rw-video-card-width` | `260px` default (config `layout.video.tileWidth`, clamp 140–320) | Videowise 4-up @1440 ≈ 309px scaled to our container |
| `--rw-video-card-ratio` | `9 / 16` default (config `layout.video.aspect`: 3:4 / 9:16 / 1:1) | live Bearaby 309×549 |
| `--rw-video-card-overlay` | `linear-gradient(180deg, transparent 62%, rgba(0,0,0,.7) 100%)` | VW fade (70px ≈ bottom 13% of card) |
| `--rw-video-chip-bg` | `transparent` | no-pill finding |
| `--rw-video-chip-ink` | `#ffffff` | white text + text-shadow |

Card chrome: radius 10px, borderless, rail gap 8px, play badge 72px translucent-white circle (rgba(255,255,255,.32)) with white triangle centered, marketplace label top-left 10px/700 uppercase, author bottom-left 12px/700, hover = white ring `0 0 0 2px rgba(255,255,255,.55)` + img scale 1.04. Scrollbar hidden, `scroll-snap-type: x proximity`.

## 5. Reconciliation decisions

- **Our homepage/PDP video rail → Videowise pattern wins** (uniform 9:16 cards, centered translucent play, bottom gradient, plain white text). DrMartens staggered masonry rejected: our rail is a single-row scroller, not a centered "active card" carousel; masonry heights conflict with the uniform aspect-ratio config key.
- **Yotpo corner video badge**: considered, rejected — our cards already carry the centered play affordance; a second corner icon would duplicate it (Yotpo shows corner badge *because* it has no center play).
- **DrMartens product cutout+price under rail / VIEW ALL**: not adopted now — our widget has no product-catalog binding in the widget data contract; needs a real product feed. Revisit if `products` config lands.
- **Princess Polly lightbox (media left / product right)**: our lightbox keeps Videowise-style player behavior; a right-hand product panel is the same pattern and remains the target when product binding exists.
- **Submission form → Yotpo/Polly pattern (light)**: theme-token alignment (border/panel/text/accent tokens instead of hard-coded Tailwind-ish grays/blue), focus-visible accent ring, dashed accent-tint dropzone with pill file-selector button, accent hover/disabled states on send buttons. No JS changes; POST flow untouched.

## 6. Submission form patterns (adopted)

From Princess Polly Yotpo write-review flow and our own form:
- Field grid 3-up desktop / 1-col mobile (kept), labels small muted above inputs (kept, tokenized).
- Rating: select dropdown (kept — a star-picker widget is a larger JS change, out of low-risk scope).
- Media upload styled as dropzone (new): dashed border, tinted background, pill `::file-selector-button`.
- Buttons: accent pill, hover = darker accent token, disabled 60% opacity.
- Success/error states: existing `.rw-submit-ok`/`.rw-submit-error` colors kept.
