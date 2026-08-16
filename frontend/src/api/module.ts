import request from '@/utils/request'
import { consumeSseStream } from '@/utils/sseStream'

// 模块相关 API
export const ModuleApi = {
  // 创建模块
  createModule: (data: any) => request.post('/modules', data),
  // 获取模块详情
  getModule: (id: number) => request.get(`/modules/${id}`),
  // 更新模块
  updateModule: (id: number, data: any) => request.put(`/modules/${id}`, data),
  // 删除模块
  deleteModule: (id: number) => request.delete(`/modules/${id}`),
  // 删除项目下所有模块和任务
  deleteAllByProjectId: (projectId: number) => request.delete(`/modules/project/${projectId}/all`),
  // 获取项目下所有模块
  getProjectModules: (projectId: number) => request.get(`/modules/project/${projectId}`),
  // 批量保存模块和任务（层级结构）
  batchSave: (projectId: number, modulesWithTasks: any[]) =>
    request.post('/modules/batch-save', { projectId, modulesWithTasks }),
  // 获取可执行的模块列表
  getExecutableModules: (projectId: number) => request.get(`/modules/project/${projectId}/executable`),
  // 更新模块状态
  updateModuleStatus: (id: number) => request.put(`/modules/${id}/status`),
  // 设置模块流水线模式（LEGACY/TDD）+ simpleMode（仅 LEGACY 生效）
  // 仅在模块状态 ∈ {0=待执行, 5=测试失败, 6=失败} 时允许切换
  setPipelineMode: (id: number, payload: { pipelineMode?: 'LEGACY' | 'TDD'; simpleMode?: 0 | 1 | boolean }) =>
    request.put(`/modules/${id}/pipeline-mode`, payload),
  // 给已有模块追加任务（表单方式）
  appendTasks: (moduleId: number, tasks: any[]) =>
    request.post(`/modules/${moduleId}/append-tasks`, { tasks }),
  // 单条 TDD assertion 重跑：基于已编译的 tdd_assertions_json 重跑指定断言，同步返回更新后的模块
  tddRunSingleAssertion: (moduleId: number, assertionId: string) =>
    request.post(`/modules/${moduleId}/tdd/run-assertion?assertionId=${encodeURIComponent(assertionId)}`),
  // 模块 TDD 修复历史列表（按时间倒序）
  getFixHistory: (moduleId: number) => request.get(`/modules/${moduleId}/fix-history`),
  // 模块 TDD 修复历史详情
  getFixHistoryDetail: (historyId: number) => request.get(`/modules/fix-history/${historyId}`),
}

/**
 * 给已有模块追加任务（AI 流式拆解）
 */
export function appendTasksToModuleStream(
  moduleId: number,
  description: string,
  planFiles: string[] | null | undefined,
  onOutput: (line: string) => void,
  onDone: (fullResult: string) => void,
  onError: (msg: string) => void
): AbortController {
  const baseURL = import.meta.env.VITE_API_BASE_URL || '/api'
  return consumeSseStream(
    `${baseURL}/modules/${moduleId}/append-tasks-stream`,
    { description, planFiles: planFiles ?? undefined },
    { onOutput, onDone, onError }
  )
}

