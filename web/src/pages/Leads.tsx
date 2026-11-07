// Leads from Pocket Forms: answers to your forms that are not deals yet —
// one click makes a deal with everything they wrote.
import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { announceChange, useAppState } from '../lib/data'
import { ago } from '../lib/format'
import type { AppStateName, Deal, Lead } from '../lib/types'
import { AppOff } from '../components/Apps'
import { Icon } from '../components/icons'
import { Empty, Loading, Spinner, errMsg, useToast } from '../components/ui'

interface LeadsView { state: 'ok' | AppStateName; detail?: string; fix?: string; items: Lead[]; open_url?: string }

export default function Leads() {
  const navigate = useNavigate()
  const { toast } = useToast()
  const appState = useAppState()
  const forms = appState('forms', 'forms:read')
  const [v, setV] = useState<LeadsView | null>(null)
  const [taking, setTaking] = useState('')
  const [showTaken, setShowTaken] = useState(false)
  const load = useCallback(async () => { try { setV(await api.get<LeadsView>(`${V1}/leads`)) } catch (e) { setV({ state: 'error', detail: errMsg(e), items: [] }) } }, [])
  useEffect(() => { load() }, [load])
  const take = async (l: Lead) => {
    setTaking(l.id)
    try {
      const d = await api.post<Deal>(`${V1}/leads/${l.id}/take`)
      announceChange()
      toast(`Added “${d.title}”`)
      navigate(`/deals/${d.id}`)
    } catch (e) { toast(errMsg(e), 'err') } finally { setTaking('') }
  }
  if (!v) return <Loading />
  const fresh = v.items.filter(l => !l.deal_id)
  const taken = v.items.filter(l => l.deal_id)
  return (
    <div className="page stack">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Leads from your forms</h1><p className="muted">Answers to your Pocket Forms (a brief, a quote request) that are not deals yet.</p></div>
        {v.open_url && <a className="btn ghost" href={v.open_url} target="_blank" rel="noreferrer"><Icon name="external" size={15} /><span className="hide-sm">Pocket Forms</span></a>}
      </div>
      {v.state !== 'ok' ? (
        <AppOff slug="forms" app={forms.app}>{v.detail || 'Answers to your forms show here once it is connected.'} You can still add every lead by hand.</AppOff>
      ) : fresh.length === 0 ? (
        <Empty title="No new answers" icon="inbox">Answers waiting in Pocket Forms show here; each becomes a deal in one click.</Empty>
      ) : (
        <div className="card list">
          {fresh.map(l => (
            <div key={l.id} className="list-row lead-row">
              <span className="lead-emoji" aria-hidden="true">{l.emoji || '📝'}</span>
              <span className="row-main grow">
                <span className="row-title"><span className="ellipsis">{l.name || l.email || 'Someone'}</span><span className="muted small">· {l.form}</span></span>
                <span className="muted small ellipsis">{l.preview}</span>
                <span className="muted tiny">{ago(l.at)}{l.url && <> · <a href={l.url} target="_blank" rel="noreferrer">Open in Pocket Forms</a></>}</span>
              </span>
              <button type="button" className="btn primary sm" onClick={() => take(l)} disabled={!!taking}>{taking === l.id ? <Spinner /> : <Icon name="plus" size={14} />}Make a deal</button>
            </div>
          ))}
        </div>
      )}
      {taken.length > 0 && <>
        <button type="button" className="link-btn self-start" onClick={() => setShowTaken(!showTaken)}><Icon name={showTaken ? 'chevron-up' : 'chevron-down'} size={14} />{taken.length} already deals</button>
        {showTaken && <div className="card list">
          {taken.map(l => (
            <Link key={l.id} to={`/deals/${l.deal_id}`} className="list-row">
              <span className="lead-emoji" aria-hidden="true">{l.emoji || '📝'}</span>
              <span className="row-main grow"><span className="row-title ellipsis">{l.deal_title}</span><span className="muted small">{l.name || l.email} · {l.form}</span></span>
              <Icon name="chevron-right" size={15} />
            </Link>
          ))}
        </div>}
      </>}
    </div>
  )
}
