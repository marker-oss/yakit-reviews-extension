/*
 * Reviews embed loader for shegida.ru (Yandex Kit).
 * Loaded once by a Yandex Tag Manager Custom HTML tag. The DOM glue added in
 * the next step watches SPA navigation, fetches per-article JSON, mounts the
 * widget into Shadow DOM, and injects JSON-LD.
 */
(function () {
  "use strict";

  var CFG = Object.assign(
    {
      dataBase: "https://reviews.shegida.ru/reviews-data",
      widgetJsUrl: "https://reviews.shegida.ru/reviews-widget.js",
      widgetCssUrl: "https://reviews.shegida.ru/reviews-widget.css",
      configBase: "https://reviews.shegida.ru",
      context: "product",
      widgetConfig: null,
      productPathPrefix: "/products/",
      hostId: "reviews-embed-host",
      maxJsonLdReviews: 10,
      anchorSelector: "",
      debug: false,
    },
    window.REVIEWS_EMBED_CONFIG || {},
  );

  function log() {
    if (CFG.debug && window.console) {
      console.log.apply(console, ["[reviews-embed]"].concat([].slice.call(arguments)));
    }
  }

  function isProductPath(pathname) {
    return typeof pathname === "string" && pathname.indexOf(CFG.productPathPrefix) === 0;
  }

  function normalizeArticle(article) {
    if (!article) {
      return "";
    }
    var value = String(article).trim();
    var slash = value.indexOf("/");
    if (slash >= 0) {
      value = value.slice(0, slash).trim();
    }
    return value;
  }

  function articleFileKey(article) {
    return String(article).replace(/[\/\\ ]/g, "_");
  }

  function bundleUrl(rawArticle) {
    var article = normalizeArticle(rawArticle);
    if (!article) {
      return "";
    }
    return CFG.dataBase + "/by-article/" + encodeURIComponent(articleFileKey(article)) + ".json";
  }

  function extractArticleFromHTML(html) {
    if (!html) {
      return "";
    }
    var match = /"sku"\s*:\s*"([^"]+)"/i.exec(html);
    if (match) {
      return match[1];
    }
    match = /Артикул[^0-9A-Za-zА-Яа-яЁё]{0,8}([0-9A-Za-zА-Яа-яЁё/_.-]+)/i.exec(html);
    return match ? match[1] : "";
  }

  function samePath(urlValue, pathname) {
    if (!urlValue || !pathname) {
      return false;
    }
    try {
      return new URL(urlValue, location.origin).pathname === pathname;
    } catch (_error) {
      return false;
    }
  }

  function isProductJSONLD(value) {
    var type = value && value["@type"];
    if (Array.isArray(type)) {
      return type.some(function (item) {
        return String(item).toLowerCase() === "product";
      });
    }
    return String(type || "").toLowerCase() === "product";
  }

  function skuFromProductJSONLD(value, pathname) {
    if (!value) {
      return "";
    }
    if (Array.isArray(value)) {
      for (var i = 0; i < value.length; i++) {
        var arraySKU = skuFromProductJSONLD(value[i], pathname);
        if (arraySKU) {
          return arraySKU;
        }
      }
      return "";
    }
    if (value["@graph"]) {
      return skuFromProductJSONLD(value["@graph"], pathname);
    }
    if (!isProductJSONLD(value)) {
      return "";
    }

    var offers = value.offers || {};
    var urls = [value.url, offers.url];
    var matchesPage = urls.some(function (urlValue) {
      return samePath(urlValue, pathname);
    });
    if (!matchesPage) {
      return "";
    }

    return value.sku || offers.sku || "";
  }

  function skuFromRequestContext(value) {
    var productCard =
      value &&
      value.pageModel &&
      value.pageModel.context &&
      value.pageModel.context.productCard;
    var mainVariant = productCard && productCard.mainVariant;
    if (mainVariant && mainVariant.sku) {
      return mainVariant.sku;
    }

    var extractorContext = value && value.extractorContext;
    if (!extractorContext) {
      return "";
    }
    for (var key in extractorContext) {
      if (!Object.prototype.hasOwnProperty.call(extractorContext, key)) {
        continue;
      }
      if (key.indexOf("ProductCardExtractorFactory--") !== 0) {
        continue;
      }
      var payload = extractorContext[key] && extractorContext[key].payload;
      var extractorVariant = payload && payload.mainVariant;
      if (extractorVariant && extractorVariant.sku) {
        return extractorVariant.sku;
      }
    }
    return "";
  }

  function extractArticleFromDocument(doc) {
    if (!doc) {
      return "";
    }

    var custom = findCustomAnchor();
    if (custom && custom.getAttribute("data-article")) {
      return custom.getAttribute("data-article");
    }

    var requestContext = doc.getElementById("requestContext");
    if (requestContext) {
      try {
        var requestContextSKU = skuFromRequestContext(JSON.parse(requestContext.textContent || ""));
        if (requestContextSKU) {
          return requestContextSKU;
        }
      } catch (_error) {
        // Ignore unrelated or malformed Kit request context blocks.
      }
    }

    var scripts = doc.querySelectorAll('script[type="application/ld+json"]');
    for (var i = 0; i < scripts.length; i++) {
      try {
        var sku = skuFromProductJSONLD(JSON.parse(scripts[i].textContent || ""), location.pathname);
        if (sku) {
          return sku;
        }
      } catch (_error) {
        // Ignore unrelated or malformed JSON-LD blocks.
      }
    }

    var details = doc.querySelector('[data-testid="ProductDetails"]');
    if (details) {
      return extractArticleFromHTML(details.innerHTML);
    }
    return "";
  }

  function buildJsonLd(bundle, maxReviews) {
    if (!bundle || !bundle.aggregate || bundle.aggregate.ratingCount < 1) {
      return null;
    }

    var reviews = (bundle.reviews || [])
      .filter(function (review) {
        return review.rating != null;
      })
      .slice(0, maxReviews)
      .map(function (review) {
        return {
          "@type": "Review",
          author: { "@type": "Person", name: review.authorName || "Покупатель" },
          datePublished: review.createdAt,
          reviewRating: { "@type": "Rating", ratingValue: review.rating, bestRating: 5, worstRating: 1 },
          reviewBody: review.text || "",
        };
      });

    return {
      "@context": "https://schema.org",
      "@type": "AggregateRating",
      ratingValue: bundle.aggregate.ratingAvg,
      reviewCount: bundle.aggregate.count,
      ratingCount: bundle.aggregate.ratingCount,
      bestRating: 5,
      worstRating: 1,
      review: reviews,
    };
  }

  window.__reviewsEmbedInternals = {
    isProductPath: isProductPath,
    normalizeArticle: normalizeArticle,
    articleFileKey: articleFileKey,
    bundleUrl: bundleUrl,
    extractArticleFromHTML: extractArticleFromHTML,
    extractArticleFromDocument: extractArticleFromDocument,
    skuFromRequestContext: skuFromRequestContext,
    buildJsonLd: buildJsonLd,
    loadWidgetConfig: loadWidgetConfig,
    findCustomAnchor: findCustomAnchor,
    findAnchor: findAnchor,
    collapseCustomAnchor: collapseCustomAnchor,
    shouldReactToMutation: shouldReactToMutation,
    render: render,
    cfg: CFG,
    log: log,
  };

  var widgetLoading = null;
  var widgetConfigLoading = null;
  var widgetConfig = null;
  var widgetCssText = "";
  var currentArticle = null;
  var currentPath = null;
  var requestSeq = 0;
  var spaWatchInstalled = false;

  function loadWidgetAssets() {
    if (widgetLoading) {
      return widgetLoading;
    }

    widgetLoading = new Promise(function (resolve, reject) {
      var cssDone = fetch(CFG.widgetCssUrl)
        .then(function (response) {
          if (!response.ok) {
            throw new Error("widget css " + response.status);
          }
          return response.text();
        })
        .then(function (text) {
          widgetCssText = text;
        });

      if (window.ReviewsWidget && window.ReviewsWidget.mountShadow) {
        cssDone.then(resolve, reject);
        return;
      }

      var script = document.createElement("script");
      script.src = CFG.widgetJsUrl;
      script.onload = function () {
        cssDone.then(resolve, reject);
      };
      script.onerror = function () {
        reject(new Error("widget js failed"));
      };
      document.head.appendChild(script);
    });

    return widgetLoading;
  }

  function loadWidgetConfig() {
    if (CFG.widgetConfig) {
      widgetConfig = CFG.widgetConfig;
      return Promise.resolve(widgetConfig);
    }
    if (widgetConfigLoading) {
      return widgetConfigLoading;
    }

    var base = CFG.configBase || "";
    var url = base.replace(/\/$/, "") + "/api/widget-config?context=" + encodeURIComponent(CFG.context || "product");
    widgetConfigLoading = fetch(url, { headers: { Accept: "application/json" } })
      .then(function (response) {
        if (!response.ok) {
          throw new Error("widget config " + response.status);
        }
        return response.json();
      })
      .then(function (config) {
        widgetConfig = config || null;
        return widgetConfig;
      })
      .catch(function (error) {
        log("widget config skipped", error && error.message);
        widgetConfig = null;
        return null;
      });
    return widgetConfigLoading;
  }

  function findCustomAnchor() {
    if (!CFG.anchorSelector) {
      return null;
    }
    try {
      return document.querySelector(CFG.anchorSelector);
    } catch (error) {
      log("invalid anchor selector", CFG.anchorSelector, error && error.message);
      return null;
    }
  }

  function findAnchor() {
    var custom = findCustomAnchor();
    if (custom) {
      return { element: custom, custom: true };
    }

    var details = document.querySelector('[data-testid="ProductDetails"]');
    if (!details) {
      return null;
    }
    var main = details.querySelector("main");
    return main ? { element: main, custom: false } : null;
  }

  function collapseCustomAnchor() {
    var custom = findCustomAnchor();
    if (custom) {
      custom.style.minHeight = "0";
    }
  }

  function removeHost() {
    var existing = document.getElementById(CFG.hostId);
    if (existing && existing.parentNode) {
      existing.parentNode.removeChild(existing);
    }

    var jsonLd = document.getElementById(CFG.hostId + "-jsonld");
    if (jsonLd && jsonLd.parentNode) {
      jsonLd.parentNode.removeChild(jsonLd);
    }
  }

  function injectJsonLd(bundle) {
    var data = buildJsonLd(bundle, CFG.maxJsonLdReviews);
    if (!data) {
      return;
    }

    var script = document.createElement("script");
    script.type = "application/ld+json";
    script.id = CFG.hostId + "-jsonld";
    script.textContent = JSON.stringify(data);
    document.head.appendChild(script);
  }

  function render(bundle, normalizedArticle) {
    removeHost();

    var reviews = bundle && bundle.reviews ? bundle.reviews : [];
    if (!reviews.length) {
      currentArticle = null;
      collapseCustomAnchor();
      log("empty bundle", normalizedArticle);
      return;
    }

    var anchor = findAnchor();
    if (!anchor || !anchor.element || !anchor.element.parentNode) {
      log("no anchor");
      return;
    }

    var host = document.createElement("div");
    host.id = CFG.hostId;
    host.setAttribute("data-article", normalizedArticle);
    host.style.display = "block";
    host.style.width = "100%";
    host.style.maxWidth = "none";
    host.style.margin = anchor.custom ? "0" : "32px 0";
    host.style.padding = "0";
    host.style.boxSizing = "border-box";
    host.style.gridColumn = "1 / -1";
    host.style.flexBasis = "100%";
    host.style.order = "999";
    if (anchor.custom) {
      anchor.element.appendChild(host);
    } else {
      anchor.element.parentNode.insertBefore(host, anchor.element.nextSibling);
    }

    window.ReviewsWidget.mountShadow(host, { styleText: widgetCssText, reviews: reviews, config: widgetConfig });
    injectJsonLd(bundle);
    currentArticle = normalizedArticle;
    log("rendered", normalizedArticle, reviews.length, "reviews");
  }

  function handleNavigation() {
    if (!isProductPath(location.pathname)) {
      if (currentArticle !== null || document.getElementById(CFG.hostId)) {
        removeHost();
        currentArticle = null;
      }
      currentPath = location.pathname;
      return;
    }

    if (currentPath !== location.pathname) {
      removeHost();
      currentArticle = null;
      currentPath = location.pathname;
      requestSeq++;
    }

    var rawArticle = extractArticleFromDocument(document);
    var normalizedArticle = normalizeArticle(rawArticle);
    if (!normalizedArticle) {
      log("no article on page yet");
      return;
    }
    if (normalizedArticle === currentArticle && document.getElementById(CFG.hostId)) {
      return;
    }

    var seq = ++requestSeq;
    var url = bundleUrl(rawArticle);
    log("fetch", url);

    Promise.all([loadWidgetAssets(), loadWidgetConfig()])
      .then(function () {
        return fetch(url, { headers: { Accept: "application/json" } });
      })
      .then(function (response) {
        if (!response.ok) {
          throw new Error("bundle " + response.status);
        }
        return response.json();
      })
      .then(function (bundle) {
        if (seq !== requestSeq) {
          return;
        }
        render(bundle, normalizedArticle);
      })
      .catch(function (error) {
        if (seq !== requestSeq) {
          return;
        }
        if (currentArticle !== null || document.getElementById(CFG.hostId)) {
          removeHost();
          currentArticle = null;
        }
        collapseCustomAnchor();
        log("skip", url, error && error.message);
      });
  }

  var debounceTimer = null;
  function scheduleHandle() {
    if (debounceTimer) {
      clearTimeout(debounceTimer);
    }
    debounceTimer = setTimeout(handleNavigation, 400);
  }

  // Decide whether a DOM mutation should re-run navigation handling.
  // Reacting only while the host is absent is not enough: on SPA navigation the
  // page article (#requestContext) updates late, so the widget can mount with
  // the previous product's article. Once mounted we must keep watching and
  // re-handle when the page's article diverges from the mounted one, otherwise
  // stale reviews persist until a hard reload.
  function shouldReactToMutation(doc) {
    if (!isProductPath(location.pathname)) {
      return false;
    }
    if (!document.getElementById(CFG.hostId)) {
      return true;
    }
    var pageArticle = normalizeArticle(extractArticleFromDocument(doc || document));
    return Boolean(pageArticle && pageArticle !== currentArticle);
  }

  function installSpaWatch() {
    if (spaWatchInstalled || !document.body) {
      return;
    }
    spaWatchInstalled = true;

    ["pushState", "replaceState"].forEach(function (name) {
      var original = history[name];
      history[name] = function () {
        var result = original.apply(this, arguments);
        scheduleHandle();
        return result;
      };
    });

    window.addEventListener("popstate", scheduleHandle);

    var observer = new MutationObserver(function () {
      if (shouldReactToMutation(document)) {
        scheduleHandle();
      }
    });
    observer.observe(document.body, { childList: true, subtree: true });
  }

  function start() {
    installSpaWatch();
    scheduleHandle();
  }

  function boot() {
    if (window.__reviewsEmbedBooted) {
      return;
    }
    window.__reviewsEmbedBooted = true;

    if (document.readyState === "loading") {
      document.addEventListener("DOMContentLoaded", start);
      return;
    }
    start();
  }

  window.__reviewsEmbedBoot = boot;
  boot();
})();
