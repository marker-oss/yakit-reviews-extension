import { useEffect, useState } from 'react'
import { apiGet, apiWrite } from '../api'
import type { Review } from '../types'

type ListResponse = {
  reviews: Review[]
  total: number
}

const PAGE_SIZE = 25

export default function Reviews() {
  const [data, setData] = useState<ListResponse>({ reviews: [], total: 0 })
  const [marketplace, setMarketplace] = useState('')
  const [visibility, setVisibility] = useState('')
  const [sort, setSort] = useState('')
  const [search, setSearch] = useState('')
  const [offset, setOffset] = useState(0)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [error, setError] = useState('')

  function load(nextOffset = offset) {
    setSelected(new Set())
    const query = new URLSearchParams()
    if (marketplace) query.set('marketplace', marketplace)
    if (visibility) query.set('visibility', visibility)
    if (sort) query.set('sort', sort)
    if (search) query.set('search', search)
    query.set('limit', String(PAGE_SIZE))
    query.set('offset', String(nextOffset))
    apiGet<ListResponse>(`/admin/api/reviews?${query.toString()}`)
      .then(setData)
      .catch((err) => setError(err instanceof Error ? err.message : 'Запрос не выполнен'))
  }

  useEffect(() => load(offset), [marketplace, visibility, sort, offset])

  // Reset to the first page whenever a filter/sort narrows the result set.
  function resetTo(setter: (v: string) => void) {
    return (value: string) => {
      setOffset(0)
      setter(value)
    }
  }

  function runSearch() {
    if (offset === 0) {
      load(0)
    } else {
      setOffset(0)
    }
  }

  const from = data.total === 0 ? 0 : offset + 1
  const to = Math.min(offset + PAGE_SIZE, data.total)
  const hasPrev = offset > 0
  const hasNext = offset + PAGE_SIZE < data.total

  async function moderate(id: number, body: { visibility?: 'visible' | 'hidden'; pinned?: boolean }) {
    setError('')
    try {
      await apiWrite('PATCH', `/admin/api/reviews/${id}`, body)
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Запрос не выполнен')
    }
  }

  async function bulkModerate(body: { visibility?: 'visible' | 'hidden'; pinned?: boolean }) {
    if (selected.size === 0) return
    setError('')
    try {
      await apiWrite('POST', '/admin/api/reviews/bulk', { ids: [...selected], ...body })
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Запрос не выполнен')
    }
  }

  function toggleOne(id: number) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const allOnPageSelected = data.reviews.length > 0 && data.reviews.every((r) => selected.has(r.id))
  function toggleAllOnPage() {
    setSelected((prev) => {
      if (data.reviews.every((r) => prev.has(r.id))) return new Set()
      return new Set(data.reviews.map((r) => r.id))
    })
  }

  return (
    <section className="stack">
      <div className="toolbar">
        <select value={marketplace} onChange={(e) => resetTo(setMarketplace)(e.target.value)}>
          <option value="">Все маркетплейсы</option>
          <option value="wb">Wildberries</option>
          <option value="ym">Yandex Market</option>
          <option value="ozon">Ozon</option>
        </select>
        <select value={visibility} onChange={(e) => resetTo(setVisibility)(e.target.value)}>
          <option value="">Любой статус</option>
          <option value="visible">Показан</option>
          <option value="hidden">Скрыт</option>
        </select>
        <select value={sort} onChange={(e) => resetTo(setSort)(e.target.value)}>
          <option value="">Сначала новые</option>
          <option value="highest">Высокий рейтинг</option>
          <option value="lowest">Низкий рейтинг</option>
          <option value="media">Сначала с фото</option>
        </select>
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && runSearch()}
          placeholder="Поиск по тексту"
        />
        <button className="secondary" onClick={runSearch}>
          Найти
        </button>
        <span className="muted">Отзывов: {data.total}</span>
      </div>
      {error && <p className="error">{error}</p>}
      {selected.size > 0 && (
        <div className="toolbar bulk-bar">
          <span className="muted">Выбрано: {selected.size}</span>
          <button className="secondary" onClick={() => bulkModerate({ visibility: 'hidden' })}>
            Скрыть
          </button>
          <button className="secondary" onClick={() => bulkModerate({ visibility: 'visible' })}>
            Показать
          </button>
          <button className="secondary" onClick={() => bulkModerate({ pinned: true })}>
            Закрепить
          </button>
          <button className="secondary" onClick={() => bulkModerate({ pinned: false })}>
            Открепить
          </button>
          <button className="secondary" onClick={() => setSelected(new Set())}>
            Снять выбор
          </button>
        </div>
      )}
      <section className="panel">
        <div className="table">
          <div className="table-head grid-reviews">
            <span>
              <input type="checkbox" checked={allOnPageSelected} onChange={toggleAllOnPage} aria-label="Выбрать все" />
            </span>
            <span>Отзыв</span>
            <span>Оценка</span>
            <span>Статус</span>
            <span></span>
          </div>
          {data.reviews.map((review) => (
            <div className="table-row grid-reviews" key={review.id}>
              <span>
                <input
                  type="checkbox"
                  checked={selected.has(review.id)}
                  onChange={() => toggleOne(review.id)}
                  aria-label={`Выбрать отзыв ${review.id}`}
                />
              </span>
              <div>
                <strong>{review.authorName || review.marketplace}</strong>
                <p>{review.text || review.pros || review.cons || review.externalReviewId}</p>
                <small>{review.marketplace} · {review.sellerArticle || review.externalProductId}</small>
              </div>
              <span>{review.rating ?? '-'} / 5</span>
              <span className={review.visibility === 'visible' ? 'status-ok' : 'status-muted'}>
                {review.pinned ? 'закреплён · ' : ''}{review.visibility === 'visible' ? 'показан' : 'скрыт'}
              </span>
              <div className="actions">
                <button
                  className="secondary"
                  onClick={() => moderate(review.id, { visibility: review.visibility === 'visible' ? 'hidden' : 'visible' })}
                >
                  {review.visibility === 'visible' ? 'Скрыть' : 'Показать'}
                </button>
                <button className="secondary" onClick={() => moderate(review.id, { pinned: !review.pinned })}>
                  {review.pinned ? 'Открепить' : 'Закрепить'}
                </button>
              </div>
            </div>
          ))}
          {data.reviews.length === 0 && <p className="muted empty">Под эти фильтры отзывов нет.</p>}
        </div>
      </section>
      {data.total > 0 && (
        <div className="toolbar pager">
          <button className="secondary" disabled={!hasPrev} onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}>
            ← Назад
          </button>
          <span className="muted">
            {from}–{to} из {data.total}
          </span>
          <button className="secondary" disabled={!hasNext} onClick={() => setOffset(offset + PAGE_SIZE)}>
            Вперёд →
          </button>
        </div>
      )}
    </section>
  )
}
