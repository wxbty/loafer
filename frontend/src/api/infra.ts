import request from '@/utils/request'
import { consumeSseStream, type SseCallbacks } from '@/utils/sseStream'

const baseURL = import.meta.env.VITE_API_BASE_URL || '/api'

// 端口管理 API
export const PortApi = {
  getPortRange: () => request.get('/infra/ports/range'),
  getAllocatedPorts: () => request.get('/infra/ports/allocated'),
  getProjectPorts: (projectId: number) => request.get(`/infra/ports/project/${projectId}`),
  allocatePort: (projectId: number, data: { portType: string; description: string }) =>
    request.post(`/infra/ports/project/${projectId}/allocate`, data),
  releasePort: (port: number) => request.delete(`/infra/ports/${port}`),
}

// 数据库供给 API
export const DatabaseApi = {
  provision: (projectId: number) => request.post(`/infra/database/project/${projectId}/provision`),
  drop: (projectId: number) => request.delete(`/infra/database/project/${projectId}`),
  get: (projectId: number) => request.get(`/infra/database/project/${projectId}`),
}

// 短信服务 API
export const SmsApi = {
  send: (data: { phoneNumbers: string[]; templateParams: Record<string, string> }) =>
    request.post('/infra/sms/send', data),
  notify: (projectId: number, data: { phoneNumbers: string[]; projectName: string; status: string }) =>
    request.post(`/infra/sms/project/${projectId}/notify`, data),
  getConfig: (projectId: number) => request.get(`/infra/sms/project/${projectId}/config`),
  saveConfig: (projectId: number, data: any) => request.put(`/infra/sms/project/${projectId}/config`, data),
}

// 测试管理 API
export const TestApi = {
  // 运行测试（SSE 流式）
  runTest: (projectId: number, data: { moduleId?: number; taskId?: number; testType?: string; workDir?: string }, callbacks: SseCallbacks) => {
    return consumeSseStream(
      `${baseURL}/infra/tests/project/${projectId}/run`,
      data,
      callbacks
    )
  },
  generateSpec: (projectId: number, data: { url: string; description: string }) =>
    request.post(`/infra/tests/project/${projectId}/generate-spec`, data),
  listTests: (projectId: number) => request.get(`/infra/tests/project/${projectId}`),
  getTestRun: (runId: number) => request.get(`/infra/tests/${runId}`),
}