// 模块化拆解 SSE 流式接口
export function moduleDecomposeStream(
  description: string,
  projectId: number | null | undefined,
  planFilePaths: string[] | null | undefined,
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

  // 兜底：当流式响应被中断或后端 emitter.complete() 时没发 done/error（典型场景：
  // Tomcat / SSE 10 分钟超时被强制 complete），下面这两个回调要保证 loading 至少会清掉。
  let terminated = false
  const safeOnDone = (payload: string) => {
    if (terminated) return
    terminated = true
    onDone(payload)
  }
  const safeOnError = (msg: string) => {
    if (terminated) return
    terminated = true
    onError(msg)
  }

  fetch(`${baseURL}/tasks/ai-decompose`, {
    method: 'POST',
    headers,
    body: JSON.stringify({
      description,
      projectId: projectId ?? undefined,
      planFiles: planFilePaths ?? undefined,
      mock: mock || undefined,
    }),
    signal: controller.signal,
  })
    .then(async (response) => {
      if (!response.ok) {
        const text = await response.text().catch(() => '')
        safeOnError(`请求失败: ${response.status} ${text}`)
        return
      }
      const reader = response.body?.getReader()
      if (!reader) {
        safeOnError('无法获取响应流')
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
            safeOnDone(payload)
            break
          case 'error':
            safeOnError(payload)
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
              const valuePart = line.slice(5).startsWith(' ') ? line.slice(6) : line.slice(5)
              frameDataLines.push(valuePart)
            }
          }
        }
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
        // 流自然结束但既没收到 done 也没收到 error（典型：后端 SSE 超时静默 complete），
        // 这里必须收尾，否则前端的 decomposeLoading 永远卡住。
        if (!terminated) {
          safeOnError('流式响应提前结束（未收到 done/error 帧）。如果 AI 已经返回，请到「LLM 调用日志」查看原始 JSON。')
        }
      } catch (e: unknown) {
        const msg = e instanceof Error ? e.message : String(e)
        safeOnError(`读取 SSE 流中断: ${msg}`)
      }
    })
    .catch((err: unknown) => {
      if (err instanceof DOMException && err.name === 'AbortError') return
      if (err instanceof Error && err.name === 'AbortError') return
      const raw = err instanceof Error ? err.message || err.name : String(err)
      safeOnError(raw || 'SSE 连接失败')
    })

  return controller
}

/**
 * 模块详情页「生成测试用例」SSE 流。
 *   mode='LEGACY' → 生成 apiIntegrationTest + webIntegrationTest 并写回 module
 *   mode='TDD'    → 生成 tddTestSpecJson 并写回 module
 * onDone 回调收到的是更新后的 module JSON 字符串。
 */
export function moduleGenerateTestStream(
  moduleId: number,
  mode: 'LEGACY' | 'TDD',
  envVarKeys: string[],
  onOutput: (line: string) => void,
  onDone: (moduleJson: string) => void,
  onError: (msg: string) => void,
  force = false
): AbortController {
  const controller = new AbortController()
  const baseURL = import.meta.env.VITE_API_BASE_URL || '/api'
  const token = localStorage.getItem('token')
  const headers: HeadersInit = {
    Accept: 'text/event-stream',
    'Content-Type': 'application/json',
  }
  if (token) headers['Authorization'] = `Bearer ${token}`

  let terminated = false
  const safeOnDone = (p: string) => { if (!terminated) { terminated = true; onDone(p) } }
  const safeOnError = (m: string) => { if (!terminated) { terminated = true; onError(m) } }

  const forceQS = force ? '&force=1' : ''
  fetch(`${baseURL}/modules/${moduleId}/generate-test-stream?mode=${mode}${forceQS}`, {
    method: 'POST',
    headers,
    body: JSON.stringify({ envVarKeys: Array.isArray(envVarKeys) ? envVarKeys : [] }),
    signal: controller.signal,
  })
    .then(async (response) => {
      if (!response.ok) {
        const text = await response.text().catch(() => '')
        safeOnError(`请求失败: ${response.status} ${text}`)
        return
      }
      const reader = response.body?.getReader()
      if (!reader) { safeOnError('无法获取响应流'); return }
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
          case 'done': safeOnDone(payload); break
          case 'error': safeOnError(payload); break
          default: onOutput(payload); break
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
            if (line === '') { flushSseFrame(); continue }
            if (line.startsWith('event:')) { flushSseFrame(); frameEvent = line.slice(6).trim(); continue }
            if (line.startsWith('data:')) {
              const valuePart = line.slice(5).startsWith(' ') ? line.slice(6) : line.slice(5)
              frameDataLines.push(valuePart)
            }
          }
        }
        if (buffer.length > 0) {
          const line = buffer.replace(/\r$/, '')
          if (line === '') flushSseFrame()
          else if (line.startsWith('data:')) {
            const valuePart = line.slice(5).startsWith(' ') ? line.slice(6) : line.slice(5)
            frameDataLines.push(valuePart)
          }
        }
        flushSseFrame()
        if (!terminated) safeOnError('流式响应提前结束（未收到 done/error 帧）')
      } catch (e: unknown) {
        const msg = e instanceof Error ? e.message : String(e)
        safeOnError(`读取 SSE 流中断: ${msg}`)
      }
    })
    .catch((err: unknown) => {
      if (err instanceof DOMException && err.name === 'AbortError') return
      if (err instanceof Error && err.name === 'AbortError') return
      const raw = err instanceof Error ? err.message || err.name : String(err)
      safeOnError(raw || 'SSE 连接失败')
    })

  return controller
}

