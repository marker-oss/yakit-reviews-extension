import { useEffect, useState } from 'react'
import { apiGet } from '../api'
import { toast } from '../toast'

type TenantInfo = {
  id: number
  slug: string
  publicKey: string
  plan: string
  status: string
  paidUntil: string | null
  trialEndsAt: string
}

const PLAN_LABELS: Record<string, string> = {
  trial: 'Триал',
  free: 'Бесплатный',
  base: 'Базовый',
  pro: 'Pro',
  'pro+': 'Pro+',
}

const PRICES: { plan: string; label: string; price1: string; price12: string; features: string[] }[] = [
  {
    plan: 'base',
    label: 'Базовый',
    price1: '300 ₽/мес',
    price12: '3 000 ₽/год',
    features: ['1 маркетплейс', 'Виджет на сайте', 'Синхронизация отзывов', 'Модерация и витрина'],
  },
  {
    plan: 'pro',
    label: 'Pro',
    price1: '590 ₽/мес',
    price12: '5 900 ₽/год',
    features: ['3 маркетплейса', 'Ответы на отзывы', 'Вопросы-ответы', 'Приоритетная поддержка'],
  },
  {
    plan: 'pro+',
    label: 'Pro+',
    price1: '990 ₽/мес',
    price12: '9 900 ₽/год',
    features: ['Все маркетплейсы', 'Всё из Pro', 'ИИ-сводки и аналитика', 'Персональный менеджер'],
  },
]

function fmtDate(v: string | null): string {
  if (!v) return '—'
  const d = new Date(v)
  return d.toLocaleDateString('ru-RU', { day: 'numeric', month: 'long', year: 'numeric' })
}

export default function Billing() {
  const [tenant, setTenant] = useState<TenantInfo | null>(null)
  const [loadError, setLoadError] = useState('')
  const [paying, setPaying] = useState(false)

  useEffect(() => {
    apiGet<TenantInfo>('/admin/api/tenant')
      .then(setTenant)
      .catch((e) => {
        const msg = e instanceof Error ? e.message : 'Не удалось загрузить данные'
        toast.error(msg)
        setLoadError(msg)
      })
  }, [])

  async function pay(plan: string, months: 1 | 12) {
    setPaying(true)
    try {
      const res = await fetch(`/billing/pay?plan=${encodeURIComponent(plan)}&months=${months}`)
      if (!res.ok) {
        const data = (await res.json().catch(() => ({}))) as { error?: string }
        throw new Error(data.error ?? 'Не удалось создать счёт')
      }
      const { url } = (await res.json()) as { url: string }
      window.location.href = url
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Не удалось создать счёт')
      setPaying(false)
    }
  }

  if (loadError && !tenant) return <p className="error">{loadError}</p>
  if (!tenant) return <p className="muted">Загрузка...</p>

  const active = tenant.status === 'active' || tenant.status === 'trial'

  return (
    <section className="stack">
      <section className="panel">
        <div className="billing-status">
          <div>
            <h3>Подписка</h3>
            <p className="muted">
              Тариф: <strong>{PLAN_LABELS[tenant.plan] ?? tenant.plan}</strong> · Статус:{' '}
              <strong>
                {tenant.status === 'active'
                  ? 'активна'
                  : tenant.status === 'trial'
                    ? 'триал'
                    : tenant.status === 'grace'
                      ? 'истекает'
                      : 'приостановлена'}
              </strong>
            </p>
            {tenant.status === 'trial' && (
              <p className="muted">Триал до {fmtDate(tenant.trialEndsAt)}</p>
            )}
            {tenant.paidUntil && <p className="muted">Оплачено до {fmtDate(tenant.paidUntil)}</p>}
            {!active && (
              <p className="error">Виджет на сайте скрыт: возобновите подписку, чтобы вернуть отзывы.</p>
            )}
          </div>
        </div>
      </section>

      <section className="panel">
        <h3>Оплата</h3>
        <p className="muted">
          Оплата через Робокассу: карты, СБП, ЮMoney. После оплаты подписка активируется автоматически
          в течение минуты.
        </p>
        <div className="billing-plans">
          {PRICES.map((p) => (
            <div className={`billing-plan${tenant.plan === p.plan ? ' current' : ''}`} key={p.plan}>
              <h4>{p.label}</h4>
              {tenant.plan === p.plan && <span className="plan-current">текущий</span>}
              <p className="plan-price">{p.price1}</p>
              <p className="muted">{p.price12}</p>
              <ul>
                {p.features.map((f) => (
                  <li key={f}>{f}</li>
                ))}
              </ul>
              <button disabled={paying} onClick={() => pay(p.plan, 1)}>
                Оплатить месяц
              </button>
              <button className="secondary" disabled={paying} onClick={() => pay(p.plan, 12)}>
                Оплатить год
              </button>
            </div>
          ))}
        </div>
      </section>
    </section>
  )
}
