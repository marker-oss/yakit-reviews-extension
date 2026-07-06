import { useEffect, useMemo } from 'react'

// Returns true when `current` differs from the last-saved `baseline`, and
// installs a beforeunload guard while dirty so a hard navigation warns.
export function useDirty(current: unknown, baseline: unknown): boolean {
  const dirty = useMemo(
    () => JSON.stringify(current) !== JSON.stringify(baseline),
    [current, baseline],
  )
  useEffect(() => {
    if (!dirty) return
    const handler = (e: BeforeUnloadEvent) => {
      e.preventDefault()
      e.returnValue = ''
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [dirty])
  return dirty
}
