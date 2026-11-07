// One deal: where it stands, the next step, who it is with, the timeline,
// what the other apps know about the person and what the deal has there.
import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { ACTIVITY, APP_INFO, TARGET_APP, TARGET_LABEL, sourceLabel, stageColor } from '../lib/crm'
import { announceChange, useAppState, useData, useOnChange } from '../lib/data'
import { ago, fmtDate, fmtDateTime, fmtMoney } from '../lib/format'
import type { Activity, Deal, DealView, ElseSection, Made } from '../lib/types'
import { AppOff, offTitle } from '../components/Apps'
import { DealForm } from '../components/DealForm'
import { LogSheet, LostSheet, MAKE_SCOPE, MakeSheet, StepSheet, WonSheet } from '../components/DealActions'
import { Icon } from '../components/icons'
import { useCrumb } from '../components/Layout'
import { Confirm, Empty, Loading, type MenuItem, MenuButton, Spinner, errMsg, useToast } from '../components/ui'

const MAKE: { target: string; label: string }[] = [
  { target: 'email', label: 'Email' }, { target: 'event', label: 'Meeting' }, { target: 'task', label: 'Task' }, { target: 'note', label: 'Note' },
  { target: 'signing', label: 'To sign' }, { target: 'project', label: 'Project' }, { target: 'invoice', label: 'Invoice' },
]

export default function DealPage() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { toast } = useToast()
  const [view, setView] = useState<DealView | null>(null)
  const [err, setErr] = useState('')
  const load = useCallback(async () => {
    try { setView(await api.get<DealView>(`${V1}/deals/${id}`)); setErr('') } catch (e) { setErr(errMsg(e)) }
  }, [id])
  useEffect(() => { setView(null); load() }, [load])
  useOnChange(load)
  // what was sent out may have changed (signed, paid, ticked off): look once when the page opens
  useEffect(() => {
    if (!id) return
    api.post<Made[]>(`${V1}/deals/${id}/sync`).then(made => setView(v => v ? { ...v, made } : v)).catch(() => {})
  }, [id])
  useCrumb(view?.deal.title)
  if (err) return <Empty title="This deal is not there" icon="alert">{err} · <Link to="/deals">All deals</Link></Empty>
  if (!view) return <Loading />
  return <Deal1 view={view} reload={load} onDeleted={() => { toast('Deleted'); navigate('/pipeline') }} />
}

