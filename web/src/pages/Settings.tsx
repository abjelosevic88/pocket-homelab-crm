import { AskButton } from '../components/AskButton'
import React, { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import type { APIToken, AppSlug, AppState, ConnMode, Settings as SettingsT, Stage, User } from '../lib/types'
import { DATE_FORMATS, TIME_FORMATS, fmtBytes, fmtDateTime, type DateFormat, type TimeFormat } from '../lib/format'
import { Avatar, Confirm, CoreHint, Field, Loading, Modal, Segmented, SettingRow, Switch, Tabs, useAsync, useIsMobile, useToast } from '../components/ui'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { ShortcutsSheet } from '../components/ShortcutsSheet'
import { PocketModelSelect } from '../components/PocketModelSelect'
import { ImportModal } from '../components/ImportModal'
import { APP_INFO, CURRENCIES, STAGE_COLORS, STATE_WORDS, stageColor } from '../lib/crm'
import { announceChange } from '../lib/data'

type TabId = 'general' | 'pipeline' | 'notifications' | 'apps' | 'ai' | 'profile' | 'people' | 'tokens' | 'data' | 'about'

const TABS: { id: TabId; label: string; icon: string; sub: string }[] = [
  { id: 'general', label: 'General', icon: 'home', sub: 'Dates, times, your currency and the app title.' },
  { id: 'pipeline', label: 'Pipeline', icon: 'layers', sub: 'The stages a deal goes through, when a deal goes quiet, the reasons deals are lost, and what happens when one is won.' },
  { id: 'notifications', label: 'Notifications', icon: 'bell', sub: 'A morning note with the next steps due and the deals gone quiet.' },
  { id: 'apps', label: 'Connected apps', icon: 'link', sub: 'Your contacts, mail, calendar, tasks, notes, documents to sign, projects, invoices and forms — what each deal can reach.' },
  { id: 'ai', label: 'AI', icon: 'sparkles', sub: 'Who helps you: Pocket AI through Core, or your own OpenAI key.' },
  { id: 'profile', label: 'Profile', icon: 'user', sub: 'Your name, email and password.' },
  { id: 'people', label: 'People', icon: 'users', sub: 'Everyone who has an account on this box. Each person has their own deals.' },
  { id: 'tokens', label: 'API & shortcuts', icon: 'key', sub: 'Personal tokens for Siri Shortcuts, scripts and other tools.' },
  { id: 'data', label: 'Backup & data', icon: 'database', sub: 'Bring deals from a spreadsheet, Pipedrive or HubSpot, export everything, download the database.' },
  { id: 'about', label: 'About', icon: 'info', sub: 'Version, numbers and the server.' },
]

export default function Settings() {
  const { tab: raw } = useParams()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const tab: TabId = TABS.find(t => t.id === raw)?.id || 'general'
  const current = TABS.find(t => t.id === tab)!
  const go = (id: TabId) => navigate(`/settings/${id}`, { replace: true })
  useEffect(() => { document.querySelector('.set-content')?.scrollTo({ top: 0 }) }, [tab])
  useEffect(() => {
    const h = (e: KeyboardEvent) => { if (e.key === 'Escape' && !document.querySelector('.modal-backdrop, .menu, .sel-pop')) navigate('/') }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [navigate])
  return (
    <div className="set-page">
      <header className="set-header">
        <button type="button" className="btn ghost icon-only" aria-label="Back" title="Back (esc)" onClick={() => navigate('/')}><Icon name="chevron-left" size={18} /></button>
        <img src="/favicon.svg" alt="" width={24} height={24} className="set-logo" />
        <h1>Settings</h1>
        <span style={{ flex: 1 }} />
        <AskButton />
      </header>
      {mobile && <div className="set-tabs-mobile"><Tabs tabs={TABS.map(t => ({ id: t.id, label: t.label, icon: t.icon }))} value={tab} onChange={go} /></div>}
      <div className="set-body">
        {!mobile && (
          <nav className="set-nav" aria-label="Settings sections">
            {TABS.map(t => (
              <button key={t.id} type="button" className={`set-nav-item ${t.id === tab ? 'active' : ''}`} aria-current={t.id === tab ? 'page' : undefined} onClick={() => go(t.id)}>
                <Icon name={t.icon} size={16} />{t.label}
              </button>
            ))}
          </nav>
        )}
        <main className="set-content">
          <div className="set-inner" key={tab}>
            <div className="set-title"><h2>{current.label}</h2><p>{current.sub}</p></div>
            {tab === 'general' && <General />}
            {tab === 'pipeline' && <PipelineTab />}
            {tab === 'notifications' && <Notifications />}
            {tab === 'apps' && <Integrations />}
            {tab === 'ai' && <AITab />}
            {tab === 'profile' && <Profile />}
            {tab === 'people' && <Users />}
            {tab === 'tokens' && <Tokens />}
            {tab === 'data' && <Data />}
            {tab === 'about' && <About />}
          </div>
        </main>
      </div>
    </div>
  )
}

function Section({ title, sub, icon, actions, children }: { title: React.ReactNode; sub?: React.ReactNode; icon?: string; actions?: React.ReactNode; children?: React.ReactNode }) {
  return (
    <section className="set-section">
      <div className="set-section-head">
        {icon && <span className="set-app-ico"><Icon name={icon} size={17} /></span>}
        <div className="grow"><h3>{title}</h3>{sub && <p>{sub}</p>}</div>
        {actions && <div className="row">{actions}</div>}
      </div>
      {children && <div className="set-section-body">{children}</div>}
    </section>
  )
}

function useSettingsForm() {
  const { settings, setSettings, user } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState<SettingsT | null>(settings)
  const [busy, setBusy] = useState(false)
  useEffect(() => { if (!f && settings) setF(settings) }, [settings]) // eslint-disable-line react-hooks/exhaustive-deps
  const save = async (patch?: Partial<SettingsT>) => {
    if (!f) return
    setBusy(true)
    try { const s = await api.put<SettingsT>(`${V1}/settings`, { ...f, ...patch }); setSettings(s); setF(s); toast('Saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return { f, setF, save, busy, isAdmin: user?.role === 'admin' }
}

function useTester() {
  const [test, setTest] = useState<Record<string, string>>({})
  const run = async (which: string, url: string, key: string, extra: Record<string, string> = {}) => {
    setTest(t => ({ ...t, [which]: 'testing…' }))
    try { const r = await api.post<{ ok: boolean; message?: string; error?: string }>(`${V1}/integrations/test`, { which, url, key, ...extra }); setTest(t => ({ ...t, [which]: r.ok ? `✓ ${r.message}` : `✕ ${r.error}` })) } catch (e) { setTest(t => ({ ...t, [which]: `✕ ${(e as Error).message}` })) }
  }
  const status = (which: string) => <span className={`small ${test[which]?.startsWith('✓') ? 'ok' : test[which]?.startsWith('✕') ? 'bad' : 'muted'}`}>{test[which]}</span>
  return { run, status }
}

const HOURS = Array.from({ length: 24 }, (_, h) => ({ value: String(h), label: `${String(h).padStart(2, '0')}:00` }))

function General() {
  const { f, setF, save, busy, isAdmin } = useSettingsForm()
  if (!f) return <Loading />
  return (
    <div className="stack loose">
      <Section title="Dates" icon="calendar">
        <SettingRow title="Week starts on"><Segmented size="sm" value={f.week_start} onChange={v => setF({ ...f, week_start: v })} options={[{ value: 1, label: 'Monday' }, { value: 6, label: 'Saturday' }, { value: 7, label: 'Sunday' }]} /></SettingRow>
        <SettingRow title="Date format"><Select ariaLabel="Date format" value={f.date_format} onChange={v => setF({ ...f, date_format: v as DateFormat })} options={DATE_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        <SettingRow title="Time format"><Select ariaLabel="Time format" value={f.time_format} onChange={v => setF({ ...f, time_format: v as TimeFormat })} options={TIME_FORMATS.map(d => ({ value: d.id, label: d.label }))} /></SettingRow>
        {isAdmin && <SettingRow title="App title" help="Shown in the sidebar for everyone on this box."><input value={f.title} onChange={e => setF({ ...f, title: e.target.value })} /></SettingRow>}
      </Section>
      <Section title="Money" icon="wallet">
        <SettingRow title="Your currency" help="New deals start in it; totals and reports count in it. A deal can be in another currency."><Select ariaLabel="Currency" value={f.currency} onChange={v => setF({ ...f, currency: v })} options={Array.from(new Set([f.currency, ...CURRENCIES])).map(c => ({ value: c, label: c }))} /></SettingRow>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function PipelineTab() {
  const { f, setF, save, busy } = useSettingsForm()
  const { data: teams } = useAsync(() => api.get<{ id: number; name: string; key: string }[]>(`${V1}/apps/options/teams`).catch(() => []), [])
  const [reason, setReason] = useState('')
  if (!f) return <Loading />
  const projectsOn = f.projects_mode !== 'off' && f.projects_via !== ''
  const mailOn = f.mail_mode !== 'off' && f.mail_via !== ''
  const tasksOn = f.tasks_mode !== 'off' && f.tasks_via !== ''
  const addReason = () => { const r = reason.trim(); if (r && !f.lost_reasons.includes(r)) setF({ ...f, lost_reasons: [...f.lost_reasons, r] }); setReason('') }
  return (
    <div className="stack loose">
      <StagesEditor />
      <Section title="Deals" icon="target">
        <SettingRow title="A new next step is due after" help="Days; offered when you set a step."><input type="number" className="num-sm" min={0} max={365} value={f.action_days} onChange={e => setF({ ...f, action_days: Math.max(0, Number(e.target.value) || 0) })} /></SettingRow>
        <SettingRow title="A deal goes quiet after" help="Days with nothing happening and no next step; Today lists it. 0 = never."><input type="number" className="num-sm" min={0} max={365} value={f.stale_days} onChange={e => setF({ ...f, stale_days: Math.max(0, Number(e.target.value) || 0) })} /></SettingRow>
        <SettingRow title="Next steps also in Pocket Tasks" help={tasksOn ? 'Each next step becomes a task there; ticking it off there ticks it here.' : 'Connect Pocket Tasks first (Connected apps).'}><Switch checked={f.task_mirror} onChange={v => setF({ ...f, task_mirror: v })} label="Next steps in Pocket Tasks" disabled={!tasksOn} /></SettingRow>
        <SettingRow title="A meeting takes" help="Minutes, when you plan one."><input type="number" className="num-sm" min={5} max={600} step={5} value={f.meeting_minutes} onChange={e => setF({ ...f, meeting_minutes: Number(e.target.value) || 60 })} /></SettingRow>
      </Section>
      <Section title="Why deals are lost" icon="x-circle" sub="Offered when you mark a deal lost; Reports counts them.">
        <div className="chips">{f.lost_reasons.map(r => <span key={r} className="chip">{r}<button type="button" className="chip-x" aria-label={`Remove ${r}`} onClick={() => setF({ ...f, lost_reasons: f.lost_reasons.filter(x => x !== r) })}><Icon name="x" size={11} /></button></span>)}</div>
        <div className="row tight"><input value={reason} onChange={e => setReason(e.target.value)} onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); addReason() } }} placeholder="Add a reason" aria-label="Add a reason" /><button type="button" className="btn" onClick={addReason} disabled={!reason.trim()}>Add</button></div>
      </Section>
      <Section title="When a deal is won" icon="trophy" sub="“Won” offers these, ticked. Each one only when its app is connected.">
        <SettingRow title="Start the project" help={projectsOn ? 'In Pocket Projects.' : 'Connect Pocket Projects first.'}><Switch checked={f.won_project} onChange={v => setF({ ...f, won_project: v })} label="Start the project" disabled={!projectsOn} /></SettingRow>
        <SettingRow title="Projects team" help={projectsOn ? 'Empty: your default team in Pocket Projects.' : 'Connect Pocket Projects first.'}>
          <Select ariaLabel="Team" value={String(f.project_team || '')} onChange={v => setF({ ...f, project_team: Number(v) })} placeholder="Projects’ default" clearLabel="Projects’ default" disabled={!projectsOn} options={(teams || []).map(t => ({ value: String(t.id), label: t.name, hint: t.key }))} />
        </SettingRow>
        <SettingRow title="Make a draft invoice" help={f.invoicing_via ? 'In Pocket Invoicing, for the deal’s value.' : 'Connect Pocket Invoicing first.'}><Switch checked={f.won_invoice} onChange={v => setF({ ...f, won_invoice: v })} label="Draft invoice" disabled={!f.invoicing_via || f.invoicing_mode === 'off'} /></SettingRow>
        <SettingRow title="A task to kick it off" help={tasksOn ? 'In Pocket Tasks.' : 'Connect Pocket Tasks first.'}><Switch checked={f.won_task} onChange={v => setF({ ...f, won_task: v })} label="Kick-off task" disabled={!tasksOn} /></SettingRow>
      </Section>
      <Section title="Emails" icon="mail" sub="Follow-ups leave from your own mailbox through Pocket Mail and land in your Sent folder.">
        <SettingRow title="Send from" help={mailOn ? 'The address of one of your Pocket Mail accounts; empty: its default.' : 'Connect Pocket Mail first; without it, emails open in your own email app.'}><input value={f.mail_from} onChange={e => setF({ ...f, mail_from: e.target.value })} placeholder="you@example.com" disabled={!mailOn} /></SettingRow>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

/** The stages, in order: rename, recolour, set the chance, add, remove. */
function StagesEditor() {
  const { toast } = useToast()
  const { data: stages, reload } = useAsync(() => api.get<Stage[]>(`${V1}/stages`), [])
  const [del, setDel] = useState<Stage | null>(null)
  const [moveTo, setMoveTo] = useState('')
  if (!stages) return <Loading />
  const put = async (s: Stage, patch: Partial<Stage>) => { try { await api.put(`${V1}/stages/${s.id}`, patch); reload(); announceChange() } catch (e) { toast((e as Error).message, 'err') } }
  const add = async () => { try { await api.post(`${V1}/stages`, { name: 'New stage', probability: 50 }); reload(); announceChange() } catch (e) { toast((e as Error).message, 'err') } }
  const order = async (ids: number[]) => { await api.post(`${V1}/stages/order`, { ids }).catch(() => {}); reload(); announceChange() }
  const shift = (i: number, by: number) => { const ids = stages.map(s => s.id); const j = i + by; if (j < 0 || j >= ids.length) return; [ids[i], ids[j]] = [ids[j], ids[i]]; order(ids) }
  const remove = async () => {
    if (!del) return
    try { await api.del(`${V1}/stages/${del.id}${moveTo ? `?move_to=${moveTo}` : ''}`); setDel(null); setMoveTo(''); reload(); announceChange() } catch (e) { toast((e as Error).message, 'err') }
  }
  return (
    <Section title="Stages" icon="layers" sub="The board has one column per open stage. The chance is how likely a deal there is won; “Likely” totals use it." actions={<button type="button" className="btn sm" onClick={add}><Icon name="plus" size={14} />Add a stage</button>}>
      <div className="stage-list">
        {stages.map((s, i) => (
          <div key={s.id} className="stage-row">
            <div className="stage-move">
              <button type="button" className="btn ghost icon-only sm" onClick={() => shift(i, -1)} disabled={i === 0} aria-label="Up"><Icon name="chevron-up" size={14} /></button>
              <button type="button" className="btn ghost icon-only sm" onClick={() => shift(i, 1)} disabled={i === stages.length - 1} aria-label="Down"><Icon name="chevron-down" size={14} /></button>
            </div>
            <span className="stage-swatches">{STAGE_COLORS.map(c => <button key={c} type="button" className={`swatch sm ${stageColor(s) === c ? 'on' : ''}`} style={{ background: c }} aria-label={`Colour ${c}`} onClick={() => put(s, { color: c })} />)}</span>
            <input className="grow" defaultValue={s.name} aria-label="Name" onBlur={e => { const v = e.target.value.trim(); if (v && v !== s.name) put(s, { name: v }) }} />
            <Select className="stage-kind" ariaLabel="Kind" value={s.kind} onChange={v => put(s, { kind: v as Stage['kind'] })} options={[{ value: 'open', label: 'Open' }, { value: 'won', label: 'Won' }, { value: 'lost', label: 'Lost' }]} />
            {s.kind === 'open' ? <label className="stage-prob"><input type="number" min={0} max={100} defaultValue={s.probability} aria-label="Chance" onBlur={e => { const v = Math.min(100, Math.max(0, Number(e.target.value) || 0)); if (v !== s.probability) put(s, { probability: v }) }} />%</label> : <span className="stage-prob muted">{s.probability}%</span>}
            <span className="muted small tnum stage-n" title="Deals in this stage">{s.deals}</span>
            <button type="button" className="btn ghost icon-only sm" onClick={() => setDel(s)} aria-label={`Remove ${s.name}`} disabled={stages.length < 2}><Icon name="trash" size={14} /></button>
          </div>
        ))}
      </div>
      {del && <Modal title={`Remove “${del.name}”?`} onClose={() => setDel(null)} size="sm" footer={<>
        <button type="button" className="btn" onClick={() => setDel(null)}>Cancel</button>
        <button type="button" className="btn danger" onClick={remove} disabled={del.deals > 0 && !moveTo}>Remove</button>
      </>}>
        {del.deals > 0 ? <Field label={`Its ${del.deals} ${del.deals === 1 ? 'deal goes' : 'deals go'} to`} group>
          <Select ariaLabel="Move to" value={moveTo} onChange={setMoveTo} placeholder="Pick a stage" options={stages.filter(x => x.id !== del.id).map(x => ({ value: String(x.id), label: x.name }))} />
        </Field> : <p className="muted">No deals are in it.</p>}
      </Modal>}
    </Section>
  )
}

function Notifications() {
  const { f, setF, save, busy } = useSettingsForm()
  const { toast } = useToast()
  const { run, status } = useTester()
  if (!f) return <Loading />
  const now = async () => { try { const r = await api.post<{ sent: boolean; body: string }>(`${V1}/notify/check`); toast(r.sent ? 'Sent' : r.body ? `Nothing sent (no push URL): ${r.body.split('\n')[0]}` : 'Nothing to tell: no steps due and no quiet deals') } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <div className="stack loose">
      <Section title="Push" icon="bell" actions={<span className={`badge ${f.notify_configured ? 'ok' : ''}`}>{f.notify_configured ? 'on' : 'not set'}</span>}
        sub={<>Where messages go: an Apprise gateway (<code>…/notify/&lt;key&gt;</code>), an ntfy topic URL, or any webhook that takes JSON.</>}>
        <SettingRow title="URL" stack><input value={f.notify_url} onChange={e => setF({ ...f, notify_url: e.target.value })} placeholder="http://apprise:8000/notify/crm" /></SettingRow>
        <SettingRow title="Format"><Segmented value={f.notify_format} onChange={v => setF({ ...f, notify_format: v })} options={[{ value: 'apprise', label: 'Apprise' }, { value: 'ntfy', label: 'ntfy' }, { value: 'json', label: 'JSON' }]} size="sm" /></SettingRow>
      </Section>
      <Section title="What to tell you" icon="clock">
        <SettingRow title="Morning note" help="The next steps due today, how many are overdue, and the deals gone quiet. Nothing is sent on a day with nothing to do."><Switch checked={f.notify_digest} onChange={v => setF({ ...f, notify_digest: v })} label="Morning note" /></SettingRow>
        <SettingRow title="Morning message at" help="Server time."><Select ariaLabel="Hour" value={String(f.notify_hour)} onChange={v => setF({ ...f, notify_hour: Number(v) })} options={HOURS} /></SettingRow>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        <button type="button" className="btn" onClick={() => run('notify', f.notify_url, '', { format: f.notify_format })} disabled={!f.notify_url}><Icon name="mail" size={15} />Send a test</button>
        <button type="button" className="btn" onClick={now}><Icon name="refresh" size={15} />Check now</button>
        {status('notify')}
      </div>
    </div>
  )
}

const APPS: { slug: AppSlug; title: string; sub: string; token: string; port: string }[] = [
  { slug: 'contacts', title: 'Pocket Contacts — people and companies', sub: 'Pick who a deal is with from your contacts, add new people and companies there, open their card from the deal. Without it, the deal keeps their name, email and phone itself.', token: 'Pocket Contacts → Settings → API tokens', port: '8191' },
  { slug: 'mail', title: 'Pocket Mail — emails with them', sub: 'The emails you exchanged with a deal’s person on its page, and follow-ups from your own mailbox. Without it, emails open in your own email app.', token: 'Pocket Mail → Settings → API tokens', port: '8208' },
  { slug: 'calendar', title: 'Pocket Calendar — meetings', sub: 'Plan meetings about a deal, and see the meetings you had with that person. Without it, a meeting downloads as a file any calendar opens.', token: 'Pocket Calendar → Settings → API tokens', port: '8190' },
  { slug: 'tasks', title: 'Pocket Tasks — next steps', sub: 'A task for a step, or every next step mirrored there; ticking it off there ticks it here. Without it, steps live here.', token: 'Pocket Tasks → Settings → API tokens', port: '8184' },
  { slug: 'notes', title: 'Pocket Notes — deals as notes', sub: 'Keep a deal, with its timeline, as a note.', token: 'Pocket Notes → Settings → API tokens', port: '8182' },
  { slug: 'sign', title: 'Pocket Sign — proposals and contracts', sub: 'Send a proposal or a contract from your templates; the deal shows when it is signed.', token: 'Pocket Sign → Settings → API tokens', port: '8234' },
  { slug: 'projects', title: 'Pocket Projects — won deals become projects', sub: 'Start the project when a deal is won, with what was agreed as its description.', token: 'Pocket Projects → Settings → API tokens', port: '8196' },
  { slug: 'invoicing', title: 'Pocket Invoicing — invoices', sub: 'A draft invoice when a deal is won, and a client’s invoices (paid or not) on the deal.', token: 'Pocket Invoicing → Settings → API tokens', port: '8168' },
  { slug: 'forms', title: 'Pocket Forms — leads', sub: 'Answers to your forms (a brief, a quote request) become deals in one click, and show on the person’s deal.', token: 'Pocket Forms → Settings → API tokens', port: '8248' },
]

function Integrations() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  const { data: states } = useAsync(() => api.get<{ apps: AppState[] }>(`${V1}/apps`).then(r => r.apps).catch(() => [] as AppState[]), [f?.contacts_mode, f?.mail_mode, f?.calendar_mode, f?.tasks_mode, f?.notes_mode, f?.sign_mode, f?.projects_mode, f?.invoicing_mode, f?.forms_mode])
  if (!f) return <Loading />
  const coreOn = f.core_configured && f.core_registered !== false
  const eff = (m: ConnMode): ConnMode => !coreOn && m === 'core' ? 'off' : m
  const set = (k: string, v: string) => setF({ ...f, [k]: v } as SettingsT)
  return (
    <div className="stack loose">
      <div className="callout info"><Icon name="info" size={15} /><div>These connections are yours. CRM only adds things in them when you ask, and every deal works without them. Through <b>Pocket Core</b> CRM acts as you in your own Pocket apps with nothing to paste (switch it off for yourself in Pocket Core → Settings → Apps acting for me); <b>Direct</b> uses a URL and an API token you enter.{!coreOn && <CoreHint />}</div></div>
      {APPS.map(a => {
        const mode = f[`${a.slug}_mode`]
        const via = f[`${a.slug}_via`]
        const name = APP_INFO[a.slug].name
        const st = states?.find(x => x.slug === a.slug)
        return (
          <Section key={a.slug} title={a.title} icon={APP_INFO[a.slug].icon} actions={st && st.state !== 'on' && st.state !== 'off' ? <span className={`badge ${st.state === 'partly' ? 'ok' : 'warn'}`}>{STATE_WORDS[st.state]}</span> : connBadge(eff(mode), via)} sub={a.sub}>
            <Connection app={name} mode={mode} via={via} detail={f[`${a.slug}_detail`]} core={coreOn} hasToken={!!f[`${a.slug}_token`]}
              onMode={m => save({ [`${a.slug}_mode`]: m } as Partial<SettingsT>)} onDisconnect={() => save({ [`${a.slug}_url`]: '', [`${a.slug}_token`]: '' } as Partial<SettingsT>)}
              test={<><button type="button" className="btn sm" onClick={() => run(a.slug, f[`${a.slug}_url`], f[`${a.slug}_token`])}>Test connection</button>{status(a.slug)}</>}
              direct={<div className="two-col tight">
                <Field label="URL" help="Inside Docker use http://host.docker.internal:PORT or the service name."><input value={f[`${a.slug}_url`]} onChange={e => set(`${a.slug}_url`, e.target.value)} placeholder={`http://host.docker.internal:${a.port}`} /></Field>
                <Field label="API token" help={a.token}><input value={f[`${a.slug}_token`]} onChange={e => set(`${a.slug}_token`, e.target.value)} autoComplete="off" /></Field>
              </div>} />
            {st?.fix && st.state !== 'on' && <p className="muted small"><Icon name="info" size={12} className="inline-ico" /> {st.fix}.</p>}
          </Section>
        )
      })}
      <Section title="Other Pocket apps" icon="layers" sub="CRM offers your deals to the rest of the box through Pocket Core: Pocket Contacts shows a person’s deals on their page, Pocket Forms and Pocket Mail turn an answer or an email into a lead, Pocket Day writes what happened into your journal, Pocket Search finds deals, Pocket AI tells you what is due and adds leads when you ask, and Pocket Flows can start a flow when a deal is won. Switched off until you allow them in Pocket Core → Links.">
        <dl className="set-dl">
          <div><dt>crm:read</dt><dd className="muted">Your deals, the pipeline, the timelines and the totals.</dd></div>
          <div><dt>crm:write</dt><dd className="muted">Add a lead for you and add a line to a deal’s timeline; that app may change only the deals it added.</dd></div>
          <div><dt>events:read</dt><dd className="muted"><code>crm.deal_created</code>, <code>crm.deal_moved</code>, <code>crm.deal_won</code>, <code>crm.deal_lost</code>, <code>crm.steps_due</code> — for Pocket Flows.</dd></div>
        </dl>
      </Section>
      <div className="row"><button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button></div>
    </div>
  )
}

function AITab() {
  const { f, setF, save, busy } = useSettingsForm()
  const { run, status } = useTester()
  const { data: ai } = useAsync(() => api.get<{ ready: boolean; provider: string; detail: string }>(`${V1}/ai/status`), [])
  if (!f) return <Loading />
  const coreOn = f.core_configured && f.core_registered !== false
  const shown = !coreOn && f.ai_provider === 'pocket' ? 'openai' : (f.ai_provider || 'auto')
  return (
    <div className="stack loose">
      <Section title="What the AI does" icon="sparkles" sub="Three things, each only when you press its button on a deal: a suggested next step, where the deal stands in a few lines, and a follow-up email. Only that deal and its timeline are sent, and nothing is saved or sent until you say so. Questions about all your deals go to Pocket AI through the Ask button at the top.">
        <SettingRow title="Pocket AI buttons" help="Off: the buttons are gone and nothing is sent to an AI."><Switch checked={f.ai_help} onChange={v => save({ ai_help: v })} label="AI buttons" /></SettingRow>
        {ai && (ai.ready ? <p className="muted small row" style={{ gap: 6 }}><Icon name="sparkles" size={13} />{ai.provider === 'pocket' ? 'Pocket AI' : 'Your OpenAI key'} is ready.</p>
          : <div className="callout info"><Icon name="info" size={15} /><div>No AI connected{ai.detail ? ` (${ai.detail})` : ''}.{!coreOn && <CoreHint />}</div></div>)}
      </Section>
      <Section title="Provider" icon="sparkles">
        <div className="grid" style={{ gap: 12 }}>
          <Field group label="Who writes" help="Automatic prefers your enabled Pocket AI connection; without one it uses your OpenAI settings.">
            <Select ariaLabel="AI provider" value={shown} onChange={v => setF({ ...f, ai_provider: v as SettingsT['ai_provider'] })} options={[{ value: 'auto', label: coreOn ? 'Automatic — prefer Pocket AI' : 'Automatic' }, { value: 'pocket', label: 'Pocket AI', disabled: !coreOn, hint: coreOn ? undefined : 'needs Pocket Core' }, { value: 'openai', label: 'OpenAI / compatible API' }]} />
          </Field>
          {!coreOn && <CoreHint />}
          {coreOn && shown !== 'openai' && <PocketModelSelect value={f.ai_pocket_model || ''} onChange={v => setF({ ...f, ai_pocket_model: v })} />}
          {(shown === 'openai' || (!coreOn && shown === 'auto')) && <>
            <Field label="OpenAI API key" help="platform.openai.com → API keys. Stored on the server; shown masked."><input value={f.openai_api_key} onChange={e => setF({ ...f, openai_api_key: e.target.value })} autoComplete="off" placeholder="sk-…" /></Field>
            <div className="two-col tight">
              <Field label="Model" help="Default gpt-4o-mini."><input value={f.openai_model} onChange={e => setF({ ...f, openai_model: e.target.value })} placeholder="gpt-4o-mini" /></Field>
              <Field label="Base URL (optional)" help="Ollama, LM Studio, OpenRouter…"><input value={f.openai_base_url} onChange={e => setF({ ...f, openai_base_url: e.target.value })} placeholder="https://api.openai.com/v1" /></Field>
            </div>
          </>}
        </div>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn primary" onClick={() => save()} disabled={busy}>Save changes</button>
        {(shown === 'openai' || (!coreOn && shown === 'auto')) && <><button type="button" className="btn" onClick={() => run('openai', f.openai_base_url, f.openai_api_key)} disabled={!f.openai_api_key}>Test the OpenAI key</button>{status('openai')}</>}
      </div>
    </div>
  )
}

function Connection({ app, mode: saved, via, detail, core, onMode, onDisconnect, hasToken, test, direct }: {
  app: string; mode: ConnMode; via: string; detail: string; core: boolean; onMode: (m: ConnMode) => void; onDisconnect: () => void
  hasToken: boolean; test: React.ReactNode; direct: React.ReactNode
}) {
  const [advanced, setAdvanced] = useState(saved === 'direct')
  const mode: ConnMode = !core && saved === 'core' ? 'off' : saved
  const ok = via !== '' && mode !== 'off'
  const showDirect = !core || mode === 'direct' || advanced
  const line = mode === 'off' ? `Off: CRM does not reach ${app}.`
    : mode === 'core' ? (ok ? `Connected through Pocket Core, as you${detail ? ` (${detail})` : ''}. Nothing to fill in; switch it off for yourself in Pocket Core → Settings → Apps acting for me.` : `Not connected: ${detail}.`)
    : (ok ? `Direct connection with your own ${app} URL and token; use Test connection to check it.` : `Not connected: ${detail || 'add the URL and a token'}.`)
  return (
    <div className="conn-block">
      <div className="conn-mode">
        <Segmented size="sm" value={mode} onChange={m => onMode(m as ConnMode)} options={[
          { value: 'core', label: 'Pocket Core', title: core ? 'Through Pocket Core: per person, nothing to paste' : 'Needs Pocket Core: there is none on this box', disabled: !core },
          ...(showDirect ? [{ value: 'direct', label: 'Direct', title: 'Your own URL and API token' }] : []),
          { value: 'off', label: 'Off' },
        ]} />
        {!showDirect && <button type="button" className="conn-advanced" onClick={() => setAdvanced(true)}>Advanced: connect directly with a URL and key</button>}
      </div>
      <div className={`conn-status ${mode === 'off' ? '' : ok ? 'ok' : 'bad'}`}><span className="dot" />{line}</div>
      {mode === 'direct' && <>
        {direct}
        <div className="row wrap">{test}{hasToken && <button type="button" className="btn sm ghost danger-text" onClick={onDisconnect} title="Remove the saved URL and token">Disconnect</button>}</div>
      </>}
      {mode === 'core' && <div className="row wrap">{test}</div>}
    </div>
  )
}

const connBadge = (mode: ConnMode, via: string) => mode === 'off' ? <span className="badge">off</span>
  : via ? <span className="badge ok">{via === 'core' ? 'Pocket Core' : 'direct'}</span>
  : <span className="badge warn">not connected</span>

function Users() {
  const { user, users, reloadMeta, status } = useApp()
  const { toast } = useToast()
  const [edit, setEdit] = useState<Partial<User> | null>(null)
  const [del, setDel] = useState<User | null>(null)
  const isAdmin = user?.role === 'admin'
  const run = async (fn: () => Promise<unknown>, msg: string) => { try { await fn(); await reloadMeta(); toast(msg) } catch (e) { toast((e as Error).message, 'err') } }
  return (
    <Section title="People on this box" actions={isAdmin && <button type="button" className="btn primary sm" onClick={() => setEdit({ role: 'member' })}><Icon name="plus" size={14} />Add person</button>}
      sub="Everyone signs in with their own account and has their own forms, answers, settings and connections. Admins manage accounts; they do not see anyone else’s forms or answers.">
      <div className="set-people">
        {users.map(u => (
          <div key={u.id} className="set-person">
            <Avatar name={u.name} email={u.email} size={34} />
            <div className="grow">
              <div className="set-person-name">{u.name || <span className="muted">—</span>}{u.id === user?.id && <span className="badge">you</span>}{u.role === 'admin' && <span className="badge accent">admin</span>}</div>
              <div className="muted small ellipsis">{u.email}</div>
            </div>
            {isAdmin && <span className="row" style={{ justifyContent: 'flex-end' }}><button type="button" className="btn ghost sm" onClick={() => setEdit(u)}>Edit</button>{u.id !== user?.id && <button type="button" className="btn ghost sm danger-text" onClick={() => setDel(u)}>Remove</button>}</span>}
          </div>
        ))}
      </div>
      {status?.auth_mode !== 'local' && <p className="muted small mt">Auth mode is <code>{status?.auth_mode}</code>: users are created automatically on first visit; passwords are not used here.</p>}
      {edit && <UserModal user={edit} local={status?.auth_mode === 'local'} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reloadMeta() }} />}
      {del && <Confirm confirmLabel="Remove" title={`Remove ${del.name || del.email}?`} message="Their account is deleted together with their forms and answers; their form links stop working." onCancel={() => setDel(null)} onConfirm={() => { const d = del; setDel(null); run(() => api.del(`${V1}/users/${d.id}`), 'User removed') }} />}
    </Section>
  )
}

function UserModal({ user, local, onClose, onSaved }: { user: Partial<User>; local: boolean; onClose: () => void; onSaved: () => void }) {
  const { toast } = useToast()
  const [f, setF] = useState({ name: user.name || '', email: user.email || '', password: '', role: user.role || 'member' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { if (user.id) await api.put(`${V1}/users/${user.id}`, f); else await api.post(`${V1}/users`, f); toast(user.id ? 'User saved' : 'User added'); onSaved() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={user.id ? 'Edit person' : 'Add a person'} onClose={onClose} size="sm">
      <form onSubmit={save} className="grid" style={{ gap: 14 }}>
        <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} autoFocus required maxLength={60} /></Field>
        <Field label="Email" help="Used to sign in."><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} required /></Field>
        {local && <Field label={user.id ? 'New password (leave empty to keep)' : 'Password'} help="At least 8 characters."><input type="password" value={f.password} onChange={e => setF({ ...f, password: e.target.value })} minLength={user.id ? 0 : 8} required={!user.id} autoComplete="new-password" /></Field>}
        <Field group label="Role"><Segmented value={f.role} onChange={v => setF({ ...f, role: v })} options={[{ value: 'member', label: 'Member' }, { value: 'admin', label: 'Admin' }]} /></Field>
        <div className="form-actions"><button type="button" className="btn" onClick={onClose}>Cancel</button><button className="btn primary" disabled={busy}>{user.id ? 'Save' : 'Add person'}</button></div>
      </form>
    </Modal>
  )
}

function Profile() {
  const { user, setUser, status } = useApp()
  const { toast } = useToast()
  const [f, setF] = useState({ name: user?.name || '', email: user?.email || '', current_password: '', new_password: '' })
  const [busy, setBusy] = useState(false)
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true)
    try { setUser(await api.put(`${V1}/auth/me`, f)); setF(x => ({ ...x, current_password: '', new_password: '' })); toast('Profile saved') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <form onSubmit={save} className="stack loose">
      <div className="set-profile">
        <Avatar name={user?.name} email={user?.email} size={56} />
        <div className="grow"><h3>{user?.name || user?.email}</h3><p className="muted small">{user?.email} · {user?.role}</p></div>
      </div>
      <Section title="You">
        <div className="stack">
          <Field label="Name"><input value={f.name} onChange={e => setF({ ...f, name: e.target.value })} /></Field>
          <Field label="Email"><input type="email" value={f.email} onChange={e => setF({ ...f, email: e.target.value })} /></Field>
        </div>
      </Section>
      {status?.auth_mode === 'local' && <Section title="Password" sub="Leave empty to keep the current one."><div className="stack">
        <Field label="Current password"><input type="password" value={f.current_password} onChange={e => setF({ ...f, current_password: e.target.value })} autoComplete="current-password" /></Field>
        <Field label="New password" help="At least 8 characters."><input type="password" value={f.new_password} onChange={e => setF({ ...f, new_password: e.target.value })} autoComplete="new-password" /></Field>
      </div></Section>}
      <div className="row"><button className="btn primary" disabled={busy}>Save changes</button></div>
    </form>
  )
}

function Tokens() {
  const { toast } = useToast()
  const { settings } = useApp()
  const { data, reload } = useAsync(() => api.get<APIToken[]>(`${V1}/tokens`), [])
  const [name, setName] = useState('')
  const [fresh, setFresh] = useState('')
  const [del, setDel] = useState<APIToken | null>(null)
  const create = async (e: React.FormEvent) => {
    e.preventDefault()
    try { const r = await api.post<{ token: string }>(`${V1}/tokens`, { name: name || 'token' }); setFresh(r.token); setName(''); reload() } catch (e) { toast((e as Error).message, 'err') }
  }
  const base = settings?.base_url || window.location.origin
  return (
    <div className="stack loose">
      <Section title="Personal tokens" icon="key" sub={<>Use as <code>Authorization: Bearer &lt;token&gt;</code>. A token acts as you: it can read and change everything, feed a Homepage widget (<code>GET /api/v1/summary</code>) or send a recording from a Siri Shortcut. See <code>docs/api.md</code>.</>}>
        <form onSubmit={create} className="row mb"><input value={name} onChange={e => setName(e.target.value)} placeholder="Token name (e.g. iPhone Shortcut)" /><button className="btn primary"><Icon name="plus" size={15} />Create</button></form>
        {fresh && <div className="set-fresh mb"><div className="row"><Icon name="check-circle" size={16} /><b>Copy it now — it is shown once.</b></div><pre className="mono">{fresh}</pre></div>}
        {data && data.length > 0 ? (
          <table className="table compact">
            <thead><tr><th>Name</th><th>Prefix</th><th>Last used</th><th>Created</th><th /></tr></thead>
            <tbody>{data.map(t => <tr key={t.id}><td>{t.name}</td><td className="mono">{t.prefix}…</td><td className="muted">{t.last_used_at ? fmtDateTime(t.last_used_at) : 'never'}</td><td className="muted">{fmtDateTime(t.created_at)}</td><td className="right"><button type="button" className="btn ghost sm danger-text" onClick={() => setDel(t)}>Revoke</button></td></tr>)}</tbody>
          </table>
        ) : <p className="muted">No tokens yet.</p>}
        {del && <Confirm confirmLabel="Revoke" title={`Revoke “${del.name}”?`} message="Anything using this token stops working immediately." onCancel={() => setDel(null)} onConfirm={async () => { await api.del(`${V1}/tokens/${del.id}`); setDel(null); reload() }} />}
      </Section>
      <Section title="Endpoints" icon="code" sub={<>At <code>{base}/api/v1</code>.</>}>
        <dl className="set-dl">
          <div><dt><code>GET /responses?status=new</code></dt><dd className="muted">Your answers, with <code>form</code>, <code>q</code>, <code>email</code>, <code>day</code> and <code>starred</code> filters.</dd></div>
          <div><dt><code>GET /forms</code></dt><dd className="muted">Your forms with their answers, new answers and how many opened them; <code>POST /forms</code> with <code>{'{"template":"contact"}'}</code> makes one.</dd></div>
          <div><dt><code>GET /summary</code></dt><dd className="muted">New answers, forms taking answers and this week — for Homepage.</dd></div>
          <div><dt><code>GET /day/YYYY-MM-DD</code></dt><dd className="muted">The answers that came in that day — what Pocket Day reads.</dd></div>
          <div><dt><code>GET /forms/&lt;id&gt;/export.csv</code></dt><dd className="muted">Every answer of a form as a spreadsheet.</dd></div>
        </dl>
      </Section>
    </div>
  )
}

function Data() {
  const { user } = useApp()
  const [importing, setImporting] = useState(false)
  return (
    <div className="stack loose">
      <Section title="Bring your deals" icon="upload" sub="A CSV from Pipedrive, HubSpot, Zoho, Excel or Google Sheets (one deal per row), or a Pocket CRM export. Bringing the same file twice adds nothing twice.">
        <button type="button" className="btn primary" onClick={() => setImporting(true)}><Icon name="upload" size={15} />Bring deals…</button>
      </Section>
      <Section title="Export" icon="archive" sub="Everything of yours: the stages, every deal with its timeline and what it has in your other apps, as one JSON file (it comes back in with “Bring deals”, also on another box) — or every deal as one table for a spreadsheet.">
        <div className="row wrap"><a className="btn" href={`${V1}/export.json`}><Icon name="download" size={15} />Export JSON</a><a className="btn" href={`${V1}/export.csv`}><Icon name="download" size={15} />Deals as CSV</a></div>
      </Section>
      {user?.role === 'admin' && <Section title="Database backup" icon="database" sub="The whole box, every account — admins only.">
        <p className="muted">A consistent copy of the SQLite database (<code>VACUUM INTO</code>). Restore by stopping the app and replacing <code>pocket-crm.db</code> in the data folder.</p>
        <a className="btn" href={`${V1}/backup`}><Icon name="database" size={15} />Download database</a>
      </Section>}
      {importing && <ImportModal onClose={() => setImporting(false)} />}
    </div>
  )
}

function About() {
  const { data, error } = useAsync(() => api.get<Record<string, unknown>>(`${V1}/settings/system`), [])
  const [keys, setKeys] = useState(false)
  if (error) return <div className="callout danger"><Icon name="alert" size={15} /><span>{error}</span></div>
  if (!data) return <Loading />
  const counts = (data.counts || {}) as Record<string, number>
  const core = (data.core || {}) as { configured?: boolean; registered?: boolean; url?: string; error?: string }
  const tiles: [string, number | undefined, string][] = [['Deals', counts.deals, 'list'], ['Timeline lines', counts.activities, 'activity'], ['In other apps', counts.made, 'link'], ['Stages', counts.stages, 'layers']]
  const rows: [string, React.ReactNode][] = [
    ['Version', <code key="v">{String(data.version)}</code>],
    ['Uptime', String(data.uptime)],
    ['Runtime', `${String(data.go)} · ${String(data.os)}/${String(data.arch)}`],
    ['Time zone', String(data.timezone)],
    ['Sign-in', String(data.auth_mode)],
    ['Pocket Core', core.configured ? (core.registered ? <span key="c" className="ok">connected · {core.url}</span> : <span key="c" className="bad">{core.error || 'not registered yet'}</span>) : <span key="c" className="muted">not configured</span>],
    ['Database', <span key="db"><code>{String(data.db_path)}</code> · {fmtBytes(Number(data.db_bytes))}</span>],
    ['People', `${counts.users ?? 0} accounts on this box`],
  ]
  return (
    <div className="stack loose">
      <div className="set-about-hero">
        <img src="/favicon.svg" alt="" width={48} height={48} />
        <div>
          <h3>Pocket CRM</h3>
          <p className="muted small">Your possible work from the first talk to signed: deals by stage, what each is worth, the next step and everything that happened — linked to your contacts, mail, calendar, tasks, notes, documents to sign, projects, invoices and forms. One binary, one SQLite file, one volume.</p>
        </div>
      </div>
      <div className="set-stats">
        {tiles.map(([label, n, icon]) => (
          <div key={label} className="set-stat"><Icon name={icon} size={16} /><span className="set-stat-n tnum">{(n ?? 0).toLocaleString()}</span><span className="set-stat-l">{label}</span></div>
        ))}
      </div>
      <Section title="This server">
        <dl className="set-dl">
          {rows.map(([k, v]) => <div key={k}><dt>{k}</dt><dd>{v}</dd></div>)}
        </dl>
      </Section>
      <div className="row wrap">
        <button type="button" className="btn" onClick={() => setKeys(true)}><Icon name="keyboard" size={15} />Keyboard shortcuts</button>
        <a className="btn" href="https://github.com/abjelosevic88/pocket-homelab-crm" target="_blank" rel="noreferrer"><Icon name="external" size={15} />Project on GitHub</a>
      </div>
      <p className="muted small">Open source (MIT). Part of the Pocket apps: Pocket Core connects them — Contacts, Mail, Calendar, Tasks, Projects, Invoicing, Sign, Forms, Flows, Day, Pocket AI and more.</p>
      {keys && <ShortcutsSheet onClose={() => setKeys(false)} />}
    </div>
  )
}

