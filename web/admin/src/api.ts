let csrfToken = ''

async function readError(res: Response) {
  try {
    const data = (await res.json()) as { error?: string }
    return data.error ?? 'request failed'
  } catch {
    return 'request failed'
  }
}

async function ensureCSRF() {
  if (csrfToken) return csrfToken
  const res = await fetch('/admin/api/csrf')
  if (!res.ok) throw new Error(await readError(res))
  const data = (await res.json()) as { csrf_token: string }
  csrfToken = data.csrf_token
  return csrfToken
}

export function clearCSRF() {
  csrfToken = ''
}

export async function apiGet<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) throw new Error(await readError(res))
  return res.json() as Promise<T>
}

export async function apiWrite<T>(method: string, path: string, body?: unknown): Promise<T> {
  const token = await ensureCSRF()
  const res = await fetch(path, {
    method,
    headers: {
      'Content-Type': 'application/json',
      'X-CSRF-Token': token,
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) throw new Error(await readError(res))
  return res.json() as Promise<T>
}