function Deal1({ view, reload, onDeleted }: { view: DealView; reload: () => void; onDeleted: () => void }) {
  const d = view.deal
  const { stages } = useData()
  const { settings } = useApp()
  const appState = useAppState()
  const { toast } = useToast()
  const [sheet, setSheet] = useState<'' | 'edit' | 'step' | 'log' | 'won' | 'lost' | 'delete'>('')
  const [make, setMake] = useState('')
  const [moving, setMoving] = useState(0)
  const [summary, setSummary] = useState('')
  const [summing, setSumming] = useState(false)
  const open = (stages || []).filter(s => s.kind === 'open')
  const cur = (stages || []).find(s => s.id === d.stage_id)
  const move = async (stageID: number) => {
    if (stageID === d.stage_id) return
    setMoving(stageID)
    try { await api.post(`${V1}/deals/${d.id}/move`, { stage_id: stageID }); announceChange() } catch (e) { toast(errMsg(e), 'err') } finally { setMoving(0) }
  }
  const star = async () => { await api.put(`${V1}/deals/${d.id}`, { starred: !d.starred }).catch(() => {}); reload() }
  const del = async () => { try { await api.del(`${V1}/deals/${d.id}`); announceChange(); onDeleted() } catch (e) { toast(errMsg(e), 'err') } }
  const sumUp = async () => {
    setSumming(true)
    try { setSummary((await api.post<{ text: string }>(`${V1}/deals/${d.id}/ai`, { want: 'summary' })).text) } catch (e) { toast(errMsg(e), 'err') } finally { setSumming(false) }
  }
  const linkContact = async () => {
    try { const m = await api.post<Made>(`${V1}/deals/${d.id}/make`, { target: 'contact' }); toast(`Linked: ${m.title}`); announceChange() } catch (e) { toast(errMsg(e), 'err') }
  }
  const more: MenuItem[] = [
    { label: 'Change the deal', icon: 'edit', onSelect: () => setSheet('edit') },
    { label: 'Write something down', icon: 'note', onSelect: () => setSheet('log') },
    ...(settings?.ai_help ? [{ label: 'Sum it up (Pocket AI)', icon: 'sparkles', onSelect: sumUp }] : []),
    'sep',
    ...(d.status === 'open' ? [{ label: 'Won', icon: 'trophy', onSelect: () => setSheet('won') }, { label: 'Lost', icon: 'x-circle', onSelect: () => setSheet('lost') }] : open.length ? [{ label: 'Open it again', icon: 'undo', onSelect: () => move(open[0].id) }] : []),
    'sep',
    { label: 'Delete', icon: 'trash', danger: true, onSelect: () => setSheet('delete') },
  ]
  const contacts = appState('contacts', 'contacts:read')
  const contactURL = (ref: string) => contacts.app?.open_url && ref ? `${contacts.app.open_url.replace(/\/$/, '')}/contacts/${ref}` : ''
  return (
    <div className="deal-page">
      <header className="deal-head">
        <div className="grow">
          <div className="deal-kicker">
            <span className="stage-pill" style={{ ['--st' as string]: stageColor(cur) }}>{d.stage_name}</span>
            {d.status === 'open' && <span className="muted small">{d.chance}% chance · {d.days_in_stage ? `${d.days_in_stage} ${d.days_in_stage === 1 ? 'day' : 'days'} in this stage` : 'new in this stage'}</span>}
            {d.status !== 'open' && d.closed_at && <span className="muted small">{d.status === 'won' ? 'Won' : 'Lost'} {fmtDate(d.closed_at.slice(0, 10))}{d.lost_reason ? ` · ${d.lost_reason}` : ''}</span>}
          </div>
          <h2 className="deal-title">{d.title}</h2>
          <div className="deal-value">{d.value ? fmtMoney(d.value, d.currency) : <span className="muted">No value yet</span>}{d.status === 'open' && d.value > 0 && d.chance < 100 && <span className="muted small"> · {fmtMoney(d.weighted, d.currency)} weighted</span>}</div>
        </div>
        <div className="row deal-tools">
          <button type="button" className={`btn ghost icon-only ${d.starred ? 'star-on' : ''}`} onClick={star} aria-label={d.starred ? 'Unstar' : 'Star'} title={d.starred ? 'Starred' : 'Star'}><Icon name="star" size={18} /></button>
          <button type="button" className="btn" onClick={() => setSheet('edit')}><Icon name="edit" size={15} /><span className="hide-sm">Change</span></button>
          {d.status === 'open' && <>
            <button type="button" className="btn won-btn" onClick={() => setSheet('won')}><Icon name="trophy" size={15} />Won</button>
            <button type="button" className="btn hide-sm" onClick={() => setSheet('lost')}>Lost</button>
          </>}
          <MenuButton label="More" items={more} align="right" width={240}><Icon name="more" size={18} /></MenuButton>
        </div>
      </header>

      {d.status === 'open' && open.length > 0 && <nav className="stepper" aria-label="Stage">
        {open.map(s => {
          const i = open.findIndex(x => x.id === d.stage_id)
          const j = open.findIndex(x => x.id === s.id)
          return (
            <button key={s.id} type="button" className={`stepper-step ${s.id === d.stage_id ? 'cur' : j < i ? 'past' : ''}`} style={{ ['--st' as string]: stageColor(s) }} onClick={() => move(s.id)} disabled={!!moving} title={`Move to ${s.name}`}>
              {moving === s.id ? <Spinner /> : null}<span className="ellipsis">{s.name}</span>
            </button>
          )
        })}
      </nav>}

      <div className="deal-grid">
        <div className="deal-main stack loose">
          {d.status === 'open' && <section className={`next-card ${d.overdue ? 'overdue' : ''} ${!d.next_action ? 'none' : ''}`}>
            <div className="next-ico"><Icon name={d.next_action ? 'target' : 'alert'} size={18} /></div>
            <div className="grow">
              <div className="next-l">Next step{d.next_due && <> · <span className={d.overdue ? 'danger-text' : ''}>{d.overdue ? 'was due ' : ''}{fmtDate(d.next_due)}</span></>}</div>
              <div className="next-t">{d.next_action || 'No next step. Every open deal needs one, or it goes quiet.'}</div>
              {d.next_task_ref && <div className="muted small"><Icon name="check-square" size={12} className="inline-ico" /> Also in Pocket Tasks</div>}
            </div>
            <button type="button" className="btn primary" onClick={() => setSheet('step')}>{d.next_action ? <><Icon name="check" size={15} />Done</> : <><Icon name="plus" size={15} />Set it</>}</button>
          </section>}
          {d.status !== 'open' && d.outcome_note && <section className="callout"><b>{d.status === 'won' ? 'Agreed' : 'Note'}:</b> {d.outcome_note}</section>}
          {summary && <section className="callout ai-sum"><div className="row"><Icon name="sparkles" size={15} /><b>Where it stands</b><span className="spacer" /><button type="button" className="btn ghost icon-only sm" onClick={() => setSummary('')} aria-label="Close"><Icon name="x" size={14} /></button></div><p className="pre">{summary}</p></section>}
          {summing && <section className="callout"><Spinner /> Pocket AI is reading the deal…</section>}

          <Timeline deal={d} acts={view.activities} onLog={() => setSheet('log')} />
          <Elsewhere deal={d} />
        </div>

        <aside className="deal-side stack">
          <section className="card side-card">
            <h3 className="side-h">With</h3>
            {!(d.contact_name || d.company || d.contact_email) ? <p className="muted small">Nobody yet. <button type="button" className="link-btn" onClick={() => setSheet('edit')}>Say who</button></p> : <>
              {d.contact_name && <div className="who-row"><Icon name="user" size={15} /><span className="grow ellipsis">{contactURL(d.contact_ref) ? <a href={contactURL(d.contact_ref)} target="_blank" rel="noreferrer">{d.contact_name}</a> : d.contact_name}</span></div>}
              {d.company && <div className="who-row"><Icon name="landmark" size={15} /><span className="grow ellipsis">{contactURL(d.company_ref) ? <a href={contactURL(d.company_ref)} target="_blank" rel="noreferrer">{d.company}</a> : <Link to={`/people?q=${encodeURIComponent(d.company)}`}>{d.company}</Link>}</span></div>}
              {d.contact_email && <div className="who-row"><Icon name="mail" size={15} /><a className="grow ellipsis" href={`mailto:${d.contact_email}`}>{d.contact_email}</a></div>}
              {d.contact_phone && <div className="who-row"><Icon name="phone" size={15} /><a className="grow ellipsis" href={`tel:${d.contact_phone.replace(/\s/g, '')}`}>{d.contact_phone}</a></div>}
              {!(d.contact_ref || d.company_ref) && (contacts.ok
                ? <button type="button" className="btn sm mt" onClick={linkContact}><Icon name="users" size={14} />Find or add in Pocket Contacts</button>
                : <p className="muted small mt" title={offTitle(contacts.app, 'contacts')}><Icon name="info" size={12} className="inline-ico" /> Kept here{contacts.app?.state === 'missing' ? '' : '; connect Pocket Contacts to link their card'}.</p>)}
            </>}
          </section>

          <section className="card side-card">
            <h3 className="side-h">In your other apps</h3>
            <MadeList made={view.made} onChange={reload} />
            <div className="make-grid">
              {MAKE.map(m => {
                const slug = TARGET_APP[m.target]
                const st = appState(slug, MAKE_SCOPE[m.target])
                const fallback = m.target === 'email' || m.target === 'event'
                const off = !st.ok && !fallback
                return (
                  <button key={m.target} type="button" className={`make-btn ${!st.ok ? 'off' : ''}`} onClick={() => setMake(m.target)} disabled={off || st.loading} title={st.ok ? APP_INFO[slug].name : offTitle(st.app, slug) + (fallback ? ' You can still do it without it.' : '')}>
                    <Icon name={APP_INFO[slug].icon} size={15} />{m.label}
                  </button>
                )
              })}
            </div>
          </section>

          <section className="card side-card">
            <h3 className="side-h">About</h3>
            {d.description ? <p className="pre small">{d.description}</p> : <p className="muted small">Nothing written yet.</p>}
            <dl className="facts">
              {d.expected_close && <><dt>Hoped to close</dt><dd>{fmtDate(d.expected_close)}</dd></>}
              <dt>Came from</dt><dd>{sourceLabel(d.source)}{d.source_app && ` (${APP_INFO[d.source_app as keyof typeof APP_INFO]?.name || d.source_app})`}</dd>
              {d.tags.length > 0 && <><dt>Tags</dt><dd className="chips">{d.tags.map(t => <Link key={t} className="chip" to={`/deals?tag=${encodeURIComponent(t)}`}>{t}</Link>)}</dd></>}
              <dt>Added</dt><dd>{fmtDate(d.created_at.slice(0, 10))}</dd>
              <dt>Last touched</dt><dd>{ago(d.last_activity_at)}</dd>
            </dl>
          </section>
        </aside>
      </div>

      {sheet === 'edit' && <DealForm deal={d} onClose={() => setSheet('')} onSaved={() => reload()} />}
      {sheet === 'step' && <StepSheet deal={d} onClose={() => setSheet('')} onDone={() => { setSheet(''); reload() }} />}
      {sheet === 'log' && <LogSheet deal={d} onClose={() => setSheet('')} onDone={() => { setSheet(''); reload() }} />}
      {sheet === 'won' && <WonSheet deal={d} onClose={() => setSheet('')} onDone={() => { setSheet(''); reload() }} />}
      {sheet === 'lost' && <LostSheet deal={d} onClose={() => setSheet('')} onDone={() => { setSheet(''); reload() }} />}
      {sheet === 'delete' && <Confirm title="Delete this deal?" message={<>“{d.title}” and its timeline go. What it has in your other apps stays there.</>} confirmLabel="Delete" onConfirm={del} onCancel={() => setSheet('')} />}
      {make && <MakeSheet deal={d} target={make} onClose={() => setMake('')} onDone={() => { setMake(''); reload() }} />}
    </div>
  )
}

