// Left column: a new deal (and the import), Today, the pipeline and lists,
// then the open stages with their deals. The Pocket Calendar sidebar.
import { useState } from 'react'
import { Link, NavLink, useLocation, useNavigate } from 'react-router-dom'
import { useApp } from '../lib/app-context'
import { stageColor } from '../lib/crm'
import { useAppState, useData } from '../lib/data'
import { fmtMoney } from '../lib/format'
import { Icon } from './icons'
import { createItems, type CreateKind } from './Layout'
import { type Anchor, Menu, rectOf } from './ui'

export function Sidebar({ onCreate, onNavigate }: { onCreate: (k: CreateKind) => void; onNavigate: () => void }) {
  const { status, settings } = useApp()
  const { stages, summary } = useData()
  const appState = useAppState()
  const forms = appState('forms', 'forms:read')
  const navigate = useNavigate()
  const loc = useLocation()
  const [menu, setMenu] = useState<Anchor | null>(null)
  const title = settings?.title || status?.title || 'Pocket CRM'
  const sp = new URLSearchParams(loc.search)
  const curStage = loc.pathname === '/deals' ? Number(sp.get('stage') || 0) : 0
  const due = (summary?.due_today || 0) + (summary?.overdue || 0)
  const nav: { to: string; label: string; icon: string; active: boolean; count?: number; hint?: string; dim?: boolean }[] = [
    { to: '/', label: 'Today', icon: 'today', active: loc.pathname === '/', count: due, hint: 'Next steps due today or overdue' },
    { to: '/pipeline', label: 'Pipeline', icon: 'layers', active: loc.pathname === '/pipeline' },
    { to: '/deals', label: 'Deals', icon: 'list', active: (loc.pathname === '/deals' && !curStage) || /^\/deals\/\d/.test(loc.pathname) },
    { to: '/people', label: 'People & companies', icon: 'users', active: loc.pathname === '/people' },
    { to: '/leads', label: 'Leads from forms', icon: 'inbox', active: loc.pathname === '/leads', dim: !forms.loading && !forms.ok, hint: forms.ok ? undefined : 'Pocket Forms is not connected' },
    { to: '/reports', label: 'Reports', icon: 'chart', active: loc.pathname === '/reports' },
  ]
  const open = (stages || []).filter(s => s.kind === 'open')
  return (
    <aside className="sidebar" aria-label="Pocket CRM">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket CRM' ? title : <>Pocket <b>CRM</b></>}</span>
        {status?.version && <span className="sb-version" title={`Version ${status.version}`}>{/^v?\d+\.\d+/.test(status.version) ? 'v' + status.version.replace(/^v/, '').replace(/[-+].*$/, '') : status.version}</span>}
      </div>
      <div className="create-split">
        <button type="button" className="btn primary create-btn" onClick={() => onCreate('deal')} title="New deal (N)">
          <Icon name="plus" size={15} strokeWidth={2.2} />New deal
        </button>
        <button type="button" className="btn primary create-more" aria-label="More ways to start" aria-haspopup="menu" aria-expanded={!!menu} onClick={e => setMenu(menu ? null : rectOf(e.currentTarget))}>
          <Icon name="chevron-down" size={15} />
        </button>
      </div>
      {menu && <Menu anchor={menu} items={createItems(onCreate)} onClose={() => setMenu(null)} width={300} />}

      <nav className="sb-nav" aria-label="Sections">
        {nav.map(n => (
          <NavLink key={n.to} to={n.to} onClick={onNavigate} title={n.hint} className={() => `sb-nav-item ${n.active ? 'active' : ''} ${n.dim ? 'dim' : ''}`}>
            <Icon name={n.icon} size={16} />{n.label}
            {!!n.count && <span className="sb-count" title={n.hint}>{n.count}</span>}
          </NavLink>
        ))}
      </nav>

      {open.length > 0 && <section className="sb-sec">
        <h4 className="sb-h">Stages</h4>
        {open.map(s => (
          <button key={s.id} type="button" className={`sb-row sb-list ${curStage === s.id ? 'active' : ''}`} onClick={() => { onNavigate(); navigate(`/deals?stage=${s.id}`) }} title={s.value ? fmtMoney(s.value, settings?.currency || 'EUR') : undefined}>
            <span className="sb-stage-dot" style={{ background: stageColor(s) }} aria-hidden="true" />
            <span className="grow ellipsis">{s.name}</span>
            <span className="muted small tnum">{s.deals || ''}</span>
          </button>
        ))}
      </section>}
      {stages && summary && summary.open === 0 && summary.won_year === 0 && <section className="sb-sec">
        <button type="button" className="sb-row sb-list" onClick={() => onCreate('deal')}><span className="sb-list-ico" aria-hidden="true"><Icon name="plus" size={14} /></span><span className="grow">Add your first deal</span></button>
      </section>}

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); window.dispatchEvent(new CustomEvent('pcr:shortcuts')) }}><Icon name="keyboard" size={15} />Shortcuts</button>
      </div>
    </aside>
  )
}
