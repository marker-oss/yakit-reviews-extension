import { useEffect, useState } from 'react'
import { apiGet, apiWrite } from '../api'
import { toast } from '../toast'
import { useDirty } from '../useDirty'

interface SettingsResponse {
  agreementUrl: string
  reviewTermsUrl: string
  shopOrigin: string
  sitemapUrl: string
}

interface CustomField {
  id: string
  label: string
  type: 'select' | 'chips' | 'text'
  options: string[]
  required: boolean
}

const defaultField: Omit<CustomField, 'id'> = {
  label: '',
  type: 'chips',
  options: [],
  required: false,
}


export default function Settings() {
  const [agreementUrl, setAgreementUrl] = useState('')
  const [reviewTermsUrl, setReviewTermsUrl] = useState('')
  const [shopOrigin, setShopOrigin] = useState('')
  const [sitemapUrl, setSitemapUrl] = useState('')
  const [customFields, setCustomFields] = useState<CustomField[]>([])
  const [busy, setBusy] = useState(false)
  const [baseline, setBaseline] = useState<SettingsResponse>({
    agreementUrl: '',
    reviewTermsUrl: '',
    shopOrigin: '',
    sitemapUrl: '',
  })
  const [fieldsBaseline, setFieldsBaseline] = useState<CustomField[]>([])

  // DSR (152-ФЗ) state
  const [dsrEmail, setDsrEmail] = useState('')
  const [dsrResult, setDsrResult] = useState<{ reviews: unknown[] } | null>(null)
  const [dsrBusy, setDsrBusy] = useState(false)
  useEffect(() => {
    apiGet<SettingsResponse>('/admin/api/settings')
      .then((data) => {
        const next = {
          agreementUrl: data.agreementUrl ?? '',
          reviewTermsUrl: data.reviewTermsUrl ?? '',
          shopOrigin: data.shopOrigin ?? '',
          sitemapUrl: data.sitemapUrl ?? '',
        }
        setAgreementUrl(next.agreementUrl)
        setReviewTermsUrl(next.reviewTermsUrl)
        setShopOrigin(next.shopOrigin)
        setSitemapUrl(next.sitemapUrl)
        setBaseline(next)
      })
      .catch((err) => toast.error(err instanceof Error ? err.message : 'Запрос не выполнен'))
    apiGet<{ payload?: string }>('/admin/api/widget-config/product')
      .then((data) => {
        const parsed = data.payload ? (JSON.parse(data.payload) as { customFields?: CustomField[] }) : {}
        const fields = (parsed.customFields ?? []).map((f, i) => ({
          ...defaultField,
          ...f,
          id: f.id || `field-${i + 1}`,
        }))
        setCustomFields(fields)
        setFieldsBaseline(fields)
      })
      .catch(() => {
        /* no published config yet — empty repeater is the normal first-run state */
      })
  }, [])

  const current: SettingsResponse = { agreementUrl, reviewTermsUrl, shopOrigin, sitemapUrl }
  const dirty = useDirty(current, baseline)
  const fieldsDirty = useDirty(customFields, fieldsBaseline)

  function updateField(index: number, patch: Partial<CustomField>) {
    setCustomFields(customFields.map((field, i) => (i === index ? { ...field, ...patch } : field)))
  }

  function removeField(index: number) {
    setCustomFields(customFields.filter((_, i) => i !== index))
  }

  async function save(event: React.FormEvent) {
    event.preventDefault()
    setBusy(true)
    try {
      const data = await apiWrite<SettingsResponse>('PUT', '/admin/api/settings', {
        agreementUrl: agreementUrl.trim(),
        reviewTermsUrl: reviewTermsUrl.trim(),
        shopOrigin: shopOrigin.trim(),
        sitemapUrl: sitemapUrl.trim(),
      })
      const next = {
        agreementUrl: data.agreementUrl ?? '',
        reviewTermsUrl: data.reviewTermsUrl ?? '',
        shopOrigin: shopOrigin.trim(),
        sitemapUrl: sitemapUrl.trim(),
      }
      setAgreementUrl(next.agreementUrl)
      setReviewTermsUrl(next.reviewTermsUrl)
      setShopOrigin(next.shopOrigin)
      setSitemapUrl(next.sitemapUrl)
      setBaseline(next)

      if (fieldsDirty) {
        const existing = await apiGet<{ payload?: string }>(
          '/admin/api/widget-config/product',
        )
        const cfg = existing.payload
        const merged = cfg ? (JSON.parse(cfg) as Record<string, unknown>) : {}
        merged.customFields = customFields.map((field) => ({
          ...field,
          label: field.label.trim(),
          options: field.options,
        }))
        const published = await apiWrite<{ version: number }>(
          'POST',
          '/admin/api/widget-config/product',
          merged,
        )
        setFieldsBaseline(customFields)
        toast.info(`Поля формы опубликованы в v${published.version}`)
      }
      toast.success('Сохранено')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Запрос не выполнено')
    } finally {
      setBusy(false)
    }
  }

  async function dsrLookup() {
    setDsrResult(null)
    setDsrBusy(true)
    try {
      const data = await apiGet<{ reviews: unknown[] }>(
        `/admin/api/dsr/lookup?email=${encodeURIComponent(dsrEmail)}`,
      )
      setDsrResult(data)
      toast.info(`Найдено отзывов: ${data.reviews.length}`)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Ошибка')
    } finally {
      setDsrBusy(false)
    }
  }

  async function dsrDelete() {
    if (
      !window.confirm(
        'Удалить все данные этого субъекта без возможности восстановления?',
      )
    )
      return
    if (!window.confirm(`Подтвердите: безвозвратно удалить данные для "${dsrEmail}"?`)) return
    setDsrBusy(true)
    try {
      const r = await apiWrite<{ deleted: number }>('POST', '/admin/api/dsr/delete', {
        email: dsrEmail,
      })
      toast.success(`Удалено: ${r.deleted}`)
      setDsrResult(null)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Ошибка')
    } finally {
      setDsrBusy(false)
    }
  }

  return (
    <section className="stack">
      <section className="panel">
        <form className="stack" onSubmit={save}>
          <h3>Форма отзыва на сайте</h3>
          <p className="muted">
            Ссылка на страницу с согласием на обработку персональных данных. Показывается под
            формой отзыва рядом с галочкой согласия. Оставьте пустым, чтобы убрать ссылку.
          </p>
          <label className="stack">
            <span>URL страницы согласия / пользовательского соглашения</span>
            <input
              type="url"
              value={agreementUrl}
              onChange={(e) => setAgreementUrl(e.target.value)}
              placeholder="https://ваш-магазин.ру/personal-data-consent"
            />
          </label>
          <label className="stack">
            <span>URL правил публикации отзывов (показывается в форме, необязательно)</span>
            <input
              type="url"
              value={reviewTermsUrl}
              onChange={(e) => setReviewTermsUrl(e.target.value)}
              placeholder="https://ваш-магазин.ру/review-terms"
            />
          </label>

          <h3>Дополнительные поля формы (рост / вес / посадка…)</h3>
          <p className="muted">
            До 6 полей, до 12 вариантов в каждом. Варианты — по одному в строке. Поля публикуются
            вместе с версией виджета карточки товара при нажатии «Сохранить».
          </p>
          {customFields.map((field, index) => (
            <div className="stack" key={index}>
              <div className="form-grid">
                <label>
                  <span>Вопрос</span>
                  <input
                    value={field.label}
                    maxLength={60}
                    onChange={(e) => updateField(index, { label: e.target.value })}
                    placeholder="Например: Рост"
                  />
                </label>
                <label>
                  <span>Тип</span>
                  <select
                    value={field.type}
                    onChange={(e) => updateField(index, { type: e.target.value as CustomField['type'] })}
                  >
                    <option value="chips">Чипы (до 5 вариантов)</option>
                    <option value="select">Выпадающий список (6+ вариантов)</option>
                    <option value="text">Свободный текст</option>
                  </select>
                </label>
              </div>
              <label className="stack">
                <span>Варианты (по одному в строке)</span>
                <textarea
                  rows={3}
                  value={field.options.join('\n')}
                  disabled={field.type === 'text'}
                  onChange={(e) =>
                    updateField(index, {
                      options: e.target.value
                        .split('\n')
                        .map((line) => line.trim())
                        .filter(Boolean),
                    })
                  }
                />
              </label>
              <label className="checkbox">
                <input
                  type="checkbox"
                  checked={field.required}
                  onChange={(e) => updateField(index, { required: e.target.checked })}
                />
                <span>Обязательное</span>
              </label>
              <div className="toolbar">
                <button
                  type="button"
                  className="secondary"
                  disabled={customFields.length >= 6}
                  onClick={() =>
                    setCustomFields([
                      ...customFields,
                      { ...defaultField, id: `field-${Date.now()}` },
                    ])
                  }
                  hidden={index !== customFields.length - 1}
                >
                  Добавить поле
                </button>
                <button type="button" className="secondary" onClick={() => removeField(index)}>
                  Удалить
                </button>
              </div>
            </div>
          ))}
          {customFields.length === 0 && (
            <div className="toolbar">
              <button
                type="button"
                className="secondary"
                onClick={() => setCustomFields([{ ...defaultField, id: `field-${Date.now()}` }])}
              >
                Добавить поле
              </button>
            </div>
          )}
          {fieldsDirty && <span className="dirty-badge">Поля формы изменены</span>}

          <h3>Магазин и каталог</h3>
          <p className="muted">
            Адрес магазина используется в двух местах: он разрешает сайту магазина загружать
            виджет отзывов (CORS — без него виджет на сайте останется без стилей и данных;
            www-вариант домена разрешается автоматически, изменения применяются сразу после
            сохранения) и служит источником для кнопки «Обновить каталог товаров» на странице
            «Маркетплейсы» — sitemap возьмётся как <code>/sitemap.xml</code>, либо задайте точный
            адрес sitemap, если он по другому пути.
          </p>
          <label className="stack">
            <span>Адрес магазина (origin)</span>
            <input
              type="url"
              value={shopOrigin}
              onChange={(e) => setShopOrigin(e.target.value)}
              placeholder="https://ваш-магазин.ру"
            />
          </label>
          <label className="stack">
            <span>Адрес sitemap (необязательно — переопределяет адрес магазина)</span>
            <input
              type="url"
              value={sitemapUrl}
              onChange={(e) => setSitemapUrl(e.target.value)}
              placeholder="https://ваш-магазин.ру/sitemap.xml"
            />
          </label>

          <div className="toolbar">
            <button type="submit" disabled={busy}>
              {busy ? 'Сохраняем' : 'Сохранить'}
            </button>
            {dirty && <span className="dirty-badge">Есть изменения</span>}
          </div>
        </form>
      </section>

      <section className="panel">
        <div className="stack">
          <h3>Запросы субъектов (152-ФЗ)</h3>
          <p className="muted">
            Поиск, выгрузка и удаление персональных данных по запросу субъекта. Укажите email,
            использованный при отправке отзыва на сайте.
          </p>
          <label className="stack">
            <span>Email субъекта</span>
            <input
              type="email"
              value={dsrEmail}
              onChange={(e) => setDsrEmail(e.target.value)}
              placeholder="subject@example.com"
            />
          </label>
          <div className="toolbar">
            <button type="button" onClick={dsrLookup} disabled={dsrBusy || !dsrEmail.trim()}>
              Найти
            </button>
            {dsrResult !== null && (
              <a href={`/admin/api/dsr/export?email=${encodeURIComponent(dsrEmail)}`}>
                Скачать выгрузку
              </a>
            )}
            {dsrResult !== null && dsrResult.reviews.length > 0 && (
              <button
                type="button"
                onClick={dsrDelete}
                disabled={dsrBusy}
                style={{ color: 'var(--danger)' }}
              >
                Удалить все данные
              </button>
            )}
          </div>
          <p className="muted">
            Для отзывов с маркетплейсов удаляется только наша копия. Оригинал на WB / Ozon /
            Яндекс Маркет удаляется через сам маркетплейс.
          </p>
        </div>
      </section>
    </section>
  )
}
