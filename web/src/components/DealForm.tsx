// Adding and changing a deal: who it is with (picked from Pocket Contacts
// when it is connected, typed otherwise), what it is worth, where it stands
// and the next step.
import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { CURRENCIES, SOURCES } from '../lib/crm'
import { announceChange, useAppState, useData } from '../lib/data'
import { addDays, todayISO } from '../lib/format'
import type { Deal, PickedContact } from '../lib/types'
import { Icon } from './icons'
import { Select } from './Select'
import { Field, Modal, Spinner, errMsg, useDebounce, useToast } from './ui'

export interface DealDraft {
  title: string; amount: string; currency: string; stage_id: number; contact_name: string; contact_email: string; contact_phone: string
  contact_ref: string; company: string; company_ref: string; expected_close: string; source: string; next_action: string; next_due: string
  description: string; tags: string
}

export function draftOf(d?: Partial<Deal>, defaults?: { currency: string; stage: number; due: string }): DealDraft {
  return {
    title: d?.title || '', amount: d?.amount ? String(d.amount) : '', currency: d?.currency || defaults?.currency || 'EUR', stage_id: d?.stage_id || defaults?.stage || 0,
    contact_name: d?.contact_name || '', contact_email: d?.contact_email || '', contact_phone: d?.contact_phone || '', contact_ref: d?.contact_ref || '',
    company: d?.company || '', company_ref: d?.company_ref || '', expected_close: d?.expected_close || '', source: d?.source || '',
    next_action: d?.next_action || '', next_due: d?.next_due ?? defaults?.due ?? '', description: d?.description || '', tags: (d?.tags || []).join(', '),
  }
}

function bodyOf(f: DealDraft) {
  const amount = Number(String(f.amount).replace(/\s/g, '').replace(',', '.')) || 0
  return {
    title: f.title, amount, currency: f.currency, stage_id: f.stage_id || undefined, contact_name: f.contact_name, contact_email: f.contact_email,
    contact_phone: f.contact_phone, contact_ref: f.contact_ref, company: f.company, company_ref: f.company_ref, expected_close: f.expected_close,
    source: f.source, next_action: f.next_action, next_due: f.next_action ? f.next_due : '', description: f.description,
    tags: f.tags.split(',').map(t => t.trim()).filter(Boolean),
  }
}

/** Search Pocket Contacts as you type; pick a person or a company. */
function ContactPicker({ onPick }: { onPick: (c: PickedContact) => void }) {
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 250)
  const [list, setList] = useState<PickedContact[] | null>(null)
  const [off, setOff] = useState('')
  useEffect(() => {
    if (dq.trim().length < 2) { setList(null); return }
    let alive = true
    api.get<{ contacts: PickedContact[]; off?: string }>(`${V1}/apps/contacts?q=${encodeURIComponent(dq.trim())}`)
      .then(r => { if (alive) { setList(r.contacts); setOff(r.off || '') } }).catch(e => { if (alive) setOff(errMsg(e)) })
    return () => { alive = false }
  }, [dq])
  return (
    <div className="picker">
      <div className="search-in"><Icon name="search" size={15} /><input value={q} onChange={e => setQ(e.target.value)} placeholder="Find in Pocket Contacts…" aria-label="Find in Pocket Contacts" /></div>
      {off && <p className="muted small">{off}</p>}
      {list && (list.length === 0 ? <p className="muted small">Nobody called “{dq}” in your contacts. Type their details below.</p> : (
        <div className="pick-list">
          {list.map(c => (
            <button key={c.id} type="button" className="pick-row" onClick={() => { onPick(c); setQ(''); setList(null) }}>
              <span className="pick-ava" style={{ background: c.color || 'var(--accent)' }}>{c.kind === 'org' ? <Icon name="landmark" size={13} /> : (c.initials || c.name.slice(0, 1))}</span>
              <span className="grow ellipsis"><b>{c.name}</b>{(c.company || c.email) && <span className="muted"> · {[c.kind !== 'org' && c.company, c.email].filter(Boolean).join(' · ')}</span>}</span>
            </button>
          ))}
        </div>
      ))}
    </div>
  )
}

