// The sheets behind a deal's buttons: a step done, won, lost, a line on the
// timeline, and making something in another app — with a way that works
// without that app (a calendar file, your own email app) when it is not there.
import { useEffect, useState } from 'react'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { APP_INFO, LOG_KINDS, STATE_WORDS, TARGET_APP, who } from '../lib/crm'
import { announceChange, useAppState, useData } from '../lib/data'
import { addDays, fmtMoney, todayISO } from '../lib/format'
import type { ActivityKind, Deal, Made } from '../lib/types'
import { AppOff, offTitle } from './Apps'
import { Icon } from './icons'
import { Select } from './Select'
import { Field, Modal, Segmented, Spinner, Switch, errMsg, useToast } from './ui'

type Opt = { id: number; name: string; key?: string }

function useOptions(what: 'teams' | 'templates', active: boolean) {
  const [list, setList] = useState<Opt[] | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => {
    if (!active) return
    api.get<Opt[]>(`${V1}/apps/options/${what}`).then(setList).catch(e => { setErr(errMsg(e)); setList([]) })
  }, [what, active])
  return { list, err }
}

function useAI() {
  const { settings } = useApp()
  const [ready, setReady] = useState(false)
  useEffect(() => {
    if (!settings?.ai_help) return
    api.get<{ ready: boolean }>(`${V1}/ai/status`).then(r => setReady(r.ready)).catch(() => setReady(false))
  }, [settings?.ai_help])
  return ready && !!settings?.ai_help
}

// ---- the next step ----

export function StepSheet({ deal, onClose, onDone }: { deal: Deal; onClose: () => void; onDone: (d: Deal) => void }) {
  const { settings } = useApp()
  const { toast } = useToast()
  const ai = useAI()
  const had = !!deal.next_action
  const [kind, setKind] = useState<ActivityKind>('step')
  const [note, setNote] = useState('')
  const [next, setNext] = useState(had ? '' : deal.next_action)
  const [due, setDue] = useState(addDays(todayISO(), settings?.action_days || 2))
  const [why, setWhy] = useState('')
  const [busy, setBusy] = useState(false)
  const [thinking, setThinking] = useState(false)
  const suggest = async () => {
    setThinking(true)
    try {
      const r = await api.post<{ step: string; due: string; why: string }>(`${V1}/deals/${deal.id}/ai`, { want: 'next' })
      setNext(r.step); setDue(r.due); setWhy(r.why)
    } catch (e) { toast(errMsg(e), 'err') } finally { setThinking(false) }
  }
  const go = async (e?: React.FormEvent) => {
    e?.preventDefault()
    setBusy(true)
    try {
      const d = had
        ? await api.post<Deal>(`${V1}/deals/${deal.id}/step/done`, { kind, note, next_action: next, next_due: next ? due : '' })
        : await api.put<Deal>(`${V1}/deals/${deal.id}`, { next_action: next, next_due: next ? due : '' })
      announceChange()
      toast(had ? (next ? 'Done — next step set' : 'Done') : 'Next step set')
      onDone(d)
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={had ? 'Step done' : 'Set the next step'} onClose={onClose} size="sm" footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={() => go()} disabled={busy || (!had && !next)}>{busy ? <Spinner /> : <Icon name="check" size={15} />}{had ? 'Save' : 'Set it'}</button>
    </>}>
      <form className="stack" onSubmit={go}>
        {had && <>
          <p className="step-done-what"><Icon name="check-circle" size={16} />{deal.next_action}</p>
          <Segmented size="sm" label="What it was" value={kind} onChange={setKind} options={[{ value: 'step', label: 'Done' }, ...LOG_KINDS.filter(k => k.value !== 'note').map(k => ({ value: k.value, label: k.label }))]} />
          <Field label="How it went" help="Optional; it goes on the timeline."><textarea rows={3} value={note} onChange={e => setNote(e.target.value)} placeholder="They liked the plan, want to start in March." /></Field>
        </>}
        <Field label={had ? 'Then what' : 'What you do next'} help={why || (had ? 'Empty: no next step for now.' : undefined)} group>
          <div className="row tight">
            <input value={next} onChange={e => setNext(e.target.value)} placeholder="Send the revised offer" autoFocus={!had} aria-label="Next step" />
            {ai && <button type="button" className="btn" onClick={suggest} disabled={thinking} title="Pocket AI suggests a next step from the timeline">{thinking ? <Spinner /> : <Icon name="sparkles" size={15} />}<span className="hide-sm">Suggest</span></button>}
          </div>
        </Field>
        <Field label="When"><input type="date" value={due} onChange={e => setDue(e.target.value)} disabled={!next} /></Field>
      </form>
    </Modal>
  )
}

