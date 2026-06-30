import { useEffect, useState } from 'react'
import { apiGet, apiWrite } from '../api'
import type { Question } from '../types'

type ListResponse = {
  questions: Question[]
}

const PAGE_SIZE = 25

function sourceLabel(value: string) {
  if (value === 'site') return 'Сайт'
  if (value === 'wb') return 'Wildberries'
  if (value === 'ozon') return 'Ozon'
  return value
}

export default function Questions() {
  const [data, setData] = useState<ListResponse>({ questions: [] })
  const [marketplace, setMarketplace] = useState('')
  const [status, setStatus] = useState('')
  const [offset, setOffset] = useState(0)
  const [answerDrafts, setAnswerDrafts] = useState<Record<number, string>>({})
  const [error, setError] = useState('')

  function load(nextOffset = offset) {
    const query = new URLSearchParams()
    if (marketplace) query.set('marketplace', marketplace)
    if (status) query.set('status', status)
    query.set('limit', String(PAGE_SIZE))
    query.set('offset', String(nextOffset))
    apiGet<ListResponse>(`/admin/api/questions?${query.toString()}`)
      .then((next) => {
        setData(next)
        setAnswerDrafts(Object.fromEntries(next.questions.map((q) => [q.id, q.answer?.text ?? ''])))
      })
      .catch((err) => setError(err instanceof Error ? err.message : 'Запрос не выполнен'))
  }

  useEffect(() => load(offset), [marketplace, status, offset])

  function resetTo(setter: (v: string) => void) {
    return (value: string) => {
      setOffset(0)
      setter(value)
    }
  }

  async function saveAnswer(id: number) {
    setError('')
    try {
      await apiWrite('PUT', `/admin/api/questions/${id}/answer`, { text: answerDrafts[id] ?? '' })
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Запрос не выполнен')
    }
  }

  async function retryPublish(id: number) {
    setError('')
    try {
      await apiWrite('POST', `/admin/api/questions/${id}/answer/retry`)
      load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Запрос не выполнен')
    }
  }

  const total = data.questions.length
  const hasPrev = offset > 0
  const hasNext = offset + PAGE_SIZE < total

  return (
    <section className="stack">
      <div className="toolbar">
        <select value={marketplace} onChange={(e) => resetTo(setMarketplace)(e.target.value)}>
          <option value="">Все источники</option>
          <option value="site">Сайт</option>
          <option value="wb">Wildberries</option>
          <option value="ozon">Ozon</option>
        </select>
        <select value={status} onChange={(e) => resetTo(setStatus)(e.target.value)}>
          <option value="">Все статусы</option>
          <option value="imported">Импортированные</option>
          <option value="answered">Отвеченные</option>
          <option value="pending">Ожидают ответа</option>
        </select>
        <span className="muted">Вопросов: {total}</span>
      </div>
      {error && <p className="error">{error}</p>}
      <section className="panel">
        <div className="table">
          <div className="table-head grid-questions">
            <span>Вопрос</span>
            <span>Статус</span>
            <span></span>
          </div>
          {data.questions.map((q) => (
            <div className="table-row grid-questions" key={q.id}>
              <div>
                <strong>{q.authorName || sourceLabel(q.marketplace)}</strong>
                <p>{q.text}</p>
                <small>
                  {sourceLabel(q.marketplace)}
                  {q.sellerArticle ? ` · ${q.sellerArticle}` : ''}
                  {' · '}
                  {new Date(q.createdAt).toLocaleDateString('ru-RU')}
                </small>
              </div>
              <span className={q.status === 'answered' ? 'status-ok' : 'status-muted'}>
                {q.status === 'answered' ? 'отвечен' : q.status === 'pending' ? 'ожидает' : 'импортирован'}
              </span>
              <div className="reply-editor">
                <textarea
                  value={answerDrafts[q.id] ?? ''}
                  onChange={(e) => setAnswerDrafts((prev) => ({ ...prev, [q.id]: e.target.value }))}
                  placeholder="Ответ на вопрос"
                  rows={2}
                />
                <button className="secondary" onClick={() => saveAnswer(q.id)}>
                  Ответить
                </button>
                {q.answerPublish && (
                  <div className="reply-publish">
                    {q.answerPublish.state === 'published' && <span className="status-ok">Опубликовано на МП</span>}
                    {q.answerPublish.state === 'pending' && <span className="status-muted">Публикация…</span>}
                    {q.answerPublish.state === 'unsupported' && <span className="status-muted">Публикация на МП недоступна</span>}
                    {q.answerPublish.state === 'failed' && (
                      <>
                        <span className="status-warn">Ошибка публикации: {q.answerPublish.error}</span>
                        <button className="secondary" onClick={() => retryPublish(q.id)}>Повторить</button>
                      </>
                    )}
                  </div>
                )}
              </div>
            </div>
          ))}
          {data.questions.length === 0 && <p className="muted empty">Вопросов нет.</p>}
        </div>
      </section>
      {total > PAGE_SIZE && (
        <div className="toolbar pager">
          <button className="secondary" disabled={!hasPrev} onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}>
            ← Назад
          </button>
          <span className="muted">
            {offset + 1}–{Math.min(offset + PAGE_SIZE, total)} из {total}
          </span>
          <button className="secondary" disabled={!hasNext} onClick={() => setOffset(offset + PAGE_SIZE)}>
            Вперёд →
          </button>
        </div>
      )}
    </section>
  )
}
