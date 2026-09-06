/**
 * MentionInput —— @file 富输入（受控组件）。
 *
 * 透明 textarea 叠在 backdrop 之上；backdrop 以内层真实文件名 paint 每个 Chip
 * （双层叠加：外层占位隐藏、内层文件名显示）。键入 `@` 弹出候选下拉，选择后
 * 在该处插入 U+FFFC 占位符并记录 Chip 引用。
 *
 * 候选下拉不在此内联渲染，而是通过 `conversation.input.menu` 槽位对外提供
 * （见 export：MentionMenu），由 host（Task7 集成）摆放渲染位置。
 */

import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react'
import { create } from 'zustand'
import { PLACEHOLDER, insertChip, type Chip } from './chips'
import {
  detectMention,
  matchCandidates,
  type Candidate,
  type TriggerHit,
} from './trigger'
import { fetchWorkspaceFiles, type FileEntry } from './fileApi'
import styles from './at-file.module.css'

/* ===== 菜单共享状态：MentionInput 写入，槽位里的 MentionMenu 读取 ===== */
interface AtFileMenuState {
  hit: TriggerHit | null
  candidates: Candidate[]
  active: number
  pick: ((c: Candidate) => void) | null
  setHit: (h: TriggerHit | null) => void
  setCandidates: (cs: Candidate[]) => void
  setActive: (n: number) => void
  setPick: (fn: ((c: Candidate) => void) | null) => void
  close: () => void
}

const useAtFileMenu = create<AtFileMenuState>((set) => ({
  hit: null,
  candidates: [],
  active: 0,
  pick: null,
  setHit: (hit) => set({ hit, active: 0 }),
  setCandidates: (candidates) => set({ candidates, active: 0 }),
  setActive: (active) => set({ active }),
  setPick: (pick) => set({ pick }),
  close: () => set({ hit: null, candidates: [], active: 0 }),
}))

const KIND_COLOR: Record<string, string> = { file: '#3e8fe0', dir: '#b08a2e' }
const KIND_LABEL: Record<string, string> = { file: '文件', dir: '目录' }

function toCandidate(e: FileEntry): Candidate {
  return { id: e.path, label: e.path, kind: e.kind, desc: e.kind === 'dir' ? '目录' : '文件' }
}

export interface MentionInputProps {
  /** 含 Chip 占位符（U+FFFC）的底层文本，由父级持有。 */
  value: string
  onChange: (valueWithChips: string) => void
  placeholder?: string
  disabled?: boolean
  /** 文件发现使用的工作区 id（缺省 main）。 */
  workspaceId?: string
  /** 菜单关闭时按 Enter 提交（无 Shift）。 */
  onSubmit?: () => void
  /** 菜单开合通知，父级可据此拦截 Enter 提交。 */
  onMenuChange?: (open: boolean) => void
}

