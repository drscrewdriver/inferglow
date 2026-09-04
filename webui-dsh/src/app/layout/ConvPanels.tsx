/**
 * ConvPanels — content for the 轨迹 (trace) and 上下文 (context) conversation
 * tabs, mirroring the reference `.lc-root` context panel and a static trace
 * (steps + tool calls) layout.
 */

/* ── 轨迹 (trace): controls + 3-lane timeline ── */
const TRACE_SPANS: { left: number; type: string; lane: number }[] = [
  { left: 0, type: 'system', lane: 0 },
  { left: 9.0909, type: 'user', lane: 0 },
  { left: 18.1818, type: 'context', lane: 0 },
  { left: 27.2727, type: 'context', lane: 0 },
  { left: 36.3636, type: 'message', lane: 1 },
  { left: 45.4545, type: 'tool', lane: 2 },
  { left: 54.5454, type: 'message', lane: 1 },
  { left: 63.6363, type: 'tool', lane: 2 },
  { left: 72.7272, type: 'user', lane: 0 },
  { left: 81.8181, type: 'user', lane: 0 },
  { left: 90.909, type: 'message', lane: 1 },
]

const TRACE_TURNS = [
  { n: '第 1 轮 · 第 1 步', time: '03:50:28', dur: '2.3s', status: 'done',
    msgs: [
      { role: 'user', text: '请执行命令 Start-Sleep -Seconds 450 然后回复完成' },
      { role: 'tool', name: 'run_command', args: 'Start-Sleep -Seconds 450', out: 'DONE' },
      { role: 'assistant', text: '命令正常结束（exit code 0），输出为 DONE。' },
    ] },
  { n: '第 1 轮 · 第 2 步', time: '03:50:29', dur: '1.8s', status: 'done',
    msgs: [
      { role: 'user', text: '继续处理测试2、测试3' },
      { role: 'assistant', text: '收到，继续处理后续任务。' },
    ] },
  { n: '第 1 轮 · 第 3 步', time: '03:58:00', dur: '4.2s', status: 'done',
    msgs: [
      { role: 'tool', name: 'apply_patch', args: 'src/app.tsx', out: 'ok' },
      { role: 'assistant', text: '已完成全部修改，测试通过。' },
    ] },
]

