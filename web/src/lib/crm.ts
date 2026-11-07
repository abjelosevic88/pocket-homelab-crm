// What the deals know about the other Pocket apps, the timeline and sources.
import type { ActivityKind, AppSlug, AppStateName, Deal, Stage } from './types'

export const APP_INFO: Record<AppSlug, { name: string; icon: string; what: string }> = {
  contacts: { name: 'Pocket Contacts', icon: 'users', what: 'people and companies' },
  mail: { name: 'Pocket Mail', icon: 'mail', what: 'emails' },
  calendar: { name: 'Pocket Calendar', icon: 'calendar', what: 'meetings' },
  tasks: { name: 'Pocket Tasks', icon: 'check-square', what: 'next steps as tasks' },
  notes: { name: 'Pocket Notes', icon: 'note', what: 'notes' },
  sign: { name: 'Pocket Sign', icon: 'pen', what: 'documents to sign' },
  projects: { name: 'Pocket Projects', icon: 'layers', what: 'projects' },
  invoicing: { name: 'Pocket Invoicing', icon: 'receipt', what: 'invoices' },
  forms: { name: 'Pocket Forms', icon: 'list', what: 'leads from your forms' },
}

/** Which app makes each kind of linked thing. */
export const TARGET_APP: Record<string, AppSlug> = {
  contact: 'contacts', task: 'tasks', event: 'calendar', note: 'notes', email: 'mail', signing: 'sign', project: 'projects', invoice: 'invoicing', answer: 'forms',
}

export const TARGET_LABEL: Record<string, string> = {
  contact: 'Contact', task: 'Task', event: 'Meeting', note: 'Note', email: 'Email', signing: 'To sign', project: 'Project', invoice: 'Invoice', answer: 'Form answer',
}

/** Short words for a state that does not work, for a greyed-out button. */
export const STATE_WORDS: Record<AppStateName, string> = {
  on: 'Connected', partly: 'Partly connected', off: 'Switched off here', link_off: 'Switched off in Pocket Core', missing: 'Not on this box',
  old: 'Needs a newer version', no_core: 'No Pocket Core', not_set_up: 'Not set up', error: 'Not reachable',
}

export const ACTIVITY: Record<ActivityKind, { icon: string; label: string }> = {
  note: { icon: 'note', label: 'Note' }, call: { icon: 'phone', label: 'Call' }, meeting: { icon: 'users', label: 'Meeting' },
  email: { icon: 'mail', label: 'Email' }, step: { icon: 'check-circle', label: 'Step done' }, stage: { icon: 'move', label: 'Moved' },
  won: { icon: 'trophy', label: 'Won' }, lost: { icon: 'x-circle', label: 'Lost' }, made: { icon: 'link', label: 'In another app' },
  created: { icon: 'plus', label: 'Added' },
}

/** Kinds a person can write on the timeline by hand. */
export const LOG_KINDS: { value: ActivityKind; label: string; icon: string }[] = [
  { value: 'note', label: 'Note', icon: 'note' }, { value: 'call', label: 'Call', icon: 'phone' }, { value: 'meeting', label: 'Meeting', icon: 'users' },
  { value: 'email', label: 'Email', icon: 'mail' },
]

export const SOURCES: { value: string; label: string }[] = [
  { value: 'referral', label: 'Someone recommended you' }, { value: 'website', label: 'Your website' }, { value: 'form', label: 'A form' },
  { value: 'email', label: 'An email' }, { value: 'event', label: 'An event or meeting' }, { value: 'social', label: 'Social media' },
  { value: 'repeat', label: 'A client again' }, { value: 'cold', label: 'You reached out' }, { value: 'other', label: 'Something else' },
]
export const sourceLabel = (v: string) => SOURCES.find(s => s.value === v)?.label || (v ? v[0].toUpperCase() + v.slice(1) : 'Not said')

export const CURRENCIES = ['EUR', 'BAM', 'USD', 'GBP', 'CHF', 'RSD', 'HRK', 'SEK', 'NOK', 'DKK', 'PLN', 'CZK', 'HUF', 'RON', 'BGN', 'TRY', 'CAD', 'AUD']

export const STAGE_COLORS = ['#868e96', '#1c7ed6', '#7048e8', '#ae3ec9', '#f08c00', '#0ca678', '#2f9e44', '#e03131']

export function stageColor(s: Pick<Stage, 'color' | 'kind'> | undefined) {
  if (!s) return '#868e96'
  return s.color || (s.kind === 'won' ? '#2f9e44' : s.kind === 'lost' ? '#e03131' : '#1c7ed6')
}

/** Who a deal is with, in one line. */
export function who(d: Pick<Deal, 'contact_name' | 'company' | 'contact_email'>) {
  if (d.contact_name && d.company) return `${d.contact_name} · ${d.company}`
  return d.contact_name || d.company || d.contact_email || ''
}
