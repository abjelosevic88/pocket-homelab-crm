export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface APIToken { id: number; user_id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }

export type ConnMode = 'core' | 'direct' | 'off'
export type ConnVia = '' | 'core' | 'direct'
export type AppSlug = 'contacts' | 'mail' | 'calendar' | 'tasks' | 'notes' | 'sign' | 'projects' | 'invoicing' | 'forms'

type ConnFields = { [K in AppSlug as `${K}_mode`]: ConnMode } & { [K in AppSlug as `${K}_via`]: ConnVia } & { [K in AppSlug as `${K}_detail` | `${K}_url` | `${K}_token` | `${K}_public_url` | `${K}_open_url`]: string } & { [K in AppSlug as `${K}_configured`]: boolean }

export type Settings = ConnFields & {
  core_configured: boolean
  core_registered?: boolean
  title: string; date_format: string; time_format: string; week_start: number
  currency: string; stale_days: number; action_days: number; lost_reasons: string[]
  task_mirror: boolean; meeting_minutes: number; project_team: number; mail_from: string
  won_project: boolean; won_invoice: boolean; won_task: boolean; ai_help: boolean
  notify_digest: boolean; notify_hour: number
  notify_url: string; notify_format: string; notify_configured: boolean
  ai_provider: 'auto' | 'pocket' | 'openai'; ai_pocket_model: string; openai_api_key: string; openai_model: string; openai_base_url: string
  setup_complete: boolean; base_url: string; demo: boolean
}

export type Status = 'open' | 'won' | 'lost'

export interface Stage { id: number; name: string; kind: Status; position: number; probability: number; color: string; created_at: string; deals: number; value: number }

export interface Deal {
  id: number; title: string; stage_id: number; status: Status; value: number; amount: number; currency: string; probability: number
  expected_close: string; source: string; contact_name: string; contact_email: string; contact_phone: string; contact_ref: string
  company: string; company_ref: string; description: string; next_action: string; next_due: string; next_task_ref: string
  tags: string[]; starred: boolean; position: number; lost_reason: string; outcome_note: string; closed_at: string
  stage_changed_at: string; last_activity_at: string; source_app: string; source_ref: string; created_at: string; updated_at: string
  stage_name?: string; stage_color?: string; chance: number; weighted: number; overdue: boolean; stale: boolean
  made_count: number; activities: number; days_in_stage: number
}

export type ActivityKind = 'note' | 'call' | 'meeting' | 'email' | 'step' | 'stage' | 'won' | 'lost' | 'made' | 'created'

export interface Activity { id: number; deal_id: number; kind: ActivityKind; title: string; body: string; at: string; app: string; url: string; source_app: string; created_at: string }

export interface Made { id: number; deal_id: number; target: string; app: string; ref: string; title: string; url: string; state: string; error?: string; checked_at: string; created_at: string }

export interface DealView { deal: Deal; activities: Activity[]; made: Made[]; url: string }

export type AppStateName = 'on' | 'partly' | 'off' | 'link_off' | 'missing' | 'old' | 'no_core' | 'not_set_up' | 'error'

export interface AppState { slug: AppSlug; name: string; reason: string; mode: ConnMode; via: ConnVia; state: AppStateName; detail: string; fix: string; on: string[]; open_url: string }

export interface ElseItem { kind: 'email' | 'meeting' | 'answer' | 'invoice'; title: string; body?: string; at: string; url?: string; state?: string; amount?: string; out?: boolean; ref?: string }
export interface ElseSection { app: AppSlug; name: string; state: 'ok' | 'off' | 'error' | 'none'; detail?: string; fix?: string; items: ElseItem[]; open_url?: string }

export type Money = Record<string, number>

export interface Summary {
  currency: string; open: number; open_value: Money; weighted: Money; due_today: number; overdue: number; no_step: number; stale: number
  won_month: number; won_month_value: Money; won_year: number; won_year_value: Money; lost_year: number; win_rate: number; closing_soon: number; url: string
}

export interface TodayView { lists: Record<'overdue' | 'today' | 'week' | 'no_step' | 'stale' | 'closing' | 'won', Deal[]>; summary: Summary; date: string }

export interface Party {
  key: string; name: string; company: string; email: string; phone: string; contact_ref: string; company_ref: string; kind: 'company' | 'person'
  deals: number; open: number; open_value: Money; won: number; won_value: Money; lost: number; last_at: string; people: string[]; deal_ids: number[]
}

export interface Lead { id: string; form: string; emoji: string; name: string; email: string; preview: string; at: string; status: string; url: string; deal_id?: number; deal_title?: string }

export interface CountRow { name: string; count: number; value: number; won?: number }
export interface MonthRow { month: string; won: number; lost: number; added: number; value: number }
export interface Reports { currency: string; other_currencies: string[]; months: MonthRow[]; funnel: CountRow[]; lost_reasons: CountRow[]; sources: CountRow[]; days_to_win: number; average_won: number; summary: Summary }

export interface PickedContact { id: string; kind: string; name: string; company: string; title: string; email: string; phone: string; initials: string; color: string }
