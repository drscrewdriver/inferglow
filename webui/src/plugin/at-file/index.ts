/**
 * @file 插件入口。
 *
 * registerAtFilePlugin() 把候选下拉注册进 `conversation.input.menu` 槽位
 * （single 基数），便于 Task7 集成时在对话输入区摆放。宿主输入框用 MentionInput
 * 包装任意 textarea，即可获得 `@` 触发文件/目录引用能力。
 */
import { createElement } from 'react'
import { registerSlot, setCardinality } from '../registry'
import { MentionMenu } from './MentionInput'

export { MentionInput, MentionMenu } from './MentionInput'
export {
  PLACEHOLDER,
  encodeFileChip,
  decodeFileChips,
  serialize,
  insertChip,
  type Chip,
} from './chips'
export { detectMention, matchCandidates, type Candidate, type TriggerHit } from './trigger'
export { fetchWorkspaceFiles, DEFAULT_WORKSPACE, type FileEntry } from './fileApi'

/** 槽位约定（Task7 集成依据）：候选下拉挂载点。 */
export const AT_FILE_MENU_SLOT = 'conversation.input.menu'

/**
 * 注册 @file 插件。返回清理函数。
 * - 将 `conversation.input.menu` 置为 single 基数并注册候选下拉；
 * - MentionInput 由 host 直接作为受控组件使用，无需在此注册。
 */
export function registerAtFilePlugin(): () => void {
  setCardinality(AT_FILE_MENU_SLOT, 'single')
  return registerSlot<Record<string, unknown>>(
    AT_FILE_MENU_SLOT,
    () => createElement(MentionMenu),
    { id: 'at-file:menu', order: 10 },
  )
}