function MadeList({ made, onChange }: { made: Made[]; onChange: () => void }) {
  const { toast } = useToast()
  if (!made.length) return <p className="muted small">Nothing yet. Make it from here: it is linked back to the deal.</p>
  const forget = async (m: Made) => { try { await api.del(`${V1}/made/${m.id}`); onChange() } catch (e) { toast(errMsg(e), 'err') } }
  return (
    <ul className="made-list">
      {made.map(m => {
        const slug = TARGET_APP[m.target] || (m.app as keyof typeof APP_INFO)
        const info = APP_INFO[slug as keyof typeof APP_INFO]
        return (
          <li key={m.id} className={m.error ? 'bad' : ''}>
            <Icon name={m.error ? 'alert' : info?.icon || 'link'} size={14} />
            <div className="grow min0">
              <div className="ellipsis">{m.url ? <a href={m.url} target="_blank" rel="noreferrer">{m.title || TARGET_LABEL[m.target]}</a> : (m.title || TARGET_LABEL[m.target])}</div>
              <div className="muted tiny">{m.error || <>{TARGET_LABEL[m.target] || m.target}{m.state && <> · <span className={`state st-${m.state}`}>{m.state}</span></>}</>}</div>
            </div>
            <button type="button" className="btn ghost icon-only sm" onClick={() => forget(m)} aria-label="Forget" title="Forget here (it stays in the other app)"><Icon name="x" size={13} /></button>
          </li>
        )
      })}
    </ul>
  )
}

