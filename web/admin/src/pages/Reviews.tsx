import { useEffect, useState } from 'react'
import { apiGet, apiWrite } from '../api'
import type { Review } from '../types'

type ListResponse = {
  reviews: Review[]
  total: number
}

export default function Reviews() {
  const [data, setData] = useState<ListResponse>({ reviews: [], total: 0 })
  const [marketplace, setMarketplace] = useState('')
  const [visibility, setVisibility] = useState('')
  const [search, setSearch] = useState('')
  const [error, setError] = useState('')

  function load() {
    const query = new URLSearchParams()
    if (marketplace) query.set('marketplace', marketplace)
    if (visibility) query.set('visibility', visibility)
    if (search) query.set('search', search)
    apiGet<ListResponse>(`/admin/api/reviews?${query.toString()}`)
      .then(setData)
      .catch((err) => setError(err instanceof Error ? err.message : 'Запрос не выполнен'))
  }

  useEffect(load, [marketplace, visibility])

  async function moderate(id: number, body: { visibility?: 'visible' | 'hidden'; pinned?: boolean }) {
    setError('')
    try {
      await apiWrite('PATCH', `/admin/api/reviews/${id}`, body)
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Запрос не выполнен')
    }
  }

  return (
    <section className="stack">
      <div className="toolbar">
        <select value={marketplace} onChange={(e) => setMarketplace(e.target.value)}>
          <option value="">Все маркетплейсы</option>
          <option value="wb">Wildberries</option>
          <option value="ym">Yandex Market</option>
          <option value="ozon">Ozon</option>
        </select>
        <select value={visibility} onChange={(e) => setVisibility(e.target.value)}>
          <option value="">Любой статус</option>
          <option value="visible">Показан</option>
          <option value="hidden">Скрыт</option>
        </select>
        <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Поиск по тексту" />
        <button className="secondary" onClick={load}>
          Найти
        </button>
        <span className="muted">Отзывов: {data.total}</span>
      </div>
      {error && <p className="error">{error}</p>}
      <section className="panel">
        <div className="table">
          <div className="table-head grid-reviews">
            <span>Отзыв</span>
            <span>Оценка</span>
            <span>Статус</span>
            <span></span>
          </div>
          {data.reviews.map((review) => (
            <div className="table-row grid-reviews" key={review.id}>
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
    </section>
  )
}