// ---- a line on the timeline ----

export function LogSheet({ deal, onClose, onDone }: { deal: Deal; onClose: () => void; onDone: () => void }) {
  const { toast } = useToast()
  const [kind, setKind] = useState<ActivityKind>('note')
  const [title, setTitle] = useState('')
  const [body, setBody] = useState('')
  const [when, setWhen] = useState('')
  const [busy, setBusy] = useState(false)
  const go = async (e?: React.FormEvent) => {
    e?.preventDefault()
    setBusy(true)
    try {
      await api.post(`${V1}/deals/${deal.id}/activities`, { kind, title, body, at: when })
      announceChange()
      onDone()
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  const ph: Record<string, string> = { note: 'What to remember', call: 'Called Ana', meeting: 'Met at their office', email: 'They wrote back' }
  return (
    <Modal title="Write it down" onClose={onClose} size="sm" footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={() => go()} disabled={busy || !(title || body)}>{busy ? <Spinner /> : <Icon name="check" size={15} />}Add</button>
    </>}>
      <form className="stack" onSubmit={go}>
        <Segmented size="sm" label="What" value={kind} onChange={setKind} options={LOG_KINDS.map(k => ({ value: k.value, label: k.label }))} />
        <Field label="In a few words"><input value={title} onChange={e => setTitle(e.target.value)} placeholder={ph[kind]} autoFocus /></Field>
        <Field label="More" help="Optional."><textarea rows={4} value={body} onChange={e => setBody(e.target.value)} /></Field>
        <Field label="When" help="Empty: now."><input type="datetime-local" value={when} onChange={e => setWhen(e.target.value)} /></Field>
      </form>
    </Modal>
  )
}

// ---- lost ----

export function LostSheet({ deal, onClose, onDone }: { deal: Deal; onClose: () => void; onDone: (d: Deal) => void }) {
  const { settings } = useApp()
  const { stages } = useData()
  const { toast } = useToast()
  const lostStages = (stages || []).filter(s => s.kind === 'lost')
  const [stage, setStage] = useState(lostStages[0]?.id || 0)
  const [reason, setReason] = useState('')
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)
  const go = async () => {
    setBusy(true)
    try {
      const d = await api.post<Deal>(`${V1}/deals/${deal.id}/move`, { stage_id: stage, lost_reason: reason, note })
      announceChange()
      toast('Marked as lost')
      onDone(d)
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title="Lost this one" onClose={onClose} size="sm" footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn danger" onClick={go} disabled={busy || !stage}>{busy ? <Spinner /> : <Icon name="x-circle" size={15} />}Mark as lost</button>
    </>}>
      <div className="stack">
        {!lostStages.length && <p className="warn-text">There is no “lost” stage in your pipeline. Add one in Settings → Pipeline.</p>}
        {lostStages.length > 1 && <Field label="Stage" group><Select ariaLabel="Stage" value={String(stage)} onChange={v => setStage(Number(v))} options={lostStages.map(s => ({ value: String(s.id), label: s.name }))} /></Field>}
        <Field label="Why" help="It helps later: Reports shows why deals are lost." group>
          <div className="chips">
            {(settings?.lost_reasons || []).map(r => <button key={r} type="button" className={`chip ${reason === r ? 'on' : ''}`} onClick={() => setReason(reason === r ? '' : r)}>{r}</button>)}
          </div>
          <input value={reason} onChange={e => setReason(e.target.value)} placeholder="Or in your words" aria-label="Why" />
        </Field>
        <Field label="Anything else" help="Optional."><textarea rows={3} value={note} onChange={e => setNote(e.target.value)} placeholder="They might come back next year." /></Field>
      </div>
    </Modal>
  )
}

