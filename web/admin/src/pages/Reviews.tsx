import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { apiGet, apiWrite } from '../api'
import type { Review } from '../types'

type ListResponse = {
  reviews: Review[]
  total: number
}

const PAGE_SIZE = 25
const SEARCH_DELAY_MS = 350

export default function Reviews() {
  const [data, setData] = useState<ListResponse>({ reviews: [], total: 0 })
  const [marketplace, setMarketplace] = useState('')
  const [visibility, setVisibility] = useState('')
  const [status, setStatus] = useState('')
  const [sort, setSort] = useState('')
  const [search, setSearch] = useState('')
  const [searchDraft, setSearchDraft] = useState('')
  const [article, setArticle] = useState('')
  const [articleDraft, setArticleDraft] = useState('')
  const [offset, setOffset] = useState(0)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [articlePins, setArticlePins] = useState<Set<number>>(new Set())
  const [replyDrafts, setReplyDrafts] = useState<Record<number, string>>({})
  const [error, setError] = useState('')

  function load(nextOffset = offset) {
    setSelected(new Set())
    const query = new URLSearchParams()
    if (marketplace) query.set('marketplace', marketplace)
    if (visibility) query.set('visibility', visibility)
    if (status) query.set('status', status)
    if (sort) query.set('sort', sort)
    if (search) query.set('search', search)
    if (article) query.set('article_search', article)
    query.set('limit', String(PAGE_SIZE))
    query.set('offset', String(nextOffset))
    apiGet<ListResponse>(`/admin/api/reviews?${query.toString()}`)
      .then((next) => {
        setData(next)
        setReplyDrafts(Object.fromEntries(next.reviews.map((review) => [review.id, review.adminReply?.text ?? ''])))
        return loadPins()
      })
      .catch((err) => setError(err instanceof Error ? err.message : 'Запрос не выполнен'))
  }

  useEffect(() => load(offset), [marketplace, visibility, status, sort, search, article, offset])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setOffset(0)
      setSearch(searchDraft.trim())
      setArticle(articleDraft.trim())
    }, SEARCH_DELAY_MS)
    return () => window.clearTimeout(timer)
  }, [searchDraft, articleDraft])

  // Reset to the first page whenever a filter/sort narrows the result set.
  function resetTo(setter: (v: string) => void) {
    return (value: string) => {
      setOffset(0)
      setter(value)
    }
  }

  function runSearch() {
    const nextSearch = searchDraft.trim()
    const nextArticle = articleDraft.trim()
    setSearch(nextSearch)
    setArticle(nextArticle)
    if (offset === 0) {
      if (nextSearch === search && nextArticle === article) {
        load(0)
      }
    } else {
      setOffset(0)
    }
  }

  function clearSearch() {
    setSearchDraft('')
    setSearch('')
    setOffset(0)
  }

  function clearArticle() {
    setArticleDraft('')
    setArticle('')
    setOffset(0)
  }

  async function loadPins() {
    const currentArticle = article.trim()
    if (!currentArticle) {
      setArticlePins(new Set())
      return
    }
    const data = await apiGet<{ reviewIds: number[] }>(`/admin/api/articles/${encodeURIComponent(currentArticle)}/pins`)
    setArticlePins(new Set(data.reviewIds))
  }

  const from = data.total === 0 ? 0 : offset + 1
  const to = Math.min(offset + PAGE_SIZE, data.total)
  const hasPrev = offset > 0
  const hasNext = offset + PAGE_SIZE < data.total
  const highlightTerms = useMemo(() => tokenizeHighlightTerms(search, article), [search, article])

  async function moderate(id: number, body: { visibility?: 'visible' | 'hidden'; pinned?: boolean }) {
    setError('')
    try {
      await apiWrite('PATCH', `/admin/api/reviews/${id}`, body)
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Запрос не выполнен')
    }
  }

  async function deleteReview(id: number) {
    setError('')
    try {
      await apiWrite('DELETE', `/admin/api/reviews/${id}`)
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Запрос не выполнен')
    }
  }

  async function restoreReview(id: number) {
    setError('')
    try {
      await apiWrite('POST', `/admin/api/reviews/${id}/restore`)
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Запрос не выполнен')
    }
  }

  async function saveReply(id: number) {
    setError('')
    try {
      await apiWrite('PUT', `/admin/api/reviews/${id}/reply`, { text: replyDrafts[id] ?? '' })
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Запрос не выполнен')
    }
  }

  async function toggleArticlePin(id: number) {
    const currentArticle = article.trim()
    if (!currentArticle) return
    const next = articlePins.has(id)
      ? [...articlePins].filter((reviewID) => reviewID !== id)
      : [...articlePins, id]
    setError('')
    try {
      await apiWrite('PUT', `/admin/api/articles/${encodeURIComponent(currentArticle)}/pins`, { reviewIds: next })
      setArticlePins(new Set(next))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Запрос не выполнен')
    }
  }

  async function bulkModerate(body: { visibility?: 'visible' | 'hidden'; pinned?: boolean; status?: 'deleted' }) {
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
        <select value={status} onChange={(e) => resetTo(setStatus)(e.target.value)}>
          <option value="">Активные</option>
          <option value="deleted">Удалённые</option>
          <option value="all">Все</option>
        </select>
        <select value={sort} onChange={(e) => resetTo(setSort)(e.target.value)}>
          <option value="">Сначала новые</option>
          <option value="highest">Высокий рейтинг</option>
          <option value="lowest">Низкий рейтинг</option>
          <option value="media">Сначала с фото</option>
        </select>
        <input
          className="search-input"
          value={searchDraft}
          onChange={(e) => setSearchDraft(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && runSearch()}
          placeholder="Поиск по тексту"
        />
        {searchDraft && (
          <button className="secondary clear-button" onClick={clearSearch} aria-label="Очистить поиск по тексту">
            ×
          </button>
        )}
        <input
          className="search-input"
          value={articleDraft}
          onChange={(e) => setArticleDraft(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && runSearch()}
          placeholder="Артикул или часть"
        />
        {articleDraft && (
          <button className="secondary clear-button" onClick={clearArticle} aria-label="Очистить поиск по артикулу">
            ×
          </button>
        )}
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
          <button className="secondary danger" onClick={() => bulkModerate({ status: 'deleted' })}>
            Удалить
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
                <p>{highlight(review.text || review.pros || review.cons || review.externalReviewId, highlightTerms)}</p>
                <small>
                  {review.marketplace} · {highlight(review.sellerArticle || review.externalProductId, highlightTerms)}
                </small>
              </div>
              <span>{review.rating ?? '-'} / 5</span>
              <span className={review.visibility === 'visible' ? 'status-ok' : 'status-muted'}>
                {review.status === 'deleted'
                  ? 'удалён'
                  : `${review.pinned ? 'закреплён · ' : ''}${review.visibility === 'visible' ? 'показан' : 'скрыт'}`}
              </span>
              <div className="actions">
                {review.status === 'deleted' ? (
                  <button className="secondary" onClick={() => restoreReview(review.id)}>
                    Восстановить
                  </button>
                ) : (
                  <>
                    <button
                      className="secondary"
                      onClick={() => moderate(review.id, { visibility: review.visibility === 'visible' ? 'hidden' : 'visible' })}
                    >
                      {review.visibility === 'visible' ? 'Скрыть' : 'Показать'}
                    </button>
                    <button className="secondary" onClick={() => moderate(review.id, { pinned: !review.pinned })}>
                      {review.pinned ? 'Открепить' : 'Закрепить'}
                    </button>
                    {article.trim() && (
                      <button className="secondary" onClick={() => toggleArticlePin(review.id)}>
                        {articlePins.has(review.id) ? 'Снять с товара' : 'На страницу товара'}
                      </button>
                    )}
                    <button className="secondary danger" onClick={() => deleteReview(review.id)}>
                      Удалить
                    </button>
                  </>
                )}
              </div>
              <div className="reply-editor">
                <textarea
                  value={replyDrafts[review.id] ?? ''}
                  onChange={(e) => setReplyDrafts((prev) => ({ ...prev, [review.id]: e.target.value }))}
                  placeholder="Ответ SHEGIDA"
                  rows={2}
                />
                <button className="secondary" onClick={() => saveReply(review.id)}>
                  Сохранить ответ
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

function highlight(value: string, terms: string[]): ReactNode {
  if (!value || terms.length === 0) return value
  const needles = terms
    .map((term) => term.trim())
    .filter(Boolean)
    .sort((a, b) => b.length - a.length)
  if (needles.length === 0) return value

  const pattern = new RegExp(`(${needles.map(escapeRegExp).join('|')})`, 'gi')
  return value.split(pattern).map((part, index) => {
    if (!part) return null
    if (needles.some((term) => part.toLowerCase() === term.toLowerCase())) {
      return (
        <mark className="search-hit" key={`${part}-${index}`}>
          {part}
        </mark>
      )
    }
    return part
  })
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function tokenizeHighlightTerms(...values: string[]) {
  const terms = new Set<string>()
  for (const value of values) {
    const trimmed = value.trim()
    if (!trimmed) continue
    terms.add(trimmed)
    for (const part of trimmed.split(/[\s/_-]+/)) {
      if (part.length >= 2) terms.add(part)
    }
  }
  return [...terms]
}
