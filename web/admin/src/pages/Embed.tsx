import { useMemo, useState } from 'react'
import type { WidgetContext } from '../widgetConfig'

export default function Embed() {
  const [context, setContext] = useState<WidgetContext>('product')
  const [baseUrl, setBaseUrl] = useState(window.location.origin)
  const [anchorSelector, setAnchorSelector] = useState('')
  const [message, setMessage] = useState('')

  const snippet = useMemo(() => {
    const config = {
      dataBase: `${baseUrl.replace(/\/$/, '')}/reviews-data`,
      widgetJsUrl: `${baseUrl.replace(/\/$/, '')}/reviews-widget.js`,
      widgetCssUrl: `${baseUrl.replace(/\/$/, '')}/reviews-widget.css`,
      configBase: baseUrl.replace(/\/$/, ''),
      context,
      anchorSelector,
    }
    return `<script>
window.REVIEWS_EMBED_CONFIG = ${JSON.stringify(config, null, 2)};
</script>
<script src="${config.widgetJsUrl}" async></script>`
  }, [anchorSelector, baseUrl, context])

  async function copy() {
    await navigator.clipboard.writeText(snippet)
    setMessage('Скопировано')
  }

  return (
    <section className="stack">
      <section className="panel form-grid">
        <label>
          <span>Контекст</span>
          <select value={context} onChange={(e) => setContext(e.target.value as WidgetContext)}>
            <option value="product">Карточка товара</option>
            <option value="homepage">Главная</option>
          </select>
        </label>
        <label>
          <span>База</span>
          <input value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} />
        </label>
        <label>
          <span>Anchor selector</span>
          <input value={anchorSelector} onChange={(e) => setAnchorSelector(e.target.value)} placeholder="#reviews-widget" />
        </label>
      </section>
      <div className="toolbar">
        <button onClick={copy}>Скопировать</button>
        {message && <p className="muted">{message}</p>}
      </div>
      <pre className="snippet">{snippet}</pre>
    </section>
  )
}