// ---- won ----

type Step = { target: string; made?: Made; error?: string }

export function WonSheet({ deal, onClose, onDone }: { deal: Deal; onClose: () => void; onDone: (d: Deal) => void }) {
  const { settings } = useApp()
  const { stages } = useData()
  const appState = useAppState()
  const { toast } = useToast()
  const wonStages = (stages || []).filter(s => s.kind === 'won')
  const projects = appState('projects', 'projects:write')
  const invoicing = appState('invoicing', 'invoices:write')
  const tasks = appState('tasks', 'tasks:write')
  const contacts = appState('contacts', 'contacts:write')
  const linked = !!(deal.contact_ref || deal.company_ref)
  const [f, setF] = useState({
    stage_id: wonStages[0]?.id || 0, note: '', name: '', team_id: settings?.project_team || 0,
    project: !!settings?.won_project && projects.ok, invoice: !!settings?.won_invoice && invoicing.ok && deal.value > 0,
    task: !!settings?.won_task && tasks.ok, contact: !linked && contacts.ok,
  })
  const teams = useOptions('teams', projects.ok)
  const [busy, setBusy] = useState(false)
  const [steps, setSteps] = useState<Step[] | null>(null)
  const [result, setResult] = useState<Deal | null>(null)
  const go = async () => {
    setBusy(true)
    try {
      const r = await api.post<{ deal: Deal; steps: Step[] }>(`${V1}/deals/${deal.id}/won`, f)
      announceChange()
      setResult(r.deal)
      if (r.steps.length) setSteps(r.steps)
      else { toast('Won 🎉'); onDone(r.deal) }
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  const row = (key: 'contact' | 'project' | 'invoice' | 'task', slug: 'contacts' | 'projects' | 'invoicing' | 'tasks', ok: boolean, label: string, help: string, extra?: React.ReactNode) => (
    <div className={`won-step ${ok ? '' : 'off'}`}>
      <Switch checked={ok && f[key]} onChange={v => setF({ ...f, [key]: v })} label={label} disabled={!ok} />
      <div className="grow">
        <div className="won-step-h"><Icon name={APP_INFO[slug].icon} size={14} />{label}</div>
        {ok ? <div className="muted small">{help}</div> : <div className="won-off" title={offTitle(appState(slug).app, slug)}>{key === 'invoice' && deal.value <= 0 && invoicing.ok ? help : `${STATE_WORDS[appState(slug).app?.state || 'error']}${appState(slug).app?.fix ? ' — ' + appState(slug).app!.fix : ''}`}</div>}
        {ok && f[key] && extra}
      </div>
    </div>
  )
  if (steps && result) {
    return (
      <Modal title="Won 🎉" onClose={() => onDone(result)} size="sm" footer={<button type="button" className="btn primary" onClick={() => onDone(result)}>Done</button>}>
        <div className="stack">
          <p><b>{deal.title}</b> is won{deal.value ? <> — {fmtMoney(deal.value, deal.currency)}</> : null}.</p>
          {steps.map(s => {
            const slug = TARGET_APP[s.target]
            return (
              <div key={s.target} className={`made-step ${s.error ? 'bad' : 'ok'}`}>
                <Icon name={s.error ? 'alert' : 'check-circle'} size={15} />
                <div className="grow">
                  <b>{APP_INFO[slug]?.name}</b>
                  <div className="small">{s.error ? s.error : s.made?.url ? <a href={s.made.url} target="_blank" rel="noreferrer">{s.made.title || 'Open'}</a> : s.made?.title}</div>
                </div>
              </div>
            )
          })}
          {steps.some(s => s.error) && <p className="muted small">The deal is won either way. You can make what failed later from the deal’s page.</p>}
        </div>
      </Modal>
    )
  }
  return (
    <Modal title="Won 🎉" onClose={onClose} footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={go} disabled={busy || !f.stage_id}>{busy ? <Spinner /> : <Icon name="trophy" size={15} />}Mark as won</button>
    </>}>
      <div className="stack">
        {!wonStages.length && <p className="warn-text">There is no “won” stage in your pipeline. Add one in Settings → Pipeline.</p>}
        <Field label="What was agreed" help="Optional: the price, the start, anything to remember. It goes into the project too."><textarea rows={3} value={f.note} onChange={e => setF({ ...f, note: e.target.value })} autoFocus /></Field>
        <h4 className="form-sec-h">Take it on</h4>
        {!projects.ok && !invoicing.ok && !tasks.ok && (linked || !contacts.ok) && <p className="muted small">None of the apps that take a won deal on are connected. The deal is won either way; connect them in <a href="/settings/apps">Connected apps</a>.</p>}
        {!linked && row('contact', 'contacts', contacts.ok, 'Add them to Pocket Contacts', who(deal) ? `${who(deal)} — or link the card they already have.` : 'The person and the company.')}
        {row('project', 'projects', projects.ok, 'Start the project', 'In Pocket Projects, with what was agreed as its description.', <div className="two-col tight mt">
          <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} placeholder={deal.company && !deal.title.includes(deal.company) ? `${deal.company} — ${deal.title}` : deal.title} /></Field>
          <Field label="Team" group help={teams.err}><Select ariaLabel="Team" value={String(f.team_id || '')} onChange={v => setF({ ...f, team_id: Number(v) })} placeholder={teams.list ? 'Projects’ default' : 'Loading…'} clearLabel="Projects’ default" options={(teams.list || []).map(t => ({ value: String(t.id), label: t.name, hint: t.key }))} /></Field>
        </div>)}
        {row('invoice', 'invoicing', invoicing.ok && deal.value > 0, 'Make a draft invoice', deal.value > 0 ? `In Pocket Invoicing for ${fmtMoney(deal.value, deal.currency)}, for you to check and send.` : 'The deal has no value yet.')}
        {row('task', 'tasks', tasks.ok, 'A task to kick it off', `“Kick off ${deal.title}” in Pocket Tasks, due in ${settings?.action_days || 2} days.`)}
      </div>
    </Modal>
  )
}

