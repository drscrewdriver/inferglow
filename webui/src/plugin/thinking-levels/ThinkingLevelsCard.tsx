// thinking-levels/ThinkingLevelsCard.tsx — 设置在设置面板的卡片与调度引擎。
// 包含档位选择器 + 三个开关（enable/downgrade/upgrade）；并导出纯的调度入口
// findEffectiveLevel(history)，供宿主在消息发送前决定 reasoning_effort。

import { AUTO_LEVEL, LEVELS, levelLabel } from './levels'
import { decideEffort, type ThinkingLevelsConfig, type ToolCallSample } from './decide'
import type { ReasoningEffort } from './levels'
import { useThinkingLevelsStore } from './store'
import s from './thinking-levels.module.css'

/**
 * 设置卡片：思考档位（固定档 / auto）+ 自动调度规则开关。
 * 注册到 settings.plugin.item（keyed: 'thinking-levels'）。
 */
export function ThinkingLevelsCard() {
  const config = useThinkingLevelsStore((state) => state.config)
  const setConfig = useThinkingLevelsStore((state) => state.setConfig)
  const setLevel = useThinkingLevelsStore((state) => state.setLevel)
  const lastEffort = useThinkingLevelsStore((state) => state.lastEffort)

  const onToggle = (patch: Partial<ThinkingLevelsConfig>) => setConfig(patch)

  return (
    <div className={s.card} data-thinking-levels>
      <div className={s.head}>
        <span className={s.title}>思考级别</span>
        <span className={s.subtitle}>reasoning_effort 注入到 LLM 请求</span>
      </div>

      <div className={s.switch} role="switch" aria-checked={config.enabled} tabIndex={0} onClick={() => onToggle({ enabled: !config.enabled })}>
        <div className={s.switchTxt}>
          <b>启用自动思考调度</b>
          <small>禁用时固定使用最高档</small>
        </div>
        <div className={`${s.knob}${config.enabled ? ` ${s.on}` : ''}`} />
      </div>

      <div className={s.head}>
        <span className={s.subtitle}>档位</span>
        <span className={s.subtitle}>auto 由调度引擎按最近工具调用决策</span>
      </div>
      <div className={s.levels}>
        {LEVELS.map((o) => (
          <button
            key={o.level}
            type="button"
            aria-pressed={config.level === o.level}
            className={`${s.levelChip}${config.level === o.level ? ` ${s.levelChipOn}` : ''}`}
            onClick={() => setLevel(o.level)}
          >
            <span className={s.levelDot} style={{ background: o.dot }} />
            {o.label} 档
          </button>
        ))}
        <button
          type="button"
          aria-pressed={config.level === AUTO_LEVEL}
          className={`${s.levelChip}${config.level === AUTO_LEVEL ? ` ${s.levelChipOn}` : ''}`}
          onClick={() => setLevel(AUTO_LEVEL)}
        >
          <span className={s.levelDot} style={{ background: 'var(--accent)' }} />
          自动
        </button>
      </div>

      {config.level === AUTO_LEVEL && (
        <div className={s.autoRules}>
          <div className={s.resultRow}>
            <span className={s.resultLabel}>最近调度结果</span>
            <span className={s.resultValue}>
              effort = <b>{lastEffort}</b>
            </span>
          </div>
          <div className={s.switch} role="switch" aria-checked={config.allowDowngrade} tabIndex={0} onClick={() => onToggle({ allowDowngrade: !config.allowDowngrade })}>
            <div className={s.switchTxt}>
              <b>允许降档</b>
              <small>简单工具调用（占比 ≥75%）→ 最低档</small>
            </div>
            <div className={`${s.knob}${config.allowDowngrade ? ` ${s.on}` : ''}`} />
          </div>
          <div className={s.switch} role="switch" aria-checked={config.allowUpgrade} tabIndex={0} onClick={() => onToggle({ allowUpgrade: !config.allowUpgrade })}>
            <div className={s.switchTxt}>
              <b>允许升档</b>
              <small>超大参数（≥ HEAVY_ARGS×4）→ 最高档</small>
            </div>
            <div className={`${s.knob}${config.allowUpgrade ? ` ${s.on}` : ''}`} />
          </div>
        </div>
      )}
    </div>
  )
}

/**
 * 调度引擎：供消息发送前决定 reasoning effort。
 * 读取当前 store 配置 + 最近工具调用历史，返回 'low' | 'medium' | 'high'。
 * 纯逻辑（decideEffort 不碰 React），宿主直接注入结果即可。
 */
export function findEffectiveLevel(history: ToolCallSample[]): ReasoningEffort {
  const config = useThinkingLevelsStore.getState().config
  return decideEffort(config, history)
}

/** 便捷入口：仅用 fix 档位（不含 auto 调度）取档位标签。 */
export function currentLevelLabel(): string {
  return levelLabel(useThinkingLevelsStore.getState().config.level)
}