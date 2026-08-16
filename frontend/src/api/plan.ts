import request from '@/utils/request'
import { consumeSseStream, type SseCallbacks } from '@/utils/sseStream'

const baseURL = import.meta.env.VITE_API_BASE_URL || '/api'

// 执行计划 API
export const PlanApi = {
  // 生成执行计划（SSE 流式）
  generatePlan: (projectId: number, requirement: string, callbacks: SseCallbacks) => {
    return consumeSseStream(
      `${baseURL}/plans/generate`,
      { projectId, requirement },
      callbacks
    )
  },
  // 优化计划（SSE 流式）
  refinePlan: (planId: number, feedback: string, callbacks: SseCallbacks) => {
    return consumeSseStream(
      `${baseURL}/plans/${planId}/refine`,
      { feedback },
      callbacks
    )
  },
  // 确认计划
  confirmPlan: (planId: number) => request.put(`/plans/${planId}/confirm`),
  // 拆解计划为模块任务（SSE 流式）
  decomposePlan: (planId: number, callbacks: SseCallbacks) => {
    return consumeSseStream(
      `${baseURL}/plans/${planId}/decompose`,
      {},
      callbacks
    )
  },
  // 获取项目的计划
  getPlan: (projectId: number) => request.get(`/plans/project/${projectId}`),
  // 按ID获取计划
  getPlanByID: (planId: number) => request.get(`/plans/${planId}`),
}
