// Who you sell to: deals grouped by company (else by person), with what is
// open, what was won and when you last talked.
import { useCallback, useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { useAppState, useOnChange } from '../lib/data'
import { ago, fmtMoneyMap } from '../lib/format'
import type { Party } from '../lib/types'
import { AppOff } from '../components/Apps'
import { Icon } from '../components/icons'
import { Empty, Loading, useDebounce } from '../components/ui'

export default function People() {
  const { settings } = useApp()
  const [sp, setSp] = useSearchParams()
  const [q, setQ] = useState(sp.get('q') || '')
  const dq = useDebounce(q, 250)
  const [list, setList] = useState<Party[] | null>(null)
  const appState = useAppState()
  const contacts = appState('contacts', 'contacts:read')
  const load = useCallback(async () => { try { setList(await api.get<Party[]>(`${V1}/people?q=${encodeURIComponent(dq)}`)) } catch { setList([]) } }, [dq])
  useEffect(() => { load() }, [load])
  useOnChange(load)
  const cur = settings?.currency || 'EUR'
  const cardURL = (ref: string) => contacts.ok && contacts.app?.open_url && ref ? `${contacts.app.open_url.replace(/\/$/, '')}/contacts/${ref}` : ''
  return (
    <div className="page stack">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">People & companies</h1><p className="muted">Everyone you have deals with.</p></div>
      </div>
      <div className="search-in"><Icon name="search" size={15} /><input value={q} onChange={e => { setQ(e.target.value); setSp(e.target.value ? { q: e.target.value } : {}, { replace: true }) }} placeholder="Search names, companies, emails…" aria-label="Search" /></div>
      {!contacts.loading && !contacts.ok && <AppOff slug="contacts" app={contacts.app} compact />}
      {!list ? <Loading /> : list.length === 0 ? <Empty title={dq ? 'Nobody matches' : 'Nobody yet'} icon="users">{!dq && 'People and companies show here once you add deals with them.'}</Empty> : (
        <div className="card list">
          {list.map(p => {
            const url = cardURL(p.company_ref || p.contact_ref)
            return (
              <div key={p.key} className="list-row party-row">
                <span className={`party-ico ${p.kind}`}><Icon name={p.kind === 'company' ? 'landmark' : 'user'} size={16} /></span>
                <span className="row-main grow">
                  <span className="row-title"><span className="ellipsis">{p.name}</span>{url && <a href={url} target="_blank" rel="noreferrer" className="muted" title="Open in Pocket Contacts"><Icon name="external" size={12} /></a>}</span>
                  <span className="muted small ellipsis">{[p.kind === 'company' && p.people.join(', '), p.email, `last ${ago(p.last_at)}`].filter(Boolean).join(' · ')}</span>
                </span>
                <span className="party-nums small hide-sm">
                  {p.open > 0 && <Link to={p.kind === 'company' ? `/deals?q=${encodeURIComponent(p.company)}` : `/deals?q=${encodeURIComponent(p.email || p.name)}`} className="pill st-ok">{p.open} open · {fmtMoneyMap(p.open_value, cur, true)}</Link>}
                  {p.won > 0 && <span className="pill st-done">{p.won} won · {fmtMoneyMap(p.won_value, cur, true)}</span>}
                  {p.lost > 0 && <span className="pill st-paused">{p.lost} lost</span>}
                </span>
                <Link to={p.deals === 1 ? `/deals/${p.deal_ids[0]}` : `/deals?status=&q=${encodeURIComponent(p.kind === 'company' ? p.company : p.email || p.name)}`} className="btn sm">{p.deals} {p.deals === 1 ? 'deal' : 'deals'}</Link>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
