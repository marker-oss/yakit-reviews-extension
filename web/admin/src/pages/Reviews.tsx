import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { apiGet, apiWrite } from '../api'
import type { Review } from '../types'

type ListResponse = {
  reviews: Review[]
  total: number
}

type PublishResponse = {
  generatedAt: string
  articles: number
  reviews: number
}

type ReviewMedia = Review['media'][number]

type MediaViewerState = {
  items: ReviewMedia[]
  index: number
  title: string
}

type ReviewDraft = {
  sellerArticle: string
  rating: string
  authorName: string
  text: string
  pros: string
  cons: string
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
  const [editDrafts, setEditDrafts] = useState<Record<number, ReviewDraft>>({})
  const [error, setError] = useState('')
  const [publishMessage, setPublishMessage] = useState('')
  const [publishing, setPublishing] = useState(false)
  const [mediaViewer, setMediaViewer] = useState<MediaViewerState | null>(null)

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
        setEditDrafts(Object.fromEntries(next.reviews.map((review) => [review.id, draftFromReview(review)])))
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

  useEffect(() => {
    if (!mediaViewer) return
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') setMediaViewer(null)
      if (event.key === 'ArrowLeft') shiftMedia(-1)
      if (event.key === 'ArrowRight') shiftMedia(1)
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [mediaViewer])

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

  async function moderate(
    id: number,
    body: {
      visibility?: 'visible' | 'hidden'
      pinned?: boolean
      status?: 'pending' | 'approved' | 'rejected' | 'deleted'
      sellerArticle?: string
      rating?: number
      authorName?: string
      text?: string
      pros?: string
      cons?: string
    },
  ) {
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

  async function purgeReview(id: number) {
    setError('')
    try {
      await apiWrite('DELETE', `/admin/api/reviews/${id}/purge`)
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Запрос не выполнен')
    }
  }

  async function saveReviewEdits(id: number, status?: 'pending' | 'approved' | 'rejected') {
    const draft = editDrafts[id]
    if (!draft) return
    const rating = Number(draft.rating)
    await moderate(id, {
      sellerArticle: draft.sellerArticle,
      rating: Number.isFinite(rating) ? rating : undefined,
      authorName: draft.authorName,
      text: draft.text,
      pros: draft.pros,
      cons: draft.cons,
      status,
    })
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

  async function retryPublish(id: number) {
    setError('')
    try {
      await apiWrite('POST', `/admin/api/reviews/${id}/reply/retry`)
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

  async function bulkModerate(body: { visibility?: 'visible' | 'hidden'; pinned?: boolean; status?: 'approved' | 'rejected' | 'pending' | 'deleted' }) {
    if (selected.size === 0) return
    setError('')
    try {
      await apiWrite('POST', '/admin/api/reviews/bulk', { ids: [...selected], ...body })
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Запрос не выполнен')
    }
  }

  async function publishChanges() {
    setError('')
    setPublishMessage('')
    setPublishing(true)
    try {
      const result = await apiWrite<PublishResponse>('POST', '/admin/api/reviews/publish')
      setPublishMessage(
        `Опубликовано: ${result.reviews} отзывов, ${result.articles} артикулов · ${new Date(result.generatedAt).toLocaleString()}`,
      )
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Публикация не выполнена')
    } finally {
      setPublishing(false)
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

  function openMedia(review: Review, index: number) {
    if (!review.media.length) return
    setMediaViewer({
      items: review.media,
      index,
      title: `${review.authorName || review.marketplace} · ${review.marketplace}`,
    })
  }

  function shiftMedia(direction: number) {
    setMediaViewer((prev) => {
      if (!prev || prev.items.length === 0) return prev
      return { ...prev, index: (prev.index + direction + prev.items.length) % prev.items.length }
    })
  }

  function updateDraft(id: number, key: keyof ReviewDraft, value: string) {
    setEditDrafts((prev) => ({
      ...prev,
      [id]: { ...(prev[id] ?? emptyDraft()), [key]: value },
    }))
  }

  const activeMedia = mediaViewer?.items[mediaViewer.index] ?? null

  return (
    <section className="stack">
      <div className="toolbar">
        <select value={marketplace} onChange={(e) => resetTo(setMarketplace)(e.target.value)}>
          <option value="">Все источники</option>
          <option value="site">Сайт</option>
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
          <option value="pending">На модерации</option>
          <option value="approved">Одобренные</option>
          <option value="rejected">Отклонённые</option>
          <option value="imported">Импортированные</option>
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
        <button className="secondary publish-button" disabled={publishing} onClick={publishChanges}>
          {publishing ? 'Публикуем...' : 'Опубликовать изменения'}
        </button>
        <span className="muted">Отзывов: {data.total}</span>
      </div>
      {error && <p className="error">{error}</p>}
      {publishMessage && <p className="status-ok publish-status">{publishMessage}</p>}
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
          <button className="secondary" onClick={() => bulkModerate({ status: 'approved' })}>
            Одобрить
          </button>
          <button className="secondary" onClick={() => bulkModerate({ status: 'rejected' })}>
            Отклонить
          </button>
          <button className="secondary" onClick={() => setSelected(new Set())}>
            Снять выбор
          </button>
          <p className="muted hide-warning">
            Скрытие — только для спама, дублей и мусора. Не скрывайте негативные отзывы ради
            рейтинга: это риск по ЗоЗПП и закону «О рекламе».
          </p>
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
                  {sourceLabel(review.marketplace)} · {highlight(review.sellerArticle || review.externalProductId, highlightTerms)}
                  {review.authorEmail ? ` · ${review.authorEmail}` : ''}
                </small>
                {review.media.length > 0 && (
                  <div className="review-media-strip" aria-label="Медиа отзыва">
                    {review.media.slice(0, 6).map((item, index) => (
                      <button
                        className={`review-media-thumb ${item.kind === 'video' ? 'is-video' : ''}`}
                        type="button"
                        key={`${item.url}-${index}`}
                        onClick={() => openMedia(review, index)}
                        aria-label={item.kind === 'video' ? 'Открыть видео отзыва' : 'Открыть фото отзыва'}
                      >
                        {mediaThumbnail(item) ? <img src={mediaThumbnail(item)} alt="" loading="lazy" /> : <span>Видео</span>}
                      </button>
                    ))}
                    {review.media.length > 6 && <span className="review-media-more">+{review.media.length - 6}</span>}
                  </div>
                )}
              </div>
              <span>{review.rating ?? '-'} / 5</span>
              <span className={review.status === 'pending' ? 'status-warn' : review.visibility === 'visible' ? 'status-ok' : 'status-muted'}>
                {statusLabel(review)}
              </span>
              <div className="actions">
                {review.status === 'deleted' ? (
                  <>
                    <button className="secondary" onClick={() => restoreReview(review.id)}>
                      Восстановить
                    </button>
                    <button className="secondary danger" onClick={() => purgeReview(review.id)}>
                      Удалить навсегда
                    </button>
                  </>
                ) : (
                  <>
                    {review.marketplace === 'site' && (
                      <>
                        <button className="secondary" onClick={() => saveReviewEdits(review.id, 'approved')}>
                          Одобрить
                        </button>
                        <button className="secondary" onClick={() => saveReviewEdits(review.id, 'rejected')}>
                          Отклонить
                        </button>
                      </>
                    )}
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
                    {review.marketplace === 'site' && (
                      <button className="secondary danger" onClick={() => purgeReview(review.id)}>
                        Навсегда
                      </button>
                    )}
                  </>
                )}
              </div>
              {review.marketplace === 'site' && review.status !== 'deleted' && editDrafts[review.id] && (
                <div className="submission-editor">
                  <label>
                    <span>Артикул</span>
                    <input value={editDrafts[review.id].sellerArticle} onChange={(e) => updateDraft(review.id, 'sellerArticle', e.target.value)} />
                  </label>
                  <label>
                    <span>Оценка</span>
                    <input type="number" min={1} max={5} value={editDrafts[review.id].rating} onChange={(e) => updateDraft(review.id, 'rating', e.target.value)} />
                  </label>
                  <label>
                    <span>Имя</span>
                    <input value={editDrafts[review.id].authorName} onChange={(e) => updateDraft(review.id, 'authorName', e.target.value)} />
                  </label>
                  <label>
                    <span>Плюсы</span>
                    <input value={editDrafts[review.id].pros} onChange={(e) => updateDraft(review.id, 'pros', e.target.value)} />
                  </label>
                  <label>
                    <span>Минусы</span>
                    <input value={editDrafts[review.id].cons} onChange={(e) => updateDraft(review.id, 'cons', e.target.value)} />
                  </label>
                  <label className="wide">
                    <span>Текст</span>
                    <textarea value={editDrafts[review.id].text} rows={3} onChange={(e) => updateDraft(review.id, 'text', e.target.value)} />
                  </label>
                  <button className="secondary" onClick={() => saveReviewEdits(review.id)}>
                    Сохранить правки
                  </button>
                </div>
              )}
              <div className="reply-editor">
                <textarea
                  value={replyDrafts[review.id] ?? ''}
                  onChange={(e) => setReplyDrafts((prev) => ({ ...prev, [review.id]: e.target.value }))}
                  placeholder="Ответ магазина"
                  rows={2}
                />
                <button className="secondary" onClick={() => saveReply(review.id)}>
                  Сохранить ответ
                </button>
                {review.replyPublish && (
                  <div className="reply-publish">
                    {review.replyPublish.state === 'published' && <span className="status-ok">Опубликовано на МП</span>}
                    {review.replyPublish.state === 'pending' && <span className="status-muted">Публикация…</span>}
                    {review.replyPublish.state === 'unsupported' && <span className="status-muted">Публикация на МП недоступна</span>}
                    {review.replyPublish.state === 'failed' && (
                      <>
                        <span className="status-warn">Ошибка публикации: {review.replyPublish.error}</span>
                        <button className="secondary" onClick={() => retryPublish(review.id)}>Повторить</button>
                      </>
                    )}
                  </div>
                )}
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
      {mediaViewer && activeMedia && (
        <div className="admin-media-viewer" role="presentation">
          <div className="admin-media-backdrop" onClick={() => setMediaViewer(null)} />
          <div className="admin-media-dialog" role="dialog" aria-modal="true" aria-label="Просмотр медиа отзыва">
            <div className="admin-media-top">
              <strong>{mediaViewer.title}</strong>
              <a href={activeMedia.url} target="_blank" rel="noreferrer">
                {activeMedia.kind === 'video' ? 'Открыть видео' : 'Открыть оригинал'}
              </a>
              <button className="secondary clear-button" onClick={() => setMediaViewer(null)} aria-label="Закрыть просмотр">
                ×
              </button>
            </div>
            {mediaViewer.items.length > 1 && (
              <button className="admin-media-nav admin-media-prev" onClick={() => shiftMedia(-1)} aria-label="Предыдущее медиа">
                ‹
              </button>
            )}
            <div className="admin-media-stage">
              {activeMedia.kind === 'video' && isPlayableVideo(activeMedia.url) ? (
                <video src={activeMedia.url} controls playsInline />
              ) : mediaPreview(activeMedia) ? (
                <img src={mediaPreview(activeMedia)} alt={activeMedia.kind === 'video' ? 'Видео отзыва' : 'Фото отзыва'} />
              ) : (
                <a className="admin-media-placeholder" href={activeMedia.url} target="_blank" rel="noreferrer">
                  Открыть медиа
                </a>
              )}
            </div>
            {mediaViewer.items.length > 1 && (
              <button className="admin-media-nav admin-media-next" onClick={() => shiftMedia(1)} aria-label="Следующее медиа">
                ›
              </button>
            )}
            <div className="admin-media-count">
              {mediaViewer.index + 1} / {mediaViewer.items.length}
            </div>
          </div>
        </div>
      )}
    </section>
  )
}

function mediaThumbnail(item: ReviewMedia) {
  if (item.kind === 'video') return item.previewUrl || ''
  return item.previewUrl || item.url
}

function mediaPreview(item: ReviewMedia) {
  if (item.kind === 'video' && !isImageLike(item.url)) return item.previewUrl || ''
  return item.kind === 'video' ? item.previewUrl || item.url : item.url || item.previewUrl || ''
}

function isPlayableVideo(url: string) {
  return /\.(mp4|webm|ogg|ogv|m3u8)(\?|#|$)/i.test(url)
}

function isImageLike(url: string) {
  return /\.(avif|gif|jpe?g|png|svg|webp)(\?|#|$)/i.test(url)
}

function draftFromReview(review: Review): ReviewDraft {
  return {
    sellerArticle: review.sellerArticle || review.externalProductId || '',
    rating: review.rating == null ? '' : String(review.rating),
    authorName: review.authorName || '',
    text: review.text || '',
    pros: review.pros || '',
    cons: review.cons || '',
  }
}

function emptyDraft(): ReviewDraft {
  return { sellerArticle: '', rating: '', authorName: '', text: '', pros: '', cons: '' }
}

function sourceLabel(value: string) {
  if (value === 'site') return 'Сайт'
  if (value === 'wb') return 'Wildberries'
  if (value === 'ym') return 'Yandex Market'
  if (value === 'ozon') return 'Ozon'
  return value
}

function statusLabel(review: Review) {
  const pin = review.pinned ? 'закреплён · ' : ''
  if (review.status === 'pending') return `${pin}на модерации`
  if (review.status === 'approved') return `${pin}${review.visibility === 'visible' ? 'одобрен · показан' : 'одобрен · скрыт'}`
  if (review.status === 'rejected') return `${pin}отклонён`
  if (review.status === 'deleted') return 'удалён'
  return `${pin}${review.visibility === 'visible' ? 'показан' : 'скрыт'}`
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