function Timeline({ deal, acts, onLog }: { deal: Deal; acts: Activity[]; onLog: () => void }) {
  const { toast } = useToast()
  const [confirm, setConfirm] = useState<Activity | null>(null)
  const del = async (a: Activity) => { try { await api.del(`${V1}/activities/${a.id}`); announceChange() } catch (e) { toast(errMsg(e), 'err') } setConfirm(null) }
  return (
    <section>
      <div className="sec-head"><h3>Timeline</h3><span className="spacer" /><button type="button" className="btn sm" onClick={onLog}><Icon name="plus" size={14} />Write it down</button></div>
      {acts.length === 0 ? <p className="muted small">Nothing yet.</p> : (
        <ol className="timeline">
          {acts.map(a => {
            const info = ACTIVITY[a.kind] || ACTIVITY.note
            const own = ['note', 'call', 'meeting', 'email', 'step'].includes(a.kind) && !a.app
            return (
              <li key={a.id} className={`tl tl-${a.kind}`}>
                <span className="tl-dot"><Icon name={info.icon} size={13} /></span>
                <div className="tl-body">
                  <div className="tl-h">
                    <span className="tl-t">{a.url ? <a href={a.url} target="_blank" rel="noreferrer">{a.title || info.label}</a> : (a.title || info.label)}</span>
                    <span className="muted tiny tl-when" title={fmtDateTime(a.at)}>{fmtDateTime(a.at, new Date(a.at).getFullYear() !== new Date().getFullYear())}</span>
                    {own && <button type="button" className="btn ghost icon-only sm tl-x" onClick={() => setConfirm(a)} aria-label="Delete this line"><Icon name="trash" size={12} /></button>}
                  </div>
                  {a.body && <p className="tl-text pre">{a.body}</p>}
                  {a.source_app && <span className="muted tiny">from {APP_INFO[a.source_app as keyof typeof APP_INFO]?.name || a.source_app}</span>}
                </div>
              </li>
            )
          })}
        </ol>
      )}
      {confirm && <Confirm title="Delete this line?" message={confirm.title || confirm.body.slice(0, 80)} confirmLabel="Delete" onConfirm={() => del(confirm)} onCancel={() => setConfirm(null)} />}
      {deal.activities > acts.length && <p className="muted small">Older lines are in the export.</p>}
    </section>
  )
}

