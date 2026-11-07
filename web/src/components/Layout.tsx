// The app shell: sidebar, top bar, search palette, shortcuts — the Pocket Calendar layout.
import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { DataProvider, useData } from '../lib/data'
import { Icon } from './icons'
import { Sidebar } from './Sidebar'
import { CommandPalette, type PaletteCommand } from './CommandPalette'
import { ShortcutsSheet } from './ShortcutsSheet'
import { ImportModal } from './ImportModal'
import { DealForm } from './DealForm'
import type { Deal } from '../lib/types'
import { Avatar, Loading, type MenuItem, MenuButton, modalOpen, useIsMobile, useMediaQuery } from './ui'
import { AskButton } from './AskButton'

export { announceChange, useOnChange } from '../lib/data'

const SIDEBAR_KEY = 'pcr-sidebar'

/** The page's own name for the top bar breadcrumb. */
const CrumbCtx = createContext<(t: string) => void>(() => {})
export function useCrumb(title: string | undefined) {
  const set = useContext(CrumbCtx)
  useEffect(() => { set(title || ''); return () => set('') }, [title, set])
}

export type CreateKind = 'deal' | 'import'

/** Start something from anywhere (a page's button), with what is known already. */
export const openCreate = (k: CreateKind, initial?: Partial<Deal>) => window.dispatchEvent(new CustomEvent('pcr:create', { detail: { kind: k, initial } }))

/** The new-deal menu: the sidebar's caret. */
export function createItems(onCreate: (k: CreateKind) => void): MenuItem[] {
  return [
    { label: 'New deal', icon: 'plus', hint: 'N', onSelect: () => onCreate('deal') },
    'sep',
    { label: 'Bring deals from a spreadsheet, Pipedrive, HubSpot…', icon: 'upload', onSelect: () => onCreate('import') },
  ]
}

export default function Layout() {
  return <DataProvider><Shell /></DataProvider>
}