// ---- in another app ----

const TITLES: Record<string, string> = { task: 'Make a task', event: 'Plan a meeting', note: 'Save as a note', email: 'Send an email', signing: 'Send to sign', project: 'Start a project', invoice: 'Make a draft invoice', contact: 'Link to Pocket Contacts' }

export const MAKE_SCOPE: Record<string, string> = { contact: 'contacts:read', task: 'tasks:write', event: 'calendar:write', note: 'notes:write', email: 'mail:send', signing: 'signing:write', project: 'projects:write', invoice: 'invoices:write' }

export function MakeSheet({ deal, target, onClose, onDone }: { deal: Deal; target: string; onClose: () => void; onDone: (m?: Made) => void }) {
  const { settings } = useApp()
  const appState = useAppState()
  const { toast } = useToast()
  const ai = useAI()
  const slug = TARGET_APP[target]
  const st = appState(slug, MAKE_SCOPE[target])
  const [f, setF] = useState({
    title: target === 'task' ? (deal.next_action || `Follow up with ${deal.contact_name || deal.company || 'them'}`) : target === 'event' ? `Meeting with ${who(deal) || deal.title}` : target === 'note' ? deal.title : '',
    notes: '', due: deal.next_due || addDays(todayISO(), settings?.action_days || 2), date: addDays(todayISO(), 1), time: '10:00', minutes: settings?.meeting_minutes || 60, location: '',
    to: deal.contact_email, subject: deal.title, body: '', template_id: 0, send: true, timeline: true, team_id: settings?.project_team || 0,
  })
  const templates = useOptions('templates', target === 'signing' && st.ok)
  const teams = useOptions('teams', target === 'project' && st.ok)
  const [busy, setBusy] = useState(false)
  const [drafting, setDrafting] = useState(false)
  const draft = async () => {
    setDrafting(true)
    try {
      const r = await api.post<{ subject: string; body: string }>(`${V1}/deals/${deal.id}/ai`, { want: 'email', hint: f.body })
      setF(x => ({ ...x, subject: r.subject || x.subject, body: r.body }))
    } catch (e) { toast(errMsg(e), 'err') } finally { setDrafting(false) }
  }
  const go = async (e?: React.FormEvent) => {
    e?.preventDefault()
    setBusy(true)
    try {
      const m = await api.post<Made>(`${V1}/deals/${deal.id}/make`, { target, ...f, time: target === 'event' ? f.time : '' })
      announceChange()
      toast(target === 'email' ? 'Sent' : m.title ? `Done: ${m.title}` : 'Done')
      onDone(m)
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  // without the app: a calendar file, your own email app
  const icsURL = `${V1}/deals/${deal.id}/meeting.ics?` + new URLSearchParams({ title: f.title, date: f.date, time: f.time, minutes: String(f.minutes), location: f.location, notes: f.notes, log: '1' }).toString()
  const mailto = `mailto:${encodeURIComponent(f.to)}?subject=${encodeURIComponent(f.subject)}&body=${encodeURIComponent(f.body)}`
  const logEmail = async () => {
    await api.post(`${V1}/deals/${deal.id}/activities`, { kind: 'email', title: `Email: ${f.subject}`, body: f.body }).catch(() => {})
    announceChange()
    onDone()
  }
  const fallback = !st.ok && (target === 'event' || target === 'email')
  const canGo = st.ok && !busy && !(target === 'signing' && !f.template_id) && !(target === 'email' && (!f.to || !f.body)) && !(target === 'event' && !f.date)
  return (
    <Modal title={TITLES[target]} onClose={onClose} size={target === 'email' ? '' : 'sm'} footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      {fallback && target === 'event' && <a className="btn primary" href={icsURL} download onClick={() => setTimeout(() => { announceChange(); onDone() }, 300)}><Icon name="download" size={15} />Download for your calendar</a>}
      {fallback && target === 'email' && <a className="btn primary" href={mailto} onClick={() => setTimeout(logEmail, 300)}><Icon name="external" size={15} />Open in your email app</a>}
      {!fallback && <button type="button" className="btn primary" onClick={() => go()} disabled={!canGo}>
        {busy ? <Spinner /> : <Icon name={APP_INFO[slug].icon} size={15} />}{target === 'email' ? 'Send' : target === 'signing' ? (f.send ? 'Send it' : 'Make the document') : 'Make it'}
      </button>}
    </>}>
      <form className="stack" onSubmit={go}>
        {!st.ok && <AppOff slug={slug} app={st.app}>{fallback ? (target === 'event' ? 'You can still download the meeting for any calendar app; it goes on the timeline too.' : 'You can still write it here and send it from your own email app; it goes on the timeline too.') : 'The deal does not need it: everything stays here.'}</AppOff>}
        {(target === 'task' || target === 'event' || target === 'note' || target === 'invoice') && <Field label={target === 'invoice' ? 'Line on the invoice' : 'Title'}><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} placeholder={target === 'invoice' ? deal.title : ''} autoFocus /></Field>}
        {target === 'task' && <Field label="Due" help="Empty: no due day."><input type="date" value={f.due} onChange={e => setF({ ...f, due: e.target.value })} /></Field>}
        {target === 'event' && <>
          <div className="three-col tight">
            <Field label="Day"><input type="date" value={f.date} onChange={e => setF({ ...f, date: e.target.value })} required /></Field>
            <Field label="Time" help="Empty: all day."><input type="time" value={f.time} onChange={e => setF({ ...f, time: e.target.value })} /></Field>
            <Field label="Minutes"><input type="number" min={5} max={600} step={5} value={f.minutes} onChange={e => setF({ ...f, minutes: Number(e.target.value) || 60 })} /></Field>
          </div>
          <Field label="Where"><input value={f.location} onChange={e => setF({ ...f, location: e.target.value })} placeholder="Their office, or a video link" /></Field>
        </>}
        {target === 'note' && <Switch checked={f.timeline} onChange={v => setF({ ...f, timeline: v })} label="With the timeline" />}
        {target === 'email' && <>
          <Field label="To"><input type="email" value={f.to} onChange={e => setF({ ...f, to: e.target.value })} /></Field>
          <Field label="Subject"><input value={f.subject} onChange={e => setF({ ...f, subject: e.target.value })} /></Field>
          <Field label="Message" group help={ai ? 'Write what it should say and let Pocket AI word it, or write it all yourself.' : undefined}>
            <textarea rows={9} value={f.body} onChange={e => setF({ ...f, body: e.target.value })} placeholder={`Hi ${deal.contact_name.split(' ')[0] || ''},`} autoFocus />
            {ai && <div className="row"><button type="button" className="btn sm" onClick={draft} disabled={drafting}>{drafting ? <Spinner /> : <Icon name="sparkles" size={14} />}{f.body ? 'Word it for me' : 'Write a follow-up'}</button></div>}
          </Field>
        </>}
        {target === 'signing' && st.ok && <>
          <Field label="Template" group help={templates.err || `The first person to sign is ${deal.contact_name || deal.contact_email}.`}>
            <Select ariaLabel="Template" value={String(f.template_id || '')} onChange={v => setF({ ...f, template_id: Number(v) })} placeholder={templates.list ? (templates.list.length ? 'Pick a template' : 'No templates in Pocket Sign yet') : 'Loading…'} options={(templates.list || []).map(t => ({ value: String(t.id), label: t.name }))} />
          </Field>
          <Field label="Title" help="Empty: the template’s."><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} /></Field>
          <Field label="Message" help="Optional, in the email they get."><textarea rows={3} value={f.notes} onChange={e => setF({ ...f, notes: e.target.value })} /></Field>
          <Switch checked={f.send} onChange={v => setF({ ...f, send: v })} label="Send it now (off: make it and send it from Pocket Sign)" />
        </>}
        {target === 'project' && st.ok && <>
          <Field label="Name"><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} placeholder={deal.title} autoFocus /></Field>
          <Field label="Team" group help={teams.err}><Select ariaLabel="Team" value={String(f.team_id || '')} onChange={v => setF({ ...f, team_id: Number(v) })} placeholder={teams.list ? 'Projects’ default' : 'Loading…'} clearLabel="Projects’ default" options={(teams.list || []).map(t => ({ value: String(t.id), label: t.name, hint: t.key }))} /></Field>
        </>}
        {target === 'invoice' && <p className="muted small">One line for {fmtMoney(deal.value, deal.currency)} to {deal.company || deal.contact_name || 'them'} (added as a client when they are new). It stays a draft until you send it from Pocket Invoicing.</p>}
        {(target === 'task' || target === 'event' || target === 'project') && <Field label="Notes" help="Optional. A link back to the deal is added."><textarea rows={2} value={f.notes} onChange={e => setF({ ...f, notes: e.target.value })} /></Field>}
      </form>
    </Modal>
  )
}
