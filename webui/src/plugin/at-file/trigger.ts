/**
 * @file 触发检测（移植自 gui/src/filetag）——纯文本扫描逻辑。
 *
 * 从光标位置向回扫描，判断是否存在一个处于词边界处、由 `@` 激活的文件/目录
 * 提及触发点，并给出候选菜单应据以过滤的部分查询（query）与插入区间。
 */

/** 触发字符：`@`，用于文件/目录提及。 */
export const MENTION_CHAR = '@'

export interface TriggerHit {
  /** 光标前是否有活动的提及。 */
  active: boolean
  /** '@' 之后的查询文本（无输入时为 ''）。 */
  query: string
  /** '@' 在源串中的索引。 */
  start: number
  /** 光标位置（部分 token 的结束）。 */
  end: number
}

/** '@' 之前的字符属于词边界时（空白/行首/标点）才视为触发。 */
const BOUNDARY_CHARS = " \t/()[]{}'\"`,.:;=\\<>-"

function isWordBoundary(text: string, at: number): boolean {
  if (at <= 0) return true
  const before = text[at - 1]
  return /\s/.test(before) || BOUNDARY_CHARS.includes(before)
}

/**
 * 从 `cursor` 向回扫描是否命中 `@` 提及。若 '@' 嵌在更长 token 内
 * （如 `foo@bar`），则返回未激活结果，避免误触发。
 */
export function detectMention(text: string, cursor: number): TriggerHit {
  const pos = Math.max(0, Math.min(cursor, text.length))
  // 从光标向回走到当前 token 起点。
  let i = pos
  while (i > 0 && !/[\s]/.test(text[i - 1])) i -= 1
  const tokenStart = i
  const token = text.slice(tokenStart, pos)
  const atIdx = token.lastIndexOf(MENTION_CHAR)
  if (atIdx === -1) {
    return { active: false, query: '', start: pos, end: pos }
  }
  const absStart = tokenStart + atIdx
  // `@` 必须位于词边界才计为提及触发点。
  if (!isWordBoundary(text, absStart)) {
    return { active: false, query: '', start: pos, end: pos }
  }
  const query = text.slice(absStart + 1, pos)
  return { active: true, query, start: absStart, end: pos }
}

export interface Candidate {
  id: string
  label: string
  kind: 'file' | 'dir'
  desc?: string
}

/** 对 query 做不区分大小写的包含匹配。 */
export function matchCandidates(candidates: Candidate[], query: string): Candidate[] {
  const ql = query.trim().toLowerCase()
  if (!ql) return candidates
  return candidates.filter((c) => c.label.toLowerCase().includes(ql))
}