function Shell() {
  const navigate = useNavigate()
  const loc = useLocation()
  const { stages } = useData()
  const mobile = useIsMobile()
  const narrow = useMediaQuery('(max-width: 1099px)')
  const drawer = mobile || narrow
  const [sidebarOpen, setSidebarOpen] = useState(() => !drawer && localStorage.getItem(SIDEBAR_KEY) !== '0')
  useEffect(() => { setSidebarOpen(!drawer && localStorage.getItem(SIDEBAR_KEY) !== '0') }, [drawer])
  const drawerOpenedAt = useRef(0)
  const toggleSidebar = () => setSidebarOpen(o => { drawerOpenedAt.current = performance.now(); const n = !o; if (!drawer) localStorage.setItem(SIDEBAR_KEY, n ? '1' : '0'); return n })
  const closeDrawer = () => { if (drawer) setSidebarOpen(false) }
  useEffect(() => { document.querySelector('.content')?.scrollTo({ top: 0 }) }, [loc.pathname])

  const [crumb, setCrumbRaw] = useState('')
  const setCrumb = useCallback((t: string) => setCrumbRaw(t), [])

  const [palette, setPalette] = useState(false)
  const [shortcuts, setShortcuts] = useState(false)
  const [importing, setImporting] = useState(false)
  const [newDeal, setNewDeal] = useState<Partial<Deal> | null>(null)

  const create = (k: CreateKind, initial?: Partial<Deal>) => {
    closeDrawer()
    if (k === 'deal') setNewDeal(initial || {})
    else setImporting(true)
  }
  // /deals/new?name=&email=&company= (a link from Pocket Contacts) opens the form filled in
  useEffect(() => {
    if (loc.pathname !== '/deals/new') return
    const sp = new URLSearchParams(loc.search)
    setNewDeal({ contact_name: sp.get('name') || '', contact_email: sp.get('email') || '', company: sp.get('company') || '', title: sp.get('title') || '' })
    navigate('/pipeline', { replace: true })
  }, [loc.pathname]) // eslint-disable-line react-hooks/exhaustive-deps
  const createRef = useRef(create)
  createRef.current = create

  useEffect(() => {
    const open = () => setShortcuts(true)
    const make = (e: Event) => { const d = (e as CustomEvent<{ kind: CreateKind; initial?: Partial<Deal> }>).detail; createRef.current(d.kind, d.initial) }
    window.addEventListener('pcr:shortcuts', open)
    window.addEventListener('pcr:create', make)
    return () => { window.removeEventListener('pcr:shortcuts', open); window.removeEventListener('pcr:create', make) }
  }, [])
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setPalette(p => !p); return }
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (t.closest('input, textarea, select, [contenteditable="true"]') || modalOpen() || document.querySelector('.menu, .popover, .sel-pop')) return
      const go = (to: string) => { e.preventDefault(); navigate(to) }
      const mk = (k: CreateKind) => { e.preventDefault(); createRef.current(k) }
      switch (e.key) {
        case '/': e.preventDefault(); setPalette(true); break
        case '?': e.preventDefault(); setShortcuts(true); break
        case 'n': mk('deal'); break
        case 'h': go('/'); break
        case 'p': go('/pipeline'); break
        case 'd': go('/deals'); break
        case 'c': go('/people'); break
        case 'r': go('/reports'); break
        case ',': go('/settings'); break
        case '[': e.preventDefault(); toggleSidebar(); break
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  const commands: PaletteCommand[] = [
    { id: 'deal', label: 'New deal', icon: 'plus', hint: 'N', keywords: 'add lead opportunity create', run: () => create('deal') },
    { id: 'import', label: 'Bring deals from a spreadsheet, Pipedrive, HubSpot…', icon: 'upload', keywords: 'import csv excel zoho', run: () => create('import') },
    { id: 'today', label: 'Today', icon: 'today', hint: 'H', keywords: 'home due steps', run: () => navigate('/') },
    { id: 'pipeline', label: 'Pipeline', icon: 'layers', hint: 'P', keywords: 'board stages kanban', run: () => navigate('/pipeline') },
    { id: 'deals', label: 'All deals', icon: 'list', hint: 'D', run: () => navigate('/deals') },
    { id: 'overdue', label: 'Overdue steps', icon: 'alert', keywords: 'late', run: () => navigate('/deals?due=overdue') },
    { id: 'won', label: 'Won deals', icon: 'trophy', run: () => navigate('/deals?status=won') },
    { id: 'lost', label: 'Lost deals', icon: 'x-circle', run: () => navigate('/deals?status=lost') },
    { id: 'people', label: 'People & companies', icon: 'users', hint: 'C', keywords: 'clients customers contacts', run: () => navigate('/people') },
    { id: 'leads', label: 'Leads from your forms', icon: 'inbox', keywords: 'pocket forms answers', run: () => navigate('/leads') },
    { id: 'reports', label: 'Reports', icon: 'chart', hint: 'R', keywords: 'win rate stats', run: () => navigate('/reports') },
    ...(stages || []).filter(s => s.kind === 'open').map(s => ({ id: `stage-${s.id}`, label: `Deals in ${s.name}`, icon: 'move', keywords: 'stage', run: () => navigate(`/deals?stage=${s.id}`) })),
    { id: 'settings', label: 'Settings', icon: 'settings', hint: ',', run: () => navigate('/settings') },
    { id: 'pipe-set', label: 'Settings: pipeline stages', icon: 'sliders', keywords: 'stages lost reasons currency', run: () => navigate('/settings/pipeline') },
    { id: 'apps', label: 'Settings: connected apps', icon: 'link', keywords: 'contacts mail calendar tasks notes sign projects invoicing forms core', run: () => navigate('/settings/apps') },
    { id: 'ai-set', label: 'Settings: AI', icon: 'sparkles', keywords: 'pocket ai openai', run: () => navigate('/settings/ai') },
    { id: 'export', label: 'Settings: export your deals', icon: 'download', keywords: 'backup json csv', run: () => navigate('/settings/data') },
    { id: 'shortcuts', label: 'Keyboard shortcuts', icon: 'keyboard', hint: '?', run: () => setShortcuts(true) },
  ]

  return (
    <CrumbCtx.Provider value={setCrumb}>
      <div className={`app ${sidebarOpen ? 'with-sidebar' : 'no-sidebar'} ${mobile ? 'is-mobile' : ''} ${drawer ? 'drawer-mode' : ''}`}>
        {drawer && sidebarOpen && <div className="drawer-backdrop" onClick={e => { if (e.timeStamp - drawerOpenedAt.current > 400) setSidebarOpen(false) }} />}
        <div className={`sidebar-wrap ${sidebarOpen ? 'open' : ''}`}>
          <Sidebar onCreate={create} onNavigate={closeDrawer} />
        </div>
        <div className="main">
          <TopBar onMenu={toggleSidebar} onPalette={() => setPalette(true)} mobile={mobile} compact={narrow} sidebarOpen={sidebarOpen} crumb={crumb} />
          <main className={`content ${loc.pathname === '/pipeline' ? 'wide' : ''}`}><div className="content-inner">{stages ? <Outlet /> : <Loading />}</div></main>
        </div>
        {mobile && !/^\/(deals\/\d|settings)/.test(loc.pathname) && <button type="button" className="fab" aria-label="New deal" title="New deal" onClick={() => create('deal')}><Icon name="plus" size={24} strokeWidth={2.2} /></button>}
        {palette && <CommandPalette onClose={() => setPalette(false)} commands={commands} />}
        {shortcuts && <ShortcutsSheet onClose={() => setShortcuts(false)} />}
        {importing && <ImportModal onClose={() => setImporting(false)} />}
        {newDeal && <DealForm initial={newDeal} onClose={() => setNewDeal(null)} />}
      </div>
    </CrumbCtx.Provider>
  )
}

