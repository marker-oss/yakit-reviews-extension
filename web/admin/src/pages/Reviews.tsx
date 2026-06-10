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
      .catch((err) => setError(String(err)))
  }

  useEffect(load, [marketplace, visibility])

  async function moderate(id: number, body: { visibility?: 'visible' | 'hidden'; pinned?: boolean }) {
    setError('')
    try {
      await apiWrite('PATCH', `/admin/api/reviews/${id}`, body)
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'request failed')
    }
  }

  return (
    <section className="stack">
      <div className="toolbar">
        <select value={marketplace} onChange={(e) => setMarketplace(e.target.value)}>
          <option value="">All marketplaces</option>
          <option value="wb">Wildberries</option>
          <option value="ym">Yandex Market</option>
          <option value="ozon">Ozon</option>
        </select>
        <select value={visibility} onChange={(e) => setVisibility(e.target.value)}>
          <option value="">All visibility</option>
          <option value="visible">Visible</option>
          <option value="hidden">Hidden</option>
        </select>
        <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search text" />
        <button className="secondary" onClick={load}>
          Search
        </button>
        <span className="muted">{data.total} reviews</span>
      </div>
      {error && <p className="error">{error}</p>}
      <section className="panel">
        <div className="table">
          <div className="table-head grid-reviews">
            <span>Review</span>
            <span>Rating</span>
            <span>Status</span>
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
                {review.pinned ? 'pinned · ' : ''}{review.visibility}
              </span>
              <div className="actions">
                <button
                  className="secondary"
                  onClick={() => moderate(review.id, { visibility: review.visibility === 'visible' ? 'hidden' : 'visible' })}
                >
                  {review.visibility === 'visible' ? 'Hide' : 'Show'}
                </button>
                <button className="secondary" onClick={() => moderate(review.id, { pinned: !review.pinned })}>
                  {review.pinned ? 'Unpin' : 'Pin'}
                </button>
              </div>
            </div>
          ))}
          {data.reviews.length === 0 && <p className="muted empty">No reviews match these filters.</p>}
        </div>
      </section>
    </section>
  )
}
