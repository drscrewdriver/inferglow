import styles from './tidychat.module.css'
import { useTidychatStore } from './tidychatStore'
import { NAV_HUE_OPTIONS, NAV_LIGHT_OPTIONS } from './config'

function Chip({
  label,
  active,
  onClick,
  dot,
}: {
  label: string
  active: boolean
  onClick: () => void
  dot?: string
}) {
  return (
    <button type="button" aria-pressed={active} className={`${styles.colorChip}${active ? ` ${styles.colorChipOn}` : ''}`} onClick={onClick}>
      {dot !== undefined && <span className={styles.colorDot} style={{ background: dot }} />}
      {label}
    </button>
  )
}

/**
 * 会话整理 settings card. Registered via the `settings.plugin.item` slot so the
 * 4 toggles (fold / divider / navigator / autoLoad) and the navigator colours are
 * editable from SettingsPanel and take effect immediately through the store.
 */
export function TidySettingsCard() {
  const config = useTidychatStore((s) => s.config)
  const setConfig = useTidychatStore((s) => s.setConfig)

  const toggle = (patch: Partial<{ fold: boolean; divider: boolean; navigator: boolean; autoLoad: boolean }>) =>
    setConfig(patch)

  return (
    <div className="card" data-tidychat-settings>
      <div className="card__head">
        <span className="t">会话整理</span>
        <span className="spacer" />
      </div>
      <div className="card__body">
        <div className="switch" onClick={() => toggle({ fold: !config.fold })} role="switch" aria-checked={config.fold} tabIndex={0}>
          <div className="txt">
            <b>自动折叠已完成轮次</b>
            <small>收起过程，只留最终正文</small>
          </div>
          <div className={`knob${config.fold ? ' on' : ''}`} data-toggle />
        </div>

        <div className="switch" onClick={() => toggle({ divider: !config.divider })} role="switch" aria-checked={config.divider} tabIndex={0}>
          <div className="txt">
            <b>思考分隔线</b>
            <small>思考与正文之间插入实线</small>
          </div>
          <div className={`knob${config.divider ? ' on' : ''}`} data-toggle />
        </div>

        <div className="switch" onClick={() => toggle({ navigator: !config.navigator })} role="switch" aria-checked={config.navigator} tabIndex={0}>
          <div className="txt">
            <b>左缘定位条</b>
            <small>Canvas 迷你地图跳转用户消息</small>
          </div>
          <div className={`knob${config.navigator ? ' on' : ''}`} data-toggle />
        </div>

        <div className="switch" onClick={() => toggle({ autoLoad: !config.autoLoad })} role="switch" aria-checked={config.autoLoad} tabIndex={0}>
          <div className="txt">
            <b>智能加载更早历史</b>
            <small>性能健康时自动拉取，卡顿自动暂停</small>
          </div>
          <div className={`knob${config.autoLoad ? ' on' : ''}`} data-toggle />
        </div>

        <div className={styles.settingsCard}>
          <div className="appearance-row">
            <div>
              <div className="lbl">定位条默认色</div>
              <div className="note">auto 随机芯明暗自适应</div>
            </div>
          </div>
          <div className={styles.colorSub}>
            <span className={styles.colorSubLabel}>颜色</span>
            <div className={styles.colorChips}>
              {NAV_HUE_OPTIONS.map((o) => (
                <Chip key={o.key} label={o.label} dot={o.preview} active={config.navColor === o.key} onClick={() => setConfig({ navColor: o.key, navColorLight: config.navColorLight })} />
              ))}
            </div>
          </div>
          <div className={styles.colorSub}>
            <span className={styles.colorSubLabel}>明度</span>
            <div className={styles.colorChips}>
              {NAV_LIGHT_OPTIONS.map((o) => (
                <Chip key={o.key} label={o.label} active={config.navColorLight === o.key} onClick={() => setConfig({ navColorLight: o.key })} />
              ))}
            </div>
          </div>
          <div className="appearance-row">
            <div className="lbl">强调色</div>
          </div>
          <div className={styles.colorSub}>
            <span className={styles.colorSubLabel}>颜色</span>
            <div className={styles.colorChips}>
              {NAV_HUE_OPTIONS.map((o) => (
                <Chip key={o.key} label={o.label} dot={o.preview} active={config.navAccent === o.key} onClick={() => setConfig({ navAccent: o.key, navAccentLight: config.navAccentLight })} />
              ))}
            </div>
          </div>
          <div className={styles.colorSub}>
            <span className={styles.colorSubLabel}>明度</span>
            <div className={styles.colorChips}>
              {NAV_LIGHT_OPTIONS.map((o) => (
                <Chip key={o.key} label={o.label} active={config.navAccentLight === o.key} onClick={() => setConfig({ navAccentLight: o.key })} />
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}