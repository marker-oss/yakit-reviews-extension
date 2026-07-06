import { useEffect, useState } from 'react'
import { apiGet, apiWrite } from '../api'
import { toast } from '../toast'
import { useDirty } from '../useDirty'
import type { ShowcaseRule } from '../types'

export default function Showcase() {
  const [rule, setRule] = useState<ShowcaseRule | null>(null)
  const [baseline, setBaseline] = useState<ShowcaseRule | null>(null)
  const [loadError, setLoadError] = useState('')
  const dirty = useDirty(rule, baseline)

  useEffect(() => {
    apiGet<ShowcaseRule>('/admin/api/showcase-rule')
      .then((data) => {
        setRule(data)
        setBaseline(data)
      })
      .catch((err) => {
        const msg = err instanceof Error ? err.message : 'Запрос не выполнен'
        toast.error(msg)
        setLoadError(msg)
      })
  }, [])

  if (loadError && !rule) return <p className="error">{loadError}</p>
  if (!rule) return <p className="muted">Загрузка...</p>

  function set<K extends keyof ShowcaseRule>(key: K, value: ShowcaseRule[K]) {
    setRule({ ...rule!, [key]: value })
  }

  async function save() {
    try {
      await apiWrite('PUT', '/admin/api/showcase-rule', rule)
      setBaseline(rule)
      toast.success('Сохранено')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Запрос не выполнен')
    }
  }

  return (
    <section className="stack">
      <p className="muted">
        Витрина — это <strong>отбор отзывов для главной страницы</strong> (какие отзывы попадают в
        подборку), а не оформление виджета. Внешний вид настраивается на вкладке «Виджет».
      </p>
      <section className="panel form-grid">
        <label>
          <span>Минимальная оценка</span>
          <input type="number" min={1} max={5} value={rule.MinRating} onChange={(e) => set('MinRating', Number(e.target.value))} />
        </label>
        <label>
          <span>Минимальная длина текста</span>
          <input type="number" min={0} value={rule.MinTextLen} onChange={(e) => set('MinTextLen', Number(e.target.value))} />
        </label>
        <label>
          <span>Максимальный возраст, дней</span>
          <input type="number" min={0} value={rule.MaxAgeDays} onChange={(e) => set('MaxAgeDays', Number(e.target.value))} />
        </label>
        <label>
          <span>Лимит</span>
          <input type="number" min={1} max={100} value={rule.Limit} onChange={(e) => set('Limit', Number(e.target.value))} />
        </label>
        <label>
          <span>Сортировка</span>
          <select value={rule.SortBy} onChange={(e) => set('SortBy', e.target.value as ShowcaseRule['SortBy'])}>
            <option value="recent">Сначала новые</option>
            <option value="rating">Сначала высокий рейтинг</option>
          </select>
        </label>
        <label className="checkbox">
          <input type="checkbox" checked={rule.RequirePhoto} onChange={(e) => set('RequirePhoto', e.target.checked)} />
          <span>Только с фото</span>
        </label>
      </section>
      <div className="toolbar">
        <button onClick={save}>Сохранить правило</button>
        {dirty && <span className="dirty-badge">Есть изменения</span>}
      </div>
    </section>
  )
}