export function MentionInput({
  value,
  onChange,
  placeholder,
  disabled,
  workspaceId,
  onSubmit,
  onMenuChange,
}: MentionInputProps) {
  const [chips, setChips] = useState<Chip[]>([])
  const [hit, setHit] = useState<TriggerHit | null>(null)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  // 供闭包内读取最新值的 ref，避免陈旧闭包。
  const hitRef = useRef<TriggerHit | null>(hit); hitRef.current = hit
  const valueRef = useRef(value); valueRef.current = value
  const chipsRef = useRef<Chip[]>(chips); chipsRef.current = chips
  const workspaceRef = useRef(workspaceId); workspaceRef.current = workspaceId
  const fetchTimer = useRef<number | undefined>(undefined)

  const menuHit = useAtFileMenu((s) => s.hit)
  const candidates = useAtFileMenu((s) => s.candidates)
  const active = useAtFileMenu((s) => s.active)

  const menuOpen = menuHit !== null && candidates.length > 0

  useEffect(() => {
    onMenuChange?.(menuOpen)
  }, [menuOpen, onMenuChange])

  // 把正文划分为纯文本段与 Chip 槽位，供 backdrop 双层绘制。
  const segments = useMemo(() => {
    const segs: { text: string; chip?: Chip }[] = []
    value.split(PLACEHOLDER).forEach((p, i) => {
      if (p) segs.push({ text: p })
      segs.push({ text: chips[i] ? chips[i].label : '', chip: chips[i] })
    })
    return segs
  }, [value, chips])

  // 拉取工作区文件并据 query 过滤，写入菜单状态（带节流）。
  const loadCandidates = (query: string) => {
    window.clearTimeout(fetchTimer.current)
    const qh = query.trim()
    fetchTimer.current = window.setTimeout(async () => {
      const entries = await fetchWorkspaceFiles({ workspaceId: workspaceRef.current })
      useAtFileMenu.getState().setCandidates(matchCandidates(entries.map(toCandidate), qh))
    }, 80)
  }

  const applyHit = (m: TriggerHit | null) => {
    setHit(m && m.active ? m : null)
    if (m && m.active) {
      useAtFileMenu.getState().setHit(m)
      loadCandidates(m.query)
    } else {
      useAtFileMenu.getState().close()
      window.clearTimeout(fetchTimer.current)
    }
  }

  const pick = (c: Candidate) => {
    const h = hitRef.current
    if (!h) return
    const chip: Chip = { id: c.id, label: c.label, kind: c.kind, path: c.label }
    const { text } = insertChip(valueRef.current, h.start, h.end, chip)
    setChips([...chipsRef.current, chip])
    onChange(text)
    setHit(null)
    useAtFileMenu.getState().close()
    // 宏任务后焦点回到 textarea。
    requestAnimationFrame(() => inputRef.current?.focus())
  }

  useEffect(() => {
    useAtFileMenu.getState().setPick(pick)
    return () => useAtFileMenu.getState().setPick(null)
    // pick 只经 ref 读取最新值，注册一次即可。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const onTextChange = (v: string) => {
    onChange(v)
    const pos = inputRef.current?.selectionStart ?? v.length
    applyHit(detectMention(v, pos))
  }

  const onKeyUp = () => {
    const el = inputRef.current
    if (!el) return
    applyHit(detectMention(el.value, el.selectionStart ?? el.value.length))
  }

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (menuOpen) {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        useAtFileMenu.getState().setActive((active + 1) % candidates.length)
        return
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        useAtFileMenu.getState().setActive((active - 1 + candidates.length) % candidates.length)
        return
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault()
        const c = candidates[active]
        if (c) useAtFileMenu.getState().pick?.(c)
        return
      }
      if (e.key === 'Escape') {
        e.preventDefault()
        useAtFileMenu.getState().close()
        setHit(null)
        return
      }
    }
    // 菜单关闭状态下 Enter（无 Shift）提交。
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      onSubmit?.()
    }
  }

  return (
    <div className={styles.inputWrap}>
      <div
        className={styles.backdrop + (chips.length > 0 ? ' ' + styles.backdropPainted : '')}
        aria-hidden="true"
      >
        {segments.map((s, i) =>
          s.chip ? (
            <span key={i} className={styles.chip}>
              {/* 外层占位：隐藏/零宽（承载 U+FFFC 的度量）。 */}
              <span className={styles.chipSlot}>{PLACEHOLDER}</span>
              {/* 内层真实文件名。 */}
              <span className={styles.chipFace}>{s.chip.label}</span>
            </span>
          ) : (
            <span key={i}>{s.text}</span>
          ),
        )}
      </div>
      <textarea
        ref={inputRef}
        rows={3}
        className={styles.input}
        value={value}
        disabled={disabled}
        placeholder={placeholder}
        onChange={(e) => onTextChange(e.target.value)}
        onKeyDown={onKeyDown}
        onKeyUp={onKeyUp}
      />
    </div>
  )
}

/**
 * 候选下拉渲染器。注册进 `conversation.input.menu` 槽位；若当前无活动提及
 * 或无候选则返回 null，由槽位基数（single）决定仅渲染此菜单。
 */
export function MentionMenu() {
  const hit = useAtFileMenu((s) => s.hit)
  const candidates = useAtFileMenu((s) => s.candidates)
  const active = useAtFileMenu((s) => s.active)
  const pick = useAtFileMenu((s) => s.pick)

  if (!hit || candidates.length === 0 || !pick) return null

  return (
    <div className={styles.menu} role="listbox" data-testid="atfile-menu">
      <div className={styles.menuHead}>@ 引用 · {candidates.length} 项</div>
      {candidates.map((c, i) => (
        <div
          key={c.id}
          role="option"
          aria-selected={i === active}
          className={styles.menuItem + (i === active ? ' ' + styles.menuActive : '')}
          onMouseEnter={() => useAtFileMenu.getState().setActive(i)}
          onClick={() => pick(c)}
        >
          <span
            className={styles.menuKind}
            style={{ color: KIND_COLOR[c.kind], background: KIND_COLOR[c.kind] + '22' }}
          >
            {KIND_LABEL[c.kind]}
          </span>
          <span className={styles.menuLabel}>{c.label}</span>
          {c.desc && <span className={styles.menuDesc}>{c.desc}</span>}
        </div>
      ))}
    </div>
  )
}