/**
 * Settings modal — full settings panel with:
 * - Theme toggle (dark/light mode)
 * - Model selection dropdown
 * - API endpoint configuration
 * - Font size
 * - Auto-scroll toggle
 */

import { useEffect, useState } from 'react'
import { Button } from '../../components/Button.tsx'
import { store, subscribe, type Settings } from '../../store.ts'
import { agentName, getAgents } from '../../bridge/inferglow.ts'

interface SettingsModalProps {
  onClose: () => void
}

export function SettingsModal({ onClose }: SettingsModalProps) {
  const settings = store.settings
  const [modelId, setModelId] = useState(settings.model)

  useEffect(() => {
    return subscribe(() => setModelId(store.settings.model))
  }, [])

  useEffect(() => {
    // Keep dark mode attribute in sync with settings
    if (settings.darkMode) {
      document.body.setAttribute('data-ds-dark-theme', '')
    } else {
      document.body.removeAttribute('data-ds-dark-theme')
    }
  }, [settings.darkMode])

  function handleToggleDarkMode() {
    store.updateSetting('darkMode', !settings.darkMode)
  }

  function handleModelChange(e: React.ChangeEvent<HTMLSelectElement>) {
    store.updateSetting('model', e.target.value)
  }

  function handleApiEndpointChange(e: React.ChangeEvent<HTMLInputElement>) {
    store.updateSetting('apiEndpoint', e.target.value)
  }

  function handleFontSizeChange(e: React.ChangeEvent<HTMLSelectElement>) {
    store.updateSetting('fontSize', e.target.value as Settings['fontSize'])
  }

  function handleToggleAutoScroll() {
    store.updateSetting('autoScroll', !settings.autoScroll)
  }

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
          {/* ── Appearance ── */}
          <section className="dsh-modal-section">
            <h3 className="dsh-modal-section-title">外观</h3>
            
            <div className="dsh-modal-row">
              <span className="dsh-modal-row-label">深色模式</span>
              <ToggleSwitch
                checked={settings.darkMode}
                onChange={handleToggleDarkMode}
              />
            </div>
          </section>

          {/* ── Model ── */}
          <section className="dsh-modal-section">
            <h3 className="dsh-modal-section-title">模型</h3>
            
            <div className="dsh-modal-field">
              <label className="dsh-modal-label">Agent</label>
              <select
                className="dsh-modal-select"
                value={modelId}
                onChange={handleModelChange}
              >
                {getAgents().length === 0 && (
                  <option value="">
                    {modelId ? agentName(modelId) : '未配置(发送时取首个 Agent)'}
                  </option>
                )}
                {getAgents().map(a => (
                  <option key={a.id} value={a.id}>{a.name}</option>
                ))}
                {getAgents().length > 0 && !getAgents().some(a => a.id === modelId) && modelId && (
                  <option value={modelId}>{agentName(modelId)}</option>
                )}
              </select>
              <span className="dsh-modal-label" style={{ opacity: 0.6, fontSize: 12 }}>
                Agent 列表来自 InferGlow Server(GET /v1/agents)
              </span>
            </div>
          </section>

          {/* ── API ── */}
          <section className="dsh-modal-section">
            <h3 className="dsh-modal-section-title">API 配置</h3>
            
            <div className="dsh-modal-field">
              <label className="dsh-modal-label">API 端点</label>
              <input
                type="text"
                className="dsh-modal-input"
                value={settings.apiEndpoint}
                onChange={handleApiEndpointChange}
                placeholder="留空 = 同源(开发态经 vite 代理)"
              />
            </div>
          </section>

          {/* ── Display ── */}
          <section className="dsh-modal-section">
            <h3 className="dsh-modal-section-title">显示</h3>
            
            <div className="dsh-modal-field">
              <label className="dsh-modal-label">字体大小</label>
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
                onChange={handleToggleAutoScroll}
              />
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
