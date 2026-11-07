// The board: one column per open stage, cards you drag between them (or
// move with the card's menu on a phone), with what each column is worth.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { stageColor, who } from '../lib/crm'
import { announceChange, useData, useOnChange } from '../lib/data'
import { fmtDate, fmtMoney, fmtMoneyMap, todayISO } from '../lib/format'
import type { Deal, Stage } from '../lib/types'
import { LostSheet, WonSheet } from '../components/DealActions'
import { Icon } from '../components/icons'
import { openCreate } from '../components/Layout'
import { Loading, type MenuItem, MenuButton, errMsg, useIsMobile, useToast } from '../components/ui'

export default function Pipeline() {
  const { settings } = useApp()
  const { stages } = useData()
  const { toast } = useToast()
  const navigate = useNavigate()
  const mobile = useIsMobile()
  const [deals, setDeals] = useState<Deal[] | null>(null)
  const [drag, setDrag] = useState<number>(0)
  const [over, setOver] = useState<{ stage: number; index: number } | null>(null)
  const [closing, setClosing] = useState<{ deal: Deal; kind: 'won' | 'lost' } | null>(null)
  const load = useCallback(async () => {
    try { setDeals((await api.get<{ items: Deal[] }>(`${V1}/deals?status=open&sort=stage&limit=1000`)).items) } catch { setDeals(d => d || []) }
  }, [])
  useEffect(() => { load() }, [load])
  useOnChange(load)
  const open = useMemo(() => (stages || []).filter(s => s.kind === 'open'), [stages])
  const byStage = useMemo(() => {
    const m: Record<number, Deal[]> = {}
    for (const s of open) m[s.id] = []
    for (const d of deals || []) (m[d.stage_id] ||= []).push(d)
    for (const k in m) m[k].sort((a, b) => a.position - b.position || a.id - b.id)
    return m
  }, [deals, open])
  const won = (stages || []).find(s => s.kind === 'won')
  const lost = (stages || []).find(s => s.kind === 'lost')
  const scroller = useRef<HTMLDivElement>(null)

  const move = async (d: Deal, stage: Stage, index: number) => {
    const col = (byStage[stage.id] || []).filter(x => x.id !== d.id)
    const before = index > 0 ? col[index - 1]?.position : undefined
    const after = col[index]?.position
    // show it at once; the server says the final word
    setDeals(ds => (ds || []).map(x => x.id === d.id ? { ...x, stage_id: stage.id, stage_name: stage.name, position: before !== undefined && after !== undefined ? (before + after) / 2 : before !== undefined ? before + 1 : after !== undefined ? after - 1 : 0 } : x))
    try { await api.post(`${V1}/deals/${d.id}/move`, { stage_id: stage.id, before, after }); announceChange() } catch (e) { toast(errMsg(e), 'err'); load() }
  }
  const dropOn = (stage: Stage, index: number) => {
    const d = (deals || []).find(x => x.id === drag)
    setDrag(0); setOver(null)
    if (!d) return
    const col = byStage[stage.id] || []
    const from = col.findIndex(x => x.id === d.id)
    if (from >= 0 && (from === index || from === index - 1)) return
    move(d, stage, from >= 0 && from < index ? index - 1 : index)
  }
  if (!stages || !deals) return <Loading />
  const t = todayISO()
  return (
    <div className="board-page">
      <div className="page-head board-head">
        <div className="grow">
          <h1 className="page-title">Pipeline</h1>
          <p className="muted">{deals.length} open {deals.length === 1 ? 'deal' : 'deals'} · {fmtMoneyMap(sumBy(deals, 'value'), settings?.currency || 'EUR')} · likely {fmtMoneyMap(sumBy(deals, 'weighted'), settings?.currency || 'EUR')}</p>
        </div>
        <Link to="/settings/pipeline" className="btn ghost"><Icon name="sliders" size={15} /><span className="hide-sm">Stages</span></Link>
        <button type="button" className="btn primary hide-sm" onClick={() => openCreate('deal')}><Icon name="plus" size={15} />New deal</button>
      </div>
      <div className="board" ref={scroller}>
        {open.map(s => {
          const col = byStage[s.id] || []
          return (
            <section key={s.id} className={`col ${drag ? 'dragging' : ''} ${over?.stage === s.id ? 'over' : ''}`} style={{ ['--st' as string]: stageColor(s) }}
              onDragOver={e => { if (!drag) return; e.preventDefault(); if (over?.stage !== s.id && !col.length) setOver({ stage: s.id, index: 0 }) }}
              onDrop={e => { e.preventDefault(); dropOn(s, over?.stage === s.id ? over.index : col.length) }}>
              <header className="col-head">
                <span className="col-dot" />
                <span className="col-name ellipsis">{s.name}</span>
                <span className="count">{col.length}</span>
                <span className="spacer" />
                <span className="muted tiny tnum" title={`${s.probability}% chance`}>{fmtMoneyMap(sumBy(col, 'value'), settings?.currency || 'EUR', true)}</span>
              </header>
              <div className="col-cards">
                {col.map((d, i) => (
                  <div key={d.id} className="card-slot"
                    onDragOver={e => { if (!drag) return; e.preventDefault(); e.stopPropagation(); const r = e.currentTarget.getBoundingClientRect(); const idx = e.clientY < r.top + r.height / 2 ? i : i + 1; if (over?.stage !== s.id || over.index !== idx) setOver({ stage: s.id, index: idx }) }}
                    onDrop={e => { e.preventDefault(); e.stopPropagation(); dropOn(s, over?.stage === s.id ? over.index : i) }}>
                    {over?.stage === s.id && over.index === i && drag !== d.id && <div className="drop-line" />}
                    <DealCard d={d} t={t} dragging={drag === d.id} draggable={!mobile}
                      onDragStart={() => setDrag(d.id)} onDragEnd={() => { setDrag(0); setOver(null) }}
                      onOpen={() => navigate(`/deals/${d.id}`)}
                      menu={(): MenuItem[] => [
                        { heading: 'Move to' },
                        ...open.filter(x => x.id !== s.id).map(x => ({ label: x.name, icon: 'move', onSelect: () => move(d, x, 0) })),
                        'sep',
                        ...(won ? [{ label: 'Won', icon: 'trophy', onSelect: () => setClosing({ deal: d, kind: 'won' }) }] : []),
                        ...(lost ? [{ label: 'Lost', icon: 'x-circle', onSelect: () => setClosing({ deal: d, kind: 'lost' }) }] : []),
                      ]} />
                    {over?.stage === s.id && over.index === col.length && i === col.length - 1 && <div className="drop-line" />}
                  </div>
                ))}
                {col.length === 0 && <div className="col-empty muted small">{drag ? 'Drop here' : 'Nothing here'}</div>}
                <button type="button" className="col-add" onClick={() => openCreate('deal', { stage_id: s.id })}><Icon name="plus" size={14} />Add</button>
              </div>
            </section>
          )
        })}
        {drag > 0 && <div className="close-zones">
          {won && <div className="close-zone won" onDragOver={e => e.preventDefault()} onDrop={e => { e.preventDefault(); const d = deals.find(x => x.id === drag); setDrag(0); setOver(null); if (d) setClosing({ deal: d, kind: 'won' }) }}><Icon name="trophy" size={18} />Won</div>}
          {lost && <div className="close-zone lost" onDragOver={e => e.preventDefault()} onDrop={e => { e.preventDefault(); const d = deals.find(x => x.id === drag); setDrag(0); setOver(null); if (d) setClosing({ deal: d, kind: 'lost' }) }}><Icon name="x-circle" size={18} />Lost</div>}
        </div>}
      </div>
      {closing?.kind === 'won' && <WonSheet deal={closing.deal} onClose={() => setClosing(null)} onDone={() => { setClosing(null); load() }} />}
      {closing?.kind === 'lost' && <LostSheet deal={closing.deal} onClose={() => setClosing(null)} onDone={() => { setClosing(null); load() }} />}
    </div>
  )
}

