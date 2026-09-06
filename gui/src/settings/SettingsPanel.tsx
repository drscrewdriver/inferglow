import { useState } from 'react'
import { SlotOutlet } from '../framework'
import { SETTINGS_TABS, type AppSettings } from './settingsSchema'
import { useSettingsStore } from './settingsStore'
import { THEME_GROUPS, THEMES } from '../theme/themes'
import { applyTheme } from '../theme/ThemeProvider'
import { RowList, useServerList, useServerResource } from './serverData'
import { transport, type AuditEntriesResult, type AuditVerifyResult, type CredentialRecord, type MCPToolRecord, type ScheduleRecord, type SkillRecord } from '../api'
import '../tidychat/slots' // registers settings.plugin.item (side effect)
import '../thinking/slots' // registers settings.plugin.item (side effect)
import '../sandbox/slots' // registers settings.plugin.item (side effect)

// ─── generic setting control primitives (mirror prototype helpers) ───

function SecHead({ title, sub }: { title: string; sub: string }) {
  return (
    <header className="s-head">
      <h2>{title}</h2>
      <div className="sub">{sub}</div>
    </header>
  )
}

function Card({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="card">
      <div className="card__head">
        <span className="t">{title}</span>
        <span className="spacer" />
      </div>
      <div className="card__body">{children}</div>
    </div>
  )
}

function SwitchRow({
  label,
  note,
  checked,
  onChange,
}: {
  label: string
  note?: string
  checked: boolean
  onChange: (v: boolean) => void
}) {
  return (
    <div className="switch" onClick={() => onChange(!checked)} role="switch" aria-checked={checked} tabIndex={0}>
      <div className="txt">
        <b>{label}</b>
        {note && <small>{note}</small>}
      </div>
      <div className={`knob${checked ? ' on' : ''}`} data-toggle />
    </div>
  )
}

function AppearanceRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="appearance-row">
      <div>
        <div className="lbl">{label}</div>
      </div>
      {children}
    </div>
  )
}

function SegRow({
  label,
  options,
  value,
  onChange,
}: {
  label: string
  options: string[]
  value: string
  onChange: (v: string) => void
}) {
  return (
    <AppearanceRow label={label}>
      <div className="set-seg">
        {options.map((o) => (
          <button key={o} className={`set-seg__btn${o === value ? ' set-seg__btn--on' : ''}`} onClick={() => onChange(o)}>
            {o}
          </button>
        ))}
      </div>
    </AppearanceRow>
  )
}

function RangeRow({
  label,
  min,
  max,
  value,
  lo,
  hi,
  onChange,
}: {
  label: string
  min: number
  max: number
  value: number
  lo: string
  hi: string
  onChange: (v: number) => void
}) {
  return (
    <AppearanceRow label={label}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <span style={{ fontSize: 11, color: 'var(--fg-faint)' }}>{lo}</span>
        <input
          type="range"
          min={min}
          max={max}
          value={value}
          onChange={(e) => onChange(Number(e.target.value))}
          style={{ width: 130, accentColor: 'var(--accent)' }}
        />
        <span style={{ fontSize: 11, color: 'var(--fg-faint)' }}>{hi}</span>
      </div>
    </AppearanceRow>
  )
}

function SelectRow({
  label,
  options,
  value,
  onChange,
}: {
  label: string
  options: string[]
  value: string
  onChange: (v: string) => void
}) {
  return (
    <AppearanceRow label={label}>
      <select value={value} onChange={(e) => onChange(e.target.value)} style={{ background: 'var(--bg-elev)', color: 'var(--fg)', border: '1px solid var(--border)', borderRadius: 7, padding: '4px 8px', fontSize: 12 }}>
        {options.map((o) => (
          <option key={o} value={o}>{o}</option>
        ))}
      </select>
    </AppearanceRow>
  )
}

function InputRow({ label, value, onChange }: { label: string; value: string; onChange: (v: string) => void }) {
  return (
    <AppearanceRow label={label}>
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        style={{ background: 'var(--bg-elev)', color: 'var(--fg)', border: '1px solid var(--border)', borderRadius: 7, padding: '4px 8px', fontSize: 12, width: 220 }}
      />
    </AppearanceRow>
  )
}

