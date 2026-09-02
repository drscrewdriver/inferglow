// Settings schema — single source of truth is prototypes/inferglow-gui/
// settings-spec.md (4 groups, 15 tabs). Local-preference fields persist to
// localStorage via settingsStore; server-backed tabs (providers/models/
// credentials/skills-tools/connectors/schedules/usage/about) are wired to
// REST endpoints in a later commit.

export const SETTINGS_VERSION = 'inferglow.settings.v1'

export interface AppSettings {
  // general
  defaultModel: string
  webSearch: boolean
  writeProbe: boolean
  autoCompact: boolean
  restoreLast: boolean
  showOnboarding: boolean
  language: 'zh' | 'en'
  autoUpdate: boolean
  telemetry: boolean
  // appearance
  themeKey: string
  themeMode: 'auto' | 'light' | 'dark'
  accent: string
  fontUi: string
  fontMono: string
  fontSize: number
  zoom: number
  // interface
  contentWidth: 'standard' | 'full'
  dockVisible: boolean
  sidebarVisible: boolean
  showTimestamps: boolean
  showThought: boolean
  toolAutoExpand: boolean
  termDrawer: boolean
  termFont: string
  // shortcuts (display-only kbd rows; key names follow settings-spec)
  shortcuts: Record<string, string>
  // models
  modelDefault: string
  modelTemp: number
  modelMaxTokens: number
  // memory
  reasonLevel: 'low' | 'mid' | 'high'
  streamEcho: boolean
  showReason: boolean
  compressLevel: 'L0' | 'L1' | 'L2' | 'L3'
  rtkEnable: boolean
  lockL0: boolean
  autoCompactThreshold: number
  // permissions
  cmdApproval: boolean
  sandboxTools: boolean
  netOutbound: boolean
  netDownload: boolean
  // security
  auditEnabled: boolean
  auditPath: string
  auditBeforeValidate: boolean
  cdpStealth: boolean
  cdpAutomation: boolean
  encryptAtRest: boolean
  /** GUI request auth token (Bearer); synced to localStorage 'inferglow.apikey'. */
  apiKey: string
  // skills-tools
  toolConfirmDestructive: boolean
  // connectors
  browserProfile: boolean
  browserPartition: boolean
  // experiments
  expGoalAutoresearch: boolean
  expCrossStepFusion: boolean
  expBridgeComposer: boolean
}

export const DEFAULT_SETTINGS: AppSettings = {
  defaultModel: 'deepseek-chat',
  webSearch: true,
  writeProbe: true,
  autoCompact: true,
  restoreLast: true,
  showOnboarding: false,
  language: 'zh',
  autoUpdate: true,
  telemetry: false,
  themeKey: 'midnight',
  themeMode: 'dark',
  accent: 'moss',
  fontUi: 'system',
  fontMono: 'mono',
  fontSize: 14,
  zoom: 100,
  contentWidth: 'standard',
  dockVisible: true,
  sidebarVisible: true,
  showTimestamps: true,
  showThought: true,
  toolAutoExpand: false,
  termDrawer: true,
  termFont: 'mono',
  shortcuts: {
    k_cmd_palette: '⌘ K',
    k_goal_mode: '⌘ G',
    k_settings: '⌘ ,',
    k_new: '⌘ N',
    k_term: '⌘ T',
    k_send: 'Enter',
    k_newline: 'Shift + Enter',
    k_next: '⌘ ↓',
    k_prev: '⌘ ↑',
    k_focus: '⌘ L',
  },
  modelDefault: 'deepseek-chat',
  modelTemp: 1,
  modelMaxTokens: 8192,
  reasonLevel: 'mid',
  streamEcho: true,
  showReason: true,
  compressLevel: 'L1',
  rtkEnable: false,
  lockL0: false,
  autoCompactThreshold: 85,
  cmdApproval: false,
  sandboxTools: true,
  netOutbound: true,
  netDownload: false,
  auditEnabled: true,
  auditPath: 'audit.log',
  auditBeforeValidate: true,
  cdpStealth: true,
  cdpAutomation: true,
  encryptAtRest: true,
  apiKey: '',
  toolConfirmDestructive: true,
  browserProfile: true,
  browserPartition: true,
  expGoalAutoresearch: false,
  expCrossStepFusion: false,
  expBridgeComposer: false,
}

/** Tab registry matching settings-spec.md. dataSource marks which tabs bind
 * to server REST endpoints (wired in a later commit) vs localStorage. */
export interface SettingsTab {
  key: string
  label: string
  sub: string
  group: string
  dataSource: 'local' | 'server' | 'mixed'
}

export const SETTINGS_TABS: SettingsTab[] = [
  { key: 'general', label: '通用', sub: '偏好 · 启动', group: '常规', dataSource: 'local' },
  { key: 'appearance', label: '外观', sub: '主题 · 排版', group: '常规', dataSource: 'local' },
  { key: 'interface', label: '界面', sub: '布局 · 显示', group: '常规', dataSource: 'local' },
  { key: 'shortcuts', label: '快捷键', sub: '键位映射', group: '常规', dataSource: 'local' },
  { key: 'providers', label: '提供方', sub: '模型提供方 · 用量', group: '模型与智能', dataSource: 'mixed' },
  { key: 'models', label: '模型', sub: '默认模型 · 上下文窗口', group: '模型与智能', dataSource: 'mixed' },
  { key: 'credentials', label: '凭证', sub: 'API Keys', group: '模型与智能', dataSource: 'server' },
  { key: 'memory', label: '推理与上下文', sub: '思考等级 · 压缩 · RTK', group: '模型与智能', dataSource: 'local' },
  { key: 'permissions', label: '权限', sub: '执行 · 审批 · 沙箱', group: '执行与安全', dataSource: 'local' },
  { key: 'security', label: '安全', sub: '审计 · 浏览器指纹', group: '执行与安全', dataSource: 'mixed' },
  { key: 'skills-tools', label: '技能与工具', sub: '技能库 · 工具开关', group: '集成', dataSource: 'mixed' },
  { key: 'connectors', label: '连接器', sub: 'MCP · 桥接 · 浏览器', group: '集成', dataSource: 'mixed' },
  { key: 'schedules', label: '调度', sub: '定时任务 · Webhook', group: '集成', dataSource: 'server' },
  { key: 'experiments', label: '实验与插件', sub: '实验特性 · 插件', group: '高级', dataSource: 'local' },
  { key: 'about', label: '关于', sub: '版本 · 许可', group: '高级', dataSource: 'local' },
]
