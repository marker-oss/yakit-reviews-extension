import { useEffect, useMemo, useState } from 'react'
import { apiGet } from '../api'
import { toast } from '../toast'

type TenantInfo = { publicKey?: string }

export default function Embed() {
  const [baseUrl, setBaseUrl] = useState(window.location.origin)
  const [anchorSelector, setAnchorSelector] = useState('')
  const [publicKey, setPublicKey] = useState('')

  useEffect(() => {
    apiGet<TenantInfo>('/admin/api/tenant')
      .then((t) => setPublicKey(t.publicKey || ''))
      .catch(() => {})
  }, [])

  const snippet = useMemo(() => {
    const base = baseUrl.replace(/\/$/, '')
    const config: Record<string, string> = {
      dataBase: `${base}/reviews-data`,
      widgetJsUrl: `${base}/reviews-widget.js`,
      widgetCssUrl: `${base}/reviews-widget.css`,
      configBase: base,
    }
    if (publicKey) config.publicKey = publicKey
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
  }, [anchorSelector, baseUrl, publicKey])

  const insecureBase = baseUrl.trim().startsWith('http://')

  async function copy() {
    await navigator.clipboard.writeText(snippet)
    toast.success('Скопировано')
  }

  const installVariants = useMemo(() => {
    const base = baseUrl.replace(/\/$/, '')
    return {
      anchor: `<div id="reviews-widget"></div>`,
      anchorHome: `<div id="reviews-homepage"></div>`,
      withAnchor: `<div id="reviews-widget">
${snippet
  .split('\n')
  .map((line) => '  ' + line)
  .join('\n')}
</div>`,
      headScript: `<script src="${base}/loader.js" data-reviews-embed async></script>`,
    }
  }, [snippet, baseUrl])

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

      <section className="panel">
        <h3>Без тег-менеджера: через CMS</h3>
        <p className="muted">
          Если тег-менеджер не используется, тот же сниппет вставляется напрямую в конструктор сайта.
          Ключевое отличие: место виджета задаёт сам блок CMS, поэтому добавьте якорь рядом со сниппетом.
        </p>
        <h4>1. Тильда: блок T123 «HTML-код»</h4>
        <p className="muted">
          Библиотека блоков → Другое → T123. Вставьте сниппет вместе с якорем (контент блока и есть место виджета):
        </p>
        <pre className="snippet">{installVariants.withAnchor}</pre>
        <p className="muted">
          Для главной используйте тот же приём с <code>id=«reviews-homepage»</code>. Глобальный вариант (все страницы
          сразу) — «Настройки сайта → Ещё → HTML-код для вставки внутрь head» со сниппетом без якоря: тогда якорь
          кладётся на каждую нужную страницу отдельным блоком, а автопривязка после ProductDetails сработает без якоря.
        </p>
        <h4>2. WordPress: блок Custom HTML или шорткод</h4>
        <p className="muted">
          Gutenberg-блок «Custom HTML» на шаблоне товара — вставьте якорь и сниппет как выше. Для повторного
          использования оберните в шорткод через functions.php дочерней темы:
        </p>
        <pre className="snippet">{`function render_reviews_widget() {
  return \`${installVariants.anchor}
  <script src="${baseUrl.replace(/\/$/, '')}/loader.js" async></script>\`;
}
add_shortcode('reviews_widget', 'render_reviews_widget');`}</pre>
        <p className="muted">
          Затем <code>[reviews_widget]</code> в шаблоне карточки товара. REVIEWS_EMBED_CONFIG можно не задавать
          inline — loader поднимет его из data-атрибутов.
        </p>
        <h4>3. Произвольная CMS: якорь + скрипт</h4>
        <p className="muted">
          Минимальный вариант — div-якорь в шаблоне товара и один скрипт в head/footer всех страниц:
        </p>
        <pre className="snippet">{`<!-- в шаблон карточки товара -->
${installVariants.anchor}

<!-- в head/footer всех страниц -->
${installVariants.headScript}`}</pre>
        <p className="muted">
          Артикул loader возьмёт из JSON-LD / data-article на якоре / ссылочного индекса; без якоря сработает
          автопривязка после стандартного блока Кита.
        </p>
      </section>
    </section>
  )
}
