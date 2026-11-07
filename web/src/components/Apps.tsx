// How a deal shows the other Pocket apps it cannot reach: greyed, never
// hidden, with one line saying why and what to do.
import { Link } from 'react-router-dom'
import { APP_INFO, STATE_WORDS } from '../lib/crm'
import type { AppSlug, AppState } from '../lib/types'
import { Icon } from './icons'

/** The title of a button for an app that does not work (a tooltip). */
export function offTitle(a: AppState | undefined, slug: AppSlug): string {
  if (!a) return `${APP_INFO[slug].name}: checking…`
  return `${a.name}: ${STATE_WORDS[a.state] || a.state}${a.detail ? ' — ' + a.detail : ''}${a.fix ? '. ' + a.fix + '.' : ''}`
}

/** A quiet box in place of something an app would show. */
export function AppOff({ slug, app, children, compact = false }: { slug: AppSlug; app?: AppState; children?: React.ReactNode; compact?: boolean }) {
  const info = APP_INFO[slug]
  const state = app?.state || 'error'
  const head = state === 'missing' ? `${info.name} is not on this box`
    : state === 'old' ? `${info.name} needs a newer version`
      : state === 'off' ? `${info.name} is switched off`
        : state === 'link_off' ? `${info.name} is not switched on for CRM`
          : state === 'no_core' || state === 'not_set_up' ? `${info.name} is not connected`
            : `${info.name} is not reachable`
  return (
    <div className={`app-off ${compact ? 'compact' : ''}`}>
      <span className="app-off-ico"><Icon name={info.icon} size={15} /></span>
      <div className="grow">
        <div className="app-off-h">{head}</div>
        {!compact && children && <div className="muted small">{children}</div>}
        {app?.fix && <div className="muted small">{app.fix}{state !== 'missing' && <> · <Link to="/settings/apps">Connected apps</Link></>}</div>}
      </div>
    </div>
  )
}