const SECTION: Record<string, [string, string]> = { '': ['Today', '/'], pipeline: ['Pipeline', '/pipeline'], deals: ['Deals', '/deals'], people: ['People & companies', '/people'], leads: ['Leads', '/leads'], reports: ['Reports', '/reports'] }

function TopBar({ onMenu, onPalette, mobile, compact, sidebarOpen, crumb }: { onMenu: () => void; onPalette: () => void; mobile: boolean; compact: boolean; sidebarOpen: boolean; crumb: string }) {
  const { user, themeMode, setThemeMode, status, setUser } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const [section, sectionTo] = SECTION[loc.pathname.split('/')[1] || ''] || ['', '/']
  const signOut = async () => { await api.post(`${V1}/auth/logout`).catch(() => {}); setUser(null); navigate('/login') }
  const avatarMenu = (): MenuItem[] => [
    { heading: user?.name || user?.email || '' },
    { label: 'Profile', icon: 'user', onSelect: () => navigate('/settings/profile') },
    { label: 'Settings', icon: 'settings', onSelect: () => navigate('/settings'), hint: ',' },
    { label: 'Keyboard shortcuts', icon: 'keyboard', onSelect: () => window.dispatchEvent(new CustomEvent('pcr:shortcuts')), hint: '?' },
    'sep',
    { heading: 'Appearance' },
    { label: 'System', checked: themeMode === 'system', onSelect: () => setThemeMode('system') },
    { label: 'Light', checked: themeMode === 'light', onSelect: () => setThemeMode('light') },
    { label: 'Dark', checked: themeMode === 'dark', onSelect: () => setThemeMode('dark') },
    ...(status?.auth_mode === 'local' ? ['sep' as const, { label: 'Sign out', icon: 'log-out', onSelect: signOut }] : []),
  ]
  return (
    <header className="topbar">
      <button type="button" className="btn ghost icon-only" onClick={onMenu} aria-label={sidebarOpen ? 'Hide the sidebar' : 'Show the sidebar'} title="Menu ([)"><Icon name={mobile || compact ? 'menu' : 'sidebar'} size={18} /></button>
      <h1 className="range-title">
        {crumb ? <><button type="button" className="crumb" onClick={() => navigate(sectionTo)}>{section}</button><Icon name="chevron-right" size={16} className="crumb-sep" /><span className="ellipsis">{crumb}</span></> : section}
      </h1>
      <span className="spacer" />
      <button type="button" className="btn ghost icon-only" onClick={onPalette} aria-label="Search" title="Search (⌘K or /)"><Icon name="search" size={18} /></button>
      <AskButton />
      <MenuButton label="Account" className="avatar-btn" align="right" width={220} items={avatarMenu}>
        <Avatar name={user?.name} email={user?.email} size={30} />
      </MenuButton>
    </header>
  )
}
