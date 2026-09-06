import { Fragment, type ReactNode } from 'react'

/**
 * Lightweight Slot/Plugin system (no third-party deps).
 *
 * - `registerSlot(name, {order, render})` adds a renderable into a named slot.
 * - `renderSlot(name, props)` returns the ordered list of rendered nodes.
 * - `<SlotOutlet name props/>` is the declarative JSX version.
 *
 * Each registration is keyed by an id (auto-generated unless supplied) and
 * rendered in ascending `order`. Re-registering the same id replaces the
 * previous entry; registering a new id appends it. Registrations are global
 * and survive across hot-reloads (idempotent by id).
 */
export interface SlotRegistration<P extends object | undefined = undefined> {
  id: string
  order: number
  render: P extends undefined ? (props?: object) => ReactNode : (props: P) => ReactNode
}

const registry = new Map<string, Map<string, SlotRegistration>>()
let uid = 0

export interface RegisterSlotOptions {
  /** Stable identity for dedupe/update. Defaults to an auto-incremented id. */
  id?: string
  /** Lower renders first. Default 0. */
  order?: number
}

/** Registered render callback; receives the props passed at render time. */
export type SlotRender<P extends object | undefined> = (
  props: P extends undefined ? object | undefined : P,
) => ReactNode

/**
 * Register a plugin into a named slot. Returns an unregister function.
 * Safe to call at module top-level or inside an effect.
 */
export function registerSlot<P extends object | undefined>(
  name: string,
  render: SlotRender<P>,
  options: RegisterSlotOptions = {},
): () => void {
  const { id = `slot:${name}:${++uid}`, order = 0 } = options
  getSlot(name).set(id, { id, order, render } as SlotRegistration)
  return () => unregisterSlot(name, id)
}

/** Remove a specific registration from a slot. */
export function unregisterSlot(name: string, id: string): void {
  getSlot(name).delete(id)
}

/** Replace every registration in a slot with a single render callback. */
export function setSlot<P extends object | undefined>(
  name: string,
  render: SlotRender<P>,
  order = 0,
): () => void {
  clearSlot(name)
  return registerSlot(name, render, { order })
}

/** Remove all registrations from a slot. */
export function clearSlot(name: string): void {
  registry.delete(name)
}

function getSlot(name: string): Map<string, SlotRegistration> {
  let slot = registry.get(name)
  if (!slot) {
    slot = new Map()
    registry.set(name, slot)
  }
  return slot
}

/** Returns the render callbacks for a slot, sorted by order ascending. */
export function slotOrders(name: string): SlotRegistration[] {
  const slot = registry.get(name)
  return slot ? [...slot.values()].sort((a, b) => a.order - b.order) : []
}

/** Render every item in a slot, in ascending order. */
export function renderSlot<P extends object>(name: string, props?: P): ReactNode {
  return slotOrders(name).map((reg) => <Fragment key={reg.id}>{reg.render(props)}</Fragment>)
}

export interface SlotOutletProps<P extends object> {
  /** Slot name to render. */
  name: string
  /** Props forwarded to every registered render callback. */
  props?: P
  /** Node rendered when the slot has no registrations. */
  fallback?: ReactNode
}

/** Declarative slot outlet used in composable panes. */
export function SlotOutlet<P extends object>({ name, props, fallback }: SlotOutletProps<P>): ReactNode {
  const items = slotOrders(name)
  if (items.length === 0) return fallback ?? null
  return items.map((reg) => <Fragment key={reg.id}>{reg.render(props)}</Fragment>)
}