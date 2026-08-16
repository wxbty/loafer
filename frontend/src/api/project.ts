import request from '@/utils/request'
import { consumeSseStream, type SseCallbacks } from '@/utils/sseStream'

const baseURL = import.meta.env.VITE_API_BASE_URL || '/api'

// 项目相关 API
export const ProjectApi = {
  // 获取项目列表
  getProjectList: () => request.get('/projects/list'),
  // 分页获取项目列表
  getProjectPage: (current: number, size: number) => request.get(`/projects/page?current=${current}&size=${size}`),
  // 获取项目详情
  getProjectDetail: (id: number) => request.get(`/projects/${id}`),
  // 获取项目级 Claude 配置（CLAUDE.md / .claude/settings.local.json / Skill）
  getProjectClaudeConfig: (id: number) => request.get(`/projects/${id}/claude-config`),
  // 探测当前生效的所有 Skill（全局 + 项目级）
  getAvailableSkills: (id: number) => request.get(`/projects/${id}/available-skills`),
  // 保存项目级 Claude 配置
  updateProjectClaudeConfig: (
    id: number,
    data: { claudeMd: string; claudeSettings: string; skillContent: string; skillRelativePath?: string }
  ) => request.put(`/projects/${id}/claude-config`, data),
  // 获取项目数据库连接配置（从配置文件解析）
  getProjectDatabaseConfig: (id: number) => request.get(`/projects/${id}/database-config`),
  // 获取项目数据库当前库下的所有表
  getProjectDatabaseTables: (id: number) => request.get(`/projects/${id}/database-tables`),
  // 获取指定表的元数据（列/主键/索引）
  getProjectTableMeta: (id: number, tableName: string) =>
    request.get(`/projects/${id}/database-tables/${encodeURIComponent(tableName)}/meta`),
  // 获取指定表预览数据（支持 where/orderBy，返回前 10 条）
  getProjectTablePreview: (id: number, tableName: string, params?: { where?: string; orderBy?: string }) =>
    request.get(`/projects/${id}/database-tables/${encodeURIComponent(tableName)}/preview`, { params }),
  // 更新表记录
  updateTableRow: (id: number, tableName: string, data: { primaryKey: Record<string, any>; updates: Record<string, any> }) =>
    request.put(`/projects/${id}/database-tables/${encodeURIComponent(tableName)}/row`, data),
  // 删除表记录
  deleteTableRow: (id: number, tableName: string, primaryKey: Record<string, any>) =>
    request.delete(`/projects/${id}/database-tables/${encodeURIComponent(tableName)}/row`, { data: primaryKey }),
  // 创建项目
  createProject: (data: any) => request.post('/projects/create', data),
  // 从Git地址拉取项目
  cloneProject: (data: any) => request.post('/projects/clone', data),
  // 启动带执行流的Git拉取
  startCloneProjectStream: (data: any) => request.post('/projects/clone/stream-start', data),
  // 轮询带执行流的Git拉取任务
  getCloneProjectStream: (taskId: string) => request.get(`/projects/clone/stream/${taskId}`),
  // 对已有项目执行 Git 初始化（clone 到项目工作目录）
  initProjectFromGit: (id: number, data: any) => request.post(`/projects/${id}/git-init`, data),
  // 更新项目
  updateProject: (id: number, data: any) => request.put(`/projects/${id}`, data),
  // 仅更新 Linux CLI 执行系统用户（空字符串表示清除）
  updateProjectCliOsUser: (id: number, data: { cliOsUser: string }) =>
    request.put(`/projects/${id}/cli-os-user`, data),
  // 更新项目测试用例
  updateProjectTestCases: (id: number, testCases: any[]) =>
    request.put(`/projects/${id}/test-cases`, { testCases }),
  // 更新项目环境变量
  updateProjectEnvVars: (id: number, envVars: { key: string; value: string }[]) =>
    request.put(`/projects/${id}/env-vars`, { envVars }),
  // 更新项目提示词变量（用于模块任务提示词注入）
  updateProjectPromptVars: (id: number, promptVars: { key: string; value: string; remark?: string }[]) =>
    request.put(`/projects/${id}/prompt-vars`, { promptVars }),
  // 工程目录：环境检测（按开发语言）
  checkProjectWorkDirEnv: (id: number) => request.post(`/projects/${id}/workdir/env-check`),
  /** 未提交变更路径（相对工作目录），用于「Git 未提交」树 */
  getWorkDirGitChangedPaths: (id: number) => request.get(`/projects/${id}/workdir/git-changed-paths`),
  /** 获取最近 N 次 Git 提交记录 */
  getWorkDirGitLog: (id: number, limit?: number) => request.get(`/projects/${id}/workdir/git-log`, { params: { limit: limit || 5 } }),
  /** 获取指定 Git 提交修改的文件列表 */
  getWorkDirGitLogFiles: (id: number, hash: string) => request.get(`/projects/${id}/workdir/git-log/${encodeURIComponent(hash)}/files`),
  // 工程目录：安装依赖（当前仅 python）
  installProjectWorkDirDeps: (id: number) => request.post(`/projects/${id}/workdir/install-deps`),
  // 工程目录：启动安装依赖流
  startInstallProjectWorkDirDepsStream: (id: number) => request.post(`/projects/${id}/workdir/install-deps/stream-start`),
  // 工程目录：轮询安装依赖流
  getInstallProjectWorkDirDepsStream: (id: number, taskId: string) =>
    request.get(`/projects/${id}/workdir/install-deps/stream/${encodeURIComponent(taskId)}`),
  // 工程目录：启动重建 Python3.10 venv 流
  startRebuildPython310VenvStream: (id: number) =>
    request.post(`/projects/${id}/workdir/rebuild-python310-venv/stream-start`),
  // 工程目录：轮询重建 Python3.10 venv 流
  getRebuildPython310VenvStream: (id: number, taskId: string) =>
    request.get(`/projects/${id}/workdir/rebuild-python310-venv/stream/${encodeURIComponent(taskId)}`),
  // 工程目录：启动 git pull 流
  startGitPullWorkDirStream: (id: number) =>
    request.post(`/projects/${id}/workdir/git-pull/stream-start`),
  // 工程目录：轮询 git pull 流
  getGitPullWorkDirStream: (id: number, taskId: string) =>
    request.get(`/projects/${id}/workdir/git-pull/stream/${encodeURIComponent(taskId)}`),
  // 工程目录：启动 git push 流
  startGitPushWorkDirStream: (id: number, commitMsg: string) =>
    request.post(`/projects/${id}/workdir/git-push/stream-start`, { commitMsg }),
  // 工程目录：轮询 git push 流
  getGitPushWorkDirStream: (id: number, taskId: string) =>
    request.get(`/projects/${id}/workdir/git-push/stream/${encodeURIComponent(taskId)}`),
  // 工程目录：启动 AI 提交推送流（双击 Git Push 按钮触发）
  startAiGitPushWorkDirStream: (id: number) =>
    request.post(`/projects/${id}/workdir/git-push-ai/stream-start`),
  // 工程目录：轮询 AI 提交推送流
  getAiGitPushWorkDirStream: (id: number, taskId: string) =>
    request.get(`/projects/${id}/workdir/git-push-ai/stream/${encodeURIComponent(taskId)}`),
  // 工程目录：启动本地部署流（调用 deploy-local.sh）
  startDeployWorkDirStream: (id: number, mode: 'all' | 'backend' | 'frontend', initNginx?: boolean) =>
    request.post(`/projects/${id}/workdir/deploy/stream-start`, { mode, initNginx }),
  // 工程目录：轮询本地部署流（skipErrorMessage: 部署期间后端可能重启，重试时不显示 toast）
  getDeployWorkDirStream: (id: number, taskId: string) =>
    request.get(`/projects/${id}/workdir/deploy/stream/${encodeURIComponent(taskId)}`, { skipErrorMessage: true } as any),
  // 工程目录：删除部署任务状态
  deleteDeployStream: (id: number, taskId: string) =>
    request.delete(`/projects/${id}/workdir/deploy/stream/${encodeURIComponent(taskId)}`),
  // 工程目录：更新任务清单到 feature_list.json
  updateFeatureListJson: (id: number) =>
    request.post(`/projects/${id}/workdir/feature-list/update`),
  // 运行单条项目测试用例（按步骤执行）
  runProjectTestCase: (id: number, testCaseIndex: number) =>
    request.post(`/projects/${id}/test-cases/run`, { testCaseIndex }),
  // 启动测试用例运行流（实时日志）
  startRunProjectTestCaseStream: (id: number, testCaseIndex: number, testCases?: any[]) =>
    request.post(`/projects/${id}/test-cases/run-stream-start`, { testCaseIndex, testCases }),
  // 轮询测试用例运行流
  getRunProjectTestCaseStream: (id: number, taskId: string) =>
    request.get(`/projects/${id}/test-cases/run-stream/${encodeURIComponent(taskId)}`),
  // 项目详情「向 Claude 发送命令」：每次启动新的 CLI 进程（流式日志）
  startClaudeAdhocCommandStream: (id: number, body: { prompt: string }) =>
    request.post(`/projects/${id}/claude-adhoc-command/stream-start`, body),
  getClaudeAdhocCommandStream: (id: number, taskId: string) =>
    request.get(`/projects/${id}/claude-adhoc-command/stream/${encodeURIComponent(taskId)}`),
  // 删除项目
  deleteProject: (id: number) => request.delete(`/projects/${id}`),
  // 根据状态获取项目列表
  getProjectsByStatus: (status: number) => request.get(`/projects/by-status?status=${status}`),
  // 获取项目文档列表（docs 目录下的 .md 文件）
  getProjectDocs: (id: number) => request.get(`/projects/${id}/docs`),
  // 列出目录内容
  listDirectory: (path: string) => request.get('/files/list', { params: { path } }),
  // 读取文件内容
  readFileContent: (path: string) => request.get('/files/content', { params: { path } }),
  // 保存文件内容
  saveFileContent: (path: string, content: string) => request.put('/files/content', { path, content }),
  // 删除工程目录/数据目录中的文件或目录
  deleteFileOrDirectory: (path: string) => request.delete('/files/delete', { params: { path } }),
  // 数据目录：创建子目录
  downloadDataFile: (path: string) =>
    request.get('/files/download', { params: { path }, responseType: 'blob', timeout: 0 } as any),
  createDataSubDirectory: (parentPath: string, dirName: string) =>
    request.post('/files/data-dir/mkdir', { parentPath, dirName }),
  // 数据目录：删除子目录
  deleteDataSubDirectory: (path: string) => request.delete('/files/data-dir/delete', { params: { path } }),
  // 数据目录：重命名子目录
  renameDataSubDirectory: (path: string, newName: string) =>
    request.put('/files/data-dir/rename', { path, newName }),
  // 数据目录：向指定文件夹上传文件（覆盖同名文件），支持进度回调
  uploadDataDirFile: (targetDir: string, file: File | Blob, onProgress?: (percent: number) => void) => {
    return new Promise((resolve, reject) => {
      const fd = new FormData()
      fd.append('targetDir', targetDir)
      fd.append('file', file)

      const baseURL = import.meta.env.VITE_API_BASE_URL || '/api'
      const url = `${baseURL}/files/data-dir/upload`
      const token = localStorage.getItem('token')

      const xhr = new XMLHttpRequest()
      xhr.open('POST', url, true)

      if (token) {
        xhr.setRequestHeader('Authorization', `Bearer ${token}`)
      }

      // 上传进度
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable && onProgress) {
          const percent = Math.round((e.loaded / e.total) * 100)
          onProgress(percent)
        }
      }

      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          try {
            const data = JSON.parse(xhr.responseText)
            resolve(data)
          } catch {
            reject(new Error('响应解析失败'))
          }
        } else {
          reject(new Error(`上传失败: ${xhr.status}`))
        }
      }

      xhr.onerror = () => reject(new Error('网络错误'))
      xhr.ontimeout = () => reject(new Error('上传超时'))
      xhr.timeout = 300000 // 5分钟超时

      xhr.send(fd)
    })
  },
  // 临时图片上传（用于 Claude 命令输入框粘贴/拖拽图片）
  uploadTempImage: (file: File | Blob) => {
    const fd = new FormData()
    fd.append('file', file)
    return request.post('/files/temp-image/upload', fd, {
      timeout: 60000,
    })
  },

  // ==================== Claude Agent 配置 ====================
  // 列出项目所有 Agent
  listProjectAgents: (id: number) => request.get(`/projects/${id}/agents`),
  // 获取单个 Agent 内容
  getProjectAgent: (id: number, agentName: string) =>
    request.get(`/projects/${id}/agents/${encodeURIComponent(agentName)}`),
  // 创建/更新 Agent
  saveProjectAgent: (id: number, agentName: string, data: { frontmatter: Record<string, any>; body: string }) =>
    request.put(`/projects/${id}/agents/${encodeURIComponent(agentName)}`, data),
  // 删除 Agent
  deleteProjectAgent: (id: number, agentName: string) =>
    request.delete(`/projects/${id}/agents/${encodeURIComponent(agentName)}`),
  // 初始化默认 Agent（developer、tester）
  initDefaultAgents: (id: number) =>
    request.post(`/projects/${id}/agents/init-default`),
  // ==================== 日志文件 ====================
  // 读取日志文件末尾内容（默认读取最后1000行）
  readLogTail: (path: string, lineCount?: number) =>
    request.get('/files/tail', { params: { path, lineCount: lineCount || 1000 } }),

  // ==================== 对话式创建项目 ====================
  // AI 需求提炼（SSE 流式）
  chatExtractRequirement: (message: string, callbacks: SseCallbacks) => {
    return consumeSseStream(
      `${baseURL}/projects/chat`,
      { message },
      callbacks
    )
  },
  // 项目运行上下文预览（不实际分配资源）
  previewProjectContext: (data: { name: string; repoName: string; description: string }) =>
    request.post('/projects/preview-context', data),
  // 对话式创建项目（最终创建，自动设置开发语言、工作目录、Gitee 仓库）
  chatCreateProject: (data: { name: string; repoName: string; description: string }) =>
    request.post('/projects/chat-create', data),
  // 需求澄清：生成澄清问题列表（类似 Claude Code 的 /grill-me）
  // 超时设为 5 分钟，因为后端需要调用 AI 分析需求
  // signal 可选参数用于支持前端中止请求
  generateClarifyQuestions: (projectId: number, signal?: AbortSignal) =>
    request.post(`/pipeline/project/${projectId}/clarify`, {}, { timeout: 300000, signal }),
  // 独立需求澄清：不依赖项目ID，直接传入需求文本生成问题（不写DB）
  // 用于项目创建前的需求确认阶段
  generateClarifyQuestionsStandalone: (requirement: string, signal?: AbortSignal) =>
    request.post('/pipeline/clarify-standalone', { requirement }, { timeout: 300000, signal }),
  // 需求澄清：提交用户答案，合并到项目需求描述中
  submitClarifyAnswers: (projectId: number, answers: Array<{ questionId: string; question: string; answer: string }>) =>
    request.post(`/pipeline/project/${projectId}/clarify/answers`, { answers }),
  // 一键停止流水线：杀死所有正在运行的 Claude Code CLI 进程
  abortPipeline: (projectId: number) =>
    request.post(`/pipeline/project/${projectId}/abort`, {}, { timeout: 15000, skipErrorMessage: true } as any),
  // 获取项目流水线状态（基于计划、模块、部署记录推断各阶段进度）
  getPipelineStatus: (projectId: number) =>
    request.get(`/pipeline/project/${projectId}/status`),
}

// ==================== 运行状态 API ====================
export const RuntimeStatusApi = {
  // 获取完整运行状态
  getRuntimeStatus: (projectId?: number) => request.get('/runtime-status', { params: { projectId } }),
  // 获取后端状态
  getBackendStatus: (projectWorkDir?: string) => request.get('/runtime-status/backend', { params: { projectWorkDir } }),
  // 获取前端状态
  getFrontendStatus: (projectWorkDir?: string) => request.get('/runtime-status/frontend', { params: { projectWorkDir } }),
  // 获取访问链接（带 projectId 时会读取 project_port_allocation 中的端口与 URL 覆盖配置）
  getAccessUrls: (projectId?: number) =>
    request.get('/runtime-status/access-urls', { params: projectId ? { projectId } : {} }),
  // 获取系统信息
  getSystemInfo: () => request.get('/runtime-status/system'),
}
