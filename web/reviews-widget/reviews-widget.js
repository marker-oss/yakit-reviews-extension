(function () {
  const marketplaceLabels = {
    wb: "WB",
    ym: "ЯМ",
    ozon: "Ozon",
  };

  const defaultConfig = {
    theme: {
      accent: "#2f7a5b",
      accentInk: "#ffffff",
      text: "#1f2520",
      muted: "#687067",
      panel: "#ffffff",
      border: "#d9ded7",
      dark: false,
    },
    typography: {
      fontFamily: "inherit",
      scale: 1,
      radius: 8,
      density: "comfortable",
    },
    layout: {
      mode: "list",
      columns: 2,
      pageSize: 3,
      pagination: "more",
    },
    visibility: {
      photos: true,
      sellerAnswers: true,
      prosCons: true,
      marketplaceBadges: true,
      ratingDistribution: true,
      filters: true,
    },
    defaults: {
      minRating: 4,
      requireText: true,
      requirePhoto: false,
      marketplace: "all",
      initialSort: "relevance",
      textFirst: true,
      photoFirst: true,
      onlyWithAnswer: false,
    },
    ranking: [
      { field: "pinned", direction: "desc" },
      { field: "hasPhoto", direction: "desc" },
      { field: "hasText", direction: "desc" },
      { field: "rating", direction: "desc" },
      { field: "createdAt", direction: "desc" },
    ],
  };

  const sampleReviews = [
    {
      marketplace: "wb",
      externalReviewId: "wb-1001",
      externalProductId: "70476012",
      sellerArticle: "1523",
      marketplaceReviewUrl: "https://www.wildberries.ru/catalog/70476012/detail.aspx#comments",
      marketplaceProductUrl: "https://www.wildberries.ru/catalog/70476012/detail.aspx",
      sellerProductUrl: "https://shegida.ru/search?query=1523",
      rating: 5,
      authorName: "Мария",
      text: "Очень приятная ткань, после стирки форма осталась прежней. Цвет спокойный, хорошо смотрится с джинсами и жакетом.",
      pros: "Мягкая ткань, ровные швы",
      cons: "",
      createdAt: "2026-05-28T12:20:00+03:00",
      media: [
        { kind: "photo", url: "./assets/review-fabric.svg" },
        { kind: "photo", url: "./assets/review-outfit.svg" },
      ],
      answer: {
        text: "Мария, спасибо за отзыв. Рады, что футболка подошла.",
        state: "published",
      },
    },
    {
      marketplace: "ym",
      externalReviewId: "ym-2107",
      externalProductId: "SKU-2107",
      sellerArticle: "2107",
      marketplaceReviewUrl: "",
      marketplaceProductUrl: "",
      sellerProductUrl: "./product.html?article=SKU-2107&marketplace=ym",
      rating: 4,
      authorName: "Алексей",
      text: "Материал отличный, но я бы брал на размер больше, если нужна свободная посадка.",
      pros: "Плотность, цвет",
      cons: "Немного маломерит",
      createdAt: "2026-05-22T09:10:00+03:00",
      media: [{ kind: "photo", url: "./assets/review-label.svg" }],
      answer: null,
    },
    {
      marketplace: "wb",
      externalReviewId: "wb-1002",
      externalProductId: "70476012",
      sellerArticle: "1523",
      marketplaceReviewUrl: "https://www.wildberries.ru/catalog/70476012/detail.aspx#comments",
      marketplaceProductUrl: "https://www.wildberries.ru/catalog/70476012/detail.aspx",
      sellerProductUrl: "https://shegida.ru/search?query=1523",
      rating: 5,
      authorName: "Наталья",
      text: "Брала базовую белую. Не просвечивает, ворот держит форму, под пиджак сидит аккуратно.",
      pros: "Не просвечивает",
      cons: "",
      createdAt: "2026-05-18T17:42:00+03:00",
      media: [],
      answer: {
        text: "Спасибо, Наталья. Носите с удовольствием.",
        state: "published",
      },
    },
    {
      marketplace: "ozon",
      externalReviewId: "oz-771",
      externalProductId: "OZ-771",
      sellerArticle: "771",
      marketplaceReviewUrl: "",
      marketplaceProductUrl: "",
      sellerProductUrl: "./product.html?article=OZ-771&marketplace=ozon",
      rating: 3,
      authorName: "Ирина",
      text: "Качество хорошее, но оттенок вживую чуть теплее, чем на фото.",
      pros: "Качество пошива",
      cons: "Оттенок отличается",
      createdAt: "2026-05-12T20:05:00+03:00",
      media: [{ kind: "video", url: "./assets/review-video.svg", previewUrl: "./assets/review-video.svg" }],
      answer: null,
    },
    {
      marketplace: "ym",
      externalReviewId: "ym-2108",
      externalProductId: "SKU-2108",
      sellerArticle: "2108",
      marketplaceReviewUrl: "",
      marketplaceProductUrl: "",
      sellerProductUrl: "./product.html?article=SKU-2108&marketplace=ym",
      rating: 5,
      authorName: "Ольга",
      text: "После двух недель носки всё отлично. Горловина не растянулась, катышков нет.",
      pros: "Держит форму",
      cons: "",
      createdAt: "2026-05-04T14:12:00+03:00",
      media: [],
      answer: null,
    },
  ];

  function mount(root, options) {
    if (!root) {
      throw new Error("ReviewsWidget root is required");
    }
    options = options || {};
    const config = normalizeConfig(options.config);

    const state = {
      reviews: normalizeReviews(options.reviews || []),
      config,
      marketplace: config.defaults.marketplace || "all",
      rating: config.defaults.minRating > 0 ? `${config.defaults.minRating}+` : "all",
      sort: options.initialSort || config.defaults.initialSort || "newest",
      visible: config.layout.pageSize,
      loading: Boolean(options.source),
      error: "",
    };

    root.innerHTML = "";
    applyConfig(root, state.config);
    root.appendChild(renderShell(options.productName || "Отзывы", state.config));
    bind(root, state);
    render(root, state);

    if (options.source) {
      fetchReviews(options.source)
        .then((reviews) => {
          state.reviews = normalizeReviews(reviews);
          state.loading = false;
          state.error = "";
          render(root, state);
        })
        .catch((error) => {
          state.loading = false;
          state.error = error.message || "Не удалось загрузить отзывы";
          render(root, state);
        });
    }
  }

  function renderShell(productName) {
    const fragment = document.createDocumentFragment();
    const header = document.createElement("div");
    header.className = "rw-header";
    header.innerHTML = `
      <div class="rw-score">
        <div class="rw-score-value" data-role="score">0.0</div>
        <div class="rw-score-meta">
          <div class="rw-stars" data-role="stars" aria-label="Средний рейтинг"></div>
          <div class="rw-summary" data-role="summary"></div>
        </div>
      </div>
      <div class="rw-controls">
        <div class="rw-segments" data-role="marketplaces" aria-label="Маркетплейс"></div>
        <div class="rw-segments" data-role="ratings" aria-label="Рейтинг"></div>
        <div class="rw-select-row">
          <select class="rw-sort" data-role="sort" aria-label="Сортировка">
            <option value="newest">Сначала новые</option>
            <option value="relevance">Релевантные</option>
            <option value="highest">Сначала высокая оценка</option>
            <option value="lowest">Сначала низкая оценка</option>
            <option value="media">Сначала с медиа</option>
          </select>
        </div>
      </div>
    `;

    const body = document.createElement("div");
    body.className = "rw-body";
    body.innerHTML = `
      <aside class="rw-distribution" aria-label="Сводка отзывов">
        <h2 class="rw-dist-title">${escapeHTML(productName)}</h2>
        <div class="rw-dist-list" data-role="distribution"></div>
        <div class="rw-market-counts" data-role="market-counts"></div>
      </aside>
      <div class="rw-list-wrap">
        <div class="rw-list" data-role="list"></div>
        <div class="rw-empty" data-role="status" hidden></div>
        <div class="rw-footer">
          <button class="rw-load-more" type="button" data-role="load-more">Показать ещё</button>
        </div>
      </div>
    `;

    fragment.append(header, body);
    return fragment;
  }

  function bind(root, state) {
    const sort = root.querySelector('[data-role="sort"]');
    sort.value = state.sort;
    sort.addEventListener("change", (event) => {
      state.sort = event.target.value;
      state.visible = state.config.layout.pageSize;
      render(root, state);
    });

    root.querySelector('[data-role="load-more"]').addEventListener("click", () => {
      state.visible += state.config.layout.pageSize;
      render(root, state);
    });
  }

  function render(root, state) {
    if (state.loading || state.error) {
      renderSummary(root, state.reviews, []);
      renderSegments(root, state, state.reviews);
      renderDistribution(root, state.reviews);
      renderList(root, []);
      renderStatus(root, state.loading ? "Загружаем отзывы" : state.error, true);
      root.querySelector('[data-role="load-more"]').hidden = true;
      return;
    }

    const all = state.reviews;
    const filtered = sortReviews(
      all.filter((review) => {
        const marketplaceOk = state.marketplace === "all" || review.marketplace === state.marketplace;
        const ratingOk = ratingMatches(review.rating, state.rating);
        const defaultsOk = matchesDefaults(review, state.config.defaults);
        return marketplaceOk && ratingOk && defaultsOk;
      }),
      state.sort,
      state.config,
    );

    renderSummary(root, all, filtered);
    renderSegments(root, state, all);
    renderDistribution(root, all);
    renderList(root, filtered.slice(0, state.visible));

    renderStatus(root, "Отзывов с такими фильтрами нет", filtered.length === 0);
    root.querySelector('[data-role="load-more"]').hidden = state.visible >= filtered.length;
  }

  function renderStatus(root, text, visible) {
    const status = root.querySelector('[data-role="status"]');
    status.textContent = text;
    status.hidden = !visible;
  }

  function renderSummary(root, all, filtered) {
    const average = all.length
      ? all.reduce((sum, review) => sum + review.rating, 0) / all.length
      : 0;
    root.querySelector('[data-role="score"]').textContent = average.toFixed(1);
    root.querySelector('[data-role="stars"]').style.setProperty("--rating", average.toFixed(2));
    root.querySelector('[data-role="summary"]').textContent =
      `${pluralize(all.length, "отзыв", "отзыва", "отзывов")} · ${pluralize(filtered.length, "показан", "показано", "показано")}`;
  }

  function renderSegments(root, state, reviews) {
    const marketplaces = ["all", ...unique(reviews.map((review) => review.marketplace))];
    const marketplaceRoot = root.querySelector('[data-role="marketplaces"]');
    if (!state.config.visibility.filters) {
      marketplaceRoot.innerHTML = "";
      root.querySelector('[data-role="ratings"]').innerHTML = "";
      return;
    }
    marketplaceRoot.innerHTML = "";
    marketplaces.forEach((value) => {
      marketplaceRoot.appendChild(segmentButton(labelMarketplace(value), state.marketplace === value, () => {
        state.marketplace = value;
        state.visible = 3;
        render(root, state);
      }));
    });

    const ratings = ["all", "4+", 5, 4, 3, 2, 1].filter((value) => {
      return value === "all" || value === "4+" || reviews.some((review) => review.rating === value);
    });
    const ratingRoot = root.querySelector('[data-role="ratings"]');
    ratingRoot.innerHTML = "";
    ratings.forEach((value) => {
      const label = value === "all" ? "Все оценки" : value === "4+" ? "4+ ★" : `${value} ★`;
      ratingRoot.appendChild(segmentButton(label, String(state.rating) === String(value), () => {
        state.rating = String(value);
        state.visible = state.config.layout.pageSize;
        render(root, state);
      }));
    });
  }

  function renderDistribution(root, reviews) {
    const distRoot = root.querySelector('[data-role="distribution"]');
    distRoot.innerHTML = "";
    const max = Math.max(1, ...[1, 2, 3, 4, 5].map((rating) => countByRating(reviews, rating)));
    [5, 4, 3, 2, 1].forEach((rating) => {
      const count = countByRating(reviews, rating);
      const row = document.createElement("div");
      row.className = "rw-dist-row";
      row.innerHTML = `
        <span>${rating} ★</span>
        <span class="rw-dist-track"><span class="rw-dist-fill" style="width: ${(count / max) * 100}%"></span></span>
        <span>${count}</span>
      `;
      distRoot.appendChild(row);
    });

    const marketRoot = root.querySelector('[data-role="market-counts"]');
    marketRoot.innerHTML = "";
    unique(reviews.map((review) => review.marketplace)).forEach((marketplace) => {
      const row = document.createElement("div");
      row.className = "rw-market-pill";
      row.innerHTML = `<span>${labelMarketplace(marketplace)}</span><strong>${reviews.filter((review) => review.marketplace === marketplace).length}</strong>`;
      marketRoot.appendChild(row);
    });
  }

  function renderList(root, reviews) {
    const list = root.querySelector('[data-role="list"]');
    list.innerHTML = "";
    reviews.forEach((review) => {
      const card = document.createElement("article");
      card.className = "rw-card";
      const marketplaceLink = review.marketplaceReviewUrl || review.marketplaceProductUrl;
      if (marketplaceLink) {
        card.classList.add("is-clickable");
        card.dataset.href = marketplaceLink;
        card.tabIndex = 0;
        card.setAttribute("aria-label", "Открыть отзыв на маркетплейсе");
      }
      card.innerHTML = `
        <div class="rw-card-top">
          <div class="rw-author">
            <div class="rw-author-line">
              <span class="rw-name">${escapeHTML(review.authorName || "Покупатель")}</span>
              <span class="rw-market" data-marketplace="${escapeHTML(review.marketplace)}">${labelMarketplace(review.marketplace)}</span>
            </div>
            <div class="rw-stars" style="--rating: ${review.rating}" aria-label="${review.rating} из 5"></div>
          </div>
          <time class="rw-date" datetime="${review.createdAt.toISOString()}">${formatDate(review.createdAt)}</time>
        </div>
        ${renderMeta(review, root.__reviewsWidgetConfig)}
        ${review.text ? `<p class="rw-card-text">${escapeHTML(review.text)}</p>` : ""}
        ${renderProsCons(review, root.__reviewsWidgetConfig)}
        ${renderMedia(review.media, root.__reviewsWidgetConfig)}
        ${renderAnswer(review.answer, root.__reviewsWidgetConfig)}
      `;
      if (marketplaceLink) {
        card.addEventListener("click", (event) => {
          if (event.target.closest("a, button, select")) {
            return;
          }
          window.open(marketplaceLink, "_blank", "noreferrer");
        });
        card.addEventListener("keydown", (event) => {
          if (event.key !== "Enter") {
            return;
          }
          window.open(marketplaceLink, "_blank", "noreferrer");
        });
      }
      list.appendChild(card);
    });
  }

  function renderMeta(review, config) {
    const sellerArticle = review.sellerArticle || "не сопоставлен";
    const marketplaceArticle = review.externalProductId || "без артикула";
    const sellerProduct = review.sellerProductUrl && review.sellerArticle
      ? `<a class="rw-article-link" href="${escapeAttribute(review.sellerProductUrl)}" target="_blank" rel="noreferrer">${escapeHTML(sellerArticle)}</a>`
      : `<span>${escapeHTML(sellerArticle)}</span>`;
    const marketplaceProduct = review.marketplaceProductUrl
      ? `<a class="rw-article-link" href="${escapeAttribute(review.marketplaceProductUrl)}" target="_blank" rel="noreferrer">${escapeHTML(marketplaceArticle)}</a>`
      : `<span>${escapeHTML(marketplaceArticle)}</span>`;
    const marketplaceLink = review.marketplaceReviewUrl || review.marketplaceProductUrl;
    const marketplaceAction = marketplaceLink
      ? `<a class="rw-open-market" href="${escapeAttribute(marketplaceLink)}" target="_blank" rel="noreferrer">Открыть на маркетплейсе</a>`
      : "";
    const sellerAction = review.sellerProductUrl
      ? `<a class="rw-open-store" href="${escapeAttribute(review.sellerProductUrl)}" target="_blank" rel="noreferrer">Посмотреть в нашем магазине</a>`
      : "";
    const actions = marketplaceAction || sellerAction
      ? `<span class="rw-actions">${marketplaceAction}${sellerAction}</span>`
      : "";

    return `
      <div class="rw-meta-row">
        <span class="rw-article"><span class="rw-meta-label">Артикул продавца</span> ${sellerProduct}</span>
        <span class="rw-article"><span class="rw-meta-label">Артикул ${labelMarketplace(review.marketplace)}</span> ${marketplaceProduct}</span>
        ${actions}
      </div>
    `;
  }

  function renderProsCons(review, config) {
    if (!config.visibility.prosCons) {
      return "";
    }
    const items = [];
    if (review.pros) {
      items.push(`<div class="rw-note"><strong>Плюсы</strong>${escapeHTML(review.pros)}</div>`);
    }
    if (review.cons) {
      items.push(`<div class="rw-note"><strong>Минусы</strong>${escapeHTML(review.cons)}</div>`);
    }
    return items.length ? `<div class="rw-pros-cons">${items.join("")}</div>` : "";
  }

  function renderMedia(media, config) {
    if (!config.visibility.photos || !media.length) {
      return "";
    }
    return `
      <div class="rw-media">
        ${media
          .map((item) => {
            const src = item.kind === "video" ? item.previewUrl || "./assets/review-video.svg" : item.url;
            return `
              <a class="rw-media-item" href="${escapeAttribute(item.url)}" target="_blank" rel="noreferrer">
                <img src="${escapeAttribute(src)}" alt="${item.kind === "video" ? "Видео отзыва" : "Фото отзыва"}" loading="lazy" />
                ${item.kind === "video" ? '<span class="rw-video-badge">▶</span>' : ""}
              </a>
            `;
          })
          .join("")}
      </div>
    `;
  }

  function renderAnswer(answer, config) {
    if (!config.visibility.sellerAnswers || !answer || !answer.text) {
      return "";
    }
    return `
      <div class="rw-answer">
        <div class="rw-answer-title">Ответ продавца</div>
        <p>${escapeHTML(answer.text)}</p>
      </div>
    `;
  }

  function segmentButton(label, pressed, onClick) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "rw-segment";
    button.setAttribute("aria-pressed", String(pressed));
    button.textContent = label;
    button.addEventListener("click", onClick);
    return button;
  }

  function normalizeReviews(reviews) {
    return reviews.map((review) => ({
      ...review,
      rating: Number(review.rating || 0),
      createdAt: new Date(review.createdAt),
      media: review.media || [],
      pinned: Boolean(review.pinned),
    }));
  }

  function normalizeConfig(config) {
    config = config || {};
    const merged = {
      theme: { ...defaultConfig.theme, ...(config.theme || {}) },
      typography: { ...defaultConfig.typography, ...(config.typography || {}) },
      layout: { ...defaultConfig.layout, ...(config.layout || {}) },
      visibility: { ...defaultConfig.visibility, ...(config.visibility || {}) },
      defaults: { ...defaultConfig.defaults, ...(config.defaults || {}) },
      ranking: Array.isArray(config.ranking) && config.ranking.length ? config.ranking : defaultConfig.ranking,
    };
    merged.typography.scale = clampNumber(merged.typography.scale, 0.85, 1.25, 1);
    merged.typography.radius = clampNumber(merged.typography.radius, 0, 24, 8);
    merged.layout.columns = Math.round(clampNumber(merged.layout.columns, 1, 4, 2));
    merged.layout.pageSize = Math.round(clampNumber(merged.layout.pageSize, 1, 24, 3));
    if (!["comfortable", "compact"].includes(merged.typography.density)) {
      merged.typography.density = "comfortable";
    }
    if (!["list", "grid", "carousel"].includes(merged.layout.mode)) {
      merged.layout.mode = "list";
    }
    merged.defaults.minRating = Math.round(clampNumber(merged.defaults.minRating, 0, 5, 0));
    if (!["all", "wb", "ozon", "ym"].includes(merged.defaults.marketplace)) {
      merged.defaults.marketplace = "all";
    }
    if (!["relevance", "newest", "highest", "lowest", "media"].includes(merged.defaults.initialSort)) {
      merged.defaults.initialSort = "relevance";
    }
    return merged;
  }

  function applyConfig(root, config) {
    root.__reviewsWidgetConfig = config;
    root.style.setProperty("--rw-text", config.theme.text);
    root.style.setProperty("--rw-muted", config.theme.muted);
    root.style.setProperty("--rw-border", config.theme.border);
    root.style.setProperty("--rw-panel", config.theme.panel);
    root.style.setProperty("--rw-accent", config.theme.accent);
    root.style.setProperty("--rw-accent-ink", config.theme.accentInk);
    root.style.setProperty("--rw-radius", `${config.typography.radius}px`);
    root.style.setProperty("--rw-font-scale", String(config.typography.scale));
    root.style.setProperty("--rw-grid-columns", String(config.layout.columns));
    if (config.typography.fontFamily && config.typography.fontFamily !== "inherit") {
      root.style.fontFamily = config.typography.fontFamily;
    }
    root.classList.toggle("rw-density-compact", config.typography.density === "compact");
    root.classList.toggle("rw-layout-grid", config.layout.mode === "grid");
    root.classList.toggle("rw-layout-carousel", config.layout.mode === "carousel");
    root.classList.toggle("rw-hide-distribution", !config.visibility.ratingDistribution);
    root.classList.toggle("rw-hide-badges", !config.visibility.marketplaceBadges);
    root.classList.toggle("rw-hide-filters", !config.visibility.filters);
  }

  function clampNumber(value, min, max, fallback) {
    const num = Number(value);
    if (!Number.isFinite(num)) return fallback;
    return Math.min(max, Math.max(min, num));
  }

  async function fetchReviews(source) {
    const response = await fetch(source, {
      headers: { Accept: "application/json" },
    });
    if (!response.ok) {
      throw new Error(`API вернул ${response.status}`);
    }
    const payload = await response.json();
    if (Array.isArray(payload)) {
      return payload;
    }
    return payload.reviews || [];
  }

  function sortReviews(reviews, sort, config) {
    return [...reviews].sort((a, b) => {
      if (sort === "relevance") return compareByRanking(a, b, effectiveRanking(config));
      if (sort === "highest") return b.rating - a.rating || b.createdAt - a.createdAt;
      if (sort === "lowest") return a.rating - b.rating || b.createdAt - a.createdAt;
      if (sort === "media") return Number(b.media.length > 0) - Number(a.media.length > 0) || b.createdAt - a.createdAt;
      return b.createdAt - a.createdAt;
    });
  }

  function compareByRanking(a, b, ranking) {
    const rules = ranking && ranking.length ? ranking : defaultConfig.ranking;
    for (const rule of rules) {
      const av = rankingValue(a, rule.field);
      const bv = rankingValue(b, rule.field);
      if (av === bv) continue;
      return rule.direction === "asc" ? av - bv : bv - av;
    }
    return b.createdAt - a.createdAt;
  }

  function effectiveRanking(config) {
    return (config.ranking && config.ranking.length ? config.ranking : defaultConfig.ranking).filter((rule) => {
      if (rule.field === "hasPhoto") return config.defaults.photoFirst;
      if (rule.field === "hasText") return config.defaults.textFirst;
      return true;
    });
  }

  function rankingValue(review, field) {
    if (field === "pinned") return review.pinned ? 1 : 0;
    if (field === "hasPhoto") return review.media.some((item) => item.kind === "photo") ? 1 : 0;
    if (field === "hasText") return hasText(review) ? 1 : 0;
    if (field === "rating") return review.rating;
    if (field === "createdAt") return review.createdAt.getTime();
    return 0;
  }

  function matchesDefaults(review, defaults) {
    if (defaults.requireText && !hasText(review)) return false;
    if (defaults.requirePhoto && !review.media.some((item) => item.kind === "photo")) return false;
    if (defaults.onlyWithAnswer && (!review.answer || !review.answer.text)) return false;
    return true;
  }

  function ratingMatches(rating, selected) {
    if (selected === "all") return true;
    if (String(selected).endsWith("+")) return rating >= Number(String(selected).slice(0, -1));
    return rating === Number(selected);
  }

  function hasText(review) {
    return Boolean(String(review.text || "").trim() || String(review.pros || "").trim() || String(review.cons || "").trim());
  }

  function countByRating(reviews, rating) {
    return reviews.filter((review) => review.rating === rating).length;
  }

  function unique(values) {
    return [...new Set(values.filter(Boolean))];
  }

  function labelMarketplace(value) {
    if (value === "all") return "Все площадки";
    return marketplaceLabels[value] || value;
  }

  function formatDate(date) {
    return new Intl.DateTimeFormat("ru-RU", {
      day: "numeric",
      month: "short",
      year: "numeric",
    }).format(date);
  }

  function pluralize(count, one, few, many) {
    const mod10 = count % 10;
    const mod100 = count % 100;
    const word = mod10 === 1 && mod100 !== 11 ? one : mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14) ? few : many;
    return `${count} ${word}`;
  }

  function escapeHTML(value) {
    return String(value)
      .replaceAll("&", "&amp;")
      .replaceAll("<", "&lt;")
      .replaceAll(">", "&gt;")
      .replaceAll('"', "&quot;")
      .replaceAll("'", "&#039;");
  }

  function escapeAttribute(value) {
    return escapeHTML(value);
  }

  function mountShadow(host, options) {
    if (!host) {
      throw new Error("ReviewsWidget host is required");
    }
    options = options || {};

    const shadow = host.shadowRoot || host.attachShadow({ mode: "open" });
    shadow.innerHTML = "";

    if (options.styleText) {
      const style = document.createElement("style");
      style.textContent = options.styleText;
      shadow.appendChild(style);
    }

    const root = document.createElement("div");
    root.className = "reviews-widget reviews-widget-root";
    shadow.appendChild(root);

    mount(root, options);
    return shadow;
  }

  window.ReviewsWidget = {
    mount,
    mountShadow,
    sampleReviews,
    defaultConfig,
  };
})();
