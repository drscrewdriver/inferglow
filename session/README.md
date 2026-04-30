# session - 对话记忆管理

**模块路径**: `github.com/inferglow/session`

## 概述

session 模块提供对话记忆管理功能，包括对话历史维护、上下文窗口自动裁剪、多模态内容支持和 JSON/YAML 持久化。

## 设计定位

- **被谁依赖**: 上层业务逻辑（agently 主模块的 Agent 类通过 SessionExtension）
- **依赖谁**: 无第三方库（仅依赖 stdlib）— **不依赖 model 模块**（有自己的 ChatMessage 定义）
- **对标 Python**: `agently/core/session/Session.py`
- **独立可用性**: ✅ 完全独立

## 核心类型

### Session — 会话对象

```go
type Session struct {
    ID             string            // 会话 ID
    FullContext    []ChatMessage     // 完整历史（永不裁剪）
    ContextWindow  []ChatMessage     // 当前窗口（可能被 resize）
    Memo           map[string]any    // 长期记忆
    MaxLength      int               // 窗口大小上限（字节）
    AutoResize     bool              // 是否自动裁剪
    ResizeHandler  ResizeHandler     // 旧式 resize 回调
    resizeHandlers map[string]ResizeHandler  // 多策略注册
    analysisHandlers []AnalysisHandler         // 分析器列表
    defaultResizeName string                // 默认策略名
}
```

**关键设计决策：**
- `FullContext` 始终追加、永不裁剪，保证完整对话审计追溯
- `ContextWindow` 是发送给模型的"有效窗口"，可能被 resize 策略裁剪
- 两者的初始状态同步，但 resize 只会裁剪 `ContextWindow`

### ChatMessage — 聊天消息

```go
type ChatMessage struct {
    Role      string         // "system" | "user" | "assistant" | "tool"
    Content   any            // string | []ContentBlock
    Name      string         // 消息作者名
    Meta      map[string]any // 元数据
    Timestamp time.Time
}

type ContentBlock struct {
    Type string         // "text" | "image" | "video" | "audio" | "file"
    Data any            // 具体数据
    Meta map[string]any
}
```

### ResizeHandler — 窗口裁剪处理器

```go
type ResizeHandler func(fullContext []ChatMessage, contextWindow []ChatMessage) ([]ChatMessage, error)

// 分析器：决定是否触发裁剪
type AnalysisHandler func(full []ChatMessage, window []ChatMessage, memo map[string]any) (string, error)
```

## 核心功能

### 1. 消息管理

```go
session := NewSession("my-session", 8000)

// 添加消息（自动触发 resize）
session.AddMessage("user", "Hello", "")
session.AddMessage("assistant", "Hi! How can I help?", "")

// 获取上下文
prompt := session.PreparePrompt()        // 返回 ContextWindow（字符串化）
fullCtx := session.GetFullContext()      // 返回完整历史
window := session.GetContextWindow()     // 返回当前窗口
```

### 2. 上下文窗口管理

#### 默认分析器

```go
// 检查 ContextWindow 总字节数是否超过 MaxLength
session.AutoResize = true
session.RegisterResizeHandler("simple_cut", SimpleCutResizeHandler)
session.SetDefaultResizeHandler("simple_cut")
```

#### 内置 Resize 策略

| 策略 | 文件 | 说明 |
|------|------|------|
| `SimpleCutResizeHandler` | resize.go | 从前面丢弃消息，保留最近的 |
| `SummaryFirstResizeHandler` | resize.go | 保留首条 + 末尾 2 条，中间生成摘要 |
| `TokenAwareResizeHandler` | resize.go | Token 感知（约 len/4 = 1 token） |

```go
// 自定义 Token 感知策略
session.RegisterResizeHandler("token_aware", TokenAwareResizeHandlerWithMax(4000))
```

### 3. 多策略调度

通过 `AnalysisHandler` 列表实现多策略分析：

```go
// 注册多个分析器（按顺序调用）
session.RegisterAnalysisHandler(func(full, window []ChatMessage, memo map[string]any) (string, error) {
    if len(window) > 100 {
        return "token_aware", nil
    }
    // 检查是否超长
    return "", nil
})

// 注册多个裁剪策略
session.RegisterResizeHandler("simple_cut", SimpleCutResizeHandler)
session.RegisterResizeHandler("token_aware", TokenAwareResizeHandlerWithMax(4000))
session.RegisterResizeHandler("summary_first", SummaryFirstResizeHandler)
session.SetDefaultResizeHandler("simple_cut")
```

### 4. 持久化

```go
// 导出
jsonStr, _ := session.ToJSON()
yamlStr, _ := session.ToYAML()
session.SaveJSON("session.json")
session.SaveYAML("session.yaml")

// 加载（支持直接字符串或文件路径）
session.LoadJSON("{\"id\":\"...\",...}")
session.LoadYAML("session.yaml")
```

## 核心接口一览

```
Session               → 会话对象
ChatMessage           → 聊天消息
ContentBlock          → 多模态内容块
ResizeHandler         → 窗口裁剪处理器
AnalysisHandler       → 分析器（决定触发哪种裁剪）
SimpleCutResizeHandler → 简单截断策略
SummaryFirstResizeHandler → 摘要策略
TokenAwareResizeHandler → Token 感知策略
```

## 与 model 的区别

session 模块**有自己的 ChatMessage 定义**（不依赖 model 包），主要区别：

| 特性 | model.ChatMessage | session.ChatMessage |
|------|-------------------|---------------------|
| 模块 | github.com/inferglow/model | github.com/inferglow/session |
| Content 类型 | string | string \| []ContentBlock |
| 多模态 | ❌ | ✅ (text/image/video/audio/file) |
| 用途 | 传给 LLM 的原始消息 | 会话记忆管理 |

这种设计使得 session 可以独立演进多模态支持，不受 model 模块约束。

## 与上层的关系

```
agently 主模块 (Agent 类)
  ├── Agent 通过 SessionExtension 注入 session 能力
  ├── agent.activate_session("id") → 创建/激活 Session
  ├── session.AddMessage() → 自动 resize
  ├── session.PreparePrompt() → 注入到 LLM 请求
  ├── extension hook: request_prefixes → 请求前注入 chat_history
  ├── extension hook: finally → 请求后记录对话轮次
  └── session.SaveJSON() → 持久化到磁盘
```
