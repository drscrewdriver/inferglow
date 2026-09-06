/* InferGlow Web UI — 设置面板（渲染插件注册的 settings.* 槽位） */

import { renderSlot } from '../plugin/registry'

interface SettingsPanelProps {
  onClose: () => void
}

export function SettingsPanel({ onClose }: SettingsPanelProps) {
  return (
    <div className="settings-overlay" onClick={onClose}>
      <div className="settings-panel" role="dialog" aria-label="设置" onClick={e => e.stopPropagation()}>
        <div className="settings-head">
          <span className="settings-title">设置</span>
          <button className="settings-close" onClick={onClose} title="关闭">✕</button>
        </div>
        <div className="settings-body">
          <div className="settings-section">
            <div className="settings-section-title">通用</div>
            {renderSlot('settings.general.item', {}, { key: 'appearance' })}
          </div>
          <div className="settings-section">
            <div className="settings-section-title">插件</div>
            {renderSlot('settings.plugin.item', {}, { key: 'thinking-levels' })}
          </div>
        </div>
      </div>
    </div>
  )
}