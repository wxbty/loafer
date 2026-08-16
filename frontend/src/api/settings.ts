import request from '@/utils/request'

// 部署接口
export const DeployApi = {
  /** 检查项目是否正在部署 */
  checkDeployStatus: (projectId: number) => request.get(`/deploy/status/${projectId}`),
}

// 系统配置接口
export const SettingsApi = {
  getWorkspaceRoot: () => request.get('/system-config/workspace-root'),
  setWorkspaceRoot: (value: string) => request.put('/system-config/workspace-root', { value }),
  getAllConfigs: () => request.get('/system-config/list'),
  getConfig: (key: string) => request.get(`/system-config/${key}`),
  setConfig: (key: string, value: string, description?: string) =>
    request.put(`/system-config/${key}`, { value, description }),
  getClaudeInfo: () => request.get('/system-config/claude-info'),
  getClaudeConfig: () => request.get('/system-config/claude-config'),
  getClaudeProfiles: () => request.get('/system-config/claude-profiles'),
  setClaudeConfig: (data: ClaudeConfigPutPayload) => request.put('/system-config/claude-config', data),
  getShowMoreActions: () => request.get('/system-config/show-more-actions'),
}

export interface ClaudeModelProfile {
  id: string
  name: string
  model: string
  anthropicAuthToken: string
  anthropicBaseUrl: string
  apiTimeoutMs: string
  disableNonessentialTraffic: string
}

export type ClaudeConfigPutPayload =
  | {
      profiles: ClaudeModelProfile[]
      activeProfileId: string
      customModels?: string[]
    }
  | {
      model?: string
      anthropicAuthToken?: string
      anthropicBaseUrl?: string
      apiTimeoutMs?: string
      disableNonessentialTraffic?: string
    }

export interface ClaudeProfileSummary {
  id: string
  name: string
  model: string
}

// 提示词模板接口
export interface PromptTemplate {
  id?: number
  templateKey: string
  templateName: string
  templateContent: string
  description?: string
  variablesJson?: string
  isEnabled?: number
  isSystem?: number
  useCount?: number
  createdAt?: string
  updatedAt?: string
}

export const PromptTemplateApi = {
  getTemplates: () => request.get('/prompt-templates/list'),
  getEnabledTemplates: () => request.get('/prompt-templates/enabled'),
  getTemplate: (id: number) => request.get(`/prompt-templates/${id}`),
  getTemplateByKey: (key: string) => request.get(`/prompt-templates/key/${key}`),
  getTemplateVariables: (id: number) => request.get(`/prompt-templates/${id}/variables`),
  createTemplate: (data: PromptTemplate) => request.post('/prompt-templates/', data),
  updateTemplate: (id: number, data: PromptTemplate) => request.put(`/prompt-templates/${id}`, data),
  deleteTemplate: (id: number) => request.delete(`/prompt-templates/${id}`),
  toggleEnabled: (id: number, enabled: boolean) => request.put(`/prompt-templates/${id}/toggle`, { enabled }),
}

// Session Pool 追踪接口
export interface SessionPoolStatus {
  activeCount: number
  pendingCount: number
  maxPoolSize: number
  idleTimeoutMinutes: number
  isFull: boolean
}

export interface CcSessionInfo {
  sessionId: string
  projectId: string
  projectName: string
  taskId: number | null
  claudeSessionUuid: string | null
  status: string
  isProcessAlive: boolean
  isExecuting: boolean
  createdAt: string | null
  lastActiveAt: string | null
  idleMinutes: number
  resumed: boolean
  profileId: string | null
}

export const SessionPoolApi = {
  getStatus: () => request.get('/system-config/session-pool/status'),
  getCcSessions: () => request.get('/system-config/session-pool/cc-sessions'),
  cleanup: () => request.post('/system-config/session-pool/cleanup'),
  closeSession: (sessionId: string) => request.delete(`/system-config/session-pool/sessions/${sessionId}`),
}
