// The person's pipeline and totals, shared by every page.
import React, { createContext, useCallback, useContext, useEffect, useState } from 'react'
import { api, V1 } from './api'
import type { AppState, Stage, Summary } from './types'

const EVENT = 'pcr:changed'

interface DataState {
  stages: Stage[] | null
  summary: Summary | null
  apps: AppState[] | null
  reload: () => Promise<void>
  reloadApps: () => Promise<void>
}
const Ctx = createContext<DataState>({ stages: null, summary: null, apps: null, reload: async () => {}, reloadApps: async () => {} })
export function useData() { return useContext(Ctx) }

/** Something changed (a deal moved, a stage renamed): lists and counts reload. */
export function announceChange() { window.dispatchEvent(new CustomEvent(EVENT)) }

export function useOnChange(fn: () => void) {
  useEffect(() => {
    window.addEventListener(EVENT, fn)
    return () => window.removeEventListener(EVENT, fn)
  }, [fn])
}

export function DataProvider({ children }: { children: React.ReactNode }) {
  const [stages, setStages] = useState<Stage[] | null>(null)
  const [summary, setSummary] = useState<Summary | null>(null)
  const [apps, setApps] = useState<AppState[] | null>(null)
  const reload = useCallback(async () => {
    const [st, sum] = await Promise.all([api.get<Stage[]>(`${V1}/stages`).catch(() => null), api.get<Summary>(`${V1}/summary`).catch(() => null)])
    setStages(s => st || s || [])
    if (sum) setSummary(sum)
  }, [])
  const reloadApps = useCallback(async () => {
    try { setApps((await api.get<{ apps: AppState[] }>(`${V1}/apps`)).apps) } catch { setApps(a => a || []) }
  }, [])
  useEffect(() => { reload(); reloadApps() }, [reload, reloadApps])
  useOnChange(reload)
  // other people's apps change state (a link switched on): look again now and then, quietly
  useEffect(() => {
    const t = window.setInterval(() => { if (document.visibilityState === 'visible') { reload(); reloadApps() } }, 120000)
    return () => window.clearInterval(t)
  }, [reload, reloadApps])
  return <Ctx.Provider value={{ stages, summary, apps, reload, reloadApps }}>{children}</Ctx.Provider>
}

/** One app's state for this person; `ok` = the scope works. */
export function useAppState() {
  const { apps } = useData()
  return (slug: string, scope?: string) => {
    const a = apps?.find(x => x.slug === slug)
    const ok = !!a && a.via !== '' && (!scope || a.on.includes(scope))
    return { app: a, ok, loading: !apps }
  }
}
