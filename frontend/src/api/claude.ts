import request from '@/utils/request'

export const ClaudeApi = {
  createSession: (projectId: string) => request.post(`/claude-sessions/create?projectId=${projectId}`),
  getSession: (sessionId: string) => request.get(`/claude-sessions/${sessionId}`),
  resumeSession: (projectId: string, claudeSessionUuid: string) =>
    request.post(`/claude-sessions/resume?projectId=${projectId}&claudeSessionUuid=${claudeSessionUuid}`),
  closeSession: (sessionId: string) => request.delete(`/claude-sessions/${sessionId}`),
  listSessions: (projectId: string) => request.get(`/claude-sessions/list?projectId=${projectId}`),
}
