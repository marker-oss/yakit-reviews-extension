(function () {
  const marketplaceLabels = {
    wb: "Wildberries",
    ym: "Яндекс Маркет",
    ozon: "Ozon",
  };

  const defaultConfig = {
    theme: {
      accent: "#68478D",
      accentInk: "#ffffff",
      text: "#2A2630",
      muted: "#6E6877",
      panel: "#ffffff",
      border: "#E7DFD7",
      star: "#C99A3F",
      dark: false,
    },
    appearance: {
      preset: "default",
    },
    typography: {
      fontFamily: 'Onest, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
      inheritSite: false,
      scale: 1,
      radius: 16,
      density: "comfortable",
    },
    layout: {
      mode: "list",
      columns: 2,
      pageSize: 3,
      pagination: "more",
    },
    header: {
      title: "Отзывы покупателей",
    },
    visibility: {
      photos: true,
      sellerAnswers: true,
      prosCons: true,
      marketplaceBadges: true,
      ratingDistribution: true,
      filters: true,
      questions: true,
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
    marketplacePolicy: {
      wb: { hidden: false, label: "", showSourceLinks: true },
      ym: { hidden: false, label: "", showSourceLinks: true },
      ozon: { hidden: false, label: "", showSourceLinks: true },
    },
  };

  const sampleReviews = [
    {
      marketplace: "wb",
      externalReviewId: "wb-1001",
      externalProductId: "70476012",
      sellerArticle: "1523",
      marketplaceReviewUrl: "https://www.wildberries.ru/catalog/70476012/detail.aspx#comments",
      marketplaceProductUrl: "https://www.wildberries.ru/catalog/70476012/detail.aspx",
      sellerProductUrl: "https://example-shop.test/search?query=1523",
      rating: 5,
      authorName: "Мария К.",
      text: "Платье село по фигуре, ткань приятная и не просвечивает. Дома спокойно примерила и оставила без сомнений.",
      pros: "Аккуратная посадка, ровные швы",
      cons: "",
      createdAt: "2026-05-28T12:20:00+03:00",
      media: [
        { kind: "photo", url: "./assets/review-fabric.svg" },
        { kind: "photo", url: "./assets/review-outfit.svg" },
      ],
      answer: {
        text: "Мария, спасибо за отзыв. Рады, что платье подошло и примерка прошла спокойно.",
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
      authorName: "Елена",
      text: "Заказывала к семейному вечеру. Цвет мягкий, длина удобная, но рукав оказался чуть плотнее, чем я ожидала.",
      pros: "Красивый оттенок, хорошая длина",
      cons: "Рукав сидит плотно",
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
      sellerProductUrl: "https://example-shop.test/search?query=1523",
      rating: 5,
      authorName: "Наталья Ф.",
      text: "На рост 164 длина хорошая, можно и на работу, и в гости. После стирки форма осталась прежней.",
      pros: "Держит форму",
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
      text: "Качество пошива хорошее, но оттенок вживую чуть теплее, чем на фото. Оставила, потому что к лицу подошло.",
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
      text: "Брала маме, размер подошёл с первой примерки. Ткань мягкая, не колется, смотрится аккуратно.",
      pros: "Мягкая ткань, понятная размерность",
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
    const context = options.context || "product";
    const initialReviews = normalizeReviews(options.reviews || [], config);

    const state = {
      reviews: initialReviews,
      aggregate: shouldTrustAggregate(config) ? normalizeAggregate(options.aggregate) : null,
      config,
      context,
      marketplace: config.defaults.marketplace || "all",
      rating: "all",
      mediaFilter: "all",
      sort: options.initialSort || config.defaults.initialSort || "newest",
      visible: initialVisible(config),
      expanded: true,
      fullFeedSource: options.fullFeedSource || "",
      fullFeedLimit: clampNumber(options.fullFeedLimit, 1, 100, 24),
      fullFeedOffset: Number.isFinite(Number(options.fullFeedOffset)) ? Number(options.fullFeedOffset) : 0,
      fullFeedOffsetExplicit: Number.isFinite(Number(options.fullFeedOffset)),
      fullFeedExhausted: false,
      loadingMore: false,
      submissionUrl: options.submissionUrl || "",
      sellerArticle: options.sellerArticle || "",
      activeTab: "reviews",
      questions: {
        items: [],
        loading: false,
        loaded: false,
        error: "",
      },
      questionForm: {
        expanded: false,
        sending: false,
        message: "",
        error: "",
        openedAt: Date.now(),
      },
      submission: {
        config: options.submissionConfig || null,
        loading: Boolean(options.submissionConfigUrl && context === "product"),
        expanded: false,
        sending: false,
        message: "",
        error: "",
        openedAt: Date.now(),
      },
      loading: Boolean(options.source),
      error: "",
      moreError: "",
    };
    if (!state.fullFeedOffsetExplicit) {
      state.fullFeedOffset = state.reviews.length;
    }

    let proxyBase = options.mediaProxyBase || "";
    try {
      if (!proxyBase && options.source) {
        proxyBase = new URL(options.source, window.location.href).origin;
      }
    } catch (e) {
      proxyBase = "";
    }

    // Derive the API base for questions from the source URL origin (same server
    // that serves reviews). Falls back to submissionUrl origin, then empty.
    let questionsApiBase = options.questionsApiBase || "";
    if (!questionsApiBase) {
      try {
        if (options.source) {
          questionsApiBase = new URL(options.source, window.location.href).origin;
        } else if (options.submissionUrl) {
          questionsApiBase = new URL(options.submissionUrl, window.location.href).origin;
        }
      } catch (e) {
        questionsApiBase = "";
      }
    }
    state._questionsApiBase = questionsApiBase;

    root.innerHTML = "";
    root.classList.add("reviews-widget", "reviews-widget-root");
    applyConfig(root, state.config);
    root.__reviewsProxyBase = proxyBase;
    root.classList.toggle("rw-context-homepage", state.context === "homepage");
    root.classList.toggle("rw-is-expanded", state.expanded);
    root.appendChild(renderShell(options.productName || config.header.title, config));
    bind(root, state);
    render(root, state);

    if (options.source) {
      fetchReviews(options.source)
        .then((payload) => {
          state.reviews = normalizeReviews(payload.reviews, state.config);
          state.aggregate = shouldTrustAggregate(state.config) ? normalizeAggregate(payload.aggregate) : null;
          if (!state.fullFeedOffsetExplicit) {
            state.fullFeedOffset = state.reviews.length;
          }
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
    if (context === "product" && options.submissionConfigUrl) {
      fetchSubmissionConfig(options.submissionConfigUrl)
        .then((config) => {
          state.submission.config = config;
          state.submission.loading = false;
          render(root, state);
        })
        .catch(() => {
          state.submission.loading = false;
          render(root, state);
        });
    }
  }

  function renderShell(productName, config) {
    const fragment = document.createDocumentFragment();
    const header = document.createElement("div");
    header.className = "rw-header";
    const showQuestions = !config || config.visibility.questions !== false;
    header.innerHTML = `
      <div class="rw-tabs" aria-label="Разделы отзывов">
        <button class="rw-tab is-active" type="button" data-role="tab-reviews">Отзывы <sup data-role="review-count">0</sup></button>
        ${showQuestions ? `<button class="rw-tab" type="button" data-role="tab-questions">Вопросы <sup data-role="question-count">0</sup></button>` : ""}
      </div>
      <div class="rw-overview">
        <div class="rw-score">
          <div class="rw-score-value" data-role="score">0.0</div>
          <div class="rw-score-meta">
            <div class="rw-stars" data-role="stars" aria-label="Средний рейтинг"></div>
            <div class="rw-summary" data-role="summary"></div>
          </div>
        </div>
        <div class="rw-distribution" aria-label="Сводка отзывов">
          <h2 class="rw-dist-title">${escapeHTML(productName)}</h2>
          <div class="rw-dist-list" data-role="distribution"></div>
          <div class="rw-market-counts" data-role="market-counts"></div>
        </div>
      </div>
      <div class="rw-media-strip" data-role="media-strip"></div>
      <div class="rw-filter-bar">
        <div class="rw-controls">
          <div class="rw-segments" data-role="quick-filters" aria-label="Быстрые фильтры"></div>
          <div class="rw-segments" data-role="marketplaces" aria-label="Маркетплейс"></div>
          <div class="rw-segments" data-role="ratings" aria-label="Рейтинг"></div>
        </div>
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
      <div class="rw-list-wrap" data-role="panel-reviews">
        <div class="rw-list" data-role="list"></div>
        <div class="rw-empty" data-role="status" hidden></div>
        <div class="rw-footer">
          <button class="rw-load-more" type="button" data-role="load-more">Показать ещё</button>
        </div>
        <div class="rw-submit" data-role="submit"></div>
      </div>
      <div class="rw-questions-panel" data-role="panel-questions" hidden>
        <div class="rw-qa-list" data-role="qa-list"></div>
        <div class="rw-empty" data-role="qa-status" hidden></div>
        <div class="rw-qa-submit" data-role="qa-submit"></div>
      </div>
    `;

    const viewer = document.createElement("div");
    viewer.className = "rw-media-viewer";
    viewer.hidden = true;
    viewer.setAttribute("aria-hidden", "true");
    viewer.setAttribute("data-role", "media-viewer");
    viewer.innerHTML = `
      <div class="rw-media-viewer-backdrop" data-role="viewer-close"></div>
      <div class="rw-media-dialog" role="dialog" aria-modal="true" aria-label="Просмотр медиа отзыва">
        <div class="rw-media-dialog-top">
          <div class="rw-media-dialog-caption" data-role="viewer-caption"></div>
          <a class="rw-media-original" data-role="viewer-original" href="#" target="_blank" rel="noreferrer">Открыть оригинал</a>
          <button class="rw-media-close" type="button" data-role="viewer-close" aria-label="Закрыть просмотр">×</button>
        </div>
        <button class="rw-media-nav rw-media-prev" type="button" data-role="viewer-prev" aria-label="Предыдущее медиа">‹</button>
        <div class="rw-media-stage" data-role="viewer-stage"></div>
        <button class="rw-media-nav rw-media-next" type="button" data-role="viewer-next" aria-label="Следующее медиа">›</button>
        <div class="rw-media-counter" data-role="viewer-counter"></div>
      </div>
    `;

    fragment.append(header, body, viewer);
    return fragment;
  }

  function bind(root, state) {
    if (root.__reviewsWidgetKeydown) {
      root.ownerDocument.removeEventListener("keydown", root.__reviewsWidgetKeydown);
    }
    root.__reviewsWidgetKeydown = (event) => {
      const viewer = root.querySelector('[data-role="media-viewer"]');
      if (!viewer || viewer.hidden) {
        return;
      }
      if (event.key === "Escape") {
        closeMediaViewer(root);
      }
      if (event.key === "ArrowLeft") {
        shiftMediaViewer(root, -1);
      }
      if (event.key === "ArrowRight") {
        shiftMediaViewer(root, 1);
      }
    };
    root.ownerDocument.addEventListener("keydown", root.__reviewsWidgetKeydown);

    if (root.__reviewsWidgetMediaClick) {
      root.removeEventListener("click", root.__reviewsWidgetMediaClick);
    }
    root.__reviewsWidgetMediaClick = (event) => {
      const close = event.target.closest('[data-role="viewer-close"]');
      if (close && root.contains(close)) {
        event.preventDefault();
        closeMediaViewer(root);
        return;
      }

      const nav = event.target.closest('[data-role="viewer-prev"], [data-role="viewer-next"]');
      if (nav && root.contains(nav)) {
        event.preventDefault();
        shiftMediaViewer(root, nav.getAttribute("data-role") === "viewer-prev" ? -1 : 1);
        return;
      }

      const trigger = event.target.closest("[data-media-viewer]");
      if (trigger && root.contains(trigger)) {
        event.preventDefault();
        event.stopPropagation();
        openMediaViewer(root, trigger);
        return;
      }

      const submitToggle = event.target.closest('[data-role="submit-toggle"]');
      if (submitToggle && root.contains(submitToggle)) {
        event.preventDefault();
        state.submission.expanded = !state.submission.expanded;
        state.submission.error = "";
        state.submission.message = "";
        render(root, state);
        return;
      }

      const qaToggle = event.target.closest('[data-role="qa-submit-toggle"]');
      if (qaToggle && root.contains(qaToggle)) {
        event.preventDefault();
        state.questionForm.expanded = !state.questionForm.expanded;
        state.questionForm.error = "";
        state.questionForm.message = "";
        render(root, state);
        return;
      }

      const tabReviews = event.target.closest('[data-role="tab-reviews"]');
      if (tabReviews && root.contains(tabReviews)) {
        event.preventDefault();
        if (state.activeTab !== "reviews") {
          state.activeTab = "reviews";
          render(root, state);
        }
        return;
      }

      const tabQuestions = event.target.closest('[data-role="tab-questions"]');
      if (tabQuestions && root.contains(tabQuestions)) {
        event.preventDefault();
        if (state.activeTab !== "questions") {
          state.activeTab = "questions";
          render(root, state);
          if (!state.questions.loaded && !state.questions.loading) {
            loadQuestions(root, state);
          }
        }
        return;
      }
    };
    root.addEventListener("click", root.__reviewsWidgetMediaClick);

    root.addEventListener("submit", (event) => {
      const form = event.target.closest('[data-role="submit-form"]');
      if (form && root.contains(form)) {
        event.preventDefault();
        submitReview(root, state, form);
        return;
      }
      const qaForm = event.target.closest('[data-role="qa-submit-form"]');
      if (qaForm && root.contains(qaForm)) {
        event.preventDefault();
        submitQuestion(root, state, qaForm);
      }
    });

    const sort = root.querySelector('[data-role="sort"]');
    sort.value = state.sort;
    sort.addEventListener("change", (event) => {
      state.sort = event.target.value;
      resetListingState(state);
      render(root, state);
    });

    root.querySelector('[data-role="load-more"]').addEventListener("click", () => {
      if (state.loadingMore) {
        return;
      }
      if (state.fullFeedSource && state.visible >= state.reviews.length && !state.fullFeedExhausted) {
        loadMoreReviews(root, state);
        return;
      }
      state.visible += state.config.layout.pageSize;
      render(root, state);
    });
  }

  function render(root, state) {
    root.classList.toggle("rw-is-expanded", state.expanded);

    // Tab visibility
    const tabReviewsEl = root.querySelector('[data-role="tab-reviews"]');
    const tabQuestionsEl = root.querySelector('[data-role="tab-questions"]');
    if (tabReviewsEl) tabReviewsEl.classList.toggle("is-active", state.activeTab === "reviews");
    if (tabQuestionsEl) tabQuestionsEl.classList.toggle("is-active", state.activeTab === "questions");

    // The overview, media strip and filter bar describe reviews only — hide them
    // on the questions tab so they don't imply the ratings/filters apply there.
    root.classList.toggle("rw-showing-questions", state.activeTab === "questions");

    const panelReviews = root.querySelector('[data-role="panel-reviews"]');
    const panelQuestions = root.querySelector('[data-role="panel-questions"]');
    if (panelReviews) panelReviews.hidden = state.activeTab !== "reviews";
    if (panelQuestions) panelQuestions.hidden = state.activeTab !== "questions";

    if (state.activeTab === "questions") {
      renderQuestionsPanel(root, state);
      return;
    }

    if (state.loading || state.error) {
      renderSummary(root, state.reviews, [], state);
      renderSegments(root, state, state.reviews);
      renderDistribution(root, state.reviews);
      renderMediaStrip(root, state.reviews, state.config);
      renderList(root, []);
      renderStatus(root, state.loading ? "Загружаем отзывы" : state.error, true);
      root.querySelector('[data-role="load-more"]').hidden = true;
      renderSubmission(root, state);
      return;
    }

    const all = state.reviews;
    const filtered = sortReviews(
      all.filter((review) => {
        const marketplaceOk = state.marketplace === "all" || review.marketplace === state.marketplace;
        const ratingOk = ratingMatches(review.rating, state.rating);
        const mediaOk = mediaMatches(review, state.mediaFilter);
        const defaultsOk = matchesDefaults(review, state.config.defaults);
        return marketplaceOk && ratingOk && mediaOk && defaultsOk;
      }),
      state.sort,
      state.config,
    );

    const visibleCount = effectiveVisibleCount(state, filtered.length);

    renderSummary(root, all, filtered, state);
    renderSegments(root, state, all);
    renderDistribution(root, all);
    renderMediaStrip(root, filtered, state.config);
    renderList(root, filtered.slice(0, visibleCount));

    renderStatus(root, state.moreError || "Отзывов с такими фильтрами нет", filtered.length === 0 || Boolean(state.moreError));
    const loadMore = root.querySelector('[data-role="load-more"]');
    const canLoadRemote = Boolean(state.fullFeedSource && !state.fullFeedExhausted);
    loadMore.textContent = state.loadingMore
      ? "Загружаем"
      : "Показать ещё";
    loadMore.disabled = state.loadingMore;
    loadMore.hidden = visibleCount >= filtered.length && !canLoadRemote;
    renderSubmission(root, state);
  }

  function renderStatus(root, text, visible) {
    const status = root.querySelector('[data-role="status"]');
    status.textContent = text;
    status.hidden = !visible;
  }

  function renderSummary(root, all, filtered, state) {
    const aggregate = summaryAggregate(all, state);
    const total = aggregate.totalReviews;
    const average = aggregate.averageRating;
    root.querySelector('[data-role="score"]').textContent = average.toFixed(1);
    root.querySelector('[data-role="stars"]').style.setProperty("--rating", average.toFixed(2));
    root.querySelector('[data-role="review-count"]').textContent = String(total);
    root.querySelector('[data-role="summary"]').textContent = `${pluralize(total, "отзыв", "отзыва", "отзывов")} покупателей · ${pluralize(filtered.length, "показан", "показано", "показано")}`;
  }

  function renderSegments(root, state, reviews) {
    const marketplaces = ["all", ...unique(reviews.map((review) => review.marketplace))];
    const marketplaceRoot = root.querySelector('[data-role="marketplaces"]');
    if (!state.config.visibility.filters) {
      root.querySelector('[data-role="quick-filters"]').innerHTML = "";
      marketplaceRoot.innerHTML = "";
      root.querySelector('[data-role="ratings"]').innerHTML = "";
      return;
    }
    const quickRoot = root.querySelector('[data-role="quick-filters"]');
    quickRoot.innerHTML = "";
    quickRoot.appendChild(segmentButton("Новые", state.sort === "newest", () => {
      state.sort = "newest";
      resetListingState(state);
      root.querySelector('[data-role="sort"]').value = state.sort;
      render(root, state);
    }));
    quickRoot.appendChild(segmentButton("С фото", state.mediaFilter === "photo", () => {
      state.mediaFilter = state.mediaFilter === "photo" ? "all" : "photo";
      resetListingState(state);
      render(root, state);
    }));
    quickRoot.appendChild(segmentButton("С видео", state.mediaFilter === "video", () => {
      state.mediaFilter = state.mediaFilter === "video" ? "all" : "video";
      resetListingState(state);
      render(root, state);
    }));

    marketplaceRoot.innerHTML = "";
    marketplaces.forEach((value) => {
      marketplaceRoot.appendChild(segmentButton(labelMarketplaceValue(value, reviews), state.marketplace === value, () => {
        state.marketplace = value;
        resetListingState(state);
        render(root, state);
      }));
    });

    const ratingSource = reviews.filter((review) => matchesDefaults(review, state.config.defaults));
    const ratings = ["all", 5, 4, 3, 2, 1].filter((value) => {
      return value === "all" || ratingSource.some((review) => review.rating === value);
    });
    const ratingRoot = root.querySelector('[data-role="ratings"]');
    ratingRoot.hidden = state.context === "homepage";
    if (state.context === "homepage") {
      ratingRoot.innerHTML = "";
      return;
    }
    ratingRoot.innerHTML = "";
    ratings.forEach((value) => {
      const label = value === "all" ? "Все оценки" : `${value} ★`;
      ratingRoot.appendChild(segmentButton(label, String(state.rating) === String(value), () => {
        state.rating = String(value);
        resetListingState(state);
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
      row.innerHTML = `<span>${labelMarketplaceValue(marketplace, reviews)}</span><strong>${reviews.filter((review) => review.marketplace === marketplace).length}</strong>`;
      marketRoot.appendChild(row);
    });
  }

  function renderMediaStrip(root, reviews, config) {
    const mediaRoot = root.querySelector('[data-role="media-strip"]');
    if (!config.visibility.photos) {
      mediaRoot.innerHTML = "";
      mediaRoot.hidden = true;
      return;
    }
    const media = reviews.flatMap((review) => {
      return review.media.map((item) => ({
        ...absolutizeUserMedia(item, root.__reviewsProxyBase),
        authorName: review.authorName || "Покупатель",
      }));
    });
    mediaRoot.hidden = media.length === 0;
    if (media.length === 0) {
      mediaRoot.innerHTML = "";
      return;
    }
    mediaRoot.innerHTML = `
      <div class="rw-media-head">
        <strong>Фото покупателей</strong>
        <span>${pluralize(media.length, "материал", "материала", "материалов")}</span>
      </div>
      <div class="rw-media-rail" tabindex="0" aria-label="Фото и видео покупателей">
        ${media
          .slice(0, 18)
          .map((item) => {
            const rawSrc = item.kind === "video" ? item.previewUrl || "./assets/review-video.svg" : item.url;
            const src = item.kind === "video" ? rawSrc : mediaProxyURL(rawSrc, root.__reviewsProxyBase);
            const caption = item.kind === "video" ? `Видео отзыва, ${item.authorName}` : `Фото отзыва, ${item.authorName}`;
            return `
              <a class="rw-strip-media-item" href="${escapeAttribute(item.url)}" ${mediaTriggerAttributes(item, caption)}>
                <img src="${escapeAttribute(src)}" alt="${escapeAttribute(item.kind === "video" ? "Видео отзыва" : `Фото отзыва, ${item.authorName}`)}" loading="lazy" />
                ${item.kind === "video" ? '<span class="rw-play-badge"></span>' : ""}
              </a>
            `;
          })
          .join("")}
      </div>
    `;
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
        card.setAttribute("aria-label", "Открыть источник отзыва");
      }
      card.innerHTML = `
        <div class="rw-card-top">
          <div class="rw-avatar" aria-hidden="true">${escapeHTML(initials(review.authorName || "Покупатель"))}</div>
          <div class="rw-author">
            <div class="rw-author-line">
              <span class="rw-name">${escapeHTML(review.authorName || "Покупатель")}</span>
              <span class="rw-market" data-marketplace="${escapeHTML(review.marketplace)}">${escapeHTML(reviewMarketplaceLabel(review))}</span>
            </div>
            <div class="rw-stars" style="--rating: ${review.rating}" aria-label="${review.rating} из 5"></div>
          </div>
          <time class="rw-date" datetime="${review.createdAt.toISOString()}">${formatDate(review.createdAt)}</time>
        </div>
        ${renderMeta(review, root.__reviewsWidgetConfig)}
        ${review.text ? `<p class="rw-card-text">${escapeHTML(review.text)}</p>` : ""}
        ${renderProsCons(review, root.__reviewsWidgetConfig)}
        ${renderMedia(review.media, root.__reviewsWidgetConfig, root.__reviewsProxyBase)}
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
      ? `<a class="rw-open-market" href="${escapeAttribute(marketplaceLink)}" target="_blank" rel="noreferrer">Открыть источник отзыва</a>`
      : "";
    const sellerAction = review.sellerProductUrl
      ? `<a class="rw-open-store" href="${escapeAttribute(review.sellerProductUrl)}" target="_blank" rel="noreferrer">Посмотреть товар в магазине</a>`
      : "";
    const actions = marketplaceAction || sellerAction
      ? `<span class="rw-actions">${marketplaceAction}${sellerAction}</span>`
      : "";

    return `
      <div class="rw-meta-row">
        <span class="rw-article"><span class="rw-meta-label">Артикул</span> ${sellerProduct}</span>
        <span class="rw-article"><span class="rw-meta-label">На площадке</span> ${marketplaceProduct}</span>
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

  function renderMedia(media, config, proxyBase) {
    if (!config.visibility.photos || !media.length) {
      return "";
    }
    return `
      <div class="rw-media">
        ${media
          .map((raw) => {
            const item = absolutizeUserMedia(raw, proxyBase);
            const rawSrc = item.kind === "video" ? item.previewUrl || "./assets/review-video.svg" : item.url;
            const src = item.kind === "video" ? rawSrc : mediaProxyURL(rawSrc, proxyBase);
            const caption = item.kind === "video" ? "Видео отзыва" : "Фото отзыва";
            return `
              <a class="rw-media-item" href="${escapeAttribute(item.url)}" ${mediaTriggerAttributes(item, caption)}>
                <img src="${escapeAttribute(src)}" alt="${item.kind === "video" ? "Видео отзыва" : "Фото отзыва"}" loading="lazy" />
                ${item.kind === "video" ? '<span class="rw-video-badge" aria-hidden="true"></span>' : ""}
              </a>
            `;
          })
          .join("")}
      </div>
    `;
  }

  function mediaTriggerAttributes(item, caption) {
    const key = `${item.kind || "media"}:${item.url || ""}`;
    return [
      'data-media-viewer="true"',
      `data-media-key="${escapeAttribute(key)}"`,
      `data-media-kind="${escapeAttribute(item.kind || "photo")}"`,
      `data-media-url="${escapeAttribute(item.url || "")}"`,
      `data-media-preview="${escapeAttribute(item.previewUrl || "")}"`,
      `data-media-caption="${escapeAttribute(caption || "")}"`,
      'target="_blank"',
      'rel="noreferrer"',
    ].join(" ");
  }

  function openMediaViewer(root, trigger) {
    const viewer = root.querySelector('[data-role="media-viewer"]');
    if (!viewer) {
      return;
    }
    const items = collectRenderedMedia(root);
    if (items.length === 0) {
      return;
    }
    const key = trigger.getAttribute("data-media-key");
    const index = Math.max(0, items.findIndex((item) => item.key === key));
    viewer.__items = items;
    viewer.__index = index;
    viewer.__previousFocus = root.ownerDocument.activeElement;
    viewer.hidden = false;
    viewer.setAttribute("aria-hidden", "false");
    root.classList.add("rw-viewer-open");
    renderMediaViewer(root);
    const close = viewer.querySelector(".rw-media-close");
    if (close && typeof close.focus === "function") {
      close.focus();
    }
  }

  function closeMediaViewer(root) {
    const viewer = root.querySelector('[data-role="media-viewer"]');
    if (!viewer || viewer.hidden) {
      return;
    }
    viewer.hidden = true;
    viewer.setAttribute("aria-hidden", "true");
    root.classList.remove("rw-viewer-open");
    const previousFocus = viewer.__previousFocus;
    if (previousFocus && typeof previousFocus.focus === "function") {
      previousFocus.focus();
    }
  }

  function shiftMediaViewer(root, direction) {
    const viewer = root.querySelector('[data-role="media-viewer"]');
    if (!viewer || viewer.hidden || !viewer.__items || viewer.__items.length === 0) {
      return;
    }
    viewer.__index = (viewer.__index + direction + viewer.__items.length) % viewer.__items.length;
    renderMediaViewer(root);
  }

  function renderMediaViewer(root) {
    const viewer = root.querySelector('[data-role="media-viewer"]');
    const items = viewer.__items || [];
    const item = items[viewer.__index];
    if (!item) {
      closeMediaViewer(root);
      return;
    }
    const stage = viewer.querySelector('[data-role="viewer-stage"]');
    const caption = viewer.querySelector('[data-role="viewer-caption"]');
    const original = viewer.querySelector('[data-role="viewer-original"]');
    const counter = viewer.querySelector('[data-role="viewer-counter"]');
    const prev = viewer.querySelector('[data-role="viewer-prev"]');
    const next = viewer.querySelector('[data-role="viewer-next"]');
    const canPlayVideo = item.kind === "video" && isLikelyVideoURL(item.url);
    const canShowImage = item.kind !== "video" || item.previewUrl || isLikelyImageURL(item.url);
    const rawViewerSrc = item.kind === "video" ? item.previewUrl || item.url : item.url || item.previewUrl;
    const viewerSrc = item.kind === "video" ? rawViewerSrc : mediaProxyURL(rawViewerSrc, root.__reviewsProxyBase);
    caption.textContent = item.caption || (item.kind === "video" ? "Видео отзыва" : "Фото отзыва");
    original.href = item.url;
    original.textContent = item.kind === "video" ? "Открыть видео" : "Открыть оригинал";
    counter.textContent = `${viewer.__index + 1} / ${items.length}`;
    prev.hidden = items.length < 2;
    next.hidden = items.length < 2;
    stage.innerHTML = canPlayVideo
      ? `<video class="rw-media-viewer-video" src="${escapeAttribute(item.url)}" controls playsinline></video>`
      : canShowImage ? `
        <img class="rw-media-viewer-image" src="${escapeAttribute(viewerSrc)}" alt="${escapeAttribute(caption.textContent)}" />
        ${item.kind === "video" ? '<span class="rw-media-viewer-play" aria-hidden="true"></span>' : ""}
      `
        : `<a class="rw-media-viewer-placeholder" href="${escapeAttribute(item.url)}" target="_blank" rel="noreferrer">Открыть медиа</a>`;
  }

  function collectRenderedMedia(root) {
    const seen = new Set();
    return Array.from(root.querySelectorAll("[data-media-viewer]"))
      .map((node) => ({
        key: node.getAttribute("data-media-key") || node.getAttribute("data-media-url") || node.href,
        kind: node.getAttribute("data-media-kind") || "photo",
        url: node.getAttribute("data-media-url") || node.href,
        previewUrl: node.getAttribute("data-media-preview") || "",
        caption: node.getAttribute("data-media-caption") || "",
      }))
      .filter((item) => {
        if (!item.url || seen.has(item.key)) {
          return false;
        }
        seen.add(item.key);
        return true;
      });
  }

  function isLikelyVideoURL(url) {
    return /\.(mp4|webm|ogg|ogv|m3u8)(\?|#|$)/i.test(String(url || ""));
  }

  function isLikelyImageURL(url) {
    return /\.(avif|gif|jpe?g|png|svg|webp)(\?|#|$)/i.test(String(url || ""));
  }

  function renderAnswer(answer, config) {
    if (!config.visibility.sellerAnswers || !answer || !answer.text) {
      return "";
    }
    const title = answer.kind === "seller" ? "Ответ продавца" : "Ответ магазина";
    return `
      <div class="rw-answer" data-answer-kind="${escapeHTML(answer.kind || "")}">
        <div class="rw-answer-title">${title}</div>
        <p>${escapeHTML(answer.text)}</p>
      </div>
    `;
  }

  function renderSubmission(root, state) {
    const submitRoot = root.querySelector('[data-role="submit"]');
    if (!submitRoot) return;
    const cfg = state.submission.config;
    if (state.context !== "product" || state.submission.loading || !cfg || cfg.enabled === false || !state.submissionUrl) {
      submitRoot.hidden = true;
      submitRoot.innerHTML = "";
      return;
    }
    submitRoot.hidden = false;
    const accepts = (cfg.allowedTypes || []).join(",");
    const consentText = "Согласие на обработку персональных данных";
    const consent = cfg.privacyUrl
      ? `<a href="${escapeAttribute(cfg.privacyUrl)}" target="_blank" rel="noreferrer">${consentText}</a>`
      : consentText;
    submitRoot.innerHTML = `
      <div class="rw-submit-head">
        <div>
          <h3>Оставить отзыв</h3>
          <p>Отзыв появится после проверки модератором.</p>
          ${state.submission.message && !state.submission.expanded ? `<p class="rw-submit-ok">${escapeHTML(state.submission.message)}</p>` : ""}
        </div>
        <button class="rw-submit-toggle" type="button" data-role="submit-toggle">${state.submission.expanded ? "Свернуть" : "Написать отзыв"}</button>
      </div>
      ${state.submission.expanded ? `
        <form class="rw-submit-form" data-role="submit-form">
          <input type="text" name="website" class="rw-hp" tabindex="-1" autocomplete="off" aria-hidden="true" />
          <input type="hidden" name="openedAt" value="${state.submission.openedAt}" />
          <input type="hidden" name="sellerArticle" value="${escapeAttribute(state.sellerArticle)}" />
          <div class="rw-submit-grid">
            <label><span>Оценка</span><select name="rating" required>
              <option value="5">5 — отлично</option>
              <option value="4">4 — хорошо</option>
              <option value="3">3 — нормально</option>
              <option value="2">2 — плохо</option>
              <option value="1">1 — ужасно</option>
            </select></label>
            <label><span>Имя</span><input name="authorName" required maxlength="80" autocomplete="name" /></label>
            <label><span>Email</span><input name="authorEmail" type="email" required maxlength="320" autocomplete="email" /></label>
            <label class="rw-submit-wide"><span>Отзыв</span><textarea name="text" required maxlength="3000" rows="4"></textarea></label>
            <label><span>Плюсы</span><input name="pros" maxlength="1000" /></label>
            <label><span>Минусы</span><input name="cons" maxlength="1000" /></label>
            <label class="rw-submit-wide"><span>Фото или видео</span><input name="media" type="file" accept="${escapeAttribute(accepts)}" multiple /></label>
          </div>
          <label class="rw-consent"><input name="privacyConsent" type="checkbox" required /> <span>Я даю ${consent}</span></label>
          <div class="rw-submit-actions">
            <button class="rw-submit-send" type="submit" ${state.submission.sending ? "disabled" : ""}>${state.submission.sending ? "Отправляем" : "Отправить на модерацию"}</button>
            ${state.submission.message ? `<span class="rw-submit-ok">${escapeHTML(state.submission.message)}</span>` : ""}
            ${state.submission.error ? `<span class="rw-submit-error">${escapeHTML(state.submission.error)}</span>` : ""}
          </div>
        </form>
      ` : ""}
    `;
  }

  async function submitReview(root, state, form) {
    if (state.submission.sending) return;
    state.submission.sending = true;
    state.submission.error = "";
    state.submission.message = "";
    render(root, state);
    try {
      const formData = new FormData(form);
      formData.set("sellerArticle", state.sellerArticle || formData.get("sellerArticle") || "");
      formData.set("openedAt", String(state.submission.openedAt));
      const response = await fetch(state.submissionUrl, { method: "POST", body: formData });
      if (!response.ok) {
        const payload = await response.json().catch(() => ({ error: "Не удалось отправить отзыв" }));
        throw new Error(payload.error || "Не удалось отправить отзыв");
      }
      state.submission.sending = false;
      state.submission.expanded = false;
      state.submission.message = "Отзыв отправлен на модерацию";
      state.submission.openedAt = Date.now();
      render(root, state);
    } catch (error) {
      state.submission.sending = false;
      state.submission.error = error.message || "Не удалось отправить отзыв";
      render(root, state);
    }
  }

  // ── Questions tab ──────────────────────────────────────────────────────────

  function renderQuestionsPanel(root, state) {
    const qaList = root.querySelector('[data-role="qa-list"]');
    const qaStatus = root.querySelector('[data-role="qa-status"]');
    const qaSubmit = root.querySelector('[data-role="qa-submit"]');
    if (!qaList) return;

    // Update question count badge
    const qCountEl = root.querySelector('[data-role="question-count"]');
    if (qCountEl) qCountEl.textContent = String(state.questions.items.length);

    if (state.questions.loading) {
      qaList.innerHTML = "";
      if (qaStatus) { qaStatus.textContent = "Загружаем вопросы"; qaStatus.hidden = false; }
    } else if (state.questions.error) {
      qaList.innerHTML = "";
      if (qaStatus) { qaStatus.textContent = state.questions.error; qaStatus.hidden = false; }
    } else if (state.questions.items.length === 0) {
      qaList.innerHTML = "";
      if (qaStatus) { qaStatus.textContent = "Вопросов пока нет"; qaStatus.hidden = false; }
    } else {
      if (qaStatus) qaStatus.hidden = true;
      qaList.innerHTML = state.questions.items.map((q) => {
        const date = q.date ? new Date(q.date) : null;
        const dateStr = date && !isNaN(date)
          ? date.toLocaleDateString("ru-RU", { day: "numeric", month: "long", year: "numeric" })
          : "";
        return `
          <div class="rw-qa-item">
            <div class="rw-qa-question">
              <span class="rw-qa-label">Вопрос</span>
              <p>${escapeHTML(q.question || "")}</p>
            </div>
            <div class="rw-qa-answer">
              <span class="rw-qa-label">Ответ продавца</span>
              <p>${escapeHTML(q.answer || "")}</p>
              ${dateStr ? `<time class="rw-qa-date" datetime="${escapeAttribute(q.date || "")}">${escapeHTML(dateStr)}</time>` : ""}
            </div>
          </div>
        `;
      }).join("");
    }

    renderQuestionForm(root, state, qaSubmit);
  }

  function renderQuestionForm(root, state, submitRoot) {
    if (!submitRoot) return;
    const qf = state.questionForm;
    const apiBase = state._questionsApiBase || "";
    // Only show question form when there is an API base to POST to.
    if (!apiBase || state.context !== "product") {
      submitRoot.hidden = true;
      submitRoot.innerHTML = "";
      return;
    }
    submitRoot.hidden = false;
    const privacyUrl = state.submission.config && state.submission.config.privacyUrl
      ? state.submission.config.privacyUrl
      : (state._submissionConfig && state._submissionConfig.privacyUrl ? state._submissionConfig.privacyUrl : "");
    const consentText = "Согласие на обработку персональных данных";
    const consent = privacyUrl
      ? `<a href="${escapeAttribute(privacyUrl)}" target="_blank" rel="noreferrer">${consentText}</a>`
      : consentText;
    submitRoot.innerHTML = `
      <div class="rw-submit-head">
        <div>
          <h3>Задать вопрос</h3>
          <p>Вопрос появится после ответа продавца.</p>
          ${qf.message && !qf.expanded ? `<p class="rw-submit-ok">${escapeHTML(qf.message)}</p>` : ""}
        </div>
        <button class="rw-submit-toggle" type="button" data-role="qa-submit-toggle">${qf.expanded ? "Свернуть" : "Задать вопрос"}</button>
      </div>
      ${qf.expanded ? `
        <form class="rw-submit-form" data-role="qa-submit-form">
          <input type="text" name="website" class="rw-hp" tabindex="-1" autocomplete="off" aria-hidden="true" />
          <input type="hidden" name="openedAt" value="${qf.openedAt}" />
          <input type="hidden" name="sellerArticle" value="${escapeAttribute(state.sellerArticle)}" />
          <div class="rw-submit-grid">
            <label><span>Имя</span><input name="authorName" required maxlength="80" autocomplete="name" /></label>
            <label><span>Email</span><input name="authorEmail" type="email" required maxlength="320" autocomplete="email" /></label>
            <label class="rw-submit-wide"><span>Вопрос</span><textarea name="text" required maxlength="2000" rows="4"></textarea></label>
          </div>
          <label class="rw-consent"><input name="privacyConsent" type="checkbox" required /> <span>Я даю ${consent}</span></label>
          <div class="rw-submit-actions">
            <button class="rw-submit-send" type="submit" ${qf.sending ? "disabled" : ""}>${qf.sending ? "Отправляем" : "Отправить вопрос"}</button>
            ${qf.message ? `<span class="rw-submit-ok">${escapeHTML(qf.message)}</span>` : ""}
            ${qf.error ? `<span class="rw-submit-error">${escapeHTML(qf.error)}</span>` : ""}
          </div>
        </form>
      ` : ""}
    `;
  }

  async function loadQuestions(root, state) {
    const apiBase = state._questionsApiBase || "";
    if (!apiBase || !state.sellerArticle) {
      state.questions.loaded = true;
      render(root, state);
      return;
    }
    state.questions.loading = true;
    state.questions.error = "";
    render(root, state);
    try {
      const url = `${apiBase}/api/questions?article=${encodeURIComponent(state.sellerArticle)}`;
      const response = await fetch(url, { headers: { Accept: "application/json" } });
      if (!response.ok) {
        throw new Error(`API вернул ${response.status}`);
      }
      const payload = await response.json();
      // Server returns {"questions": [{question, answer, date}, ...]}
      state.questions.items = Array.isArray(payload) ? payload : (payload.questions || []);
      state.questions.loading = false;
      state.questions.loaded = true;

      // Update question count badge
      const qCountEl = root.querySelector('[data-role="question-count"]');
      if (qCountEl) qCountEl.textContent = String(state.questions.items.length);

      render(root, state);
    } catch (error) {
      state.questions.loading = false;
      state.questions.loaded = true;
      state.questions.error = error.message || "Не удалось загрузить вопросы";
      render(root, state);
    }
  }

  async function submitQuestion(root, state, form) {
    if (state.questionForm.sending) return;
    state.questionForm.sending = true;
    state.questionForm.error = "";
    state.questionForm.message = "";
    render(root, state);
    try {
      const formData = new FormData(form);
      formData.set("sellerArticle", state.sellerArticle || formData.get("sellerArticle") || "");
      formData.set("openedAt", String(state.questionForm.openedAt));
      const apiBase = state._questionsApiBase || "";
      const url = `${apiBase}/api/questions`;
      const response = await fetch(url, { method: "POST", body: formData });
      if (!response.ok) {
        const payload = await response.json().catch(() => ({ error: "Не удалось отправить вопрос" }));
        throw new Error(payload.error || "Не удалось отправить вопрос");
      }
      state.questionForm.sending = false;
      state.questionForm.expanded = false;
      state.questionForm.message = "Вопрос отправлен";
      state.questionForm.openedAt = Date.now();
      render(root, state);
    } catch (error) {
      state.questionForm.sending = false;
      state.questionForm.error = error.message || "Не удалось отправить вопрос";
      render(root, state);
    }
  }

  // ── End questions tab ───────────────────────────────────────────────────────

  function segmentButton(label, pressed, onClick) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "rw-segment";
    button.setAttribute("aria-pressed", String(pressed));
    button.textContent = label;
    button.addEventListener("click", onClick);
    return button;
  }

  function isUserMediaURL(rawUrl) {
    return /(^|\/)user-media\//.test(String(rawUrl || ""));
  }

  // Visitor-submitted media is returned by the API as a server-relative path
  // (/user-media/<token>). When the widget is embedded cross-origin, resolve it
  // against the reviews server so the <img>/<video>/href all point at the right
  // host. Marketplace CDN URLs (already absolute) pass through untouched.
  function absolutizeUserMedia(item, proxyBase) {
    if (!item || !proxyBase) {
      return item;
    }
    const fix = (url) =>
      url && isUserMediaURL(url) && !/^https?:\/\//i.test(url)
        ? proxyBase.replace(/\/$/, "") + url
        : url;
    return { ...item, url: fix(item.url), previewUrl: fix(item.previewUrl) };
  }

  // Visitor-submitted media is served straight from the reviews server and must
  // NOT pass through the face-blur proxy: the author consented to publication,
  // so their photos are shown as uploaded. Marketplace CDN images still go
  // through /media for face blur.
  function resolveDirectURL(rawUrl, proxyBase) {
    if (/^https?:\/\//i.test(rawUrl)) {
      return rawUrl;
    }
    if (!proxyBase) {
      return rawUrl;
    }
    return proxyBase.replace(/\/$/, "") + rawUrl;
  }

  function mediaProxyURL(rawUrl, proxyBase) {
    if (!rawUrl) {
      return "";
    }
    if (isUserMediaURL(rawUrl)) {
      return resolveDirectURL(rawUrl, proxyBase);
    }
    if (!proxyBase) {
      return rawUrl;
    }
    return proxyBase.replace(/\/$/, "") + "/media?u=" + encodeURIComponent(rawUrl);
  }

  function initials(name) {
    const words = String(name || "")
      .trim()
      .split(/\s+/)
      .filter(Boolean);
    return words
      .slice(0, 2)
      .map((word) => word[0])
      .join("")
      .toUpperCase() || "П";
  }

  function normalizeReviews(reviews, config) {
    return reviews
      .map((review) => applyMarketplacePolicy({
        ...review,
        rating: Number(review.rating || 0),
        createdAt: new Date(review.createdAt),
        media: review.media || [],
        pinned: Boolean(review.pinned),
      }, config))
      .filter(Boolean);
  }

  function applyMarketplacePolicy(review, config) {
    const policy = marketplacePolicyFor(review.marketplace, config);
    if (policy.hidden) {
      return null;
    }
    if (policy.label) {
      review.marketplaceLabel = policy.label;
    }
    if (policy.showSourceLinks === false) {
      review.marketplaceReviewUrl = "";
      review.marketplaceProductUrl = "";
    }
    return review;
  }

  function marketplacePolicyFor(marketplace, config) {
    return (config && config.marketplacePolicy && config.marketplacePolicy[marketplace]) || {};
  }

  function shouldTrustAggregate(config) {
    return !Object.values(config.marketplacePolicy || {}).some((policy) => policy && policy.hidden);
  }

  function normalizeAggregate(aggregate) {
    if (!aggregate) {
      return null;
    }
    const totalReviews = Number(aggregate.totalReviews ?? aggregate.count ?? 0);
    const ratingCount = Number(aggregate.ratingCount ?? totalReviews);
    const averageRating = Number(aggregate.averageRating ?? aggregate.ratingAvg ?? 0);
    return {
      totalReviews: Number.isFinite(totalReviews) ? totalReviews : 0,
      ratingCount: Number.isFinite(ratingCount) ? ratingCount : 0,
      averageRating: Number.isFinite(averageRating) ? averageRating : 0,
    };
  }

  function normalizeConfig(config) {
    config = config || {};
    const merged = {
      theme: { ...defaultConfig.theme, ...(config.theme || {}) },
      typography: { ...defaultConfig.typography, ...(config.typography || {}) },
      layout: { ...defaultConfig.layout, ...(config.layout || {}) },
      header: { ...defaultConfig.header, ...(config.header || {}) },
      appearance: { ...defaultConfig.appearance, ...(config.appearance || {}) },
      visibility: { ...defaultConfig.visibility, ...(config.visibility || {}) },
      defaults: { ...defaultConfig.defaults, ...(config.defaults || {}) },
      ranking: Array.isArray(config.ranking) && config.ranking.length ? config.ranking : defaultConfig.ranking,
      marketplacePolicy: normalizeMarketplacePolicy(config.marketplacePolicy),
    };
    merged.header.title = String(merged.header.title || "").trim() || defaultConfig.header.title;
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
    merged.typography.inheritSite = Boolean(merged.typography.inheritSite);
    merged.defaults.minRating = Math.round(clampNumber(merged.defaults.minRating, 0, 5, 0));
    if (!["all", "wb", "ozon", "ym"].includes(merged.defaults.marketplace)) {
      merged.defaults.marketplace = "all";
    }
    if (!["relevance", "newest", "highest", "lowest", "media"].includes(merged.defaults.initialSort)) {
      merged.defaults.initialSort = "relevance";
    }
    return merged;
  }

  function normalizeMarketplacePolicy(policy) {
    const normalized = {};
    Object.keys(defaultConfig.marketplacePolicy).forEach((marketplace) => {
      normalized[marketplace] = {
        ...defaultConfig.marketplacePolicy[marketplace],
        ...((policy && policy[marketplace]) || {}),
      };
      normalized[marketplace].hidden = Boolean(normalized[marketplace].hidden);
      normalized[marketplace].label = String(normalized[marketplace].label || "").trim();
      normalized[marketplace].showSourceLinks = normalized[marketplace].showSourceLinks !== false;
    });
    return normalized;
  }

  function initialVisible(config) {
    return config.layout.pageSize;
  }

  function resetListingState(state) {
    state.visible = initialVisible(state.config);
    state.expanded = true;
  }

  function effectiveVisibleCount(state, total) {
    return Math.min(total, state.visible);
  }

  function summaryAggregate(reviews, state) {
    const fallback = aggregateFromReviews(reviews);
    if (state.context === "homepage" && state.aggregate && state.aggregate.totalReviews > 0) {
      return state.aggregate;
    }
    return fallback;
  }

  function aggregateFromReviews(reviews) {
    const rated = reviews.filter((review) => review.rating > 0);
    const sum = rated.reduce((total, review) => total + review.rating, 0);
    return {
      totalReviews: reviews.length,
      ratingCount: rated.length,
      averageRating: rated.length ? sum / rated.length : 0,
    };
  }

  function colorChannels(value, fallback) {
    let hex = String(value || "").trim().replace(/^#/, "");
    if (hex.length === 3) {
      hex = hex.replace(/./g, (digit) => digit + digit);
    }
    if (!/^[0-9a-f]{6}$/i.test(hex)) {
      return colorChannels(fallback, "#000000");
    }
    return [0, 2, 4].map((offset) => parseInt(hex.slice(offset, offset + 2), 16));
  }

  function mixChannels(base, overlay, overlayWeight) {
    return `#${base.map((channel, index) =>
      Math.round(channel * (1 - overlayWeight) + overlay[index] * overlayWeight)
        .toString(16)
        .padStart(2, "0")
    ).join("")}`;
  }

  function rgba(channels, alpha) {
    return `rgba(${channels.join(", ")}, ${alpha})`;
  }
  function applyConfig(root, config) {
    const siteStyle = config.typography.inheritSite ? getComputedStyle(root.ownerDocument.body) : null;
    const siteValue = (name, fallback) => siteStyle?.getPropertyValue(name).trim() || fallback;
    const theme = config.typography.inheritSite ? {
      ...config.theme,
      accent: siteValue("--ys-color-token-brand-primary", config.theme.accent),
      accentInk: siteValue("--ys-color-token-brand-primary-contrast", config.theme.accentInk),
      text: siteValue("--ys-color-token-text-primary", config.theme.text),
      muted: siteValue("--ys-color-token-text-secondary", config.theme.muted),
      panel: siteValue("--ys-background-primary", config.theme.panel),
      border: siteValue("--ys-color-token-border-primary", config.theme.border),
    } : config.theme;
    const accent = colorChannels(theme.accent, defaultConfig.theme.accent);
    const text = colorChannels(theme.text, defaultConfig.theme.text);
    const muted = colorChannels(theme.muted, defaultConfig.theme.muted);
    const panel = colorChannels(theme.panel, defaultConfig.theme.panel);
    const star = colorChannels(theme.star, defaultConfig.theme.star);
    const trust = colorChannels("#4E7C59", "#4E7C59");

    root.__reviewsWidgetConfig = config;
    root.style.setProperty("--rw-text", theme.text);
    root.style.setProperty("--rw-muted", theme.muted);
    root.style.setProperty("--rw-border", theme.border);
    root.style.setProperty("--rw-panel", theme.panel);
    root.style.setProperty("--rw-accent", theme.accent);
    root.style.setProperty("--rw-accent-ink", theme.accentInk);
    root.style.setProperty("--rw-accent-hover", mixChannels(accent, [0, 0, 0], 0.15));
    root.style.setProperty("--rw-accent-tint", mixChannels(panel, accent, 0.12));
    root.style.setProperty("--rw-accent-tint-2", mixChannels(panel, accent, 0.24));
    root.style.setProperty("--rw-soft", mixChannels(panel, text, 0.04));
    root.style.setProperty("--rw-soft-muted", mixChannels(panel, muted, 0.55));
    root.style.setProperty("--rw-star", mixChannels(star, star, 0));
    root.style.setProperty("--rw-trust-tint", mixChannels(panel, trust, 0.1));
    root.style.setProperty("--rw-shadow-sm", `0 1px 3px ${rgba(text, 0.06)}`);
    root.style.setProperty("--rw-shadow-md", `0 4px 16px ${rgba(accent, 0.08)}`);
    root.style.setProperty("--rw-radius", `${config.typography.radius}px`);
    root.style.setProperty("--rw-font-scale", String(config.typography.scale));
    root.style.setProperty("--rw-grid-columns", String(config.layout.columns));
    root.style.fontFamily = config.typography.inheritSite
      ? siteValue("--ys-font-family", "inherit")
      : config.typography.fontFamily || defaultConfig.typography.fontFamily;
    root.classList.toggle("rw-preset-native-kit", config.appearance?.preset === "native-kit");
    root.classList.toggle("rw-inherit-site", config.typography.inheritSite || config.appearance?.preset === "native-kit");
    root.classList.toggle("rw-preset-minimal", config.appearance?.preset === "minimal");
    root.classList.toggle("rw-preset-editorial", config.appearance?.preset === "editorial");
    root.classList.toggle("rw-preset-compact-commerce", config.appearance?.preset === "compact-commerce");
    root.classList.toggle("rw-preset-lead-summary", config.appearance?.preset === "lead-summary");
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
      return { reviews: payload, aggregate: null };
    }
    return { reviews: payload.reviews || [], aggregate: payload.aggregate || null };
  }

  async function fetchSubmissionConfig(source) {
    const response = await fetch(source, { headers: { Accept: "application/json" } });
    if (!response.ok) {
      throw new Error(`API вернул ${response.status}`);
    }
    return response.json();
  }

  async function loadMoreReviews(root, state) {
    state.loadingMore = true;
    state.moreError = "";
    render(root, state);
    try {
      const payload = await fetchReviews(pagedSource(state.fullFeedSource, state.fullFeedOffset, state.fullFeedLimit));
      const next = normalizeReviews(payload.reviews, state.config);
      const existing = new Set(state.reviews.map(reviewKey));
      const uniqueNext = next.filter((review) => {
        const key = reviewKey(review);
        if (existing.has(key)) {
          return false;
        }
        existing.add(key);
        return true;
      });
      state.reviews = state.reviews.concat(uniqueNext);
      state.fullFeedOffset += next.length;
      state.visible = state.reviews.length;
      if (next.length < state.fullFeedLimit) {
        state.fullFeedExhausted = true;
      }
      state.loadingMore = false;
      render(root, state);
    } catch (error) {
      state.loadingMore = false;
      state.moreError = error.message || "Не удалось загрузить отзывы";
      render(root, state);
    }
  }

  function pagedSource(source, offset, limit) {
    const url = new URL(source, window.location.href);
    url.searchParams.set("offset", String(offset));
    url.searchParams.set("limit", String(limit));
    return url.toString();
  }

  function reviewKey(review) {
    if (review.id != null) {
      return `id:${review.id}`;
    }
    if (review.marketplace && review.externalReviewId) {
      return `${review.marketplace}:${review.externalReviewId}`;
    }
    return `${review.marketplace || ""}:${review.externalProductId || ""}:${review.createdAt ? review.createdAt.toISOString() : ""}:${review.authorName || ""}`;
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
    if (defaults.minRating > 0 && review.rating < defaults.minRating) return false;
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

  function mediaMatches(review, selected) {
    if (selected === "photo") return review.media.some((item) => item.kind === "photo");
    if (selected === "video") return review.media.some((item) => item.kind === "video");
    return true;
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

  function labelMarketplaceValue(value, reviews) {
    if (value === "all") return labelMarketplace(value);
    const review = reviews.find((item) => item.marketplace === value && item.marketplaceLabel);
    return review ? review.marketplaceLabel : labelMarketplace(value);
  }

  function reviewMarketplaceLabel(review) {
    return review.marketplaceLabel || labelMarketplace(review.marketplace);
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
    mediaProxyURL,
  };
})();
