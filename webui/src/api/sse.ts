/* InferGlow Web UI — SSE 流式事件解析器 */

export interface SSEEvent {
  event: string
  data: string
}

/**
 * 解析 SSE 流，逐条 yield 事件。
 * 使用方式：
 *   for await (const ev of consumeSSE(response)) { ... }
 */
export async function* consumeSSE(
  response: Response,
): AsyncGenerator<SSEEvent, void, unknown> {
  const reader = response.body?.getReader()
  if (!reader) return

  const decoder = new TextDecoder()
  let buffer = ''
  let eventName = ''

  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() ?? ''

      for (const line of lines) {
        if (line.startsWith('event:')) {
          eventName = line.slice(6).trim()
        } else if (line.startsWith('data:')) {
          const data = line.slice(5).trimStart()
          yield { event: eventName || 'message', data }
          eventName = ''
        } else if (line === '') {
          // 空行 = 事件分隔
          eventName = ''
        }
      }
    }
  } finally {
    reader.releaseLock()
  }
}
