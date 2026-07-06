import { useMemo, useState } from 'react'
import { toast } from '../toast'

export default function Embed() {
  const [baseUrl, setBaseUrl] = useState(window.location.origin)
  const [anchorSelector, setAnchorSelector] = useState('')

  const snippet = useMemo(() => {
    const base = baseUrl.replace(/\/$/, '')
    const config: Record<string, string> = {
      dataBase: `${base}/reviews-data`,
      widgetJsUrl: `${base}/reviews-widget.js`,
      widgetCssUrl: `${base}/reviews-widget.css`,
      configBase: base,
    }
    if (anchorSelector.trim()) config.anchorSelector = anchorSelector.trim()
    // На страницу вставляется loader.js — он читает REVIEWS_EMBED_CONFIG,
    // находит якорь и артикул и уже сам подгружает reviews-widget.js/css.
    // Контекст (карточка/главная) loader определяет по URL страницы, поэтому
    // сниппет один для всего сайта.
    const json = JSON.stringify(config, null, 2).replace(/</g, '\\u003c')
    return `<script>
window.REVIEWS_EMBED_CONFIG = ${json};
</script>
<script src="${base}/loader.js" async></script>`
  }, [anchorSelector, baseUrl])

  const insecureBase = baseUrl.trim().startsWith('http://')

  async function copy() {
    await navigator.clipboard.writeText(snippet)
    toast.success('Скопировано')
  }

  return (
    <section className="stack">
      <section className="panel form-grid">
        <label>
          <span>База</span>
          <input value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} />
        </label>
        <label>
          <span>Anchor selector (необязательно)</span>
          <input value={anchorSelector} onChange={(e) => setAnchorSelector(e.target.value)} placeholder="#reviews-widget" />
        </label>
      </section>
      <div className="toolbar">
        <button onClick={copy}>Скопировать</button>
      </div>
      {insecureBase && (
        <p className="status-warn">
          База указана с http:// — браузеры молча заблокируют такой скрипт на https-сайте (mixed content). Настройте
          HTTPS для сервера отзывов и укажите https-адрес.
        </p>
      )}
      <pre className="snippet">{snippet}</pre>
      <section className="panel">
        <p className="muted">
          Один и тот же код вставляется на все страницы (Custom HTML в Тег Менеджере, триггер DOM Ready): виджет сам
          различает карточку товара и главную по адресу страницы. На карточке он монтируется в блок с
          id=«reviews-widget» (или, если его нет, после стандартного блока Кита), на главной нужен блок с
          id=«reviews-homepage». Свой селектор можно указать в поле выше.
        </p>
        <p className="muted">
          Чтобы браузер не блокировал запросы виджета (CORS), укажите адрес магазина на странице
          «Настройки» (www-вариант домена разрешится автоматически, рестарт не нужен) — либо задайте
          REVIEWS_SHOP_ORIGIN=https://ваш-магазин.ru в .env сервера. Изменения в Тег Менеджере
          попадают на сайт только после публикации контейнера.
        </p>
      </section>
    </section>
  )
}
