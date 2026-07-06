import { useEffect, useMemo, useState } from 'react'
import { apiGet, apiWrite } from '../api'
import { toast } from '../toast'
import { useDirty } from '../useDirty'
import { defaultWidgetConfig, mergeWidgetConfig, type MarketplacePolicy, type WidgetConfig, type WidgetContext } from '../widgetConfig'

type VersionItem = {
  version: number
  active: boolean
  created_at: string
}

const visibilityLabels: Record<keyof WidgetConfig['visibility'], string> = {
  photos: 'Фото',
  sellerAnswers: 'Ответы',
  prosCons: 'Плюсы и минусы',
  marketplaceBadges: 'Бейджи',
  ratingDistribution: 'Распределение',
  filters: 'Фильтры',
}

const rankingLabels: Record<WidgetConfig['ranking'][number]['field'], string> = {
  pinned: 'Закрепленные',
  hasPhoto: 'С фото',
  hasText: 'С текстом',
  rating: 'Высокая оценка',
  createdAt: 'Свежие',
}

const marketplaceLabels: Record<keyof WidgetConfig['marketplacePolicy'], string> = {
  wb: 'Wildberries',
  ym: 'Яндекс Маркет',
  ozon: 'Ozon',
}

type EditorTab = 'look' | 'content' | 'marketplaces' | 'versions'

