// sandbox/schema.ts — runtime sandbox mode, permission preset & shell env
// constants for the Web UI (Phase 6, Task 21; align the spec's program table).
// The actual sandbox engine lives in the inferglow main binary; the GUI only
// selects/describes the modes and surfaces escalation requests.

/** Inferglow sandbox modes (mirrors the Go SandboxMode enum). */
export type SandboxMode = 'trusted_local' | 'local' | 'docker' | 'gvisor' | 'auto'

export interface SandboxModeInfo {
  key: SandboxMode
  label: string
  hint: string
  /** Risk tier shown in the selector (lower = safer). */
  risk: 'safe' | 'medium' | 'high'
}

export const SANDBOX_MODES: SandboxModeInfo[] = [
  { key: 'local', label: '本地', hint: '受控本地执行（Landlock/seatbelt/WRITE_RESTRICTED）', risk: 'medium' },
  { key: 'trusted_local', label: '可信本地', hint: '全权本地进程，不隔离', risk: 'high' },
  { key: 'docker', label: 'Docker', hint: '容器隔离执行', risk: 'safe' },
  { key: 'gvisor', label: 'gVisor', hint: 'runsc 高隔离运行时', risk: 'safe' },
  { key: 'auto', label: '自动', hint: '按可用后端自动选择', risk: 'medium' },
]

/** Filesystem effect permission presets. */
export type PermissionPreset = 'read-only' | 'workspace-write' | 'full-access'

export interface PermissionPresetInfo {
  key: PermissionPreset
  label: string
  hint: string
}

export const PERMISSION_PRESETS: PermissionPresetInfo[] = [
  { key: 'read-only', label: '只读', hint: '禁写文件系统' },
  { key: 'workspace-write', label: '工作区写入', hint: '仅可写当前工作区' },
  { key: 'full-access', label: '完全访问', hint: '不受限（危险）' },
]

/** Shell environments offered to the executor (align spec §7 table). */
export type ShellEnv = 'bash' | 'powershell' | 'cmd' | 'wsl'

export interface ShellEnvInfo {
  key: ShellEnv
  label: string
  hint: string
}

export const SHELL_ENVS: ShellEnvInfo[] = [
  { key: 'bash', label: 'bash', hint: 'Linux / macOS / Git Bash' },
  { key: 'powershell', label: 'PowerShell', hint: 'Windows (pwsh→5.1 自动探测)' },
  { key: 'cmd', label: 'cmd', hint: 'Windows 传统 cmd.exe' },
  { key: 'wsl', label: 'WSL', hint: 'Windows 子系统 Linux' },
]

/** Map a sandbox mode to its risk label for escalation UI. */
export function modeInfo(key: SandboxMode): SandboxModeInfo {
  return SANDBOX_MODES.find((m) => m.key === key) ?? SANDBOX_MODES[0]
}

/** True when switching to this mode is a downgrade in isolation (riskier). */
export function isRiskier(from: SandboxMode, to: SandboxMode): boolean {
  const rank: Record<SandboxModeInfo['risk'], number> = { safe: 0, medium: 1, high: 2 }
  return rank[modeInfo(to).risk] > rank[modeInfo(from).risk]
}

export interface SandboxConfig {
  mode: SandboxMode
  preset: PermissionPreset
  shell: ShellEnv
  /** When true, a riskier mode/preset change requires an approval popup. */
  requireEscalationApproval: boolean
}

export const DEFAULT_SANDBOX_CONFIG: SandboxConfig = {
  mode: 'auto',
  preset: 'workspace-write',
  shell: 'bash',
  requireEscalationApproval: true,
}