/**
 * 通用 SSE POST 调用：复用 moduleGenerateTestStream 的解析逻辑，避免重复。
 * payload 为 null/undefined 时不发 body。
 */
function sseStreamPost(
  url: string,
  payload: Record<string, any> | null,
  onOutput: (line: string) => void,
  onDone: (data: string) => void,
  onError: (msg: string) => void,
): AbortController {
  const controller = new AbortController()
  const baseURL = import.meta.env.VITE_API_BASE_URL || '/api'
  const token = localStorage.getItem('token')
  const headers: HeadersInit = { Accept: 'text/event-stream' }
  if (payload) headers['Content-Type'] = 'application/json'
  if (token) headers['Authorization'] = `Bearer ${token}`

  let terminated = false
  const safeOnDone = (p: string) => { if (!terminated) { terminated = true; onDone(p) } }
  const safeOnError = (m: string) => { if (!terminated) { terminated = true; onError(m) } }

  fetch(`${baseURL}${url}`, {
    method: 'POST',
    headers,
    body: payload ? JSON.stringify(payload) : undefined,
    signal: controller.signal,
  }).then(async (response) => {
    if (!response.ok) {
      const text = await response.text().catch(() => '')
      safeOnError(`请求失败: ${response.status} ${text}`)
      return
    }
    const reader = response.body?.getReader()
    if (!reader) { safeOnError('无法获取响应流'); return }
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
        case 'done': safeOnDone(payload); break
        case 'error': safeOnError(payload); break
        default: onOutput(payload); break
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
          if (line === '') { flushSseFrame(); continue }
          if (line.startsWith('event:')) { flushSseFrame(); frameEvent = line.slice(6).trim(); continue }
          if (line.startsWith('data:')) {
            const valuePart = line.slice(5).startsWith(' ') ? line.slice(6) : line.slice(5)
            frameDataLines.push(valuePart)
          }
        }
      }
      if (buffer.length > 0) {
        const line = buffer.replace(/\r$/, '')
        if (line === '') flushSseFrame()
        else if (line.startsWith('data:')) {
          const valuePart = line.slice(5).startsWith(' ') ? line.slice(6) : line.slice(5)
          frameDataLines.push(valuePart)
        }
      }
      flushSseFrame()
      if (!terminated) safeOnError('流式响应提前结束（未收到 done/error 帧）')
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      safeOnError(`读取 SSE 流中断: ${msg}`)
    }
  }).catch((err: unknown) => {
    if (err instanceof DOMException && err.name === 'AbortError') return
    if (err instanceof Error && err.name === 'AbortError') return
    const raw = err instanceof Error ? err.message || err.name : String(err)
    safeOnError(raw || 'SSE 连接失败')
  })

  return controller
}

/** 「AI 添加场景」SSE：用户一句话 → 单条 scenario JSON 追加到模块；done 帧返回更新后的模块。 */
export function moduleGenerateScenarioStream(
  moduleId: number,
  type: 'api' | 'web',
  description: string,
  envVarKeys: string[],
  onOutput: (line: string) => void,
  onDone: (moduleJson: string) => void,
  onError: (msg: string) => void,
): AbortController {
  return sseStreamPost(
    `/modules/${moduleId}/scenarios/${type}/generate-stream`,
    { description, envVarKeys: Array.isArray(envVarKeys) ? envVarKeys : [] },
    onOutput, onDone, onError,
  )
}

/**
 * 「TDD tab 运行全部测试」SSE：TestAuthor 编译 criteria → assertions，跑全部并写回 tddAssertionsJson。
 * done 帧 data = 更新后的 Module JSON。
 */