export function TracePanel() {
  return (
    <div className="dsh-trace">
      <div className="dsh-trace-inner">
        <div className="dsh-trace-actions">
          <button type="button" className="dsh-trace-toggle" aria-pressed="false">
            <svg className="dsh-trace-toggle-icon" viewBox="0 0 16 16" fill="none"><circle cx="8" cy="8" r="5.25"/><path d="M8 4.75V8l2.25 1.5"/></svg>
            Duration
          </button>
          <button type="button" className="dsh-trace-action" aria-pressed="false"><span>⊟</span>Turns</button>
          <button type="button" className="dsh-trace-action" aria-pressed="false"><span>⊟</span>Calls</button>
        </div>
        <div className="dsh-trace-search">
          <svg width="11" height="11" viewBox="0 0 16 16" fill="none"><path d="M11.89 6.65A5.3 5.3 0 1 1 1.35 6.65a5.3 5.3 0 0 1 10.54 0"/><path d="M16 15.04l-4.47-4.53"/></svg>
          <input type="search" aria-label="搜索轨迹" placeholder="搜索" value="" />
        </div>
      </div>

      <section className="dsh-trace-timeline" aria-label="Trajectory timeline">
        <div className="dsh-trace-plot">
          <div className="dsh-trace-lanes-labels" aria-hidden="true">
            <span>Input</span><span>Model</span><span>Tools</span>
          </div>
          <div className="dsh-trace-track">
            <div className="dsh-trace-lanes">
              {TRACE_SPANS.map((s, i) => (
                <span
                  key={i}
                  aria-hidden="true"
                  className={`dsh-trace-span span-${s.type}`}
                  style={{ left: `${s.left}%`, width: '9.09%', top: `${s.lane * 33.33}%` }}
                />
              ))}
            </div>
          </div>
        </div>
      </section>

      {/* Turn records */}
      <div className="dsh-trace-turns">
        {TRACE_TURNS.map((t, i) => (
          <div key={i} className="dsh-turn">
            <div className="dsh-turn-head">
              <span className="dsh-turn-num">{t.n}</span>
              <span className="dsh-turn-time">{t.time}</span>
              <span className="dsh-turn-dur">{t.dur}</span>
              <span className={`dsh-turn-status ${t.status}`}>✓</span>
            </div>
            <div className="dsh-turn-msgs">
              {t.msgs.map((m, j) => (
                <div key={j} className={`dsh-turn-msg msg-${m.role}`}>
                  <span className="dsh-turn-msg-role">
                    {m.role === 'user' ? '用户' : m.role === 'tool' ? `工具 · ${m.name}` : '助手'}
                  </span>
                  <span className="dsh-turn-msg-text">
                    {m.role === 'tool' ? `${m.args} → ${m.out}` : m.text}
                  </span>
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

/* ── 上下文 (context): replicate `.lc-root` cards ── */
const CTX_STATS = [
  ['轮次', '1'], ['步数', '3'], ['注入', '2'], ['压缩', '0'],
  ['剪枝', '0'], ['工具调用', '2'], ['图片', '0'], ['缓存命中', '66.54%'],
]

const CTX_LEGEND = [
  { label: '系统提示词', color: '#6366f1', num: '≈2.4k', pct: '16%' },
  { label: '工具定义', color: '#f59e0b', num: '≈8.7k', pct: '57%' },
  { label: '用户消息', color: '#22c55e', num: '≈36', pct: '0%' },
  { label: '注入内容', color: '#a855f7', num: '≈3.8k', pct: '25%' },
  { label: '助手消息', color: '#3b82f6', num: '≈222', pct: '1%' },
  { label: '工具结果', color: '#14b8a6', num: '≈45', pct: '0%' },
]

const TREND_BARS = [
  { seg: [['#6366f1', 18], ['#f59e0b', 64], ['#22c55e', 1], ['#a855f7', 28]] },
  { seg: [['#6366f1', 18], ['#f59e0b', 64], ['#22c55e', 1], ['#a855f7', 28], ['#3b82f6', 1], ['#14b8a6', 1]] },
  { seg: [['#6366f1', 18], ['#f59e0b', 65], ['#22c55e', 1], ['#a855f7', 28], ['#3b82f6', 1], ['#14b8a6', 1]] },
]

const DETAIL_ROWS = [
  { label: '系统提示词', color: '#6366f1', w: '15.88%', num: '≈2.4k', pct: '16%' },
  { label: '工具定义', color: '#f59e0b', w: '57.66%', num: '≈8.7k', pct: '58%' },
  { label: '用户消息', color: '#22c55e', w: '0.24%', num: '≈36', pct: '0%' },
  { label: '注入内容', color: '#a855f7', w: '25.03%', num: '≈3.8k', pct: '25%' },
  { label: '助手消息', color: '#3b82f6', w: '0.89%', num: '≈134', pct: '1%' },
  { label: '工具结果', color: '#14b8a6', w: '0.30%', num: '≈45', pct: '0%' },
]

const BR_CATS = [
  { label: '系统提示词', color: '#6366f1', count: '1 项', tokens: '≈2.4k', pct: '16%' },
  { label: '工具定义', color: '#f59e0b', count: '30 项', tokens: '≈8.7k', pct: '57%' },
  { label: '用户消息', color: '#22c55e', count: '3 项', tokens: '≈36', pct: '0%' },
  { label: '注入内容', color: '#a855f7', count: '2 项', tokens: '≈3.8k', pct: '25%' },
  { label: '助手消息', color: '#3b82f6', count: '3 项', tokens: '≈222', pct: '1%', delta: '+1', tdelta: '+88' },
  { label: '工具结果', color: '#14b8a6', count: '2 项', tokens: '≈45', pct: '0%' },
]

const CTX_EVENTS = [
  { kind: '注入', label: '目录更新 · skill-catalog', at: '第 1 轮 · 第 1 步', tok: '+3.6k', time: '03:50:27' },
  { kind: '注入', label: '状态快照 · @deepseek-ai/dsh-system-prompt', at: '第 1 轮 · 第 1 步', tok: '+126', time: '03:50:27' },
]

export function ContextPanel() {
  return (
    <div className="lc-root">
      {/* Head row: stats + plugin info */}
      <div className="lc-cols lc-head">
        <div className="lc-card lc-col lc-col-stats">
          <div className="lc-card-title">
            <span className="lc-card-title-text">上下文统计</span>
            <span className="lc-card-sub">当前上下文存在和包含的内容</span>
          </div>
          <div className="lc-stats">
            {CTX_STATS.map(([label, value]) => (
              <div key={label} className="lc-stat">
                <span className="lc-stat-label">{label}</span>
                <b className="lc-stat-value">{value}</b>
              </div>
            ))}
            <div className="lc-stat lc-stat-tipped">
              <span className="lc-stat-label">预估费用<i className="lc-stat-q" aria-hidden="true">?</i></span>
              <b className="lc-stat-value">¥0.028</b>
            </div>
          </div>
        </div>
        <div className="lc-card">
          <div className="lc-card-title">
            <span className="lc-card-title-text">插件信息</span>
            <span className="lc-card-sub">The best DSH context plugin ⭐</span>
          </div>
          <div className="lc-pi-grid">
            <div className="lc-pi-row"><div className="lc-pi-label">插件</div><div className="lc-pi-value">dsh-context (v0.33.1)<span className="lc-pi-update">↑ v0.38.2</span></div></div>
            <div className="lc-pi-row"><div className="lc-pi-label">GitHub</div><div className="lc-pi-value">bowenliang123/dsh-context</div></div>
          </div>
        </div>
      </div>

      {/* Main row: left column (overview/trend/detail) + right browser */}
      <div className="lc-cols">
        <div className="lc-col">
          <div className="lc-card">
            <div className="lc-card-title">
              <span className="lc-card-title-text">当前上下文</span>
              <span className="lc-card-sub">deepseek-v4-flash · deepseek-official</span>
            </div>
            <div className="lc-overview-num">
              <b>16.6k</b><span> / 1.0M tokens</span>
              <span className="lc-overview-pct"><b>2%</b>上下文已用</span>
            </div>
            <div className="lc-stacked-wrap">
              <div className="lc-stacked" style={{ height: 16 }}>
                {CTX_LEGEND.map(l => (
                  <div key={l.label} className="lc-stacked-seg" style={{ width: l.pct, background: l.color }} />
                ))}
                <div className="lc-stacked-free" style={{ width: '80%' }} />
                <div className="lc-reserve" />
              </div>
            </div>
            <div className="lc-legend">
              {CTX_LEGEND.map(l => (
                <span key={l.label} className="lc-chip" title="占已用上下文">
                  <i style={{ background: l.color }} />
                  <span className="lc-chip-label">{l.label}</span>
                  <span className="lc-chip-nums">{l.num}<em>{l.pct}</em></span>
                </span>
              ))}
            </div>
          </div>

          <div className="lc-card">
            <div className="lc-card-title">
              <span className="lc-card-title-text">上下文趋势</span>
              <span className="lc-card-sub">✂ 表示压缩/剪枝，步骤/轮次 切换粒度</span>
              <div className="lc-trend-ctl">
                <div className="lc-gran"><button className="lc-gran-btn lc-gran-on">步骤</button><button className="lc-gran-btn">轮次</button></div>
                <div className="lc-gran"><button className="lc-gran-btn lc-gran-on">全量</button><button className="lc-gran-btn">增量</button></div>
              </div>
            </div>
            <div className="lc-chartrow">
              <div className="lc-axis"><span className="lc-axis-top">16.5k</span><span className="lc-axis-mid">8.3k</span><span className="lc-axis-bot">0</span></div>
              <div className="lc-chart-scroll">
                <div className="lc-chart">
                  <div className="lc-grid lc-grid-top" />
                  <div className="lc-grid lc-grid-mid" />
                  {TREND_BARS.map((b, i) => (
                    <div key={i} className="lc-bar" style={{ width: '14px' }}>
                      <div className="lc-bar-stack">
                        {b.seg.map(([c, h], j) => <div key={j} style={{ height: h, background: c }} />)}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            </div>
            <div className="lc-turns"><span className="lc-turn"><span className="lc-turn-label">T1</span></span></div>
          </div>

          <div className="lc-card">
            <div className="lc-detail-head">
              <b>第 1 轮 · 第 3 步</b><span className="lc-detail-time">03:58:00</span>
              <span className="lc-detail-metric">实际 prompt 16.5k</span>
              <span className="lc-detail-metric">输出 101</span>
              <span className="lc-detail-metric">缓存 99.29%</span>
            </div>
            <div className="lc-brief">
              <div className="lc-brief-row"><span className="lc-brief-tag">本轮</span><span className="lc-brief-text" title="请执行命令 Start-Sleep -Seconds 450 然后回复完成">请执行命令 Start-Sleep -Seconds 450 然后回复完成</span></div>
              <div className="lc-brief-row"><span className="lc-brief-tag">输入</span><span className="lc-brief-chip lc-brief-chip-tag">job_output</span><span className="lc-brief-chip lc-brief-chip-text">测试2</span><span className="lc-brief-chip lc-brief-chip-text">测试3</span></div>
              <div className="lc-brief-row"><span className="lc-brief-tag">回复</span><span className="lc-brief-chip lc-brief-chip-grow">已完成 `Start-Sleep -Seconds 450`，命令正常结束（exit code 0），输出为 `DONE`。</span></div>
            </div>
            <div className="lc-detail-rows">
              {DETAIL_ROWS.map(r => (
                <div key={r.label} className="lc-detail-row">
                  <i style={{ background: r.color }} />
                  <span className="lc-detail-label">{r.label}</span>
                  <span className="lc-bar-track"><span className="lc-bar-fill" style={{ width: r.w, background: r.color }} /></span>
                  <span className="lc-detail-num">{r.num}</span>
                  <span className="lc-detail-pct">{r.pct}</span>
                </div>
              ))}
            </div>
          </div>
        </div>

        <div className="lc-col lc-col-browser">
          <div className="lc-card">
            <div className="lc-card-title">
              <span className="lc-card-title-text">上下文浏览器</span>
              <span className="lc-card-sub">对比上轮末步的变动</span>
              <span className="lc-br-pick">当前（下一次请求）</span>
            </div>
            <div className="lc-br-meta"><b>当前 · 下一次请求</b><span>估算合计 ≈ 15.1k</span></div>
            <div className="lc-br-bar">
              <div className="lc-stacked" style={{ height: 10 }}>
                {CTX_LEGEND.map(l => <div key={l.label} className="lc-stacked-seg" style={{ width: l.pct, background: l.color }} />)}
              </div>
            </div>
            <div className="lc-br-cats">
              {BR_CATS.map(c => (
                <div key={c.label} className="lc-br-cat">
                  <div className="lc-br-cat-row">
                    <span className="lc-br-chev">▸</span>
                    <i style={{ background: c.color }} />
                    <span className="lc-br-cat-label">{c.label}</span>
                    <span className="lc-br-cat-count">{c.count}{c.delta && <span className="lc-br-delta lc-br-delta-up">{c.delta}</span>}</span>
                    <span className="lc-br-tokens-grp">{c.tdelta && <span className="lc-br-tdelta lc-br-tdelta-up">{c.tdelta}</span>}<span className="lc-br-tokens">{c.tokens}</span></span>
                    <span className="lc-br-pct">{c.pct}</span>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>

      {/* Bottom row: events + file activity */}
      <div className="lc-cols">
        <div className="lc-card lc-col">
          <div className="lc-card-title">
            <span className="lc-card-title-text">上下文事件</span>
          </div>
          <div className="lc-events">
            {CTX_EVENTS.map((ev, i) => (
              <div key={i} className="lc-event">
                <span className="lc-kind lc-kind-inject">{ev.kind}</span>
                <span className="lc-event-label">{ev.label}</span>
                <span className="lc-event-at">{ev.at}</span>
                <span className="lc-event-tokens lc-up">{ev.tok}</span>
                <span className="lc-event-time">{ev.time}</span>
              </div>
            ))}
          </div>
        </div>
        <div className="lc-card lc-col">
          <div className="lc-card-title">
            <span className="lc-card-title-text">文件活动</span>
            <span className="lc-card-sub">截至最新 · 跟随趋势图的选择</span>
          </div>
          <div className="lc-empty">该范围内没有文件读取、写入或搜索操作</div>
        </div>
      </div>

      <div className="lc-foot">估算口径：与 dsh 内置 tokenMeter 相同的固定密度启发式（约 4 字符 ≈ 1 token）；「实际」为供应商上报用量。</div>
    </div>
  )
}