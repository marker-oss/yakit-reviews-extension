# Tag Manager API Smoke Test

Use this snippet in Yandex Tag Manager before the reviews backend is deployed.
It verifies the full browser-side path:

- Custom HTML tag runs on the product page.
- The widget host is injected after the product photo/details grid.
- Shadow DOM styles are isolated from the storefront.
- `fetch()` can read an external HTTPS JSON API with CORS.
- SPA navigation does not duplicate the block.

Recommended trigger:

- Page view
- DOM Ready
- All pages

The snippet renders only on `/products/` pages.

```html
<script>
(function () {
  var HOST_ID = "reviews-productdetails-after-test";
  var API_URL = "https://api.open-meteo.com/v1/forecast?latitude=55.7558&longitude=37.6173&current=temperature_2m,wind_speed_10m&timezone=Europe%2FMoscow";

  function isProductPage() {
    return location.pathname.indexOf("/products/") === 0;
  }

  function extractSku() {
    var html = document.documentElement.innerHTML;
    var match = /"sku"\s*:\s*"([^"]+)"/i.exec(html);
    return match ? match[1] : "";
  }

  function removeHost() {
    var old = document.getElementById(HOST_ID);
    if (old && old.parentNode) {
      old.parentNode.removeChild(old);
    }
  }

  function placeAfterProductMain(host) {
    var details = document.querySelector('[data-testid="ProductDetails"]');
    if (!details) {
      return false;
    }

    var main = details.querySelector("main");
    if (!main || !main.parentNode) {
      return false;
    }

    host.style.display = "block";
    host.style.width = "100%";
    host.style.maxWidth = "none";
    host.style.margin = "32px 0";
    host.style.padding = "0";
    host.style.boxSizing = "border-box";
    host.style.gridColumn = "1 / -1";
    host.style.flexBasis = "100%";
    host.style.order = "999";

    main.parentNode.insertBefore(host, main.nextSibling);
    return true;
  }

  function buildHost(sku) {
    var host = document.createElement("div");
    host.id = HOST_ID;
    host.setAttribute("data-sku", sku || "");

    var shadow = host.attachShadow({ mode: "open" });
    shadow.innerHTML = ''
      + '<style>'
      + '.rw{box-sizing:border-box;font-family:Inter,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color:#1f2520;border:1px solid #d9ded7;border-radius:8px;background:#fff;overflow:hidden;box-shadow:0 12px 32px rgba(38,54,44,.08)}'
      + '.rw *{box-sizing:border-box}'
      + '.rw-head{display:flex;gap:16px;align-items:center;justify-content:space-between;padding:18px 20px;border-bottom:1px solid #d9ded7}'
      + '.rw-score{display:flex;gap:14px;align-items:center}'
      + '.rw-num{display:grid;place-items:center;width:72px;aspect-ratio:1;border-radius:8px;background:#f5f7f2;font-size:28px;font-weight:800}'
      + '.rw-title{margin:0;font-size:18px;line-height:1.25;font-weight:800}'
      + '.rw-sub{margin:5px 0 0;color:#687067;font-size:14px}'
      + '.rw-stars{color:#d8a316;white-space:nowrap}'
      + '.rw-body{display:grid;gap:12px;padding:16px 20px 20px}'
      + '.rw-card{padding:14px;border:1px solid #d9ded7;border-radius:8px;background:#fbfcfa}'
      + '.rw-card-top{display:flex;align-items:flex-start;justify-content:space-between;gap:12px;margin-bottom:8px}'
      + '.rw-name{font-weight:800}'
      + '.rw-market{display:inline-flex;margin-left:8px;padding:3px 7px;border-radius:6px;background:#7b4de3;color:#fff;font-size:12px;font-weight:800}'
      + '.rw-text{margin:0;color:#1f2520;line-height:1.5}'
      + '.rw-note{margin-top:10px;color:#687067;font-size:13px;line-height:1.45}'
      + '.rw-api{margin-top:10px;padding:10px 12px;border-radius:8px;background:#f5f7f2;color:#687067;font-size:13px}'
      + '.rw-ok{color:#2f7a5b;font-weight:800}'
      + '.rw-err{color:#9b2c2c;font-weight:800}'
      + '@media(max-width:560px){.rw-head{align-items:flex-start;flex-direction:column}.rw-num{width:64px;font-size:24px}.rw-card-top{flex-direction:column}}'
      + '</style>'
      + '<section class="rw" aria-label="Отзывы покупателей">'
      + '<div class="rw-head">'
      + '<div class="rw-score">'
      + '<div class="rw-num" id="score">...</div>'
      + '<div>'
      + '<h2 class="rw-title">Отзывы покупателей</h2>'
      + '<p class="rw-sub">Тест API + вставка после ProductDetails main · артикул ' + (sku || "не найден") + '</p>'
      + '</div>'
      + '</div>'
      + '<div class="rw-stars">★★★★★</div>'
      + '</div>'
      + '<div class="rw-body">'
      + '<article class="rw-card">'
      + '<div class="rw-card-top">'
      + '<div><span class="rw-name">API-покупатель</span><span class="rw-market">TEST</span></div>'
      + '<div class="rw-stars">★★★★★</div>'
      + '</div>'
      + '<p class="rw-text" id="review-text">Загружаем тестовые данные из публичного API...</p>'
      + '<div class="rw-note">Блок вставлен после основной сетки товара: ниже фото, цены и описания, перед следующими секциями.</div>'
      + '<div class="rw-api" id="api-state">fetch pending...</div>'
      + '</article>'
      + '</div>'
      + '</section>';

    return host;
  }

  function render() {
    if (!document.body) {
      return setTimeout(render, 100);
    }

    if (!isProductPage()) {
      removeHost();
      return;
    }

    if (document.getElementById(HOST_ID)) {
      return;
    }

    var sku = extractSku();
    var host = buildHost(sku);

    if (!placeAfterProductMain(host)) {
      setTimeout(render, 300);
      return;
    }

    var shadow = host.shadowRoot;

    fetch(API_URL, { headers: { "Accept": "application/json" } })
      .then(function (response) {
        if (!response.ok) {
          throw new Error("HTTP " + response.status);
        }
        return response.json();
      })
      .then(function (data) {
        var current = data.current || {};
        var temp = current.temperature_2m;
        var wind = current.wind_speed_10m;

        shadow.getElementById("score").textContent = "5.0";
        shadow.getElementById("review-text").textContent =
          "Связь с API работает. Сейчас в Москве " + temp + " °C, ветер " + wind + " км/ч. Значит Tag Manager может получить JSON и отрисовать блок отзывов.";

        shadow.getElementById("api-state").innerHTML =
          '<span class="rw-ok">API OK</span> · Open-Meteo · temperature_2m=' + temp + ', wind_speed_10m=' + wind;

        console.log("[reviews-tag-manager-smoke] fetch ok", data);
      })
      .catch(function (error) {
        shadow.getElementById("score").textContent = "!";
        shadow.getElementById("review-text").textContent =
          "Fetch не прошел. Это может быть CORS, сеть, sandbox или ошибка API.";

        shadow.getElementById("api-state").innerHTML =
          '<span class="rw-err">API ERROR</span> · ' + String(error && error.message ? error.message : error);

        console.warn("[reviews-tag-manager-smoke] fetch failed", error);
      });
  }

  function schedule() {
    clearTimeout(schedule.timer);
    schedule.timer = setTimeout(render, 500);
  }

  ["pushState", "replaceState"].forEach(function (name) {
    var original = history[name];
    history[name] = function () {
      var result = original.apply(this, arguments);
      removeHost();
      schedule();
      return result;
    };
  });

  window.addEventListener("popstate", function () {
    removeHost();
    schedule();
  });

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", schedule);
  } else {
    schedule();
  }

  var observerTimer = setInterval(function () {
    if (!document.body) {
      return;
    }
    clearInterval(observerTimer);

    new MutationObserver(function () {
      if (isProductPage() && !document.getElementById(HOST_ID)) {
        schedule();
      }
    }).observe(document.body, { childList: true, subtree: true });
  }, 100);
})();
</script>
```
