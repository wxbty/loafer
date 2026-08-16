import request from '@/utils/request'
import { consumeSseStream } from '@/utils/sseStream'

/**
 * AI 拆解任务（SSE 流式）
 * 返回 AbortController 供调用方取消请求
 * @param mock 如果为 true，直接返回预设的 mock 数据，不调用 AI
 */
export function aiDecomposeStream(
  description: string,
  projectId: number | null | undefined,
  planFiles: string[] | null | undefined,
  onOutput: (line: string) => void,
  onDone: (fullResult: string) => void,
  onError: (msg: string) => void,
  mock: boolean = false
): AbortController {
  const controller = new AbortController()
  const baseURL = import.meta.env.VITE_API_BASE_URL || '/api'

  const token = localStorage.getItem('token')
  const headers: HeadersInit = {
    'Content-Type': 'application/json',
    'Accept': 'text/event-stream',
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  fetch(`${baseURL}/tasks/ai-decompose`, {
    method: 'POST',
    headers,
    body: JSON.stringify({
      description,
      projectId: projectId ?? undefined,
      planFiles: planFiles ?? undefined,
      mock: mock || undefined,
    }),
    signal: controller.signal,
  })
    .then(async (response) => {
      if (!response.ok) {
        const text = await response.text().catch(() => '')
        onError(`请求失败: ${response.status} ${text}`)
        return
      }
      const reader = response.body?.getReader()
      if (!reader) {
        onError('无法获取响应流')
        return
      }
      const decoder = new TextDecoder('utf-8')
      let buffer = ''
      /** 当前 SSE 帧的 event 名（未遇到 event: 时默认为 message，与 Spring 行为一致） */
      let frameEvent = 'message'
      /** 同一帧内可多行 data:，须用换行拼接后再交给 onDone，否则长 JSON 会被拆碎导致 JSON.parse 失败 */
      const frameDataLines: string[] = []

      const flushSseFrame = () => {
        if (frameDataLines.length === 0) return
        const payload = frameDataLines.join('\n')
        frameDataLines.length = 0
        const ev = frameEvent
        frameEvent = 'message'
        switch (ev) {
          case 'done':
            onDone(payload)
            break
          case 'error':
            onError(payload)
            break
          default:
            onOutput(payload)
            break
        }
      }

      try {
        while (true) {
          const { done, value } = await reader.read()
          if (done) break

          buffer += decoder.decode(value, { stream: true })
          const lines = buffer.split('\n')
          buffer = lines.pop() || ''

          for (const raw of lines) {
            const line = raw.replace(/\r$/, '')
            if (line === '') {
              flushSseFrame()
              continue
            }
            if (line.startsWith('event:')) {
              flushSseFrame()
              frameEvent = line.slice(6).trim()
              continue
            }
            if (line.startsWith('data:')) {
              // SSE: "data:" 后允许一个空格
              const valuePart = line.slice(5).startsWith(' ') ? line.slice(6) : line.slice(5)
              frameDataLines.push(valuePart)
            }
          }
        }
        // 流结束时尚未以 \n 结尾的最后一行
        if (buffer.length > 0) {
          const line = buffer.replace(/\r$/, '')
          if (line === '') {
            flushSseFrame()
          } else if (line.startsWith('event:')) {
            flushSseFrame()
            frameEvent = line.slice(6).trim()
          } else if (line.startsWith('data:')) {
            const valuePart = line.slice(5).startsWith(' ') ? line.slice(6) : line.slice(5)
            frameDataLines.push(valuePart)
          }
        }
        flushSseFrame()
      } catch (e: unknown) {
        const msg = e instanceof Error ? e.message : String(e)
        const lower = msg.toLowerCase()
        if (lower.includes('incomplete_chunked') || lower.includes('err_incomplete_chunked')) {
          onError(
            '流式响应被中断（ERR_INCOMPLETE_CHUNKED_ENCODING）：多为网关/Tomcat 超时或缓冲导致，请拉长反代与 Tomcat 超时、关闭 proxy_buffering，并部署最新后端配置。'
          )
        } else {
          onError(`读取 SSE 流中断: ${msg}`)
        }
      }
    })
    .catch((err: unknown) => {
      if (err instanceof DOMException && err.name === 'AbortError') return
      if (err instanceof Error && err.name === 'AbortError') return
      const raw = err instanceof Error ? err.message || err.name : String(err)
      const lower = raw.toLowerCase()
      if (
        lower.includes('failed to fetch') ||
        lower.includes('networkerror') ||
        lower.includes('network error') ||
        lower.includes('incomplete_chunked') ||
        lower.includes('err_incomplete_chunked') ||
        lower === 'load failed'
      ) {
        onError(
          `网络或流式连接异常（含 ERR_INCOMPLETE_CHUNKED_ENCODING 时多为反代/Tomcat 掐断长连接）: ${raw}`
        )
        return
      }
      onError(raw || 'SSE 连接失败')
    })

  return controller
}

/**
 * 任务分片执行 SSE（失败重试等），事件名与拆解一致：output / done / error
 * @param onStreamEnd 流完全结束（含连接关闭、abort）时必调一次，用于结束 loading；可与 onDone/onError 同时发生
 */
export function taskExecuteStream(
  taskId: number,
  onOutput: (line: string) => void,
  onDone: (payload: string) => void,
  onError: (msg: string) => void,
  onStreamEnd?: () => void,
  /** 失败重试时是否跳过上一次已通过的步骤，默认 true */
  skipPassed: boolean = true
): AbortController {
  const controller = new AbortController()
  const baseURL = import.meta.env.VITE_API_BASE_URL || '/api'

  const streamEnd = () => {
    onStreamEnd?.()
  }

  const token = localStorage.getItem('token')
  const headers: HeadersInit = {
    Accept: 'text/event-stream',
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  fetch(`${baseURL}/tasks/${taskId}/execute-stream?skipPassed=${skipPassed}`, {
    method: 'POST',
    headers,
    signal: controller.signal,
  })
    .then(async (response) => {
      if (!response.ok) {
        const text = await response.text().catch(() => '')
        onError(`请求失败: ${response.status} ${text}`)
        streamEnd()
        return
      }
      const reader = response.body?.getReader()
      if (!reader) {
        onError('无法获取响应流')
        streamEnd()
        return
      }
      const decoder = new TextDecoder('utf-8')
      let buffer = ''
      let frameEvent = 'message'
      const frameDataLines: string[] = []

      const flushSseFrame = () => {
        if (frameDataLines.length === 0) return
        const payload = frameDataLines.join('\n')
        frameDataLines.length = 0
        const ev = frameEvent
        frameEvent = 'message'
        switch (ev) {
          case 'done':
            onDone(payload)
            break
          case 'error':
            onError(payload)
            break
          default:
            onOutput(payload)
            break
        }
      }

      const ingestLine = (line: string) => {
        if (line === '') {
          flushSseFrame()
          return
        }
        if (line.startsWith(':')) {
          return
        }
        if (line.startsWith('event:')) {
          flushSseFrame()
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

          for (const raw of lines) {
            ingestLine(raw.replace(/\r$/, ''))
          }
        }

        // 连接已关闭：把缓冲区里剩余整行全部吃掉（避免仅处理了 event: 而丢了同行的 data）
        while (buffer.includes('\n')) {
          const nl = buffer.indexOf('\n')
          ingestLine(buffer.slice(0, nl).replace(/\r$/, ''))
          buffer = buffer.slice(nl + 1)
        }
        if (buffer.length > 0) {
          ingestLine(buffer.replace(/\r$/, ''))
          buffer = ''
        }

        flushSseFrame()
      } catch (e: unknown) {
        const aborted =
          (e instanceof DOMException && e.name === 'AbortError') ||
          (e instanceof Error && e.name === 'AbortError')
        if (!aborted) {
          const msg = e instanceof Error ? e.message : String(e)
          const lower = msg.toLowerCase()
          if (lower.includes('incomplete_chunked') || lower.includes('err_incomplete_chunked')) {
            onError(
              '流式响应被中断（ERR_INCOMPLETE_CHUNKED_ENCODING）：多为网关/Tomcat 超时或缓冲导致，请拉长反代与 Tomcat 超时、关闭 proxy_buffering。'
            )
          } else {
            onError(`读取 SSE 流中断: ${msg}`)
          }
        }
      } finally {
        flushSseFrame()
        streamEnd()
      }
    })
    .catch((err: unknown) => {
      if (err instanceof DOMException && err.name === 'AbortError') {
        streamEnd()
        return
      }
      if (err instanceof Error && err.name === 'AbortError') {
        streamEnd()
        return
      }
      const raw = err instanceof Error ? err.message || err.name : String(err)
      const lower = raw.toLowerCase()
      if (
        lower.includes('failed to fetch') ||
        lower.includes('networkerror') ||
        lower.includes('network error') ||
        lower.includes('incomplete_chunked') ||
        lower.includes('err_incomplete_chunked') ||
        raw === 'load failed'
      ) {
        onError(`网络或流式连接异常: ${raw}`)
        streamEnd()
        return
      }
      onError(raw || 'SSE 连接失败')
      streamEnd()
    })

  return controller
}

/**
 * 已完成任务：SSE 调用 Claude Code 生成执行总结，并写入 task_state.execution_summary
 */
export function taskSummarizeExecutionStream(
  taskId: number,
  onOutput: (line: string) => void,
  onDone: (payload: string) => void,
  onError: (msg: string) => void,
  onStreamEnd?: () => void
): AbortController {
  const controller = new AbortController()
  const baseURL = import.meta.env.VITE_API_BASE_URL || '/api'

  const streamEnd = () => {
    onStreamEnd?.()
  }

  const token = localStorage.getItem('token')
  const headers: HeadersInit = {
    Accept: 'text/event-stream',
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  fetch(`${baseURL}/tasks/${taskId}/summarize-execution-stream`, {
    method: 'POST',
    headers,
    signal: controller.signal,
  })
    .then(async (response) => {
      if (!response.ok) {
        const text = await response.text().catch(() => '')
        onError(`请求失败: ${response.status} ${text}`)
        streamEnd()
        return
      }
      const reader = response.body?.getReader()
      if (!reader) {
        onError('无法获取响应流')
        streamEnd()
        return
      }
      const decoder = new TextDecoder('utf-8')
      let buffer = ''
      let frameEvent = 'message'
      const frameDataLines: string[] = []

      const flushSseFrame = () => {
        if (frameDataLines.length === 0) return
        const payload = frameDataLines.join('\n')
        frameDataLines.length = 0
        const ev = frameEvent
        frameEvent = 'message'
        switch (ev) {
          case 'done':
            onDone(payload)
            break
          case 'error':
            onError(payload)
            break
          default:
            onOutput(payload)
            break
        }
      }

      const ingestLine = (line: string) => {
        if (line === '') {
          flushSseFrame()
          return
        }
        if (line.startsWith(':')) {
          return
        }
        if (line.startsWith('event:')) {
          flushSseFrame()
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

          for (const raw of lines) {
            ingestLine(raw.replace(/\r$/, ''))
          }
        }

        while (buffer.includes('\n')) {
          const nl = buffer.indexOf('\n')
          ingestLine(buffer.slice(0, nl).replace(/\r$/, ''))
          buffer = buffer.slice(nl + 1)
        }
        if (buffer.length > 0) {
          ingestLine(buffer.replace(/\r$/, ''))
          buffer = ''
        }

        flushSseFrame()
      } catch (e: unknown) {
        const aborted =
          (e instanceof DOMException && e.name === 'AbortError') ||
          (e instanceof Error && e.name === 'AbortError')
        if (!aborted) {
          const msg = e instanceof Error ? e.message : String(e)
          const lower = msg.toLowerCase()
          if (lower.includes('incomplete_chunked') || lower.includes('err_incomplete_chunked')) {
            onError(
              '流式响应被中断（ERR_INCOMPLETE_CHUNKED_ENCODING）：多为网关/Tomcat 超时或缓冲导致，请拉长反代与 Tomcat 超时、关闭 proxy_buffering。'
            )
          } else {
            onError(`读取 SSE 流中断: ${msg}`)
          }
        }
      } finally {
        flushSseFrame()
        streamEnd()
      }
    })
    .catch((err: unknown) => {
      if (err instanceof DOMException && err.name === 'AbortError') {
        streamEnd()
        return
      }
      if (err instanceof Error && err.name === 'AbortError') {
        streamEnd()
        return
      }
      const raw = err instanceof Error ? err.message || err.name : String(err)
      const lower = raw.toLowerCase()
      if (
        lower.includes('failed to fetch') ||
        lower.includes('networkerror') ||
        lower.includes('network error') ||
        lower.includes('incomplete_chunked') ||
        lower.includes('err_incomplete_chunked') ||
        raw === 'load failed'
      ) {
        onError(`网络或流式连接异常: ${raw}`)
        streamEnd()
        return
      }
      onError(raw || 'SSE 连接失败')
      streamEnd()
    })

  return controller
}

// 任务相关 API
export const TaskApi = {
  // 获取任务列表
  getTaskList: () => request.get('/tasks/list'),
  // 分页获取任务列表
  getTaskPage: (current: number, size: number) => request.get(`/tasks/page?current=${current}&size=${size}`),
  // 获取任务详情
  getTaskDetail: (id: number) => request.get(`/tasks/${id}`),
  /** 与启动 Claude 时一致：前置依赖上下文 + 项目概况（供任务详情页展示） */
  getTaskPromptContext: (id: number) => request.get(`/tasks/${id}/prompt-context`),
  // 创建任务
  createTask: (data: any) => request.post('/tasks/create', data),
  // 更新任务
  updateTask: (id: number, data: any) => request.put(`/tasks/${id}`, data),
  // 删除任务
  deleteTask: (id: number) => request.delete(`/tasks/${id}`),
  // 根据状态获取任务列表
  getTasksByStatus: (status: number) => request.get(`/tasks/by-status?status=${status}`),
  // 启动任务
  // - skipPassed：失败重试时是否跳过上一次已通过的步骤，默认 true
  startTask: (id: number, skipPassed?: boolean) => {
    const sp = skipPassed === false ? false : true
    return request.put(`/tasks/${id}/start?skipPassed=${sp}`)
  },
  // 暂停任务
  pauseTask: (id: number) => request.put(`/tasks/${id}/pause`),
  // 恢复任务
  resumeTask: (id: number) => request.put(`/tasks/${id}/resume`),
  // 停止任务
  stopTask: (id: number) => request.put(`/tasks/${id}/stop`),
  // 完成任务
  completeTask: (id: number) => request.put(`/tasks/${id}/complete`),
  // 任务进入审查
  reviewTask: (id: number) => request.put(`/tasks/${id}/review`),

  // ===== 执行控制 API =====
  // 启动项目执行
  startProjectExecution: (projectId: number) => request.post(`/tasks/projects/${projectId}/execute`),
  // 暂停项目执行
  pauseProjectExecution: (projectId: number) => request.post(`/tasks/projects/${projectId}/pause`),
  // 恢复项目执行
  resumeProjectExecution: (projectId: number) => request.post(`/tasks/projects/${projectId}/resume`),
  // 停止项目执行
  stopProjectExecution: (projectId: number) => request.post(`/tasks/projects/${projectId}/stop`),
  // 获取项目执行状态
  getProjectExecutionStatus: (projectId: number) => request.get(`/tasks/projects/${projectId}/execution-status`),

  // ===== 集成测试 API =====
  // 检查模块是否可执行集成测试
  checkModuleCanTest: (moduleId: number) => request.get(`/tasks/modules/${moduleId}/can-test`),
  // 执行模块集成测试
  executeModuleTest: (moduleId: number) => request.post(`/tasks/modules/${moduleId}/test`),

  // ===== 步骤追加 =====
  // 给已有任务追加步骤（表单方式，后端会合并到 stepsJson 并自动延续 seq）
  appendSteps: (taskId: number, steps: any[]) =>
    request.post(`/tasks/${taskId}/append-steps`, { steps }),

  // ===== 单个步骤编辑 =====
  // 更新单个步骤（stepIndex基于0，仅允许待办/暂停/失败状态）
  updateStep: (taskId: number, stepIndex: number, stepData: any) =>
    request.put(`/tasks/${taskId}/steps/${stepIndex}`, stepData),
  // 删除单个步骤（stepIndex基于0，仅允许待办/暂停/失败状态，删除后自动重排seq）
  deleteStep: (taskId: number, stepIndex: number) =>
    request.delete(`/tasks/${taskId}/steps/${stepIndex}`)
}

/**
 * 给已有任务追加步骤（AI 流式拆解）
 */
export function appendStepsToTaskStream(
  taskId: number,
  description: string,
  planFiles: string[] | null | undefined,
  onOutput: (line: string) => void,
  onDone: (fullResult: string) => void,
  onError: (msg: string) => void
): AbortController {
  const baseURL = import.meta.env.VITE_API_BASE_URL || '/api'
  return consumeSseStream(
    `${baseURL}/tasks/${taskId}/append-steps-stream`,
    { description, planFiles: planFiles ?? undefined },
    { onOutput, onDone, onError }
  )
}
