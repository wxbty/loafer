import request from '@/utils/request'

export const TaskStateApi = {
  getByTaskId: (taskId: number) => request.get(`/task-states/by-task/${taskId}`),
  update: (id: number, data: any) => request.put(`/task-states/${id}`, data),
}

export const SliceHistoryApi = {
  getByTaskId: (taskId: number) => request.get(`/slice-histories/by-task/${taskId}`),
  create: (data: any) => request.post('/slice-histories/create', data),
  update: (id: number, data: any) => request.put(`/slice-histories/${id}`, data),
}

export interface TddAssertion {
  id: string
  description: string
  status: 'pending' | 'passed' | 'failed'
  errorMessage?: string
}

export type TddPhase = 'requirement' | 'test' | 'implement' | 'run' | 'refactor'

export interface TddStepStatus {
  phase: TddPhase
  status: 'pending' | 'running' | 'done' | 'failed'
  startedAt?: string
  completedAt?: string
  summary?: string
}
