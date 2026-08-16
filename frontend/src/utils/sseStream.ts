/**
 * 与后端 SseEmitter 协议对齐的浏览器端 SSE 消费器。
 * 与 utils/request 的 axios 不同，这里必须用 fetch 才能流式读取 body。
 * 事件名 → 回调：
 *   - output（默认/message）→ onOutput（每帧一次，可能含多行 data）
 *   - done                    → onDone（整帧 payload）
 *   - error                   → onError（整帧 payload）
 * 网络异常或 abort 不会触发 onError，按需在调用方 catch 处理。
 */
export interface SseCallbacks {
  onOutput: (payload: string) => void
  onDone: (payload: string) => void
  onError: (msg: string) => void
}

export function consumeSseStream(
  url: string,
  body: unknown,
  callbacks: SseCallbacks
): AbortController {
  const controller = new AbortController()
  const token = localStorage.getItem('token')
  const headers: HeadersInit = {
    'Content-Type': 'application/json',
    Accept: 'text/event-stream',
  }
  if (token) headers['Authorization'] = `Bearer ${token}`

  // 兜底：流被后端静默 complete（典型为 SSE 10 分钟超时）时，确保至少触发一次 done 或 error，
  // 避免上层 loading 永远卡住。
  let terminated = false
  const safeDone = (payload: string) => {
    if (terminated) return
    terminated = true
    callbacks.onDone(payload)
  }
  const safeErr = (msg: string) => {
    if (terminated) return
    terminated = true
    callbacks.onError(msg)
  }

  fetch(url, {
    method: 'POST',
    headers,
    body: body == null ? undefined : JSON.stringify(body),
    signal: controller.signal,
  })
    .then(async (response) => {
      if (!response.ok) {
        const text = await response.text().catch(() => '')
        safeErr(`请求失败: ${response.status} ${text}`)
        return
      }
      const reader = response.body?.getReader()
      if (!reader) {
        safeErr('无法获取响应流')
        return
      }
      const decoder = new TextDecoder('utf-8')
      let buffer = ''
      let frameEvent = 'message'
      const frameDataLines: string[] = []

      const flushFrame = () => {
        if (frameDataLines.length === 0) return
        const payload = frameDataLines.join('\n')
        frameDataLines.length = 0
        const ev = frameEvent
        frameEvent = 'message'
        switch (ev) {
          case 'done':
            safeDone(payload)
            break
          case 'error':
            safeErr(payload)
            break
          default:
            callbacks.onOutput(payload)
            break
        }
      }

      const ingest = (line: string) => {
        if (line === '') {
          flushFrame()
          return
        }
        if (line.startsWith(':')) return
        if (line.startsWith('event:')) {
          flushFrame()
          frameEvent = line.slice(6).trim()
          return
        }
        if (line.startsWith('data:')) {
          const valuePart = line.slice(5).startsWith(' ') ? line.slice(6) : line.slice(5)
          frameDataLines.push(valuePart)
        }
      }

      try {
        while (true) {
          const { done, value } = await reader.read()
          if (done) break
          buffer += decoder.decode(value, { stream: true })
          const lines = buffer.split('\n')
          buffer = lines.pop() ?? ''
          for (const raw of lines) ingest(raw.replace(/\r$/, ''))
        }
        while (buffer.includes('\n')) {
          const nl = buffer.indexOf('\n')
          ingest(buffer.slice(0, nl).replace(/\r$/, ''))
          buffer = buffer.slice(nl + 1)
        }
        if (buffer.length > 0) {
          ingest(buffer.replace(/\r$/, ''))
        }
        flushFrame()
        if (!terminated) {
          safeErr('流式响应提前结束（未收到 done/error 帧）。如果 AI 已经返回，请到「LLM 调用日志」查看原始结果。')
        }
      } catch (e: unknown) {
        const aborted =
          (e instanceof DOMException && e.name === 'AbortError') ||
          (e instanceof Error && e.name === 'AbortError')
        if (aborted) return
        const msg = e instanceof Error ? e.message : String(e)
        const lower = msg.toLowerCase()
        if (lower.includes('incomplete_chunked') || lower.includes('err_incomplete_chunked')) {
          safeErr(
            '流式响应被中断（ERR_INCOMPLETE_CHUNKED_ENCODING）：多为网关/Tomcat 超时或缓冲导致，请拉长反代与 Tomcat 超时、关闭 proxy_buffering。'
          )
        } else {
          safeErr(`读取 SSE 流中断: ${msg}`)
        }
      }
    })
    .catch((err: unknown) => {
      if (err instanceof DOMException && err.name === 'AbortError') return
      if (err instanceof Error && err.name === 'AbortError') return
      const raw = err instanceof Error ? err.message || err.name : String(err)
      safeErr(raw || 'SSE 连接失败')
    })

  return controller
}
