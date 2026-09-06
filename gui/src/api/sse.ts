// SSE wire-format parser for the stream-run endpoint.
//
// The server writes `event: <type>\n data: <json>\n\n` frames (see
// writeSSEEvent in server/handlers.go). EventSource cannot be used because
// stream-run is POST, so we consume the response body with fetch +
// ReadableStream. The parser buffers across chunk boundaries and handles
// UTF-8 multi-byte sequences split between chunks.

export interface SSEFrame {
  event: string
  data: string
}

/** Splits raw text into complete `event:`/`data:` frames. Returns the frames
 * and any trailing partial buffer (cross-chunk frame tails). Exported for
 * unit testing. CRLF line endings are normalized before splitting because a
 * `\r\n\r\n` terminator does not contain the bare `\n\n` separator. */
export function splitFrames(buf: string): { frames: SSEFrame[]; rest: string } {
  const frames: SSEFrame[] = []
  const norm = buf.replace(/\r\n/g, '\n')
  let rest = norm
  for (;;) {
    const idx = rest.indexOf('\n\n')
    if (idx === -1) break
    const block = rest.slice(0, idx)
    rest = rest.slice(idx + 2)
    const frame: SSEFrame = { event: '', data: '' }
    for (const line of block.split('\n')) {
      if (line.startsWith('event:')) frame.event = line.slice(6).trim()
      else if (line.startsWith('data:')) frame.data = line.slice(5).trim()
    }
    frames.push(frame)
  }
  return { frames, rest }
}

/** Consumes a Response body as an SSE stream, invoking onFrame for every
 * complete frame until EOF or abort. */
export async function parseSSE(
  resp: Response,
  onFrame: (frame: SSEFrame) => void,
  signal?: AbortSignal,
): Promise<void> {
  if (!resp.body) return
  const reader = resp.body.getReader()
  const decoder = new TextDecoder('utf-8')
  let buffer = ''
  // Abort cancels a pending read() on an idle stream (e.g. stop button).
  const onAbort = () => {
    void reader.cancel()
  }
  signal?.addEventListener('abort', onAbort, { once: true })
  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const { frames, rest } = splitFrames(buffer)
      buffer = rest
      for (const f of frames) onFrame(f)
      if (signal?.aborted) return
    }
    // Flush any final partial block (streams are normally \n\n-terminated).
    const { frames, rest } = splitFrames(buffer)
    if (rest.trim() !== '') {
      const frame: SSEFrame = { event: '', data: rest.trim() }
      frames.push(frame)
    }
    for (const f of frames) onFrame(f)
  } finally {
    signal?.removeEventListener('abort', onAbort)
  }
}
