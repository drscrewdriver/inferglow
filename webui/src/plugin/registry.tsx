/**
 * InferGlow WebUI — DSH 风格 Slot 插件注册系统
 *
 * 支持四种基数（cardinality）：
 *   - single  : 只渲染 order 最小的一个注册
 *   - list    : 按 order 升序渲染全部注册（默认）
 *   - keyed   : 仅渲染 key === 渲染时传入 key 的注册（用于设置面板等 keyed 分区）
 *   - chain   : 同 list，但链式覆盖场景用 chainFinal() 取最后一个非空节点
 *
 * 每个注册以 id 去重；重复注册同 id 会替换旧项。注册全局有效，热重载幂等。
 */
import { Fragment, type ReactNode } from 'react'

export type SlotCardinality = 'single' | 'list' | 'keyed' | 'chain'

export interface SlotRegistration<P extends Record<string, unknown> = Record<string, unknown>> {
  id: string
  order: number
  /** keyed 基数下用于匹配渲染 key */
  key?: string
  render: (props: P) => ReactNode
}

export interface RegisterOptions {
  /** 稳定身份，用于去重/更新；缺省自动生成 */
  id?: string
  /** 值越小渲染越靠前。默认 0。 */
  order?: number
  /** keyed 基数下的匹配 key */
  key?: string
}

const registry = new Map<string, Map<string, SlotRegistration>>()
const cardinality = new Map<string, SlotCardinality>()
let uid = 0

/** 声明或以 host 身份设置某 slot 的基数（幂等）。 */
export function setCardinality(name: string, kind: SlotCardinality): void {
  cardinality.set(name, kind)
}

export function getCardinality(name: string): SlotCardinality {
  return cardinality.get(name) ?? 'list'
}

/** 注册一个渲染回调到一个命名 slot。返回注销函数。 */
export function registerSlot<P extends Record<string, unknown>>(
  name: string,
  render: (props: P) => ReactNode,
  options: RegisterOptions = {},
): () => void {
  const { id = `slot:${name}:${++uid}`, order = 0, key } = options
  const slot = getOrCreate(name)
  slot.set(id, { id, order, key, render } as SlotRegistration)
  return () => unregisterSlot(name, id)
}

export function unregisterSlot(name: string, id: string): void {
  registry.get(name)?.delete(id)
}

/** 清空某个 slot 的全部注册。 */
export function clearSlot(name: string): void {
  registry.delete(name)
  cardinality.delete(name)
}

/** 用单个渲染回调替换某个 slot 的全部注册。 */
export function setSlot<P extends Record<string, unknown>>(
  name: string,
  render: (props: P) => ReactNode,
  options: Omit<RegisterOptions, 'id'> & { id?: string } = {},
): () => void {
  clearSlot(name)
  return registerSlot(name, render, { ...options, id: options.id ?? `${name}:default` })
}

export function slotEntries(name: string): SlotRegistration[] {
  const slot = registry.get(name)
  return slot
    ? [...slot.values()].sort((a, b) => a.order - b.order)
    : []
}

export interface RenderOptions {
  /** keyed 基数下只渲染此 key 的注册 */
  key?: string
}

/** 按 slot 的基数渲染，返回节点数组。 */
export function renderSlot<N extends Record<string, unknown>>(
  name: string,
  props?: N,
  opts?: RenderOptions,
): ReactNode[] {
  const entries = slotEntries(name)
  if (entries.length === 0) return []
  const kind = getCardinality(name)
  if (kind === 'single') {
    const first = entries[0]
    return [<Fragment key={first.id}>{first.render(props ?? ({} as N))}</Fragment>]
  }
  if (kind === 'keyed') {
    const k = opts?.key
    // 把 key 同时注入 props：keyed 注册的渲染函数通常从 props.key 判断命中，
    // 而过滤用的是 renderSlot 的 opts.key。两者必须一致才渲染。
    const keyedProps = (props ?? ({} as N)) as N & { key?: string }
    return entries
      .filter((r) => r.key === k)
      .map((r) => (
        <Fragment key={r.id}>
          {r.render(k ? { ...keyedProps, key: k } : keyedProps)}
        </Fragment>
      ))
  }
  return entries.map((r) => <Fragment key={r.id}>{r.render(props ?? ({} as N))}</Fragment>)
}

/** chain 基数：返回最后一个非空节点的渲染结果（用于链式覆盖）。 */
export function chainFinal<N extends Record<string, unknown>>(
  name: string,
  props?: N,
): ReactNode {
  const entries = slotEntries(name)
  if (entries.length === 0) return null
  let last: ReactNode = null
  for (const r of entries) {
    const node = r.render(props ?? ({} as N))
    if (node != null) last = node
  }
  return last
}

function getOrCreate(name: string): Map<string, SlotRegistration> {
  let slot = registry.get(name)
  if (!slot) {
    slot = new Map()
    registry.set(name, slot)
  }
  return slot
}