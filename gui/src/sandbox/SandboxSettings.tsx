import styles from './sandbox.module.css'
import { useSandboxStore } from './sandboxStore'
import {
  isRiskier,
  PERMISSION_PRESETS,
  SANDBOX_MODES,
  SHELL_ENVS,
  type PermissionPreset,
  type SandboxMode,
  type ShellEnv,
} from './schema'

function ChipGroup<T extends string>({
  options,
  value,
  onPick,
  typeOf,
}: {
  options: { key: T; label: string; hint: string }[]
  value: T
  onPick: (k: T) => void
  typeOf?: (k: T) => 'safe' | 'medium' | 'high'
}) {
  return (
    <div className={styles.chipGroup}>
      {options.map((o) => (
        <button
          key={o.key}
          type="button"
          aria-pressed={value === o.key}
          className={`${styles.chip}${value === o.key ? ` ${styles.chipOn}` : ''}`}
          onClick={() => onPick(o.key)}
          title={o.hint}
        >
          {typeOf && <span className={`${styles.dot} ${styles['dot_' + typeOf(o.key)]}`} />}
          {o.label}
        </button>
      ))}
    </div>
  )
}

/**
 * 沙箱与执行 settings card. Registered via `settings.plugin.item`. Selects the
 * runtime sandbox mode, the filesystem permission preset, and the Shell
 * environment the executor uses (the actual engine lives in the Go binary).
 */
export function SandboxSettingsCard() {
  const config = useSandboxStore((s) => s.config)
  const setMode = useSandboxStore((s) => s.setMode)
  const setPreset = useSandboxStore((s) => s.setPreset)
  const setShell = useSandboxStore((s) => s.setShell)
  const setRequire = useSandboxStore((s) => s.setRequireEscalationApproval)

  const pickedMode = config.mode
  const onMode = (k: SandboxMode) => {
    if (config.requireEscalationApproval && isRiskier(config.mode, k)) {
      const ok = window.confirm(
        `切换到「${SANDBOX_MODES.find((m) => m.key === k)?.label}」会降低隔离级别（更危险）。\n\n确认「允许」后仍会触发审批记录。`,
      )
      if (!ok) return
    }
    setMode(k)
  }

  return (
    <div className="card" data-sandbox-settings>
      <div className="card__head">
        <span className="t">沙箱与执行</span>
        <span className="spacer" />
      </div>
      <div className="card__body">
        <div className="appearance-row">
          <div>
            <div className="lbl">运行时沙箱模式</div>
            <div className="note">
              {SANDBOX_MODES.find((m) => m.key === pickedMode)?.hint ?? ''}
            </div>
          </div>
        </div>
        <ChipGroup
          options={SANDBOX_MODES}
          value={pickedMode}
          onPick={onMode}
          typeOf={(k) => SANDBOX_MODES.find((m) => m.key === k)!.risk}
        />

        <div className="appearance-row">
          <div>
            <div className="lbl">文件系统权限预设</div>
            <div className="note">{PERMISSION_PRESETS.find((p) => p.key === config.preset)?.hint}</div>
          </div>
        </div>
        <ChipGroup<PermissionPreset> options={PERMISSION_PRESETS} value={config.preset} onPick={setPreset} />

        <div className="appearance-row">
          <div>
            <div className="lbl">执行环境 Shell</div>
            <div className="note">bash / PowerShell / cmd / WSL</div>
          </div>
        </div>
        <ChipGroup<ShellEnv> options={SHELL_ENVS} value={config.shell} onPick={setShell} />

        <div
          className="switch"
          onClick={() => setRequire(!config.requireEscalationApproval)}
          role="switch"
          aria-checked={config.requireEscalationApproval}
          tabIndex={0}
        >
          <div className="txt">
            <b>切换更危险模式需确认</b>
            <small>降级隔离时弹审批确认</small>
          </div>
          <div className={`knob${config.requireEscalationApproval ? ' on' : ''}`} data-toggle />
        </div>
      </div>
    </div>
  )
}