// How sales go: won per month, where the open deals stand, why deals are
// lost and where the good ones come from.
import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { sourceLabel } from '../lib/crm'
import { fmtMoney } from '../lib/format'
import type { CountRow, Reports as R } from '../lib/types'
import { Icon } from '../components/icons'
import { Loading, Segmented } from '../components/ui'

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

export default function Reports() {
  const [months, setMonths] = useState(12)
  const [r, setR] = useState<R | null>(null)
  useEffect(() => { api.get<R>(`${V1}/reports?months=${months}`).then(setR).catch(() => setR(null)) }, [months])
  if (!r) return <Loading />
  const cur = r.currency
  const maxV = Math.max(1, ...r.months.map(m => m.value))
  const wonTotal = r.months.reduce((n, m) => n + m.value, 0)
  const wonCount = r.months.reduce((n, m) => n + m.won, 0)
  const maxF = Math.max(1, ...r.funnel.map(f => f.count))
  return (
    <div className="page stack loose">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Reports</h1><p className="muted">In {cur}{r.other_currencies.length ? ` (deals in ${r.other_currencies.join(', ')} are counted, not added up)` : ''}.</p></div>
        <Segmented size="sm" label="Months" value={months} onChange={setMonths} options={[{ value: 6, label: '6 months' }, { value: 12, label: 'A year' }, { value: 24, label: '2 years' }]} />
      </div>
      <div className="tiles">
        <div className="tile static"><div className="tile-head"><span className="tile-ico"><Icon name="trophy" size={14} /></span>Won</div><div className="tile-val">{fmtMoney(wonTotal, cur, true)}</div><div className="muted small">{wonCount} {wonCount === 1 ? 'deal' : 'deals'}</div></div>
        <div className="tile static"><div className="tile-head"><span className="tile-ico"><Icon name="percent" size={14} /></span>Win rate</div><div className="tile-val">{r.summary.won_year + r.summary.lost_year ? `${r.summary.win_rate}%` : '—'}</div><div className="muted small">of the deals closed in a year</div></div>
        <div className="tile static"><div className="tile-head"><span className="tile-ico"><Icon name="receipt" size={14} /></span>A won deal</div><div className="tile-val">{r.average_won ? fmtMoney(Math.round(r.average_won / 100) * 100, cur, true) : '—'}</div><div className="muted small">on average</div></div>
        <div className="tile static"><div className="tile-head"><span className="tile-ico"><Icon name="hourglass" size={14} /></span>To win one</div><div className="tile-val">{r.days_to_win ? `${r.days_to_win} days` : '—'}</div><div className="muted small">from first talk, on average</div></div>
      </div>

      <section className="card pad-card">
        <h3 className="card-h">Won per month</h3>
        <div className="month-bars" role="img" aria-label="Won per month">
          {r.months.map(m => {
            const [y, mo] = m.month.split('-').map(Number)
            return (
              <div key={m.month} className="mb-col" title={`${MONTHS[mo - 1]} ${y}: ${m.won} won (${fmtMoney(m.value, cur)}), ${m.lost} lost, ${m.added} added`}>
                <span className="mb-v tnum">{m.value ? fmtMoney(m.value, '', true).trim() : ''}</span>
                <span className="mb-track"><span className="mb-bar" style={{ height: `${Math.round((m.value / maxV) * 100)}%` }} /></span>
                <span className="mb-l">{MONTHS[mo - 1]}</span>
              </div>
            )
          })}
        </div>
      </section>

      <div className="two-col">
        <section className="card pad-card">
          <h3 className="card-h">Open deals by stage</h3>
          <Bars rows={r.funnel} max={maxF} cur={cur} />
        </section>
        <section className="card pad-card">
          <h3 className="card-h">Why deals are lost</h3>
          {r.lost_reasons.length ? <Bars rows={r.lost_reasons} max={Math.max(...r.lost_reasons.map(x => x.count))} cur={cur} bad /> : <p className="muted small">None lost yet.</p>}
        </section>
      </div>
      <section className="card pad-card">
        <h3 className="card-h">Where deals come from</h3>
        <table className="table compact">
          <thead><tr><th>Came from</th><th className="right">Deals</th><th className="right">Won</th><th className="right">Won worth</th></tr></thead>
          <tbody>{r.sources.map(s => <tr key={s.name}><td>{s.name === 'not said' ? <span className="muted">Not said</span> : sourceLabel(s.name)}</td><td className="right tnum">{s.count}</td><td className="right tnum">{s.won || 0}</td><td className="right tnum">{s.value ? fmtMoney(s.value, cur, true) : ''}</td></tr>)}</tbody>
        </table>
      </section>
    </div>
  )
}

function Bars({ rows, max, cur, bad = false }: { rows: CountRow[]; max: number; cur: string; bad?: boolean }) {
  return (
    <div className="bars">
      {rows.map(x => (
        <div key={x.name} className="bar-row">
          <span className="bar-l ellipsis">{x.name}</span>
          <span className="bar-track"><span className={`bar-fill ${bad ? 'bad' : ''}`} style={{ width: `${Math.max(2, Math.round((x.count / max) * 100))}%` }} /></span>
          <span className="bar-n tnum">{x.count}{x.value ? <span className="muted"> · {fmtMoney(x.value, cur, true)}</span> : null}</span>
        </div>
      ))}
    </div>
  )
}
