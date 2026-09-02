import styles from './thinking.module.css'
import { useThinkingStore } from './thinkingStore'
import { LEVEL_OPTIONS, levelLabel } from './levels'

/**
 * 思考级别 Auto settings card. Registered via `settings.plugin.item` so the
 * enabled / level / allow-downgrade / allow-upgrade controls are editable from
 * SettingsPanel and take effect immediately through the store.
 */
export function ThinkingSettingsCard() {
  const config = useThinkingStore((s) => s.config)
  const setConfig = useThinkingStore((s) => s.setConfig)
  const setLevel = useThinkingStore((s) => s.setLevel)
  const lastEffort = useThinkingStore((s) => s.lastEffort)

  return (
    <div className="card" data-thinking-settings>
      <div className="card__head">
        <span className="t">思考级别</span>
        <span className="spacer" />
      </div>
      <div className="card__body">
        <div className="switch" onClick={() => setConfig({ enabled: !config.enabled })} role="switch" aria-checked={config.enabled} tabIndex={0}>
          <div className="txt">
            <b>启用自动思考调度</b>
            <small>reasoning_effort 注入到 LLM 请求</small>
          </div>
          <div className={`knob${config.enabled ? ' on' : ''}`} data-toggle />
        </div>

        <div className="appearance-row">
          <div>
            <div className="lbl">档位</div>
            <div className="note">auto 由调度引擎按最近工具调用决策</div>
          </div>
        </div>
        <div className={styles.levels}>
          {LEVEL_OPTIONS.map((o) => (
            <button
              key={o.key}
              type="button"
              aria-pressed={config.level === o.key}
              className={`${styles.levelChip}${config.level === o.key ? ` ${styles.levelChipOn}` : ''}`}
              onClick={() => setLevel(o.key)}
            >
              <span className={styles.levelDot} style={{ background: o.dot }} />
              {levelLabel(o.key)}
            </button>
          ))}
        </div>

        {config.level === 'auto' && (
          <div className={styles.autoRules}>
            <div className="appearance-row">
              <div>
                <div className="lbl">最近调度结果</div>
                <div className="note">
                  effort = <b style={{ color: 'var(--accent)' }}>{lastEffort}</b>
                </div>
              </div>
            </div>
            <div className="switch" onClick={() => setConfig({ allowDowngrade: !config.allowDowngrade })} role="switch" aria-checked={config.allowDowngrade} tabIndex={0}>
              <div className="txt">
                <b>允许降档</b>
                <small>简单工具调用（&lt; HEAVY_ARGS× 跨度 ≥75%）→ low</small>
              </div>
              <div className={`knob${config.allowDowngrade ? ' on' : ''}`} data-toggle />
            </div>
            <div className="switch" onClick={() => setConfig({ allowUpgrade: !config.allowUpgrade })} role="switch" aria-checked={config.allowUpgrade} tabIndex={0}>
              <div className="txt">
                <b>允许升档</b>
                <small>超大参数（≥ HEAVY_ARGS×4）→ max</small>
              </div>
              <div className={`knob${config.allowUpgrade ? ' on' : ''}`} data-toggle />
            </div>
          </div>
        )}
      </div>
    </div>
  )
}