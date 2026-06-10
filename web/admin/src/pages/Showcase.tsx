import { useEffect, useState } from 'react'
import { apiGet, apiWrite } from '../api'
import type { ShowcaseRule } from '../types'

export default function Showcase() {
  const [rule, setRule] = useState<ShowcaseRule | null>(null)
  const [message, setMessage] = useState('')

  useEffect(() => {
    apiGet<ShowcaseRule>('/admin/api/showcase-rule')
      .then(setRule)
      .catch((err) => setMessage(String(err)))
  }, [])

  if (!rule) return <p className={message ? 'error' : 'muted'}>{message || 'Loading...'}</p>

  function set<K extends keyof ShowcaseRule>(key: K, value: ShowcaseRule[K]) {
    setRule({ ...rule!, [key]: value })
  }

  async function save() {
    setMessage('')
    try {
      await apiWrite('PUT', '/admin/api/showcase-rule', rule)
      setMessage('Saved')
    } catch (err) {
      setMessage(err instanceof Error ? err.message : 'request failed')
    }
  }

  return (
    <section className="stack">
      {message && <p className="muted">{message}</p>}
      <section className="panel form-grid">
        <label>
          <span>Minimum rating</span>
          <input type="number" min={1} max={5} value={rule.MinRating} onChange={(e) => set('MinRating', Number(e.target.value))} />
        </label>
        <label>
          <span>Minimum text length</span>
          <input type="number" min={0} value={rule.MinTextLen} onChange={(e) => set('MinTextLen', Number(e.target.value))} />
        </label>
        <label>
          <span>Maximum age, days</span>
          <input type="number" min={0} value={rule.MaxAgeDays} onChange={(e) => set('MaxAgeDays', Number(e.target.value))} />
        </label>
        <label>
          <span>Limit</span>
          <input type="number" min={1} max={100} value={rule.Limit} onChange={(e) => set('Limit', Number(e.target.value))} />
        </label>
        <label>
          <span>Sort by</span>
          <select value={rule.SortBy} onChange={(e) => set('SortBy', e.target.value as ShowcaseRule['SortBy'])}>
            <option value="recent">Recent</option>
            <option value="rating">Rating</option>
          </select>
        </label>
        <label className="checkbox">
          <input type="checkbox" checked={rule.RequirePhoto} onChange={(e) => set('RequirePhoto', e.target.checked)} />
          <span>Require photo</span>
        </label>
      </section>
      <div className="toolbar">
        <button onClick={save}>Save rule</button>
      </div>
    </section>
  )
}