function sumBy(ds: Deal[], k: 'value' | 'weighted') {
  const m: Record<string, number> = {}
  for (const d of ds) m[d.currency] = (m[d.currency] || 0) + d[k]
  return m
}

function DealCard({ d, t, dragging, draggable, onDragStart, onDragEnd, onOpen, menu }: { d: Deal; t: string; dragging: boolean; draggable: boolean; onDragStart: () => void; onDragEnd: () => void; onOpen: () => void; menu: () => MenuItem[] }) {
  return (
    <article className={`deal-card ${dragging ? 'is-dragging' : ''} ${d.overdue ? 'overdue' : ''}`} draggable={draggable}
      onDragStart={e => { e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', String(d.id)); onDragStart() }} onDragEnd={onDragEnd}
      onClick={e => { if (!(e.target as HTMLElement).closest('button, a')) onOpen() }} role="link" tabIndex={0} onKeyDown={e => { if (e.key === 'Enter') onOpen() }}>
      <div className="dc-top">
        <span className="dc-title">{d.starred && <Icon name="star" size={12} className="star-on" />}{d.title}</span>
        <MenuButton label="Move" items={menu} className="btn ghost icon-only sm dc-menu" align="right" width={210}><Icon name="more" size={15} /></MenuButton>
      </div>
      {who(d) && <div className="dc-who ellipsis">{who(d)}</div>}
      <div className="dc-foot">
        {d.value > 0 && <span className="dc-val tnum">{fmtMoney(d.value, d.currency, true)}</span>}
        <span className="spacer" />
        {d.next_action
          ? <span className={`dc-step ${d.overdue ? 'late' : d.next_due === t ? 'today' : ''}`} title={d.next_action}><Icon name="target" size={11} />{d.next_due ? (d.next_due === t ? 'Today' : fmtDate(d.next_due, false)) : 'No day'}</span>
          : <span className="dc-step none" title="No next step"><Icon name="alert" size={11} />No step</span>}
      </div>
    </article>
  )
}
