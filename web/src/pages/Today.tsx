// Today: the next steps due, the deals going quiet, what closes soon and
// what was won — and the totals at a glance.
import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { who } from '../lib/crm'
import { useOnChange } from '../lib/data'
import { ago, fmtDate, fmtMoney, fmtMoneyMap, todayISO } from '../lib/format'
import type { Deal, TodayView } from '../lib/types'
import { StepSheet } from '../components/DealActions'
import { Icon } from '../components/icons'
import { openCreate } from '../components/Layout'
import { Empty, Loading } from '../components/ui'

export default function Today() {
  const { user } = useApp()
  const [v, setV] = useState<TodayView | null>(null)
  const [step, setStep] = useState<Deal | null>(null)
  const load = useCallback(async () => { try { setV(await api.get<TodayView>(`${V1}/today`)) } catch { setV(x => x) } }, [])
  useEffect(() => { load() }, [load])
  useOnChange(load)
  if (!v) return <Loading />
  const s = v.summary
  const L = v.lists
  const hour = new Date().getHours()
  const hello = hour < 12 ? 'Good morning' : hour < 18 ? 'Good afternoon' : 'Good evening'
  const nothing = s.open === 0 && L.won.length === 0
  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">{hello}{user?.name ? `, ${user.name.split(' ')[0]}` : ''}</h1>
          <p className="muted">{L.overdue.length + L.today.length ? `${L.overdue.length + L.today.length} ${L.overdue.length + L.today.length === 1 ? 'thing' : 'things'} to do today` : s.open ? 'Nothing due today.' : 'No open deals yet.'}</p></div>
        <button type="button" className="btn primary hide-sm" onClick={() => openCreate('deal')}><Icon name="plus" size={15} />New deal</button>
      </div>

      <div className="tiles">
        <Link to="/pipeline" className="tile"><div className="tile-head"><span className="tile-ico"><Icon name="layers" size={14} /></span>Open deals</div><div className="tile-val">{s.open}</div><div className="muted small ellipsis">{fmtMoneyMap(s.open_value, s.currency, true)}</div></Link>
        <Link to="/pipeline" className="tile"><div className="tile-head"><span className="tile-ico"><Icon name="percent" size={14} /></span>Likely</div><div className="tile-val small-val">{fmtMoneyMap(s.weighted, s.currency, true)}</div><div className="muted small">each deal × its chance</div></Link>
        <Link to="/deals?status=won" className="tile" style={{ ['--tile' as string]: 'var(--success)' }}><div className="tile-head"><span className="tile-ico"><Icon name="trophy" size={14} /></span>Won this month</div><div className="tile-val">{s.won_month}</div><div className="muted small ellipsis">{fmtMoneyMap(s.won_month_value, s.currency, true)}</div></Link>
        <Link to="/reports" className="tile"><div className="tile-head"><span className="tile-ico"><Icon name="chart" size={14} /></span>Win rate</div><div className="tile-val">{s.won_year + s.lost_year ? `${s.win_rate}%` : '—'}</div><div className="muted small">{s.won_year} won, {s.lost_year} lost in a year</div></Link>
      </div>

      {nothing && <Empty title="Your first deal" icon="layers">
        A deal is possible work for someone: who it is with, what it is worth and what you do next. <button type="button" className="link-btn" onClick={() => openCreate('deal')}>Add one</button>, or <button type="button" className="link-btn" onClick={() => openCreate('import')}>bring them from a spreadsheet</button>.
      </Empty>}

      <DealList title="Overdue" icon="alert" tone="bad" deals={L.overdue} onStep={setStep} />
      <DealList title="Today" icon="today" deals={L.today} onStep={setStep} empty={s.open ? 'Nothing for today.' : undefined} />
      <DealList title="This week" icon="calendar" deals={L.week} onStep={setStep} />
      <DealList title="Gone quiet" sub={`No next step and nothing for a while. Set a step, or let it go.`} icon="clock" deals={L.stale} onStep={setStep} quiet />
      <DealList title="No next step" icon="target" deals={L.no_step} onStep={setStep} quiet />
      <DealList title="Hoped to close soon" icon="flag" deals={L.closing} close />
      <DealList title="Won lately" icon="trophy" tone="good" deals={L.won} won />
      {step && <StepSheet deal={step} onClose={() => setStep(null)} onDone={() => { setStep(null); load() }} />}
    </div>
  )
}

function DealList({ title, sub, icon, deals, onStep, tone = '', empty, quiet = false, close = false, won = false }: { title: string; sub?: string; icon: string; deals: Deal[]; onStep?: (d: Deal) => void; tone?: '' | 'bad' | 'good'; empty?: string; quiet?: boolean; close?: boolean; won?: boolean }) {
  if (!deals.length && !empty) return null
  const t = todayISO()
  return (
    <section className={`day-sec ${tone}`}>
      <h3 className="day-h"><Icon name={icon} size={15} />{title}{deals.length > 0 && <span className="count">{deals.length}</span>}</h3>
      {sub && <p className="muted small">{sub}</p>}
      {!deals.length ? <p className="muted small">{empty}</p> : (
        <div className="card list">
          {deals.slice(0, 12).map(d => (
            <div key={d.id} className="list-row deal-row">
              <Link to={`/deals/${d.id}`} className="row-main grow">
                <span className="row-title"><span className="ellipsis">{quiet || close || won ? d.title : (d.next_action || d.title)}</span></span>
                <span className="muted small ellipsis">
                  {quiet || close || won ? who(d) : <>{d.title}{who(d) && ` · ${who(d)}`}</>}
                  {quiet && <> · last touched {ago(d.last_activity_at)}</>}
                  {close && <> · by {fmtDate(d.expected_close)}</>}
                  {won && d.closed_at && <> · {ago(d.closed_at)}</>}
                </span>
              </Link>
              {!quiet && !close && !won && d.next_due && d.next_due !== t && <span className={`pill ${d.next_due < t ? 'st-overdue' : 'st-ok'}`}>{fmtDate(d.next_due, false)}</span>}
              {d.value > 0 && <span className="row-amount tnum muted">{fmtMoney(d.value, d.currency, true)}</span>}
              {onStep && <button type="button" className="btn sm" onClick={() => onStep(d)} title={d.next_action ? 'Mark the step done' : 'Set the next step'}>{d.next_action && !quiet ? <><Icon name="check" size={14} /><span className="hide-sm">Done</span></> : <><Icon name="plus" size={14} /><span className="hide-sm">Step</span></>}</button>}
            </div>
          ))}
          {deals.length > 12 && <Link to="/deals" className="list-row muted small">and {deals.length - 12} more…</Link>}
        </div>
      )}
    </section>
  )
}
