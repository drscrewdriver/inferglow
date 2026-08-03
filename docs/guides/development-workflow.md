# InferGlow TDD 开发工作流

## 1. TDD 核心原则

InferGlow 严格遵循 **Red-Green-Refactor** 循环：

1. **Red** — 先编写一个失败的测试，明确你要实现的功能或要修复的行为。
2. **Green** — 编写 **最少量的代码** 让测试通过，不追求完美，只求通过。
3. **Refactor** — 在测试的保护下重构代码，消除重复、改善设计，确保测试依然全部通过。

> 核心信条：**没有测试的代码，就是遗留代码。**

---

## 2. 测试命名规范

测试函数采用以下格式：

```
TestXxx_WhenYyy_Zzz
```

- `Test` 开头（Go 编译器要求）
- `Xxx` — 被测函数或方法名
- `WhenYyy` — 测试场景描述
- `Zzz` — 期望结果

**示例：**

```go
func TestParse_WhenValidInput_ReturnsRule(t *testing.T) { ... }
func TestParse_WhenEmptyString_ReturnsError(t *testing.T) { ... }
func TestApply_WhenScoreBelowThreshold_ReturnsEmptyAction(t *testing.T) { ... }
```

---

## 3. 测试文件位置

- 每个 Go 包内，测试文件与源文件同目录。
- 命名格式：`xxx_test.go`（与源文件 `xxx.go` 对应）。

```
pkg/rewriter/
├── rewriter.go
├── rewriter_test.go       ← 测试文件
├── rule.go
└── rule_test.go           ← 测试文件
```

---

## 4. 运行方式

```bash
# 运行单个模块的所有测试
go test ./pkg/rewriter/...

# 运行整个项目的所有测试
go test ./...

# 带覆盖率输出
go test ./... -coverprofile=coverage.out

# 查看覆盖率详情
go tool cover -html=coverage.out
```

---

## 5. 代码审查要求

所有 Pull Request **必须包含测试**，且测试必须证明功能正确性：

- 新功能必须附带对应的单元测试。
- Bug 修复必须先提交暴露该 Bug 的失败测试，再提交修复代码。
- 审查者有权拒绝任何没有测试的 PR。

---

## 6. Go 测试约定

在 InferGlow 项目中，我们遵循以下 Go 测试最佳实践：

| 实践 | 说明 | 示例 |
|------|------|------|
| `t.Helper()` | 辅助函数标记为测试辅助，失败时报告调用者行号 | `func assertRule(t *testing.T, got, want Rule) { t.Helper(); ... }` |
| `t.Cleanup()` | 注册清理函数，避免泄漏 | `t.Cleanup(func() { os.RemoveAll(tmpDir) })` |
| `t.Run()` | 子测试，组织同类场景 | `t.Run("valid input", func(t *testing.T) { ... })` |
| `require` | 使用 `github.com/stretchr/testify/require`，失败即停止 | `require.Equal(t, want, got)` |

**推荐使用 `require` 而非 `assert`**：一旦断言失败，尽早停止可以避免混淆的级联错误。

---

## 7. 测试覆盖原则

- **核心路径** — 功能的主流程必须覆盖。
- **边界条件** — 空值、零值、最大值、极限情况。
- **错误路径** — 输入无效、依赖失败、权限不足等。
- **不过度追求 100%** — 工具代码、简单 getter/setter、样板代码可跳过。

目标：核心逻辑覆盖率 **≥ 80%**，整体项目覆盖率 **≥ 60%**。

---

## 8. 示例：一次完整的 TDD 循环

假设我们要实现一个简单的加法函数 `Add(a, b int) int`。

### 🔴 Red — 先写测试

```go
// math_test.go
package math

import (
    "testing"
    "github.com/stretchr/testify/require"
)

func TestAdd_WhenPositiveNumbers_ReturnsSum(t *testing.T) {
    result := Add(2, 3)
    require.Equal(t, 5, result)
}

func TestAdd_WhenNegativeNumbers_ReturnsSum(t *testing.T) {
    result := Add(-1, -2)
    require.Equal(t, -3, result)
}
```

运行测试：`go test ./...` → 编译失败（`Add` 未定义）。✅ Red 确认。

### 🟢 Green — 写最小代码通过

```go
// math.go
package math

func Add(a, b int) int {
    return a + b
}
```

运行测试：全部通过。✅ Green 确认。

### 🔵 Refactor — 重构

此例代码已足够简单，无需重构。若存在重复或可改进之处，在测试保护下安全修改，然后重新运行测试确保全部通过。

---

*InferGlow 团队 · 让每一行代码都有测试守护。*