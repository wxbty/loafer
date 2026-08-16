import request from '@/utils/request'
import { consumeSseStream, type SseCallbacks } from '@/utils/sseStream'

const baseURL = import.meta.env.VITE_API_BASE_URL || '/api'

// 部署管理 API
export const DeployApi = {
  // 部署项目（SSE 流式）；force=true 时即使已运行也强制重新部署（复用端口，URL 不变）
  deploy: (projectId: number, callbacks: SseCallbacks, force = false) => {
    return consumeSseStream(
      `${baseURL}/deploy/project/${projectId}`,
      { force },
      callbacks
    )
  },
  // 卸载项目
  undeploy: (projectId: number) => request.delete(`/deploy/project/${projectId}`),
  // 获取部署信息
  getDeployment: (projectId: number) => request.get(`/deploy/project/${projectId}`),
  // 获取部署状态
  getStatus: (projectId: number) => request.get(`/deploy/project/${projectId}/status`),
  // 获取部署日志
  getLogs: (projectId: number) => request.get(`/deploy/project/${projectId}/logs`),
}
