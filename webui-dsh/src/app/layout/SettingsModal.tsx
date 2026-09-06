/**
 * Settings modal — six sections following the upstream dsh-modal organization
 * (section + row/field paradigms, zero custom CSS):
 *   外观 / Agent / API 配置 / 终端 / 会话管理 / 关于
 * All values persist through store.updateSetting (localStorage JSON).
 */

import { useEffect, useState } from 'react'
import { Button } from '../../components/Button.tsx'
import { store, subscribe, clearPersistedSettings, type Settings, type ThemeSetting } from '../../store.ts'
import { agentName, getAgents } from '../../bridge/inferglow.ts'

interface SettingsModalProps {
  onClose: () => void
}

const THEME_LABELS: [ThemeSetting, string][] = [
  ['dark', '深色'],
  ['light', '浅色'],
  ['system', '跟随系统'],
]

export function SettingsModal({ onClose }: SettingsModalProps) {
  const settings = store.settings
  const [modelId, setModelId] = useState(settings.model)
  const [apiKey, setApiKey] = useState(settings.apiKey)
  const [apiEndpoint, setApiEndpoint] = useState(settings.apiEndpoint)

  useEffect(() => {
    return subscribe(() => setModelId(store.settings.model))
  }, [])

  function handleThemeChange(e: React.ChangeEvent<HTMLSelectElement>) {
    store.updateSetting('theme', e.target.value as ThemeSetting)
  }

  function handleModelChange(e: React.ChangeEvent<HTMLSelectElement>) {
    store.updateSetting('model', e.target.value)
  }

  function handleFontSizeChange(e: React.ChangeEvent<HTMLSelectElement>) {
    // ChatArea reacts via the store subscription (zoom on the conversation
    // container) — no extra wiring needed here.
    store.updateSetting('fontSize', e.target.value as Settings['fontSize'])
  }

  function saveApiFields() {
    store.updateSetting('apiEndpoint', apiEndpoint.trim())
    store.updateSetting('apiKey', apiKey.trim())
  }

  function handleClearCache() {
    if (confirm('清除本地设置缓存并刷新页面？')) {
      clearPersistedSettings()
      window.location.reload()
    }
  }

  const activeAgent = getAgents().find(a => a.id === (modelId || store.settings.model))

  return (
    <div className="dsh-modal-overlay" onClick={onClose}>
      <div className="dsh-modal dsh-modal-settings" onClick={e => e.stopPropagation()}>
        <div className="dsh-modal-header">
          <h2 className="dsh-modal-title">设置</h2>
          <Button variant="ghost" size="sm" onClick={onClose} title="关闭">
            <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
              <path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
            </svg>
          </Button>
        </div>

        <div className="dsh-modal-content">
          {/* ── 外观 ── */}
          <section className="dsh-modal-section">
            <h3 className="dsh-modal-section-title">外观</h3>

            <div className="dsh-modal-field">
              <label className="dsh-modal-label">主题</label>
              <select
                className="dsh-modal-select"
                value={settings.theme}
                onChange={handleThemeChange}
              >
                {THEME_LABELS.map(([v, label]) => (
                  <option key={v} value={v}>{label}</option>
                ))}
              </select>
            </div>

            <div className="dsh-modal-field">
              <label className="dsh-modal-label">字体大小（对话区）</label>
              <select
                className="dsh-modal-select"
                value={settings.fontSize}
                onChange={handleFontSizeChange}
              >
                <option value="small">小</option>
                <option value="medium">中</option>
                <option value="large">大</option>
              </select>
            </div>

            <div className="dsh-modal-row">
              <span className="dsh-modal-row-label">自动滚动</span>
              <ToggleSwitch
                checked={settings.autoScroll}
                onChange={() => store.updateSetting('autoScroll', !settings.autoScroll)}
              />
            </div>
          </section>

          {/* ── Agent ── */}
          <section className="dsh-modal-section">
            <h3 className="dsh-modal-section-title">Agent</h3>

            <div className="dsh-modal-field">
              <label className="dsh-modal-label">新会话使用的 Agent</label>
              <select
                className="dsh-modal-select"
                value={modelId}
                onChange={handleModelChange}
              >
                {getAgents().length === 0 && (
                  <option value="">
                    {modelId ? agentName(modelId) : '未配置（发送时取首个 Agent）'}
                  </option>
                )}
                {getAgents().map(a => (
                  <option key={a.id} value={a.id}>{a.name}</option>
                ))}
                {getAgents().length > 0 && !getAgents().some(a => a.id === modelId) && modelId && (
                  <option value={modelId}>{agentName(modelId)}</option>
                )}
              </select>
            </div>

            {activeAgent && (
              <div className="dsh-modal-field">
                <label className="dsh-modal-label">Agent 详情</label>
                <div className="dsh-modal-label" style={{ opacity: 0.8, lineHeight: 1.6 }}>
                  id: {activeAgent.id}
                  {activeAgent.model && <> · model: {activeAgent.model}</>}
                  {activeAgent.description && <><br />{activeAgent.description}</>}
                </div>
              </div>
            )}
            <div className="dsh-modal-row">
              <span className="dsh-modal-row-label">列表来源</span>
              <span className="dsh-modal-row-label" style={{ opacity: 0.6 }}>GET /v1/agents（实时）</span>
            </div>
          </section>

          {/* ── API 配置 ── */}
          <section className="dsh-modal-section">
            <h3 className="dsh-modal-section-title">API 配置</h3>

            <div className="dsh-modal-field">
              <label className="dsh-modal-label">API 端点</label>
              <input
                type="text"
                className="dsh-modal-input"
                value={apiEndpoint}
                onChange={e => setApiEndpoint(e.target.value)}
                onBlur={saveApiFields}
                placeholder="留空 = 同源（开发态经 vite 代理）"
              />
            </div>

            <div className="dsh-modal-field">
              <label className="dsh-modal-label">API Key（Bearer）</label>
              <input
                type="password"
                className="dsh-modal-input"
                value={apiKey}
                onChange={e => setApiKey(e.target.value)}
                onBlur={saveApiFields}
                placeholder="server 以 -api-key 启动时必填"
              />
              <span className="dsh-modal-label" style={{ opacity: 0.6, fontSize: 12 }}>
                保存在浏览器本地 localStorage；请求 /v1/* 时自动附加 Authorization 头
              </span>
            </div>
          </section>

          {/* ── 终端 ── */}
          <section className="dsh-modal-section">
            <h3 className="dsh-modal-section-title">终端</h3>
            <div className="dsh-modal-row">
              <span className="dsh-modal-row-label">执行通道</span>
              <span className="dsh-modal-row-label" style={{ opacity: 0.6 }}>
                未启用 — server 需以 -api-key 与终端开关启动
              </span>
            </div>
            <span className="dsh-modal-label" style={{ opacity: 0.6, fontSize: 12 }}>
              v1 为非交互命令执行（白名单 + 审计），不是交互式 shell
            </span>
          </section>

          {/* ── 会话管理 ── */}
          <section className="dsh-modal-section">
            <h3 className="dsh-modal-section-title">会话管理</h3>
            <div className="dsh-modal-row">
              <span className="dsh-modal-row-label">本地设置缓存</span>
              <Button variant="ghost" size="sm" onClick={handleClearCache}>清除并刷新</Button>
            </div>
            <span className="dsh-modal-label" style={{ opacity: 0.6, fontSize: 12 }}>
              会话记录保存在 server（内存态，重启即清）；此处仅清除浏览器侧设置
            </span>
          </section>

          {/* ── 关于 ── */}
          <section className="dsh-modal-section">
            <h3 className="dsh-modal-section-title">关于</h3>
            <div className="dsh-modal-label" style={{ opacity: 0.7, fontSize: 12, lineHeight: 1.8 }}>
              InferGlow WebUI (DSH) — vendored dsh-transition-webui @ 9e14e99（MIT）<br />
              上游：github.com/drscrewdriver/dsh-transition-webui · 详见 webui-dsh/NOTICE.md<br />
              挂载端点：/webui-dsh/（本界面）· /web/（原版）· /webui2/（原型）· /gui/（桌面）
            </div>
          </section>
        </div>
      </div>
    </div>
  )
}

/* ── Toggle switch component ── */
function ToggleSwitch({ checked, onChange }: {
  checked: boolean
  onChange: () => void
}) {
  return (
    <button
      className={`dsh-toggle${checked ? ' dsh-toggle-active' : ''}`}
      onClick={onChange}
      role="switch"
      aria-checked={checked}
    >
      <span className="dsh-toggle-thumb" />
    </button>
  )
}
