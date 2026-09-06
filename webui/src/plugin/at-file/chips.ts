/**
 * @file Chip 模型与文件引用编解码（移植自 gui/src/filetag）。
 *
 * 编辑器底层仅持有一条文本串，每个 Chip 在正文中以一个 U+FFFC 占位符表示，
 * 保持光标/键盘度量 1:1。serialize 把占位符回填为稳定的文件引用结构
 * （PUA 哨兵包裹），decodeFileChips 可从序列化后的 block 中解回 Chip。
 */

/** U+FFFC —— OBJECT REPLACEMENT CHARACTER：Chip 在正文中占用的单字符。 */
export const PLACEHOLDER = '\uFFFC'

/** 文件引用结构的定界哨兵（PUA 区，正常路径不会出现）。 */
const REF_START = '\uE010'
const REF_SEP = '\uE011'
const REF_END = '\uE012'

export type ChipKind = 'file' | 'dir'

export interface Chip {
  id: string
  label: string
  kind: ChipKind
  /** 工作区相对路径。 */
  path: string
  /** 所引用文件已知已删除/失效时置位。 */
  invalid?: boolean
}

export interface Occurrence {
  offset: number
  chip: Chip
}

/** 扫描文本构建占位符出位表。 */
export function buildOccurrences(text: string): Occurrence[] {
  const out: Occurrence[] = []
  let idx = text.indexOf(PLACEHOLDER)
  while (idx !== -1) {
    const chip: Chip = { id: `chip:${out.length}`, label: `#${out.length}`, kind: 'file', path: '' }
    out.push({ offset: idx, chip })
    idx = text.indexOf(PLACEHOLDER, idx + 1)
  }
  return out
}

/** 把单个 Chip 编码为稳定的文件引用结构（可写入序列化后的消息）。 */
export function encodeFileChip(chip: Chip): string {
  return `${REF_START}${chip.kind}${REF_SEP}${encodeURIComponent(chip.path)}${REF_END}`
}

/** 从一段文本中扫描出全部内嵌的文件引用，按出现顺序返回 Chip。 */
export function decodeFileChips(text: string): Chip[] {
  const chips: Chip[] = []
  let i = 0
  while (i < text.length) {
    const start = text.indexOf(REF_START, i)
    if (start === -1) break
    const end = text.indexOf(REF_END, start)
    if (end === -1) break
    const body = text.slice(start + REF_START.length, end)
    const sep = body.indexOf(REF_SEP)
    const kindPart = sep === -1 ? '' : body.slice(0, sep)
    const raw = sep === -1 ? body : body.slice(sep + REF_SEP.length)
    let path = raw
    try {
      path = decodeURIComponent(raw)
    } catch {
      // 保留原始串，容忍未编码内容。
    }
    const kind: ChipKind = kindPart === 'dir' ? 'dir' : 'file'
    chips.push({ id: `chip:${chips.length}`, label: path, kind, path })
    i = end + REF_END.length
  }
  return chips
}

export interface SerializeResult {
  /** 占位符被替换为其引用负载后的文本。 */
  serialized: string
  /** 内嵌 Chip 数量。 */
  count: number
}

/**
 * 编解码：遍历正文，将每个占位符替换为对应 Chip 的文件引用结构。
 * 非目录用 @file 语义：目录在引用中带出 kind。
 */
export function serialize(text: string, chips: Chip[]): SerializeResult {
  const occs = buildOccurrences(text)
  if (occs.length === 0) return { serialized: text, count: 0 }
  const bySlot = new Map<number, Chip>()
  for (let i = 0; i < occs.length; i++) {
    const chip = chips[i]
    if (chip) bySlot.set(occs[i].offset, chip)
  }
  let out = ''
  let last = 0
  for (const { offset } of occs) {
    out += text.slice(last, offset)
    const chip = bySlot.get(offset)
    out += chip ? encodeFileChip(chip) : ''
    last = offset + 1
  }
  out += text.slice(last)
  return { serialized: out, count: occs.length }
}

export interface ReplaceResult {
  text: string
  chip: Chip
}

/**
 * 用单个占位符替换 `@query` 区间 [start, end)，并记录 Chip。
 * 返回新正文与该 Chip 供调用方存储。
 */
export function insertChip(text: string, start: number, end: number, chip: Chip): ReplaceResult {
  const next = text.slice(0, start) + PLACEHOLDER + text.slice(end)
  return { text: next, chip: { ...chip } }
}

/** 在给定 offset 处把占位符替换为其 label 文本。 */
export function replaceChipLabelAt(text: string, offset: number, label: string): string {
  return text.slice(0, offset) + label + text.slice(offset + 1)
}