/** What the other apps know about the person, read live, each on its own. */
function Elsewhere({ deal }: { deal: Deal }) {
  const appState = useAppState()
  const [secs, setSecs] = useState<ElseSection[] | null>(null)
  useEffect(() => {
    let alive = true
    setSecs(null)
    api.get<ElseSection[]>(`${V1}/deals/${deal.id}/elsewhere`).then(s => { if (alive) setSecs(s) }).catch(() => { if (alive) setSecs([]) })
    return () => { alive = false }
  }, [deal.id, deal.contact_email, deal.company, deal.contact_name])
  if (!secs) return <section><div className="sec-head"><h3>From your other apps</h3></div><div className="skeleton" style={{ height: 64 }} /></section>
  const shown = secs.map(s => ({ ...s, items: s.items || [] })).filter(s => s.state !== 'none' || s.items.length)
  const off = shown.filter(s => s.state === 'off')
  return (
    <section>
      <div className="sec-head"><h3>From your other apps</h3></div>
      <div className="stack">
        {off.length === 1 && <AppOff slug={off[0].app} app={appState(off[0].app).app} compact />}
        {off.length > 1 && <div className="app-off compact else-off">
          <span className="app-off-ico"><Icon name="link" size={15} /></span>
          <div className="grow">
            <div className="app-off-h">Not connected yet</div>
            <div className="muted small">{off.map((s, i) => <span key={s.app} title={offTitle(appState(s.app).app, s.app)}>{i ? ', ' : ''}{APP_INFO[s.app].name}</span>)} — with them, the emails, meetings, form answers and invoices you have with this person show here. <Link to="/settings/apps">Connected apps</Link></div>
          </div>
        </div>}
        {shown.map(s => {
          if (s.state === 'off') return null
          if (s.state === 'error') return <div key={s.app} className="app-off compact"><span className="app-off-ico"><Icon name={APP_INFO[s.app].icon} size={15} /></span><div className="grow muted small">{s.detail}</div></div>
          if (s.state === 'none') return null
          return (
            <div key={s.app} className="else-sec">
              <div className="else-h"><Icon name={APP_INFO[s.app].icon} size={14} />{s.name}<span className="muted small">· {s.items.length ? s.items.length : 'nothing'} {s.app === 'mail' ? 'with them' : s.app === 'calendar' ? 'about them' : s.app === 'forms' ? 'from them' : 'for them'}</span></div>
              {s.items.slice(0, 6).map((it, i) => (
                <a key={i} className="else-row" href={it.url || undefined} target="_blank" rel="noreferrer">
                  {it.kind === 'email' && <Icon name={it.out ? 'send' : 'inbox'} size={13} />}
                  <span className="grow min0"><span className="ellipsis block-line">{it.title}</span>{it.body && <span className="muted tiny ellipsis block-line">{it.body}</span>}</span>
                  {it.amount && <span className="tnum small">{it.amount}</span>}
                  {it.state && <span className={`state st-${it.state}`}>{it.state}</span>}
                  <span className="muted tiny">{it.at ? fmtDate(it.at.slice(0, 10), false) : ''}</span>
                </a>
              ))}
            </div>
          )
        })}
        {shown.every(s => s.state === 'none') && <p className="muted small">Add their email or company to see their emails, meetings, form answers and invoices here.</p>}
      </div>
    </section>
  )
}
