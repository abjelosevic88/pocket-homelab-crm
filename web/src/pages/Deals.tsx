// Every deal in one list: open, won, lost — with a search and filters.
import { useCallback, useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { SOURCES, stageColor, who } from '../lib/crm'
import { useData, useOnChange } from '../lib/data'
import { ago, fmtDate, fmtMoney, todayISO } from '../lib/format'
import type { Deal } from '../lib/types'
import { Icon } from '../components/icons'
import { openCreate } from '../components/Layout'
import { Select } from '../components/Select'
import { Empty, Loading, Segmented, useDebounce } from '../components/ui'

type StatusF = 'open' | 'won' | 'lost' | ''

export default function Deals() {
  const [sp, setSp] = useSearchParams()
  const { stages } = useData()
  const status = (sp.get('status') ?? 'open') as StatusF
  const [q, setQ] = useState(sp.get('q') || '')
  const dq = useDebounce(q, 250)
  const stage = sp.get('stage') || ''
  const tag = sp.get('tag') || ''
  const source = sp.get('source') || ''
  const due = sp.get('due') || ''
  const sort = sp.get('sort') || (status === 'open' ? 'due' : 'close')
  const [list, setList] = useState<{ items: Deal[]; total: number } | null>(null)
  const set = (k: string, v: string) => { const n = new URLSearchParams(sp); if (v) n.set(k, v); else n.delete(k); setSp(n, { replace: true }) }
  const load = useCallback(async () => {
    const p = new URLSearchParams({ status, q: dq, stage, tag, source, due, sort, limit: '500' })
    for (const [k, v] of [...p.entries()]) if (!v) p.delete(k)
    try { setList(await api.get(`${V1}/deals?${p}`)) } catch { setList({ items: [], total: 0 }) }
  }, [status, dq, stage, tag, source, due, sort])
  useEffect(() => { load() }, [load])
  useOnChange(load)
  const t = todayISO()
  return (
    <div className="page stack">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Deals</h1>{list && <p className="muted">{list.total} {list.total === 1 ? 'deal' : 'deals'}</p>}</div>
        <button type="button" className="btn primary hide-sm" onClick={() => openCreate('deal')}><Icon name="plus" size={15} />New deal</button>
      </div>
      <div className="toolbar row wrap filters">
        <Segmented size="sm" label="Which" value={status} onChange={v => set('status', v === 'open' ? '' : v || 'all')} options={[{ value: 'open', label: 'Open' }, { value: 'won', label: 'Won' }, { value: 'lost', label: 'Lost' }, { value: '' as StatusF, label: 'All' }]} />
        <div className="search-in grow"><Icon name="search" size={15} /><input value={q} onChange={e => { setQ(e.target.value); set('q', e.target.value) }} placeholder="Search deals, people, companies…" aria-label="Search" /></div>
        <Select ariaLabel="Stage" value={stage} onChange={v => set('stage', v)} placeholder="Any stage" clearLabel="Any stage" options={(stages || []).map(s => ({ value: String(s.id), label: s.name, group: s.kind === 'open' ? 'Open' : 'Closed' }))} />
        {status === 'open' && <Select ariaLabel="Next step" value={due} onChange={v => set('due', v)} placeholder="Any next step" clearLabel="Any next step" options={[{ value: 'overdue', label: 'Overdue' }, { value: 'today', label: 'Due today or before' }, { value: 'week', label: 'Due this week' }, { value: 'none', label: 'No next step' }]} />}
        <Select ariaLabel="Came from" value={source} onChange={v => set('source', v)} placeholder="Any source" clearLabel="Any source" options={SOURCES} />
        <Select ariaLabel="Sort" value={sort} onChange={v => set('sort', v)} options={[{ value: 'due', label: 'Next step first' }, { value: 'value', label: 'Biggest first' }, { value: 'updated', label: 'Changed lately' }, { value: 'created', label: 'Newest' }, { value: 'expected', label: 'Closing soonest' }, { value: 'close', label: 'Closed lately' }, { value: 'stage', label: 'By stage' }]} />
        {tag && <button type="button" className="chip on" onClick={() => set('tag', '')}>#{tag}<Icon name="x" size={12} /></button>}
      </div>
      {!list ? <Loading /> : list.items.length === 0 ? <Empty title={dq || stage || tag || due || source ? 'Nothing matches' : status === 'open' ? 'No open deals' : 'Nothing here yet'} icon="search">{status === 'open' && !dq && <button type="button" className="link-btn" onClick={() => openCreate('deal')}>Add a deal</button>}</Empty> : (
        <div className="card list">
          {list.items.map(d => (
            <Link key={d.id} to={`/deals/${d.id}`} className="list-row deal-row">
              <span className="stage-dot" style={{ background: stageColor({ color: d.stage_color || '', kind: d.status }) }} title={d.stage_name} />
              <span className="row-main grow">
                <span className="row-title">{d.starred && <Icon name="star" size={12} className="star-on" />}<span className="ellipsis">{d.title}</span></span>
                <span className="muted small ellipsis">{[who(d), d.stage_name, d.status === 'lost' && d.lost_reason].filter(Boolean).join(' · ')}</span>
              </span>
              {d.status === 'open' && (d.next_action
                ? <span className={`pill hide-sm ${d.overdue ? 'st-overdue' : d.next_due === t ? 'st-due' : 'st-ok'}`} title={d.next_action}>{d.next_due ? fmtDate(d.next_due, false) : 'No day'}</span>
                : <span className="pill st-paused hide-sm">No step</span>)}
              {d.status !== 'open' && d.closed_at && <span className="muted small hide-sm">{ago(d.closed_at)}</span>}
              <span className="row-amount tnum">{d.value ? fmtMoney(d.value, d.currency, true) : ''}</span>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}