export function DealForm({ deal, initial, onClose, onSaved }: { deal?: Deal; initial?: Partial<Deal>; onClose: () => void; onSaved?: (d: Deal) => void }) {
  const { settings } = useApp()
  const { stages } = useData()
  const appState = useAppState()
  const contacts = appState('contacts', 'contacts:read')
  const navigate = useNavigate()
  const { toast } = useToast()
  const openStages = useMemo(() => (stages || []).filter(s => s.kind === 'open'), [stages])
  const [f, setF] = useState<DealDraft>(() => draftOf(deal || initial, { currency: settings?.currency || 'EUR', stage: openStages[0]?.id || 0, due: '' }))
  const [more, setMore] = useState(!!deal && !!(deal.description || deal.tags.length || deal.source || deal.expected_close))
  const [busy, setBusy] = useState(false)
  const titleRef = useRef<HTMLInputElement>(null)
  useEffect(() => { titleRef.current?.focus() }, [])
  const set = <K extends keyof DealDraft>(k: K, v: DealDraft[K]) => setF(x => ({ ...x, [k]: v }))
  const pick = (c: PickedContact) => setF(x => c.kind === 'org'
    ? { ...x, company: c.name, company_ref: c.id, title: x.title || '' }
    : { ...x, contact_name: c.name, contact_email: c.email || x.contact_email, contact_phone: c.phone || x.contact_phone, contact_ref: c.id, company: c.company || x.company })
  const save = async (e?: React.FormEvent) => {
    e?.preventDefault()
    setBusy(true)
    try {
      const body = bodyOf(f)
      const d = deal ? await api.put<Deal>(`${V1}/deals/${deal.id}`, body) : await api.post<Deal>(`${V1}/deals`, body)
      announceChange()
      toast(deal ? 'Saved' : `Added “${d.title}”`)
      if (onSaved) onSaved(d)
      else navigate(`/deals/${d.id}`)
      onClose()
    } catch (err) { toast(errMsg(err), 'err') } finally { setBusy(false) }
  }
  const linked = f.contact_ref || f.company_ref
  return (
    <Modal title={deal ? 'Change the deal' : 'New deal'} onClose={onClose} size="lg" footer={<>
      <button type="button" className="btn" onClick={onClose}>Cancel</button>
      <button type="button" className="btn primary" onClick={() => save()} disabled={busy || !(f.title || f.company || f.contact_name || f.contact_email)}>{busy ? <Spinner /> : <Icon name="check" size={15} />}{deal ? 'Save' : 'Add the deal'}</button>
    </>}>
      <form className="stack" onSubmit={save}>
        <Field label="What is it" help={deal ? undefined : 'A few words: “Online shop for the bakery”. Empty: the company’s or the person’s name.'}>
          <input ref={titleRef} value={f.title} onChange={e => set('title', e.target.value)} placeholder="Online shop for the bakery" />
        </Field>
        <section className="form-sec">
          <h4 className="form-sec-h">With whom{linked && <span className="badge ok"><Icon name="link" size={11} />In Pocket Contacts</span>}</h4>
          {contacts.ok ? <ContactPicker onPick={pick} /> : contacts.app && <p className="muted small"><Icon name="info" size={13} className="inline-ico" /> Type their details. {contacts.app.state === 'missing' ? 'With Pocket Contacts on this box you could pick them from your contacts.' : 'Connect Pocket Contacts (Settings → Connected apps) to pick them from your contacts.'}</p>}
          <div className="two-col tight">
            <Field label="Person"><input value={f.contact_name} onChange={e => setF({ ...f, contact_name: e.target.value, contact_ref: '' })} placeholder="Ana Marić" autoComplete="off" /></Field>
            <Field label="Company"><input value={f.company} onChange={e => setF({ ...f, company: e.target.value, company_ref: '' })} placeholder="Pekara Marić" autoComplete="off" /></Field>
            <Field label="Email"><input type="email" value={f.contact_email} onChange={e => set('contact_email', e.target.value)} placeholder="ana@example.com" autoComplete="off" /></Field>
            <Field label="Phone"><input type="tel" value={f.contact_phone} onChange={e => set('contact_phone', e.target.value)} autoComplete="off" /></Field>
          </div>
        </section>
        <section className="form-sec">
          <h4 className="form-sec-h">Worth and stage</h4>
          <div className="two-col tight">
            <Field label="Worth" group>
              <div className="input-group">
                <input inputMode="decimal" value={f.amount} onChange={e => set('amount', e.target.value)} placeholder="0" aria-label="Worth" />
                <Select ariaLabel="Currency" value={f.currency} onChange={v => set('currency', v)} options={Array.from(new Set([f.currency, ...CURRENCIES])).map(c => ({ value: c, label: c }))} />
              </div>
            </Field>
            {!deal && <Field label="Stage" group>
              <Select ariaLabel="Stage" value={String(f.stage_id || '')} onChange={v => set('stage_id', Number(v))} options={openStages.map(s => ({ value: String(s.id), label: s.name, hint: `${s.probability}%` }))} />
            </Field>}
            {deal && <Field label="Hoped to close by"><input type="date" value={f.expected_close} onChange={e => set('expected_close', e.target.value)} /></Field>}
          </div>
        </section>
        <section className="form-sec">
          <h4 className="form-sec-h">Next step</h4>
          <div className="two-col tight next-row">
            <Field label="What you do next"><input value={f.next_action} onChange={e => set('next_action', e.target.value)} placeholder="Call to go through what they need" /></Field>
            <Field label="When" group>
              <div className="row tight">
                <input type="date" value={f.next_due} onChange={e => set('next_due', e.target.value)} disabled={!f.next_action} aria-label="When" />
                {!f.next_due && f.next_action && <button type="button" className="btn sm" onClick={() => set('next_due', addDays(todayISO(), settings?.action_days || 2))}>In {settings?.action_days || 2} days</button>}
              </div>
            </Field>
          </div>
        </section>
        {!more && <button type="button" className="link-btn self-start" onClick={() => setMore(true)}><Icon name="plus" size={14} />Where it came from, a description, tags{deal ? '' : ', when it should close'}</button>}
        {more && <section className="form-sec">
          <div className="two-col tight">
            {!deal && <Field label="Hoped to close by"><input type="date" value={f.expected_close} onChange={e => set('expected_close', e.target.value)} /></Field>}
            <Field label="Came from" group><Select ariaLabel="Came from" value={f.source} onChange={v => set('source', v)} placeholder="Not said" clearLabel="Not said" options={SOURCES} /></Field>
            <Field label="Tags" help="Separated by commas."><input value={f.tags} onChange={e => set('tags', e.target.value)} placeholder="website, shop" /></Field>
          </div>
          <Field label="About the deal" help="What they want, what was said, anything to remember."><textarea rows={4} value={f.description} onChange={e => set('description', e.target.value)} /></Field>
        </section>}
      </form>
    </Modal>
  )
}