function KbdRow({ name, keys }: { name: string; keys: string }) {
  return (
    <div className="kbd-row">
      <span className="name">{name}</span>
      <span className="kbd">{keys}</span>
    </div>
  )
}

function ThemeGrid({ current, onPick }: { current: string; onPick: (key: string) => void }) {
  return (
    <>
      {THEME_GROUPS.map((group) => (
        <div key={group.label}>
          <div className="theme-group">{group.label}</div>
          <div className="theme-grid">
            {group.keys.map((k) => {
              const t = THEMES[k]
              return (
                <div
                  key={k}
                  className={`theme-card${k === current ? ' theme-card--active' : ''}`}
                  onClick={() => onPick(k)}
                >
                  <div className="theme-card__swatches">
                    {[t.bg, t.panel2, t.accent, t.text, t.gold ?? t.accent].map((c, i) => (
                      <span key={i} className="th-swatch" style={{ background: c }} />
                    ))}
                  </div>
                  <div className="theme-card__name">{t.name}</div>
                  <div className="theme-card__idea">{t.idea || t.mode || ''}</div>
                  <div className="theme-card__origin">
                    {t.origin || 'OpenHana 主题'}
                    <span className="dark-badge">{t.dark ? '深色' : '浅色'}</span>
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      ))}
    </>
  )
}

// ─── tab bodies ───

function GeneralTab({ s, set }: { s: AppSettings; set: (p: Partial<AppSettings>) => void }) {
  return (
    <>
      <SecHead title="通用" sub="偏好 · 启动 · 语言与更新" />
      <Card title="偏好">
        <SelectRow label="默认模型" options={['deepseek-chat', 'deepseek-reasoner', 'openai-compatible']} value={s.defaultModel} onChange={(v) => set({ defaultModel: v })} />
        <SwitchRow label="服务端网页搜索" note="deepseek 端检索，可按模型启用" checked={s.webSearch} onChange={(v) => set({ webSearch: v })} />
        <SwitchRow label="写入权限探测" note="未租约会话写入前探测归属" checked={s.writeProbe} onChange={(v) => set({ writeProbe: v })} />
        <SwitchRow label="自动缩略历史" note="接近上下文上限时自动 compact" checked={s.autoCompact} onChange={(v) => set({ autoCompact: v })} />
      </Card>
      <Card title="启动">
        <SwitchRow label="启动时恢复上次会话" checked={s.restoreLast} onChange={(v) => set({ restoreLast: v })} />
        <SwitchRow label="首次启动显示引导" checked={s.showOnboarding} onChange={(v) => set({ showOnboarding: v })} />
      </Card>
      <Card title="语言与更新">
        <SelectRow label="界面语言" options={['简体中文', 'English']} value={s.language === 'zh' ? '简体中文' : 'English'} onChange={(v) => set({ language: v === '简体中文' ? 'zh' : 'en' })} />
        <SwitchRow label="自动更新" checked={s.autoUpdate} onChange={(v) => set({ autoUpdate: v })} />
        <SwitchRow label="匿名遥测" checked={s.telemetry} onChange={(v) => set({ telemetry: v })} />
      </Card>
    </>
  )
}

function AppearanceTab({ s, set }: { s: AppSettings; set: (p: Partial<AppSettings>) => void }) {
  return (
    <>
      <SecHead title="外观" sub="主题 · 配色 · 排版 · 缩放" />
      <Card title="主题">
        <SegRow label="主题模式" options={['自动', '浅色', '深色']} value={s.themeMode === 'auto' ? '自动' : s.themeMode === 'light' ? '浅色' : '深色'} onChange={(v) => set({ themeMode: v === '自动' ? 'auto' : v === '浅色' ? 'light' : 'dark' })} />
        <div className="appearance-row" style={{ alignItems: 'flex-start' }}>
          <div>
            <div className="lbl">主题</div>
            <div className="note">两组对比：OpenHana 全套 12 套 + 原创 8 套</div>
          </div>
        </div>
        <ThemeGrid
          current={s.themeKey}
          onPick={(k) => {
            applyTheme(k)
            set({ themeKey: k })
          }}
        />
      </Card>
      <Card title="排版">
        <SelectRow label="字体" options={['系统默认', '微软雅黑', '苹方']} value={s.fontUi === 'system' ? '系统默认' : s.fontUi} onChange={(v) => set({ fontUi: v })} />
        <SelectRow label="等宽字体" options={['Cascadia Code', 'JetBrains Mono', 'SF Mono']} value={s.fontMono === 'mono' ? 'Cascadia Code' : s.fontMono} onChange={(v) => set({ fontMono: v })} />
        <RangeRow label="字号" min={12} max={20} value={s.fontSize} lo="12" hi={`${s.fontSize}px`} onChange={(v) => set({ fontSize: v })} />
      </Card>
      <Card title="缩放">
        <RangeRow label="显示缩放" min={50} max={200} value={s.zoom} lo="50%" hi="200%" onChange={(v) => set({ zoom: v })} />
      </Card>
    </>
  )
}

function InterfaceTab({ s, set }: { s: AppSettings; set: (p: Partial<AppSettings>) => void }) {
  return (
    <>
      <SecHead title="界面" sub="布局 · 消息 · 终端" />
      <Card title="布局">
        <SegRow label="会话内容宽度" options={['标准 960px', '全宽 90%']} value={s.contentWidth === 'standard' ? '标准 960px' : '全宽 90%'} onChange={(v) => set({ contentWidth: v === '标准 960px' ? 'standard' : 'full' })} />
        <SwitchRow label="右侧 dock 默认显示" checked={s.dockVisible} onChange={(v) => set({ dockVisible: v })} />
        <SwitchRow label="左侧会话栏默认显示" checked={s.sidebarVisible} onChange={(v) => set({ sidebarVisible: v })} />
      </Card>
      <Card title="消息">
        <SwitchRow label="显示消息时间戳" checked={s.showTimestamps} onChange={(v) => set({ showTimestamps: v })} />
        <SwitchRow label="显示思考帧" checked={s.showThought} onChange={(v) => set({ showThought: v })} />
        <SwitchRow label="工具卡片自动展开输出" checked={s.toolAutoExpand} onChange={(v) => set({ toolAutoExpand: v })} />
      </Card>
      <Card title="终端">
        <SwitchRow label="终端抽屉吸附" checked={s.termDrawer} onChange={(v) => set({ termDrawer: v })} />
        <SelectRow label="终端字体" options={['Cascadia Code', 'JetBrains Mono', 'SF Mono']} value={s.termFont === 'mono' ? 'Cascadia Code' : s.termFont} onChange={(v) => set({ termFont: v })} />
      </Card>
      <SlotOutlet name="settings.plugin.item" />
    </>
  )
}

function ShortcutsTab({ s }: { s: AppSettings }) {
  const rows: [string, string][] = [
    ['打开命令面板', s.shortcuts.k_cmd_palette],
    ['切换目标模式', s.shortcuts.k_goal_mode],
    ['打开设置', s.shortcuts.k_settings],
    ['新建会话', s.shortcuts.k_new],
    ['切换终端抽屉', s.shortcuts.k_term],
    ['发送消息', s.shortcuts.k_send],
    ['换行', s.shortcuts.k_newline],
    ['下一个会话', s.shortcuts.k_next],
    ['上一个会话', s.shortcuts.k_prev],
    ['聚焦输入框', s.shortcuts.k_focus],
  ]
  return (
    <>
      <SecHead title="快捷键" sub="键位映射 · 展示只读" />
      <Card title="全局快捷键">
        {rows.slice(0, 7).map(([n, k]) => (
          <KbdRow key={n} name={n} keys={k} />
        ))}
      </Card>
      <Card title="导航快捷键">
        {rows.slice(7).map(([n, k]) => (
          <KbdRow key={n} name={n} keys={k} />
        ))}
      </Card>
    </>
  )
}

function ProvidersTab() {
  const report = useServerResource<import('../api').CacheReport>('/usage/report')
  const overall = report.data?.overall
  return (
    <>
      <SecHead title="提供方" sub="模型提供方 · 用量" />
      <Card title="提供方列表">
        <div className="cred-row"><span className="name">deepseek</span><span className="st">已连接</span></div>
        <div className="cred-row"><span className="name">openai-compatible</span><span className="st st--warning">未配置</span></div>
      </Card>
      <Card title="用量统计">
        <div className="appearance-row">
          <span className="lbl">本月 tokens</span>
          <b style={{ fontFamily: 'var(--font-mono)' }}>{overall ? (overall.total_prompt_tokens + overall.total_cached_tokens).toLocaleString() : '—'}</b>
        </div>
        <div className="appearance-row">
          <span className="lbl">缓存命中率</span>
          <b style={{ fontFamily: 'var(--font-mono)', color: 'var(--ok)' }}>{overall ? `${(overall.cache_hit_rate * 100).toFixed(1)}%` : '—'}</b>
        </div>
        <div className="appearance-row">
          <span className="lbl">本月费用</span>
          <b style={{ fontFamily: 'var(--font-mono)' }}>{overall ? `$${overall.actual_cost.toFixed(4)}` : '—'}</b>
        </div>
        {report.error && <div className="note" style={{ color: 'var(--err)' }}>{report.error}</div>}
      </Card>
    </>
  )
}

function ModelsTab({ s, set }: { s: AppSettings; set: (p: Partial<AppSettings>) => void }) {
  return (
    <>
      <SecHead title="模型" sub="默认模型 · 上下文窗口" />
      <Card title="默认">
        <SelectRow label="默认对话模型" options={['deepseek-chat', 'deepseek-reasoner', 'openai-compatible/gpt']} value={s.modelDefault} onChange={(v) => set({ modelDefault: v })} />
        <RangeRow label="温度" min={0} max={2} value={Math.round(s.modelTemp * 10)} lo="0" hi={`${s.modelTemp.toFixed(1)}`} onChange={(v) => set({ modelTemp: v / 10 })} />
        <RangeRow label="最大输出 tokens" min={512} max={32768} value={s.modelMaxTokens} lo="512" hi={`${s.modelMaxTokens}`} onChange={(v) => set({ modelMaxTokens: v })} />
      </Card>
      <Card title="模型列表">
        <div className="cred-row"><span className="name">deepseek-chat</span><span className="st">ctx 128k</span></div>
        <div className="cred-row"><span className="name">deepseek-reasoner</span><span className="st">ctx 128k</span></div>
        <div className="cred-row"><span className="name">openai-compatible/gpt</span><span className="st st--warning">ctx 取决于提供方</span></div>
      </Card>
    </>
  )
}

function CredentialsTab() {
  const { items, loading, error, reload } = useServerList<CredentialRecord>('/credentials')
  return (
    <>
      <SecHead title="凭证" sub="API Keys · 加密存储" />
      <Card title="已保存凭证">
        <RowList
          items={items}
          loading={loading}
          error={error}
          empty="暂无凭证"
          statusOf={(c) => (c.secret ? '可用' : '未验证')}
          action={(c) => (
            <button
              className="btn btn--small"
              onClick={() => {
                void transport.request('DELETE', `/credentials/${c.id}`).then(reload)
              }}
            >删除</button>
          )}
        />
      </Card>
      <Card title="操作">
        <AppearanceRow label="添加凭证">
          <button
            className="btn btn--small"
            onClick={() => {
              const name = window.prompt('凭证名称（如 deepseek）')
              if (!name) return
              const secret = window.prompt('API Key（本地加密存储，不落明文）')
              void transport.request('POST', '/credentials', { name, secret_value: secret }).then(reload)
            }}
          >＋ 添加凭证</button>
        </AppearanceRow>
        <div className="note" style={{ fontSize: 11 }}>本地加密存储，不落明文；删除后需重新配置。</div>
      </Card>
    </>
  )
}

function MemoryTab({ s, set }: { s: AppSettings; set: (p: Partial<AppSettings>) => void }) {
  return (
    <>
      <SecHead title="推理与上下文" sub="思考等级 · 压缩 · RTK" />
      <Card title="推理">
        <SegRow label="思考等级" options={['低', '中', '高']} value={s.reasonLevel === 'low' ? '低' : s.reasonLevel === 'mid' ? '中' : '高'} onChange={(v) => set({ reasonLevel: v === '低' ? 'low' : v === '中' ? 'mid' : 'high' })} />
        <SwitchRow label="流式回显" checked={s.streamEcho} onChange={(v) => set({ streamEcho: v })} />
        <SwitchRow label="显示推理帧" checked={s.showReason} onChange={(v) => set({ showReason: v })} />
      </Card>
      <Card title="上下文压缩">
        <SegRow label="压缩级别" options={['L0 原文', 'L1 摘要', 'L2 语义', 'L3 极简']} value={`L${s.compressLevel.slice(1)} ${s.compressLevel === 'L0' ? '原文' : s.compressLevel === 'L1' ? '摘要' : s.compressLevel === 'L2' ? '语义' : '极简'}`} onChange={(v) => set({ compressLevel: v.slice(0, 2) as AppSettings['compressLevel'] })} />
        <SwitchRow label="启用 RTK 过滤" checked={s.rtkEnable} onChange={(v) => set({ rtkEnable: v })} />
        <SwitchRow label="允许锁定 L0" note="开启后可用 context_lock_l0 工具" checked={s.lockL0} onChange={(v) => set({ lockL0: v })} />
        <RangeRow label="自动压缩阈值" min={60} max={95} value={s.autoCompactThreshold} lo="60%" hi={`${s.autoCompactThreshold}%`} onChange={(v) => set({ autoCompactThreshold: v })} />
      </Card>
    </>
  )
}

function PermissionsTab({ s, set }: { s: AppSettings; set: (p: Partial<AppSettings>) => void }) {
  return (
    <>
      <SecHead title="权限" sub="执行 · 审批 · 沙箱" />
      <Card title="执行">
        <SwitchRow label="写入文件前探测" checked={s.writeProbe} onChange={(v) => set({ writeProbe: v })} />
        <SwitchRow label="执行命令需审批" checked={s.cmdApproval} onChange={(v) => set({ cmdApproval: v })} />
        <SwitchRow label="沙箱内运行工具" checked={s.sandboxTools} onChange={(v) => set({ sandboxTools: v })} />
      </Card>
      <Card title="网络">
        <SwitchRow label="允许出站网络请求" checked={s.netOutbound} onChange={(v) => set({ netOutbound: v })} />
        <SwitchRow label="允许下载可执行文件" checked={s.netDownload} onChange={(v) => set({ netDownload: v })} />
      </Card>
    </>
  )
}

function SecurityTab({ s, set }: { s: AppSettings; set: (p: Partial<AppSettings>) => void }) {
  const audit = useServerResource<AuditVerifyResult>('/audit/verify')
  const entries = useServerResource<AuditEntriesResult>('/audit/entries')
  return (
    <>
      <SecHead title="安全" sub="审计 · 浏览器指纹 · 密钥" />
      <Card title="审计">
        <SwitchRow label="启用审计日志" checked={s.auditEnabled} onChange={(v) => set({ auditEnabled: v })} />
        <InputRow label="日志路径" value={s.auditPath} onChange={(v) => set({ auditPath: v })} />
        <SwitchRow label="审计先于校验" note="硬约束" checked={s.auditBeforeValidate} onChange={(v) => set({ auditBeforeValidate: v })} />
        <div className="cred-row">
          <span className="name">审计链完整性</span>
          <span className={audit.data?.valid ? 'st' : 'st st--warning'}>
            {audit.loading ? '校验中…' : audit.data ? (audit.data.valid ? '有效' : '无效') : audit.error ?? '未配置'}
          </span>
        </div>
        {entries.data && entries.data.count > 0 && (
          <div className="note" style={{ fontSize: 11, lineHeight: 1.7, marginTop: 4 }}>
            最近审计条目：{entries.data.count} 条
            <br />
            {entries.data.entries.slice(0, 3).map((e) => `${e.action}@${e.source}`).join(' · ')}
          </div>
        )}
      </Card>
      <Card title="浏览器">
        <SwitchRow label="指纹规避" checked={s.cdpStealth} onChange={(v) => set({ cdpStealth: v })} />
        <SwitchRow label="自动化标记隐藏" checked={s.cdpAutomation} onChange={(v) => set({ cdpAutomation: v })} />
      </Card>
      <Card title="密钥">
        <SwitchRow label="本地落盘加密" checked={s.encryptAtRest} onChange={(v) => set({ encryptAtRest: v })} />
        <AppearanceRow label="GUI API Key">
          <input
            type="password"
            value={s.apiKey}
            placeholder="服务端启用 -api-key 时填写"
            onChange={(e) => set({ apiKey: e.target.value })}
            style={{ background: 'var(--bg-elev)', color: 'var(--fg)', border: '1px solid var(--border)', borderRadius: 7, padding: '4px 8px', fontSize: 12, width: 220 }}
          />
        </AppearanceRow>
        <div className="note" style={{ fontSize: 11 }}>GUI 所有 /v1 请求将携带 Bearer 鉴权头。</div>
      </Card>
    </>
  )
}

function SkillsToolsTab({ s, set }: { s: AppSettings; set: (p: Partial<AppSettings>) => void }) {
  const skills = useServerList<SkillRecord>('/skill-hub')
  const tools = useServerList<{ name?: string }>('/tools')
  return (
    <>
      <SecHead title="技能与工具" sub="技能库 · 工具开关" />
      <Card title="技能库">
        <RowList
          items={skills.items}
          loading={skills.loading}
          error={skills.error}
          empty="技能库为空"
          statusOf={(x) => (x.executable ? '可执行' : '只读')}
        />
      </Card>
      <Card title="工具">
        <RowList items={tools.items} loading={tools.loading} error={tools.error} empty="工具列表为空" statusOf={() => '已注册'} />
        <SwitchRow label="工具卡片自动展开" checked={s.toolAutoExpand} onChange={(v) => set({ toolAutoExpand: v })} />
        <SwitchRow label="破坏性操作需确认" checked={s.toolConfirmDestructive} onChange={(v) => set({ toolConfirmDestructive: v })} />
      </Card>
    </>
  )
}

function ConnectorsTab({ s, set }: { s: AppSettings; set: (p: Partial<AppSettings>) => void }) {
  const mcp = useServerList<MCPToolRecord>('/mcp-hub')
  return (
    <>
      <SecHead title="连接器" sub="MCP · 桥接 · 浏览器" />
      <Card title="MCP 服务器">
        <RowList items={mcp.items} loading={mcp.loading} error={mcp.error} empty="未安装 MCP 工具" statusOf={() => 'stdio'} />
      </Card>
      <Card title="桥接">
        <div className="cred-row"><span className="name">飞书</span><span className="st st--warning">未连接</span></div>
        <div className="cred-row"><span className="name">社交平台</span><span className="st st--warning">未连接</span></div>
      </Card>
      <Card title="浏览器">
        <SwitchRow label="复用真实登录态 profile" checked={s.browserProfile} onChange={(v) => set({ browserProfile: v })} />
        <SwitchRow label="隔离上下文分区" checked={s.browserPartition} onChange={(v) => set({ browserPartition: v })} />
      </Card>
    </>
  )
}

function SchedulesTab() {
  const { items, loading, error, reload } = useServerList<ScheduleRecord>('/schedules')
  return (
    <>
      <SecHead title="调度" sub="定时任务 · Webhook" />
      <Card title="定时任务">
        <RowList
          items={items}
          loading={loading}
          error={error}
          empty="暂无定时任务"
          statusOf={(x) => (x.enabled ? '运行中' : '已停止')}
          action={(x) => (
            <button
              className="btn btn--small"
              onClick={() => {
                void transport.request('POST', `/schedules/${x.id}/${x.enabled ? 'stop' : 'start'}`, {}).then(reload)
              }}
            >{x.enabled ? '停止' : '启动'}</button>
          )}
        />
      </Card>
      <Card title="操作">
        <AppearanceRow label="新建调度">
          <button
            className="btn btn--small"
            onClick={() => {
              const name = window.prompt('调度名称')
              const flow = window.prompt('Flow 名称')
              if (!name || !flow) return
              void transport.request('POST', '/schedules', { name, flow }).then(reload)
            }}
          >＋ 新建调度</button>
        </AppearanceRow>
      </Card>
    </>
  )
}

function ExperimentsTab({ s, set }: { s: AppSettings; set: (p: Partial<AppSettings>) => void }) {
  return (
    <>
      <SecHead title="实验与插件" sub="实验特性 · 插件" />
      <Card title="实验特性">
        <SwitchRow label="目标自动研究" checked={s.expGoalAutoresearch} onChange={(v) => set({ expGoalAutoresearch: v })} />
        <SwitchRow label="跨步骤语义融合" note="上下文压缩" checked={s.expCrossStepFusion} onChange={(v) => set({ expCrossStepFusion: v })} />
        <SwitchRow label="富输入 Composer" note="文件 chip" checked={s.expBridgeComposer} onChange={(v) => set({ expBridgeComposer: v })} />
      </Card>
    </>
  )
}

function AboutTab() {
  return (
    <>
      <SecHead title="关于" sub="版本 · 更新 · 许可" />
      <Card title="版本">
        <div className="cred-row"><span className="name">应用名称</span><span className="st">InferGlow · 桌面 GUI</span></div>
        <div className="cred-row"><span className="name">版本号</span><span className="st">v0.2.0</span></div>
      </Card>
      <Card title="更新">
        <AppearanceRow label="检查更新">
          <button className="btn btn--small">检查更新</button>
        </AppearanceRow>
      </Card>
    </>
  )
}

// ─── panel shell ───

export function SettingsPanel({ onClose }: { onClose: () => void }) {
  const [tab, setTab] = useState('general')
  const settings = useSettingsStore((s) => s.settings)
  const set = useSettingsStore((s) => s.set)

  const groups: { name: string; tabs: string[] }[] = []
  for (const t of SETTINGS_TABS) {
    let g = groups.find((x) => x.name === t.group)
    if (!g) {
      g = { name: t.group, tabs: [] }
      groups.push(g)
    }
    g.tabs.push(t.key)
  }

  const renderTab = (key: string) => {
    switch (key) {
      case 'general': return <GeneralTab s={settings} set={set} />
      case 'appearance': return <AppearanceTab s={settings} set={set} />
      case 'interface': return <InterfaceTab s={settings} set={set} />
      case 'shortcuts': return <ShortcutsTab s={settings} />
      case 'providers': return <ProvidersTab />
      case 'models': return <ModelsTab s={settings} set={set} />
      case 'credentials': return <CredentialsTab />
      case 'memory': return <MemoryTab s={settings} set={set} />
      case 'permissions': return <PermissionsTab s={settings} set={set} />
      case 'security': return <SecurityTab s={settings} set={set} />
      case 'skills-tools': return <SkillsToolsTab s={settings} set={set} />
      case 'connectors': return <ConnectorsTab s={settings} set={set} />
      case 'schedules': return <SchedulesTab />
      case 'experiments': return <ExperimentsTab s={settings} set={set} />
      case 'about': return <AboutTab />
      default: return null
    }
  }

  return (
    <div className="mask mask--show" onClick={onClose}>
      <div className="settings-dialog" onClick={(e) => e.stopPropagation()}>
        <div className="settings-dialog__head">
          <span className="t">⚙ 设置</span>
          <span className="spacer" />
          <button className="icon-btn" title="关闭" onClick={onClose}>✕</button>
        </div>
        <div className="settings-body">
          <nav className="settings-nav">
            {groups.map((g) => (
              <div key={g.name}>
                <div className="nav-group">{g.name}</div>
                {g.tabs.map((k) => {
                  const t = SETTINGS_TABS.find((x) => x.key === k)!
                  return (
                    <button key={k} className={`navitem${tab === k ? ' navitem--active' : ''}`} onClick={() => setTab(k)}>
                      {t.label}
                      <small>{t.sub}</small>
                    </button>
                  )
                })}
              </div>
            ))}
          </nav>
          <main className="settings-main">{renderTab(tab)}</main>
        </div>
      </div>
    </div>
  )
}
