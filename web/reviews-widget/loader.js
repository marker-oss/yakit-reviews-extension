/*
 * Reviews embed loader for a Yandex Kit shop.
 * Loaded once by a Yandex Tag Manager Custom HTML tag. Watches SPA navigation,
 * fetches per-article JSON, mounts the widget into Shadow DOM, and injects
 * JSON-LD. The host URLs below MUST be provided via window.REVIEWS_EMBED_CONFIG
 * (the admin Embed page generates the full snippet); the empty defaults are
 * placeholders only.
 */
(function () {
  "use strict";

  var CFG = Object.assign(
    {
      dataBase: "",
      widgetJsUrl: "",
      widgetCssUrl: "",
      configBase: "",
      context: "product",
      widgetConfig: null,
      productPathPrefix: "/products/",
      hostId: "reviews-embed-host",
      maxJsonLdReviews: 10,
      anchorSelector: "",
      useShadowDom: true,
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

  function isHomepagePath(pathname) {
    return pathname === "/" || pathname === "";
  }

  function contextForPath(pathname) {
    return isProductPath(pathname) ? "product" : "homepage";
  }

  function shouldHandlePath(pathname) {
    return isProductPath(pathname) || isHomepagePath(pathname);
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

  function showcaseUrl() {
    var base = CFG.configBase || "";
    return base.replace(/\/$/, "") + "/api/showcase";
  }

  function reviewsUrl(contextName) {
    var base = CFG.configBase || "";
    var url = new URL(base.replace(/\/$/, "") + "/api/reviews", location.origin);
    if (contextName) {
      url.searchParams.set("context", contextName);
    }
    url.searchParams.set("sort", "newest");
    url.searchParams.set("limit", "24");
    return url.toString();
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
    isHomepagePath: isHomepagePath,
    contextForPath: contextForPath,
    shouldHandlePath: shouldHandlePath,
    normalizeArticle: normalizeArticle,
    articleFileKey: articleFileKey,
    bundleUrl: bundleUrl,
    showcaseUrl: showcaseUrl,
    reviewsUrl: reviewsUrl,
    extractArticleFromHTML: extractArticleFromHTML,
    extractArticleFromDocument: extractArticleFromDocument,
    skuFromRequestContext: skuFromRequestContext,
    buildJsonLd: buildJsonLd,
    loadWidgetConfig: loadWidgetConfig,
    findCustomAnchor: findCustomAnchor,
    findAnchor: findAnchor,
    collapseCustomAnchor: collapseCustomAnchor,
    loadLinkIndex: loadLinkIndex,
    resolveArticleFromIndex: resolveArticleFromIndex,
    render: render,
    shouldUseShadowDom: shouldUseShadowDom,
    cfg: CFG,
    log: log,
  };

  var widgetLoading = null;
  var widgetConfigLoading = {};
  var widgetConfigs = {};
  var widgetConfig = null;
  var widgetCssText = "";
  var linkIndexLoading = null;
  var linkIndex = null;
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

  function loadWidgetConfig(contextName) {
    contextName = contextName || contextForPath(location.pathname);
    if (CFG.widgetConfig) {
      widgetConfig = CFG.widgetConfig;
      return Promise.resolve(widgetConfig);
    }
    if (widgetConfigs[contextName]) {
      widgetConfig = widgetConfigs[contextName];
      return Promise.resolve(widgetConfig);
    }
    if (widgetConfigLoading[contextName]) {
      return widgetConfigLoading[contextName];
    }

    var base = CFG.configBase || "";
    var url = base.replace(/\/$/, "") + "/api/widget-config?context=" + encodeURIComponent(contextName);
    widgetConfigLoading[contextName] = fetch(url, { headers: { Accept: "application/json" } })
      .then(function (response) {
        if (!response.ok) {
          throw new Error("widget config " + response.status);
        }
        return response.json();
      })
      .then(function (config) {
        widgetConfig = config || null;
        widgetConfigs[contextName] = widgetConfig;
        return widgetConfig;
      })
      .catch(function (error) {
        log("widget config skipped", error && error.message);
        widgetConfig = null;
        return null;
      });
    return widgetConfigLoading[contextName];
  }

  // The product page's own context (#requestContext / JSON-LD) only reflects the
  // current product on a hard load; on SPA navigation it stays frozen on the
  // first product. links.json is a crawled path/id → seller-article map so the
  // loader can resolve the visited product from the URL alone, which is the only
  // reliable per-product signal during SPA navigation.
  function loadLinkIndex() {
    if (linkIndexLoading) {
      return linkIndexLoading;
    }
    var base = CFG.dataBase || "";
    var url = base.replace(/\/$/, "") + "/links.json";
    linkIndexLoading = fetch(url, { headers: { Accept: "application/json" } })
      .then(function (response) {
        if (!response.ok) {
          throw new Error("links " + response.status);
        }
        return response.json();
      })
      .then(function (data) {
        linkIndex = data && (data.byPath || data.byID) ? data : null;
        return linkIndex;
      })
      .catch(function (error) {
        log("links index skipped", error && error.message);
        linkIndex = null;
        return null;
      });
    return linkIndexLoading;
  }

  // Resolve a product path to its seller article using the crawled index.
  // Tries the exact pathname first, then the trailing numeric Kit product id so
  // a changed slug still resolves between daily crawls. Pure for testability.
  function resolveArticleFromIndex(index, pathname) {
    if (!index) {
      return "";
    }
    var path = String(pathname || "").replace(/\/+$/, "");
    if (index.byPath && index.byPath[path]) {
      return normalizeArticle(index.byPath[path]);
    }
    var match = /-(\d+)$/.exec(path);
    if (match && index.byID && index.byID[match[1]]) {
      return normalizeArticle(index.byID[match[1]]);
    }
    return "";
  }

  // Determine the current product's article: prefer the crawled index (works on
  // both hard load and SPA navigation), fall back to the page context (fresh
  // only on hard load) for products not yet in the index.
  function resolveArticle(pathname) {
    return loadLinkIndex().then(function (index) {
      var fromIndex = resolveArticleFromIndex(index, pathname);
      if (fromIndex) {
        return fromIndex;
      }
      return normalizeArticle(extractArticleFromDocument(document));
    });
  }

  function findElementBySelectorOrID(selector) {
    var value = String(selector || "").trim();
    if (!value) {
      return null;
    }
    if (/^[A-Za-z][A-Za-z0-9_-]*$/.test(value)) {
      var byID = document.getElementById(value);
      if (byID) {
        return byID;
      }
    }
    try {
      return document.querySelector(value);
    } catch (error) {
      log("invalid anchor selector", value, error && error.message);
      return null;
    }
  }

  function findCustomAnchor() {
    var preferred = isProductPath(location.pathname) ? ["#reviews-widget"] : ["#reviews-homepage"];
    var fallback = isProductPath(location.pathname) ? ["#reviews-homepage"] : ["#reviews-widget"];
    var candidates = preferred.concat([CFG.anchorSelector || ""], fallback);
    for (var i = 0; i < candidates.length; i++) {
      var element = findElementBySelectorOrID(candidates[i]);
      if (element) {
        return element;
      }
    }
    return null;
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

  function aggregateReviews(reviews) {
    var rated = reviews.filter(function (review) {
      return review.rating != null;
    });
    var sum = rated.reduce(function (total, review) {
      return total + Number(review.rating || 0);
    }, 0);
    return {
      count: reviews.length,
      ratingCount: rated.length,
      ratingAvg: rated.length ? Math.round((sum / rated.length) * 10) / 10 : 0,
    };
  }

  function normalizeAggregate(aggregate, reviews) {
    var fallback = aggregateReviews(reviews);
    if (!aggregate) {
      return fallback;
    }
    return {
      count: numberOrFallback(aggregate.count, numberOrFallback(aggregate.totalReviews, fallback.count)),
      ratingCount: numberOrFallback(aggregate.ratingCount, fallback.ratingCount),
      ratingAvg: numberOrFallback(aggregate.ratingAvg, numberOrFallback(aggregate.averageRating, fallback.ratingAvg)),
    };
  }

  function numberOrFallback(value, fallback) {
    var number = Number(value);
    return Number.isFinite(number) ? number : fallback;
  }

  function normalizeReviewsResponse(data) {
    var reviews = data && Array.isArray(data.reviews) ? data.reviews : [];
    return {
      aggregate: normalizeAggregate(data && data.aggregate, reviews),
      reviews: reviews,
    };
  }

  function shouldUseShadowDom() {
    return CFG.useShadowDom !== false;
  }

  function ensureDocumentStyle(cssText) {
    if (!cssText) {
      return;
    }
    var styleID = CFG.hostId + "-style";
    var style = document.getElementById(styleID);
    if (!style) {
      style = document.createElement("style");
      style.id = styleID;
      style.setAttribute("data-reviews-widget-style", "true");
      document.head.appendChild(style);
    }
    if (style.textContent !== cssText) {
      style.textContent = cssText;
    }
  }

  function mountWidget(host, options) {
    if (shouldUseShadowDom() && window.ReviewsWidget.mountShadow) {
      window.ReviewsWidget.mountShadow(host, options);
      return;
    }
    ensureDocumentStyle(options.styleText);
    window.ReviewsWidget.mount(host, options);
  }

  function render(bundle, normalizedArticle, contextName) {
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

    mountWidget(host, {
      styleText: widgetCssText,
      reviews: reviews,
      aggregate: bundle && bundle.aggregate,
      config: widgetConfig,
      context: contextName || contextForPath(location.pathname),
      fullFeedSource: (contextName || contextForPath(location.pathname)) === "homepage" ? reviewsUrl("") : "",
      fullFeedOffset: 0,
      fullFeedLimit: 24,
    });
    injectJsonLd(bundle);
    currentArticle = normalizedArticle;
    log("rendered", normalizedArticle, reviews.length, "reviews");
  }

  function renderHomepage(pathAtCall) {
    var normalizedArticle = "homepage:" + pathAtCall;
    if (normalizedArticle === currentArticle && document.getElementById(CFG.hostId)) {
      return;
    }

    var seq = ++requestSeq;
    var url = showcaseUrl();
    log("fetch", url);

    Promise.all([loadWidgetAssets(), loadWidgetConfig("homepage")])
      .then(function () {
        return fetch(url, { headers: { Accept: "application/json" } });
      })
      .then(function (response) {
        if (!response.ok) {
          throw new Error("showcase " + response.status);
        }
        return response.json();
      })
      .then(function (data) {
        if (seq !== requestSeq || location.pathname !== pathAtCall) {
          return;
        }
        render(normalizeReviewsResponse(data), normalizedArticle, "homepage");
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

  function handleNavigation() {
    if (!shouldHandlePath(location.pathname)) {
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

    var pathAtCall = location.pathname;
    if (isHomepagePath(pathAtCall)) {
      renderHomepage(pathAtCall);
      return;
    }

    resolveArticle(pathAtCall).then(function (normalizedArticle) {
      // A newer navigation started while we were resolving — let it win.
      if (location.pathname !== pathAtCall) {
        return;
      }
      if (!normalizedArticle) {
        log("no article for", pathAtCall);
        return;
      }
      if (normalizedArticle === currentArticle && document.getElementById(CFG.hostId)) {
        return;
      }

      var seq = ++requestSeq;
      var url = bundleUrl(normalizedArticle);
      log("fetch", url);

      Promise.all([loadWidgetAssets(), loadWidgetConfig("product")])
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
          render(bundle, normalizedArticle, "product");
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
    });
  }

  var debounceTimer = null;
  function scheduleHandle() {
    if (debounceTimer) {
      clearTimeout(debounceTimer);
    }
    debounceTimer = setTimeout(handleNavigation, 400);
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

    // Retry until the widget mounts: the #reviews-widget anchor (a Kit page
    // block) can render after navigation. Article resolution itself comes from
    // the URL via the link index, so pushState/popstate already cover product
    // changes; this only needs to wait for the anchor to appear.
    var observer = new MutationObserver(function () {
      if (shouldHandlePath(location.pathname) && !document.getElementById(CFG.hostId)) {
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
