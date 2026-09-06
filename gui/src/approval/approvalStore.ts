// approval/approvalStore.ts — Web UI approval system (Phase 6, Task 22). Wires
// to the existing /v1/approvals + /v1/approvals/{id}/decision endpoints (Phase
// 1, Task 5). Pending records are surfaced as cards in the details panel where
// the user can allow/reject with an optional justification.

import { create } from 'zustand'
import { transport as defaultTransport, type ApprovalRecord, type ApprovalRequest, type Transport } from '../api'

export interface ApprovalDecisionInput {
  recordId: string
  approve: boolean
  approver?: string
  justification?: string
}

interface ApprovalState {
  /** Pending (unreplied) approval records, newest first. */
  pending: ApprovalRecord[]
  /** Recently resolved records (history), newest first. */
  history: ApprovalRecord[]
  loading: boolean
  error: string | null
  fetch: () => Promise<void>
  /** Resolve a record via POST /v1/approvals/{id}/decision. */
  decide: (input: ApprovalDecisionInput) => Promise<boolean>
  /** Submit a new approval request (POST /v1/approvals). */
  submit: (req: ApprovalRequest) => Promise<ApprovalRecord | null>
}

export const createApprovalStore = (t: Transport = defaultTransport) =>
  create<ApprovalState>()((set, get) => ({
    pending: [],
    history: [],
    loading: false,
    error: null,

    async fetch() {
      set({ loading: true, error: null })
      try {
        const resp = await t.request<{ approvals: ApprovalRecord[]; count: number }>('GET', '/approvals')
        const approvals = resp.approvals ?? []
        set({
          pending: approvals.filter((a) => a.status === 'pending'),
          history: approvals.filter((a) => a.status !== 'pending'),
          loading: false,
        })
      } catch (err) {
        set({ loading: false, error: err instanceof Error ? err.message : String(err) })
      }
    },

    async decide({ recordId, approve, approver, justification }) {
      try {
        await t.request('POST', `/approvals/${recordId}/decision`, {
          approve,
          approver: approver ?? 'webui',
          justification: justification ?? undefined,
        })
        const rec = get().pending.find((a) => a.id === recordId)
        if (rec) {
          set({
            pending: get().pending.filter((a) => a.id !== recordId),
            history: [
              { ...rec, status: approve ? 'approved' : 'denied', approver: approver ?? 'webui' },
              ...get().history,
            ],
          })
        }
        return true
      } catch (err) {
        set({ error: err instanceof Error ? err.message : String(err) })
        return false
      }
    },

    async submit(req) {
      try {
        const rec = await t.request<ApprovalRecord>('POST', '/approvals', req)
        if (rec.status === 'pending') {
          set({ pending: [rec, ...get().pending] })
        } else {
          set({ history: [rec, ...get().history] })
        }
        return rec
      } catch (err) {
        set({ error: err instanceof Error ? err.message : String(err) })
        return null
      }
    },
  }))