export default function Editor() {
  const [context, setContext] = useState<WidgetContext>('product')
  const [cfg, setCfg] = useState<WidgetConfig>(defaultWidgetConfig)
  const [baseline, setBaseline] = useState<WidgetConfig>(defaultWidgetConfig)
  const [versions, setVersions] = useState<VersionItem[]>([])
  const [tab, setTab] = useState<EditorTab>('look')

  function load(nextContext = context) {
    Promise.all([
      apiGet<Partial<WidgetConfig>>(`/admin/api/widget-config/${nextContext}`),
      apiGet<{ versions: VersionItem[] }>(`/admin/api/widget-config/${nextContext}/versions`),
    ])
      .then(([config, versionData]) => {
        setCfg(mergeWidgetConfig(config))
        setBaseline(mergeWidgetConfig(config))
        setVersions(versionData.versions)
      })
      .catch((err) => toast.error(err instanceof Error ? err.message : 'Запрос не выполнен'))
  }

  useEffect(() => load(context), [context])

  const preview = useMemo(() => previewDocument(cfg, context), [cfg, context])
  const dirty = useDirty(cfg, baseline)
  const activeVersion = versions.find((v) => v.active)?.version ?? null

  async function publish() {
    try {
      const res = await apiWrite<{ version: number }>('POST', `/admin/api/widget-config/${context}`, cfg)
      toast.success(`Опубликована v${res.version} — уже на сайте`)
      setBaseline(cfg)
      load(context)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Запрос не выполнен')
    }
  }

  async function rollback(version: number) {
    try {
      await apiWrite('POST', `/admin/api/widget-config/${context}/rollback/${version}`)
      toast.success(`Активна версия ${version}`)
      load(context)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Запрос не выполнен')
    }
  }

  function setTheme<K extends keyof WidgetConfig['theme']>(key: K, value: WidgetConfig['theme'][K]) {
    setCfg({ ...cfg, theme: { ...cfg.theme, [key]: value } })
  }

  function setTypography<K extends keyof WidgetConfig['typography']>(key: K, value: WidgetConfig['typography'][K]) {
    setCfg({ ...cfg, typography: { ...cfg.typography, [key]: value } })
  }

  function setLayout<K extends keyof WidgetConfig['layout']>(key: K, value: WidgetConfig['layout'][K]) {
    setCfg({ ...cfg, layout: { ...cfg.layout, [key]: value } })
  }

  function setHeader<K extends keyof WidgetConfig['header']>(key: K, value: WidgetConfig['header'][K]) {
    setCfg({ ...cfg, header: { ...cfg.header, [key]: value } })
  }

  function setVisibility<K extends keyof WidgetConfig['visibility']>(key: K, value: WidgetConfig['visibility'][K]) {
    setCfg({ ...cfg, visibility: { ...cfg.visibility, [key]: value } })
  }

  function setDefaults<K extends keyof WidgetConfig['defaults']>(key: K, value: WidgetConfig['defaults'][K]) {
    setCfg({ ...cfg, defaults: { ...cfg.defaults, [key]: value } })
  }

  function setMarketplacePolicy<K extends keyof MarketplacePolicy>(
    marketplace: keyof WidgetConfig['marketplacePolicy'],
    key: K,
    value: MarketplacePolicy[K],
  ) {
    setCfg({
      ...cfg,
      marketplacePolicy: {
        ...cfg.marketplacePolicy,
        [marketplace]: { ...cfg.marketplacePolicy[marketplace], [key]: value },
      },
    })
  }

  function moveRanking(index: number, direction: -1 | 1) {
    const next = [...cfg.ranking]
    const target = index + direction
    if (target < 0 || target >= next.length) return
    ;[next[index], next[target]] = [next[target], next[index]]
    setCfg({ ...cfg, ranking: next })
  }

  return (
    <section className="editor-layout">
      <section className="stack">
        <div className="editor-toolbar">
          <select value={context} onChange={(e) => setContext(e.target.value as WidgetContext)}>
            <option value="product">Карточка товара</option>
            <option value="homepage">Главная</option>
          </select>
          {activeVersion !== null && <span className="live-badge">Сейчас в эфире: v{activeVersion}</span>}
          {dirty && <span className="dirty-badge">Есть изменения</span>}
          <button onClick={publish}>Опубликовать</button>
        </div>
        <nav className="editor-tabs">
          <button className={tab === 'look' ? 'active' : ''} onClick={() => setTab('look')}>Внешний вид</button>
          <button className={tab === 'content' ? 'active' : ''} onClick={() => setTab('content')}>Отбор отзывов</button>
          <button className={tab === 'marketplaces' ? 'active' : ''} onClick={() => setTab('marketplaces')}>Площадки</button>
          <button className={tab === 'versions' ? 'active' : ''} onClick={() => setTab('versions')}>Версии</button>
        </nav>

        {tab === 'look' && (
          <>
            <section className="panel form-grid">
              <label>
                <span>Заголовок</span>
                <input
                  type="text"
                  value={cfg.header.title}
                  placeholder={defaultWidgetConfig.header.title}
                  onChange={(e) => setHeader('title', e.target.value)}
                />
              </label>
              <ColorField label="Акцент" value={cfg.theme.accent} onChange={(value) => setTheme('accent', value)} />
              <ColorField label="Текст" value={cfg.theme.text} onChange={(value) => setTheme('text', value)} />
              <ColorField label="Фон" value={cfg.theme.panel} onChange={(value) => setTheme('panel', value)} />
              <ColorField label="Граница" value={cfg.theme.border} onChange={(value) => setTheme('border', value)} />
              <label>
                <span>Масштаб</span>
                <input type="number" step="0.05" min="0.85" max="1.25" value={cfg.typography.scale} onChange={(e) => setTypography('scale', Number(e.target.value))} />
              </label>
              <label>
                <span>Радиус</span>
                <input type="number" min="0" max="24" value={cfg.typography.radius} onChange={(e) => setTypography('radius', Number(e.target.value))} />
              </label>
              <label>
                <span>Плотность</span>
                <select value={cfg.typography.density} onChange={(e) => setTypography('density', e.target.value as WidgetConfig['typography']['density'])}>
                  <option value="comfortable">Обычная</option>
                  <option value="compact">Компактная</option>
                </select>
              </label>
              <label>
                <span>Макет</span>
                <select value={cfg.layout.mode} onChange={(e) => setLayout('mode', e.target.value as WidgetConfig['layout']['mode'])}>
                  <option value="list">Список</option>
                  <option value="grid">Сетка</option>
                  <option value="carousel">Лента</option>
                </select>
              </label>
              <label>
                <span>Колонки</span>
                <input type="number" min="1" max="4" value={cfg.layout.columns} onChange={(e) => setLayout('columns', Number(e.target.value))} />
              </label>
              <label>
                <span>Размер страницы</span>
                <input type="number" min="1" max="24" value={cfg.layout.pageSize} onChange={(e) => setLayout('pageSize', Number(e.target.value))} />
              </label>
            </section>

            <section className="panel check-grid">
              {(Object.keys(cfg.visibility) as (keyof WidgetConfig['visibility'])[]).map((key) => (
                <label className="checkbox" key={key}>
                  <input type="checkbox" checked={cfg.visibility[key]} onChange={(e) => setVisibility(key, e.target.checked)} />
                  <span>{visibilityLabels[key]}</span>
                </label>
              ))}
            </section>
          </>
        )}

        {tab === 'content' && (
          <section className="panel">
            <h3>Выдача отзывов</h3>
            <div className="form-grid">
              <label>
                <span>Минимальная оценка</span>
                <select value={cfg.defaults.minRating} onChange={(e) => setDefaults('minRating', Number(e.target.value))}>
                  <option value={0}>Все</option>
                  <option value={4}>4 и выше</option>
                  <option value={5}>Только 5</option>
                </select>
              </label>
              <label>
                <span>Маркетплейс</span>
                <select value={cfg.defaults.marketplace} onChange={(e) => setDefaults('marketplace', e.target.value as WidgetConfig['defaults']['marketplace'])}>
                  <option value="all">Все</option>
                  <option value="wb">Wildberries</option>
                  <option value="ozon">Ozon</option>
                  <option value="ym">Яндекс Маркет</option>
                </select>
              </label>
              <label>
                <span>Сортировка</span>
                <select value={cfg.defaults.initialSort} onChange={(e) => setDefaults('initialSort', e.target.value as WidgetConfig['defaults']['initialSort'])}>
                  <option value="relevance">Релевантные</option>
                  <option value="newest">Сначала новые</option>
                  <option value="highest">Сначала высокая оценка</option>
                  <option value="media">Сначала с медиа</option>
                  <option value="lowest">Сначала низкая оценка</option>
                </select>
              </label>
            </div>
            <div className="check-grid">
              <label className="checkbox">
                <input type="checkbox" checked={cfg.defaults.requireText} onChange={(e) => setDefaults('requireText', e.target.checked)} />
                <span>Только с текстом</span>
              </label>
              <label className="checkbox">
                <input type="checkbox" checked={cfg.defaults.requirePhoto} onChange={(e) => setDefaults('requirePhoto', e.target.checked)} />
                <span>Только с фото</span>
              </label>
              <label className="checkbox">
                <input type="checkbox" checked={cfg.defaults.onlyWithAnswer} onChange={(e) => setDefaults('onlyWithAnswer', e.target.checked)} />
                <span>Только с ответом</span>
              </label>
              <label className="checkbox">
                <input type="checkbox" checked={cfg.defaults.photoFirst} onChange={(e) => setDefaults('photoFirst', e.target.checked)} />
                <span>Фото выше</span>
              </label>
              <label className="checkbox">
                <input type="checkbox" checked={cfg.defaults.textFirst} onChange={(e) => setDefaults('textFirst', e.target.checked)} />
                <span>Текст выше</span>
              </label>
            </div>
            <div className="rows">
              {cfg.ranking.map((rule, index) => (
                <div className="row" key={rule.field}>
                  <span>{rankingLabels[rule.field]}</span>
                  <span className="status-muted">{rule.direction === 'desc' ? 'выше' : 'ниже'}</span>
                  <button className="secondary" disabled={index === 0} onClick={() => moveRanking(index, -1)}>Вверх</button>
                  <button className="secondary" disabled={index === cfg.ranking.length - 1} onClick={() => moveRanking(index, 1)}>Вниз</button>
                </div>
              ))}
            </div>
          </section>
        )}

        {tab === 'marketplaces' && (
          <section className="panel">
            <h3>Площадки в публичном виджете</h3>
            <div className="marketplace-policy-grid">
              {(['wb', 'ym', 'ozon'] as (keyof WidgetConfig['marketplacePolicy'])[]).map((marketplace) => {
                const policy = cfg.marketplacePolicy[marketplace]
                return (
                  <div className="marketplace-policy-row" key={marketplace}>
                    <strong>{marketplaceLabels[marketplace]}</strong>
                    <label className="checkbox">
                      <input type="checkbox" checked={!policy.hidden} onChange={(e) => setMarketplacePolicy(marketplace, 'hidden', !e.target.checked)} />
                      <span>Показывать отзывы</span>
                    </label>
                    <label>
                      <span>Публичное название</span>
                      <input
                        value={policy.label}
                        onChange={(e) => setMarketplacePolicy(marketplace, 'label', e.target.value)}
                        placeholder={marketplace === 'wb' ? 'Маркетплейс' : marketplaceLabels[marketplace]}
                      />
                    </label>
                    <label className="checkbox">
                      <input
                        type="checkbox"
                        checked={policy.showSourceLinks}
                        onChange={(e) => setMarketplacePolicy(marketplace, 'showSourceLinks', e.target.checked)}
                      />
                      <span>Ссылки на источник</span>
                    </label>
                  </div>
                )
              })}
            </div>
          </section>
        )}

        {tab === 'versions' && (
          <section className="panel">
            <h3>Версии</h3>
            <div className="rows">
              {versions.length === 0 && <p className="muted">Версий пока нет.</p>}
              {versions.map((item) => (
                <div className="row" key={item.version}>
                  <span>v{item.version}</span>
                  <span className={item.active ? 'status-ok' : 'status-muted'}>{item.active ? 'активна' : new Date(item.created_at).toLocaleString()}</span>
                  <button className="secondary" disabled={item.active} onClick={() => rollback(item.version)}>
                    Откатить
                  </button>
                </div>
              ))}
            </div>
          </section>
        )}
      </section>

      <section className="preview-pane">
        <iframe title="Предпросмотр виджета" srcDoc={preview} />
      </section>
    </section>
  )
}

function ColorField({ label, value, onChange }: { label: string; value: string; onChange: (value: string) => void }) {
  return (
    <label>
      <span>{label}</span>
      <span className="color-field">
        <input type="color" value={value} onChange={(e) => onChange(e.target.value)} />
        <input value={value} onChange={(e) => onChange(e.target.value)} />
      </span>
    </label>
  )
}

function previewDocument(config: WidgetConfig, context: WidgetContext) {
  const configJson = JSON.stringify(config).replace(/</g, '\\u003c')
  const contextJson = JSON.stringify(context)
  return `<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <link rel="stylesheet" href="/reviews-widget.css">
  <style>body{margin:0;padding:16px;background:#FBF8F4}</style>
</head>
<body>
  <div id="preview" class="reviews-widget reviews-widget-root"></div>
  <script src="/reviews-widget.js"></script>
  <script>
    ReviewsWidget.mount(document.getElementById('preview'), {
      reviews: ReviewsWidget.sampleReviews,
      productName: 'Платье (пример)',
      context: ${contextJson},
      config: ${configJson}
    });
  </script>
</body>
</html>`
}