export function moduleTddRunAllStream(
  moduleId: number,
  onOutput: (line: string) => void,
  onDone: (moduleJson: string) => void,
  onError: (msg: string) => void,
): AbortController {
  return sseStreamPost(
    `/modules/${moduleId}/tdd/run-all-assertions`,
    null,
    onOutput, onDone, onError,
  )
}

/**
 * 「修复全部红用例」SSE：扫当前模块所有 RED assertion，调 Claude 改代码 + commit。
 * done 帧 data 是 JSON 字符串 {module, fixHistory}。不会自动重跑断言，用户改完后手动点「运行全部测试」复查。
 */
export function moduleTddFixAllStream(
  moduleId: number,
  onOutput: (line: string) => void,
  onDone: (data: string) => void,
  onError: (msg: string) => void,
): AbortController {
  return sseStreamPost(
    `/modules/${moduleId}/tdd/fix-all-stream`,
    null,
    onOutput, onDone, onError,
  )
}

/**
 * 「修复单条 criterion」SSE：针对单个 criterion 下所有 RED 断言修复。
 */
export function moduleTddFixSingleStream(
  moduleId: number,
  criteriaId: string,
  onOutput: (line: string) => void,
  onDone: (data: string) => void,
  onError: (msg: string) => void,
): AbortController {
  return sseStreamPost(
    `/modules/${moduleId}/tdd/fix-single-stream?criteriaId=${encodeURIComponent(criteriaId)}`,
    null,
    onOutput, onDone, onError,
  )
}

/**
 * 「单条 TDD criterion 重新生成」SSE：用户选择提示词变量后，重新生成指定 criterion。
 * Body: {"promptVarKeys":["KEY1",...]}
 * done 帧 data 是更新后的 Module JSON（含最新 tddTestSpecJson 和空的 tddAssertionsJson）。
 */
export function moduleTddRegenerateCriterionStream(
  moduleId: number,
  criterionId: string,
  promptVarKeys: string[],
  onOutput: (line: string) => void,
  onDone: (moduleJson: string) => void,
  onError: (msg: string) => void,
): AbortController {
  return sseStreamPost(
    `/modules/${moduleId}/tdd/regenerate-criterion-stream?criterionId=${encodeURIComponent(criterionId)}`,
    { promptVarKeys: Array.isArray(promptVarKeys) ? promptVarKeys : [] },
    onOutput, onDone, onError,
  )
}

/** 「单场景手动运行」SSE：运行 testScenarios[index]；done 帧返回回写 lastRun* 后的模块 JSON。 */
export function moduleRunScenarioStream(
  moduleId: number,
  type: 'api' | 'web',
  index: number,
  onOutput: (line: string) => void,
  onDone: (moduleJson: string) => void,
  onError: (msg: string) => void,
): AbortController {
  return sseStreamPost(
    `/modules/${moduleId}/scenarios/${type}/${index}/run-stream`,
    null,
    onOutput, onDone, onError,
  )
}

/**
 * 「全量测试」SSE：顺序执行该类型全部场景；测试失败时后端驱动开发 agent 修复 →
 * 强制重新部署 → 重测（≤3 轮），并把修复循环写入各场景步骤明细。
 * done 帧返回回写里程碑后的模块 JSON。
 */
export function moduleRunAllStream(
  moduleId: number,
  type: 'api' | 'web',
  onOutput: (line: string) => void,
  onDone: (moduleJson: string) => void,
  onError: (msg: string) => void,
): AbortController {
  return sseStreamPost(
    `/modules/${moduleId}/scenarios/${type}/run-all-stream`,
    null,
    onOutput, onDone, onError,
  )
}

// 测试配置相关 API
export const TestConfigApi = {
  // 获取项目的测试配置
  getProjectConfig: (projectId: number) => request.get(`/test-config/project/${projectId}`),
  // 保存测试配置
  saveConfig: (projectId: number, data: any) => request.post(`/test-config/project/${projectId}`, data),
  // 自动探测项目配置
  probeConfig: (projectId: number) => request.post(`/test-config/project/${projectId}/probe`),
  // 更新测试配置
  updateConfig: (id: number, data: any) => request.put(`/test-config/${id}`, data),
  // 删除测试配置
  deleteConfig: (id: number) => request.delete(`/test-config/${id}`)
}
