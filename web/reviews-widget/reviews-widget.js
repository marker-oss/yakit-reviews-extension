(function () {
  const marketplaceLabels = {
    wb: "WB",
    ym: "ЯМ",
    ozon: "Ozon",
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

    const state = {
      reviews: normalizeReviews(options.reviews || []),
      marketplace: "all",
      rating: "all",
      sort: options.initialSort || "newest",
      visible: 3,
      loading: Boolean(options.source),
      error: "",
    };

    root.innerHTML = "";
    root.appendChild(renderShell(options.productName || "Отзывы"));
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
      state.visible = 3;
      render(root, state);
    });

    root.querySelector('[data-role="load-more"]').addEventListener("click", () => {
      state.visible += 3;
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
        const ratingOk = state.rating === "all" || review.rating === Number(state.rating);
        return marketplaceOk && ratingOk;
      }),
      state.sort,
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
    marketplaceRoot.innerHTML = "";
    marketplaces.forEach((value) => {
      marketplaceRoot.appendChild(segmentButton(labelMarketplace(value), state.marketplace === value, () => {
        state.marketplace = value;
        state.visible = 3;
        render(root, state);
      }));
    });

    const ratings = ["all", 5, 4, 3, 2, 1].filter((value) => {
      return value === "all" || reviews.some((review) => review.rating === value);
    });
    const ratingRoot = root.querySelector('[data-role="ratings"]');
    ratingRoot.innerHTML = "";
    ratings.forEach((value) => {
      ratingRoot.appendChild(segmentButton(value === "all" ? "Все оценки" : `${value} ★`, String(state.rating) === String(value), () => {
        state.rating = String(value);
        state.visible = 3;
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
        ${renderMeta(review)}
        ${review.text ? `<p class="rw-card-text">${escapeHTML(review.text)}</p>` : ""}
        ${renderProsCons(review)}
        ${renderMedia(review.media)}
        ${renderAnswer(review.answer)}
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

  function renderMeta(review) {
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

  function renderProsCons(review) {
    const items = [];
    if (review.pros) {
      items.push(`<div class="rw-note"><strong>Плюсы</strong>${escapeHTML(review.pros)}</div>`);
    }
    if (review.cons) {
      items.push(`<div class="rw-note"><strong>Минусы</strong>${escapeHTML(review.cons)}</div>`);
    }
    return items.length ? `<div class="rw-pros-cons">${items.join("")}</div>` : "";
  }

  function renderMedia(media) {
    if (!media.length) {
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

  function renderAnswer(answer) {
    if (!answer || !answer.text) {
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
    }));
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

  function sortReviews(reviews, sort) {
    return [...reviews].sort((a, b) => {
      if (sort === "highest") return b.rating - a.rating || b.createdAt - a.createdAt;
      if (sort === "lowest") return a.rating - b.rating || b.createdAt - a.createdAt;
      if (sort === "media") return Number(b.media.length > 0) - Number(a.media.length > 0) || b.createdAt - a.createdAt;
      return b.createdAt - a.createdAt;
    });
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
  };
})();
