import { useEffect, useState } from 'react'
import { apiGet, apiWrite } from '../api'
import { toast } from '../toast'

type DiagItem = { id: string; level: 'ok' | 'warn' | 'fail'; title: string; detail: string; fixHref?: string }
type ActivityItem = { at: string; level: string; source: string; message: string }
type Diagnostics = { checks: DiagItem[]; activity: ActivityItem[] }

const levelLabel: Record<DiagItem['level'], string> = { ok: '✓', warn: '⚠', fail: '✗' }

export default function Status() {
  const [data, setData] = useState<Diagnostics | null>(null)
  const [productUrl, setProductUrl] = useState('')
  const [probe, setProbe] = useState<DiagItem[] | null>(null)
  const [probing, setProbing] = useState(false)

  useEffect(() => {
    apiGet<Diagnostics>('/admin/api/diagnostics')
      .then(setData)
      .catch((e) => toast.error(e instanceof Error ? e.message : 'Не удалось загрузить диагностику'))
  }, [])

  async function runProbe() {
    setProbing(true)
    try {
      const res = await apiWrite<{ checks: DiagItem[] }>('POST', '/admin/api/diagnostics/probe', {
        productUrl: productUrl.trim(),
      })
      setProbe(res.checks)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Проверка не выполнена')
    } finally {
      setProbing(false)
    }
  }

  if (!data) return <p className="muted">Загрузка...</p>

  return (
    <section className="stack">
      <section className="panel">
        <h3>Проверка настройки</h3>
        <div className="stack">
          {data.checks.map((c) => (
            <div className={`diag-item diag-${c.level}`} key={c.id}>
              <span className="diag-mark">{levelLabel[c.level]}</span>
              <div>
                <strong>{c.title}</strong>
                {c.detail && <p className="muted">{c.detail}</p>}
              </div>
              {c.fixHref && <a className="diag-fix" href={c.fixHref}>Открыть</a>}
            </div>
          ))}
        </div>
      </section>

      <section className="panel">
        <h3>Проверить страницу товара</h3>
        <p className="muted">
          Вставьте адрес страницы товара — проверим доступность сайта и что виджет сможет
          подобрать отзывы. Если все проверки зелёные, а виджета нет — убедитесь, что контейнер
          в Тег Менеджере опубликован (вставленный через Тег Менеджер сниппет сервер проверить
          не может).
        </p>
        <div className="toolbar">
          <input
            className="search-input"
            value={productUrl}
            onChange={(e) => setProductUrl(e.target.value)}
            placeholder="https://ваш-магазин.ру/product/..."
          />
          <button onClick={runProbe} disabled={probing}>
            {probing ? 'Проверяем…' : 'Проверить'}
          </button>
        </div>
        {probe && (
          <div className="stack">
            {probe.map((c) => (
              <div className={`diag-item diag-${c.level}`} key={c.id}>
                <span className="diag-mark">{levelLabel[c.level]}</span>
                <div>
                  <strong>{c.title}</strong>
                  {c.detail && <p className="muted">{c.detail}</p>}
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      <section className="panel">
        <h3>Журнал</h3>
        <div className="rows">
          {data.activity.length === 0 && <p className="muted">Событий пока нет.</p>}
          {data.activity.map((a, i) => (
            <div className={`activity-row activity-${a.level}`} key={i}>
              <span className="muted">{new Date(a.at).toLocaleString()}</span>
              <span>{a.message}</span>
            </div>
          ))}
        </div>
      </section>
    </section>
  )
}
