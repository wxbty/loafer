/**
 * 对话式创建项目的会话持久化工具。
 *
 * - 进入页面时在 URL 生成随机 session ID（?sid=xxx）
 * - 每一步完成 / 切换时自动保存到 localStorage
 * - 流水线 SSE 阶段更新实时持久化
 * - 刷新页面后根据 sid 恢复任务界面
 */

const STORAGE_PREFIX = 'loafer:chat-session:'

/** 需要持久化的完整会话状态 */
export interface ChatSessionState {
  sessionId: string
  activeStep: number
  // Step 0: 描述需求
  userMessage: string
  streamOutput: string
  summary: any
  // Step 1: 确认需求
  confirmedName: string
  confirmedRepoName: string
  confirmedDescription: string
  // Step 2: 确认环境
  contextPreview: any
  createdProject: any
  // Step 3: 需求澄清
  clarifyQuestions: any[]
  clarifyAnswers: Record<string, string>
  clarifyCustomInputs: Record<string, string>
  clarifyRequirement: string
  // Step 4: 全链路执行
  pipelineStages: any[]
  pipelineLog: string
  pipelineResult: any
  pipelineFinished: boolean
  pipelineHasError: boolean
  pipelineRunning: boolean
  // 元数据
  createdAt: string
  updatedAt: string
}

/** 生成随机 session ID（8位十六进制） */
export function generateSessionId(): string {
  const arr = new Uint8Array(4)
  crypto.getRandomValues(arr)
  return Array.from(arr, (b) => b.toString(16).padStart(2, '0')).join('')
}

/** 从 URL 查询参数获取 session ID */
export function getSessionIdFromURL(): string | null {
  const params = new URLSearchParams(window.location.search)
  return params.get('sid')
}

/** 将 session ID 写入 URL（不触发导航） */
export function setSessionIdInURL(sid: string): void {
  const url = new URL(window.location.href)
  url.searchParams.set('sid', sid)
  window.history.replaceState({}, '', url.toString())
}

/** 构建 localStorage key */
function storageKey(sid: string): string {
  return `${STORAGE_PREFIX}${sid}`
}

/** 保存会话状态到 localStorage */
export function saveSession(state: ChatSessionState): void {
  state.updatedAt = new Date().toISOString()
  try {
    localStorage.setItem(storageKey(state.sessionId), JSON.stringify(state))
  } catch (e) {
    console.warn('[chat-session] 保存失败:', e)
  }
}

/** 从 localStorage 加载会话状态 */
export function loadSession(sid: string): ChatSessionState | null {
  try {
    const raw = localStorage.getItem(storageKey(sid))
    if (!raw) return null
    const parsed = JSON.parse(raw) as ChatSessionState
    if (!parsed.sessionId || parsed.sessionId !== sid) return null
    return parsed
  } catch (e) {
    console.warn('[chat-session] 加载失败:', e)
    return null
  }
}

/** 删除会话 */
export function deleteSession(sid: string): void {
  localStorage.removeItem(storageKey(sid))
}

/** 列出所有会话 ID（用于会话管理） */
export function listSessions(): string[] {
  const ids: string[] = []
  for (let i = 0; i < localStorage.length; i++) {
    const key = localStorage.key(i)
    if (key && key.startsWith(STORAGE_PREFIX)) {
      ids.push(key.substring(STORAGE_PREFIX.length))
    }
  }
  return ids
}

/** 清理超过 7 天的过期会话 */
export function cleanExpiredSessions(maxAgeDays = 7): number {
  const now = Date.now()
  const maxAge = maxAgeDays * 24 * 60 * 60 * 1000
  let cleaned = 0
  for (let i = localStorage.length - 1; i >= 0; i--) {
    const key = localStorage.key(i)
    if (!key || !key.startsWith(STORAGE_PREFIX)) continue
    try {
      const raw = localStorage.getItem(key)
      if (!raw) continue
      const parsed = JSON.parse(raw) as ChatSessionState
      if (parsed.updatedAt) {
        const age = now - new Date(parsed.updatedAt).getTime()
        if (age > maxAge) {
          localStorage.removeItem(key)
          cleaned++
        }
      }
    } catch {
      localStorage.removeItem(key!)
      cleaned++
    }
  }
  return cleaned
}

/** 创建空白会话状态 */
export function createEmptySession(sid: string): ChatSessionState {
  const now = new Date().toISOString()
  return {
    sessionId: sid,
    activeStep: 0,
    userMessage: '',
    streamOutput: '',
    summary: null,
    confirmedName: '',
    confirmedRepoName: '',
    confirmedDescription: '',
    contextPreview: null,
    createdProject: null,
    clarifyQuestions: [],
    clarifyAnswers: {},
    clarifyCustomInputs: {},
    clarifyRequirement: '',
    pipelineStages: [],
    pipelineLog: '',
    pipelineResult: null,
    pipelineFinished: false,
    pipelineHasError: false,
    pipelineRunning: false,
    createdAt: now,
    updatedAt: now,
  }
}

/**
 * 防抖保存：避免高频 SSE 更新频繁写入 localStorage。
 * 返回一个函数，调用后延迟 500ms 执行保存，期间再次调用则重置计时。
 */
export function createDebouncedSaver(delay = 500): (state: ChatSessionState) => void {
  let timer: ReturnType<typeof setTimeout> | null = null
  return (state: ChatSessionState) => {
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => {
      saveSession(state)
      timer = null
    }, delay)
  }
}
