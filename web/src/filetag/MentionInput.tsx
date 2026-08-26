// MentionInput — @file rich input (Phase 5): a transparent textarea over a
// backdrop that paints chips inline. Typing `@` opens a candidate menu. The
// underlying editor string (owned by the parent) holds one U+FFFC placeholder
// per chip, keeping caret/keyboard metrics 1:1. The chip registry lives here;
// callers read the serialized message via the imperative handle.

import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
} from 'react'
import { PLACEHOLDER, insertChip, serialize, type Chip } from './chips'
import { detectMention, matchCandidates, type Candidate } from './trigger'
import styles from './mention.module.css'

export interface MentionInputHandle {
  serializeValue: () => string
}

const KIND_COLOR: Record<string, string> = { file: '#3e8fe0', dir: '#b08a2e', skill: '#7a5fbf' }
const KIND_LABEL: Record<string, string> = { file: '文件', dir: '目录', skill: '技能' }

const DEMO_CANDIDATES: Candidate[] = [
  { id: 'f1', label: 'src/main.go', kind: 'file', desc: 'Go 入口' },
  { id: 'f2', label: 'config.json', kind: 'file', desc: '配置文件' },
  { id: 'f3', label: 'README.md', kind: 'file' },
  { id: 'd1', label: 'web/src', kind: 'dir' },
  { id: 'd2', label: 'server', kind: 'dir' },
  { id: 's1', label: 'debug', kind: 'skill', desc: '调试技能' },
]

export const MentionInput = forwardRef<
  MentionInputHandle,
  {
    value: string
    onChange: (v: string) => void
    disabled?: boolean
    /** Called on Enter (without Shift) when the mention menu is closed. */
    onSubmit?: () => void
    /** Reports whether the mention menu is currently open, so a parent
     *  composer can block Enter-to-send while the user is picking a chip. */
    onMenuChange?: (open: boolean) => void
  }
>(function MentionInput({ value, onChange, disabled, onSubmit, onMenuChange }, ref) {
  const [chips, setChips] = useState<Chip[]>([])
  const [hit, setHit] = useState<ReturnType<typeof detectMention> | null>(null)
  const [active, setActive] = useState(0)
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const valueRef = useRef(value)
  valueRef.current = value
  const chipsRef = useRef(chips)
  chipsRef.current = chips

  useImperativeHandle(ref, () => ({
    serializeValue: () => serialize(valueRef.current, chipsRef.current).serialized,
  }))

  // Partition the editor text into plain segments and chip slots.
  const segments = useMemo(() => {
    const segs: { text: string; chip?: Chip }[] = []
    value.split(PLACEHOLDER).forEach((p, i) => {
      if (p) segs.push({ text: p })
      const chip = chips[i]
      segs.push({ text: chip ? chip.label : '', chip })
    })
    return segs
  }, [value, chips])

  const onTextChange = (v: string) => {
    onChange(v)
    const pos = inputRef.current?.selectionStart ?? v.length
    const m = detectMention(v, pos)
    setHit(m && m.active ? m : null)
    setActive(0)
  }

  const candidates = useMemo(() => matchCandidates(DEMO_CANDIDATES, hit?.query ?? ''), [hit])
  const menuOpen = hit !== null && candidates.length > 0

  // Notify parent when menu opens/closes so it can block Enter-to-send.
  useEffect(() => {
    onMenuChange?.(menuOpen)
  }, [menuOpen, onMenuChange])

  const pick = (candidate: Candidate) => {
    if (!hit) return
    const chip: Chip = { id: candidate.id, label: candidate.label, kind: candidate.kind, path: candidate.label }
    const { text } = insertChip(value, hit.start, hit.end, chip)
    setChips([...chips, chip])
    onChange(text)
    setHit(null)
    // Focus returns to the textarea after the macro task.
    requestAnimationFrame(() => inputRef.current?.focus())
  }

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (menuOpen) {
      if (e.key === 'ArrowDown') { e.preventDefault(); setActive((a) => (a + 1) % candidates.length); return }
      if (e.key === 'ArrowUp') { e.preventDefault(); setActive((a) => (a - 1 + candidates.length) % candidates.length); return }
      if (e.key === 'Enter' || e.key === 'Tab') { e.preventDefault(); pick(candidates[active]); return }
      if (e.key === 'Escape') { e.preventDefault(); setHit(null); return }
    }
    // Menu closed: Enter (no Shift) submits, Ctrl/Cmd+Enter too as a fallback.
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      onSubmit?.()
    }
  }

  return (
    <div className={styles.inputWrap}>
      <div className={styles.backdrop + ' ' + styles.backdropPainted} aria-hidden="true">
        {segments.map((s, i) =>
          s.chip ? (
            <span key={i} className={styles.mention}>{s.chip.label}</span>
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
        onChange={(e) => onTextChange(e.target.value)}
        onKeyDown={onKeyDown}
        onKeyUp={() => {
          const el = inputRef.current
          if (!el) return
          // Re-evaluate mention detection as the caret moves.
          const m = detectMention(el.value, el.selectionStart ?? el.value.length)
          setHit(m && m.active ? m : null)
        }}
        placeholder="…"
      />
      {menuOpen && (
        <div className={styles.menu} data-testid="mention-menu">
          <div className={styles.menuHead}>@ 引用 · {candidates.length} 项</div>
          {candidates.map((c, i) => (
            <div
              key={c.id}
              className={styles.menuItem + (i === active ? ' ' + styles.menuActive : '')}
              onClick={() => pick(c)}
              onMouseEnter={() => setActive(i)}
            >
              <span className={styles.menuKind} style={{ color: KIND_COLOR[c.kind], background: KIND_COLOR[c.kind] + '22' }}>
                {KIND_LABEL[c.kind]}
              </span>
              <span className={styles.menuLabel}>{c.label}</span>
              {c.desc && <span className={styles.menuDesc}>{c.desc}</span>}
            </div>
          ))}
        </div>
      )}
    </div>
  )
})