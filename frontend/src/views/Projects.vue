<template>
  <div class="page-container">
    <!-- 页面标题栏 -->
    <div class="page-header">
      <div class="page-header__left">
        <h1 class="page-title">项目管理</h1>
        <p class="page-subtitle">管理所有项目和工作目录</p>
      </div>
      <div class="page-header__right">
        <n-button type="primary" @click="router.push('/projects/chat-create')">
          <template #icon>
            <n-icon :component="Plus" />
          </template>
          AI 对话创建
        </n-button>
        <n-button @click="showCreateDialog = true">
          <template #icon>
            <n-icon :component="Plus" />
          </template>
          手动新建
        </n-button>
        <n-button @click="showCloneDialog = true">
          <template #icon>
            <n-icon :component="GitBranch" />
          </template>
          从Git拉取
        </n-button>
      </div>
    </div>

    <!-- 项目列表卡片 -->
    <div class="macos-card">
      <div class="card-header">
        <div class="card-header__title">
          <n-icon :component="FolderOpen" :size="18" />
          <span>项目列表</span>
        </div>
        <n-tag type="info" size="small" round>{{ projects.length }} 个项目</n-tag>
      </div>
      <div class="card-body">
        <n-data-table
          :columns="columns"
          :data="projects"
          :scroll-x="1200"
          :bordered="false"
          :row-key="(row: any) => row.id"
        />
      </div>
    </div>

    <!-- 创建项目对话框 -->
    <n-modal v-model:show="showCreateDialog" preset="card" title="创建项目" style="width: 560px" :bordered="false">
      <n-form label-placement="left" label-width="100" label-align="right">
        <n-form-item label="项目名称" required>
          <n-input v-model:value="createForm.name" placeholder="请输入项目名称" />
        </n-form-item>
        <n-form-item label="项目描述">
          <n-input
            v-model:value="createForm.description"
            type="textarea"
            :rows="3"
            placeholder="请输入项目描述"
          />
        </n-form-item>
        <n-form-item label="Git地址">
          <n-input v-model:value="createForm.gitUrl" placeholder="可选，输入Git仓库地址" clearable />
        </n-form-item>
        <n-form-item label="数据目录">
          <n-input-group v-if="dataRoot">
            <n-input-group-label>{{ dataRoot }}/</n-input-group-label>
            <n-input v-model:value="createDataDirRel" placeholder="默认使用项目名称" clearable />
          </n-input-group>
          <n-input
            v-else
            v-model:value="createForm.dataDir"
            placeholder="请先在系统设置中配置数据目录根路径"
            disabled
          />
        </n-form-item>
        <n-form-item label="项目状态">
          <n-select v-model:value="createForm.status" :options="statusOptions" placeholder="请选择项目状态" />
        </n-form-item>
      </n-form>
      <template #footer>
        <div class="flex justify-end gap-2">
          <n-button @click="showCreateDialog = false">取消</n-button>
          <n-button type="primary" @click="handleCreate">创建</n-button>
        </div>
      </template>
    </n-modal>

    <!-- 从Git拉取对话框 -->
    <n-modal v-model:show="showCloneDialog" preset="card" title="从Git拉取项目" style="width: 560px" :bordered="false">
      <n-form label-placement="left" label-width="100" label-align="right">
        <n-form-item label="Git地址" required>
          <n-input v-model:value="cloneForm.gitUrl" placeholder="请输入Git仓库地址" />
        </n-form-item>
        <n-form-item label="项目名称">
          <n-input v-model:value="cloneForm.name" placeholder="项目名称（默认从Git地址提取）" />
        </n-form-item>
        <n-form-item label="项目描述">
          <n-input
            v-model:value="cloneForm.description"
            type="textarea"
            :rows="3"
            placeholder="请输入项目描述"
          />
        </n-form-item>
        <n-form-item label="认证账号">
          <n-input v-model:value="cloneForm.username" placeholder="可选，私有仓库需要填写账号" clearable />
        </n-form-item>
        <n-form-item label="认证密码">
          <n-input v-model:value="cloneForm.password" type="password" placeholder="可选，私有仓库密码或Token" clearable show-password-on="click" />
        </n-form-item>
      </n-form>
      <template #footer>
        <div class="flex justify-end gap-2">
          <n-button @click="showCloneDialog = false">取消</n-button>
          <n-button type="primary" @click="handleClone">拉取</n-button>
        </div>
      </template>
    </n-modal>

    <!-- Git拉取日志对话框 -->
    <n-modal
      v-model:show="showCloneLogDialog"
      preset="card"
      title="Git拉取执行日志"
      style="width: 800px"
      :mask-closable="false"
      :closable="false"
      :bordered="false"
    >
      <pre class="clone-log">{{ cloneLogText }}</pre>
      <template #footer>
        <n-button v-if="!cloneRunning" type="primary" @click="showCloneLogDialog = false">关闭</n-button>
      </template>
    </n-modal>

    <!-- 一键部署日志对话框 -->
    <n-modal
      v-model:show="showDeployLogDialog"
      preset="card"
      title="一键部署"
      style="width: 820px"
      :mask-closable="false"
      :closable="false"
      :bordered="false"
    >
      <div class="deploy-modal-head">
        <n-tag :type="deploying ? 'warning' : (deployDone ? 'success' : 'info')" size="small" round>
          {{ deploying ? '部署中' : (deployDone ? '部署完成' : '等待开始') }}
        </n-tag>
        <span class="deploy-modal-project">{{ deployingProjectName }}</span>
      </div>
      <pre class="clone-log">{{ deployLogText || '等待部署输出...' }}</pre>
      <template #footer>
        <n-button v-if="deploying" type="warning" @click="cancelDeploy">取消部署</n-button>
        <n-button v-else type="primary" @click="showDeployLogDialog = false">关闭</n-button>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { h, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import type { DataTableColumns } from 'naive-ui'
import {
  NButton,
  NDataTable,
  NForm,
  NFormItem,
  NIcon,
  NInput,
  NInputGroup,
  NInputGroupLabel,
  NModal,
  NSelect,
  NSpace,
  NTag,
  useDialog,
  useMessage
} from 'naive-ui'
import { FolderOpen, GitBranch, Plus, Rocket } from 'lucide-vue-next'
import { ProjectApi } from '@/api/project'
import { SettingsApi } from '@/api/settings'
import { DeployApi } from '@/api/deploy'
import type { SseCallbacks } from '@/utils/sseStream'

defineOptions({
  name: 'Projects'
})

const router = useRouter()
const message = useMessage()
const dialog = useDialog()

const projects = ref<any[]>([])
const dataRoot = ref('')
const createDataDirRel = ref('')

const showCreateDialog = ref(false)
const createForm = ref({
  name: '',
  description: '',
  gitUrl: '',
  devLanguage: '',
  status: 0,
  workDir: '',
  dataDir: ''
})

const showCloneDialog = ref(false)
const cloneForm = ref({
  gitUrl: '',
  name: '',
  description: '',
  username: '',
  password: ''
})
const showCloneLogDialog = ref(false)
const cloneLogText = ref('')
const cloneRunning = ref(false)
let clonePollTimer: number | null = null

// 一键部署状态
const showDeployLogDialog = ref(false)
const deployLogText = ref('')
const deploying = ref(false)
const deployDone = ref(false)
const deployingProjectName = ref('')
let deployController: AbortController | null = null

const statusOptions = [
  { label: '未初始化', value: 0 },
  { label: '待启动', value: 1 },
  { label: '进行中', value: 2 },
  { label: '已暂停', value: 3 },
  { label: '已完成', value: 4 }
]

function statusToTagType(status: number): 'default' | 'info' | 'success' | 'warning' | 'error' {
  const map: Record<number, 'default' | 'info' | 'success' | 'warning' | 'error'> = {
    0: 'info',
    1: 'success',
    2: 'default',
    3: 'warning',
    4: 'error'
  }
  return map[status] ?? 'default'
}

const getStatusText = (status: number) => {
  const statusTexts = ['未初始化', '待启动', '进行中', '已暂停', '已完成']
  return statusTexts[status] ?? '未知'
}

const viewProject = (id: number) => {
  router.push(`/projects/${id}`)
}

/** 点击项目名称跳转到全链路任务界面（携带已有项目ID） */
const viewPipelineTask = (project: any) => {
  router.push({ path: '/projects/chat-create', query: { projectId: String(project.id) } })
}

const deleteProject = (id: number) => {
  dialog.warning({
    title: '确认删除',
    content: '确定要删除该项目吗？删除后不可恢复。',
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        const ok = await ProjectApi.deleteProject(id)
        if (ok) {
          message.success('项目已删除')
          await loadProjects()
        } else {
          message.error('删除失败')
        }
      } catch (e) {
        console.error('删除项目失败:', e)
        message.error('删除失败')
      }
    }
  })
}

const handleCreate = async () => {
  try {
    if (dataRoot.value) {
      createForm.value.dataDir = createDataDirRel.value.trim() || createForm.value.name.trim()
    } else {
      createForm.value.dataDir = ''
    }
    const response: any = await ProjectApi.createProject({
      name: createForm.value.name,
      description: createForm.value.description,
      gitUrl: createForm.value.gitUrl,
      status: createForm.value.status,
      dataDir: createForm.value.dataDir
      // devLanguage 和 workDir 由后端统一控制，前端不再传递
    })
    if (response && response.success !== false) {
      message.success('项目创建成功')
      showCreateDialog.value = false
      await loadProjects()
      createForm.value = {
        name: '',
        description: '',
        gitUrl: '',
        devLanguage: '',
        status: 0,
        workDir: '',
        dataDir: ''
      }
      createDataDirRel.value = ''
    } else {
      message.error(response?.message || '项目创建失败')
    }
  } catch (error: any) {
    console.error('项目创建失败:', error)
  }
}

const handleClone = async () => {
  try {
    const startRes: any = await ProjectApi.startCloneProjectStream(cloneForm.value)
    if (!startRes?.success || !startRes?.taskId) {
      message.error(startRes?.message || '启动拉取失败')
      return
    }
    showCloneDialog.value = false
    showCloneLogDialog.value = true
    cloneRunning.value = true
    cloneLogText.value = '$ git clone ...\n'

    const taskId = startRes.taskId as string
    if (clonePollTimer) {
      clearInterval(clonePollTimer)
      clonePollTimer = null
    }
    clonePollTimer = window.setInterval(async () => {
      try {
        const streamRes: any = await ProjectApi.getCloneProjectStream(taskId)
        if (streamRes?.success) {
          if (streamRes.logDelta) {
            cloneLogText.value += streamRes.logDelta
          }
          if (streamRes.finished) {
            if (clonePollTimer) {
              clearInterval(clonePollTimer)
              clonePollTimer = null
            }
            cloneRunning.value = false
            if (streamRes.taskSuccess) {
              message.success(streamRes.message || '项目拉取成功')
              await loadProjects()
              cloneForm.value = { gitUrl: '', name: '', description: '', username: '', password: '' }
              window.setTimeout(() => {
                showCloneLogDialog.value = false
              }, 1200)
            } else {
              message.error(streamRes.message || '项目拉取失败')
            }
          }
        } else {
          if (clonePollTimer) {
            clearInterval(clonePollTimer)
            clonePollTimer = null
          }
          cloneRunning.value = false
          message.error(streamRes?.message || '拉取日志读取失败')
        }
      } catch {
        if (clonePollTimer) {
          clearInterval(clonePollTimer)
          clonePollTimer = null
        }
        cloneRunning.value = false
        message.error('拉取日志读取失败')
      }
    }, 800)
  } catch (error: any) {
    console.error('项目拉取失败:', error)
  }
}

/** 点击「一键部署」先弹出操作选项，不直接部署：部署 / 停止 */
const handleDeployOptions = (project: any) => {
  if (deploying.value) {
    message.warning('当前有部署任务正在进行中，请稍后再试')
    return
  }
  dialog.create({
    title: '一键部署',
    content: `请选择对「${project.name || `项目#${project.id}`}」的操作：`,
    positiveText: '部署',
    negativeText: '停止',
    onPositiveClick: () => handleDeploy(project),
    onNegativeClick: () => handleStop(project),
  })
}

/** 停止（卸载）项目部署：停止远程后端、移除 Nginx 配置、释放端口、删除数据库。 */
const handleStop = async (project: any) => {
  const name = project.name || `项目#${project.id}`
  try {
    await DeployApi.undeploy(project.id)
    message.success(`「${name}」已停止`)
  } catch (e: any) {
    console.error('停止项目失败:', e)
    message.error(e?.message || `「${name}」停止失败`)
  } finally {
    loadProjects()
  }
}

/** 一键部署：SSE 流式推送部署日志，完成/失败后刷新列表状态。 */
const handleDeploy = (project: any) => {
  if (deploying.value) return
  deployingProjectName.value = project.name || `项目#${project.id}`
  deployLogText.value = ''
  deployDone.value = false
  showDeployLogDialog.value = true
  deploying.value = true

  const callbacks: SseCallbacks = {
    onOutput: (payload: string) => {
      deployLogText.value += payload + '\n'
    },
    onDone: (payload: string) => {
      deploying.value = false
      deployDone.value = true
      deployController = null
      message.success(`「${deployingProjectName.value}」部署完成`)
      loadProjects()
    },
    onError: (msg: string) => {
      deploying.value = false
      deployController = null
      deployLogText.value += `\n[错误] ${msg}\n`
      message.error(msg || `「${deployingProjectName.value}」部署失败`)
    },
  }

  // force=true：项目已运行也强制重新部署（复用端口、URL 不变），保证推送最新代码
  deployController = DeployApi.deploy(project.id, callbacks, true)
}

/** 取消进行中的部署。 */
const cancelDeploy = () => {
  if (deployController) {
    deployController.abort()
    deployController = null
  }
  deploying.value = false
  showDeployLogDialog.value = false
}

const loadProjects = async () => {
  try {
    const response = await ProjectApi.getProjectList()
    projects.value = response
  } catch (error) {
    console.error('加载项目列表失败:', error)
    message.error('加载项目列表失败')
  }
}

const loadDataRoot = async () => {
  try {
    const res: any = await SettingsApi.getDataRoot()
    dataRoot.value = res.value || ''
  } catch {
    dataRoot.value = ''
  }
}

const columns: DataTableColumns<any> = [
  {
    title: '项目名称',
    key: 'name',
    ellipsis: { tooltip: true },
    render(row) {
      return h(
        NButton,
        { type: 'primary', text: true, onClick: () => viewPipelineTask(row) },
        { default: () => row.name }
      )
    }
  },
  { title: '项目描述', key: 'description', ellipsis: { tooltip: true } },
  { title: 'Git地址', key: 'gitUrl', minWidth: 200, ellipsis: { tooltip: true } },
  { title: '数据目录', key: 'dataDir', ellipsis: { tooltip: true }, minWidth: 160 },
  {
    title: '状态',
    key: 'status',
    width: 100,
    render(row) {
      return h(NTag, { type: statusToTagType(row.status), size: 'small', round: true }, { default: () => getStatusText(row.status) })
    }
  },
  { title: '创建时间', key: 'createdAt', width: 180 },
  {
    title: '操作',
    key: 'actions',
    width: 200,
    fixed: 'right',
    render(row) {
      const children: ReturnType<typeof h>[] = [
        h(
          NButton,
          { size: 'small', type: 'primary', text: true, onClick: () => viewProject(row.id) },
          { default: () => '查看' }
        ),
        h(
          NButton,
          {
            size: 'small',
            type: 'warning',
            text: true,
            disabled: deploying.value,
            onClick: () => handleDeployOptions(row)
          },
          {
            default: () =>
              h('span', { style: 'display:inline-flex;align-items:center;gap:2px' }, [
                h(NIcon, { component: Rocket, size: 14 }),
                '一键部署'
              ])
          }
        ),
        h(
          NButton,
          { size: 'small', type: 'error', text: true, onClick: () => deleteProject(row.id) },
          { default: () => '删除' }
        )
      ]
      return h(NSpace, { size: 'small', wrap: true }, { default: () => children })
    }
  }
]

onMounted(() => {
  loadProjects()
  loadDataRoot()
})

onBeforeUnmount(() => {
  if (clonePollTimer) {
    clearInterval(clonePollTimer)
    clonePollTimer = null
  }
  if (deployController) {
    deployController.abort()
    deployController = null
  }
})
</script>

<style scoped>
.page-container {
  padding: 0;
  height: 100vh;
  display: flex;
  flex-direction: column;
  overflow-y: auto;
  background: #f5f5f7;
}

.macos-card {
  flex: 1;
  background: #fff;
  border-radius: 0;
  margin: 0;
}

.page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 0;
  gap: 16px;
  padding: 16px 20px;
  background: #fff;
  border-bottom: 1px solid #e5e5ea;
}

@media (max-width: 768px) {
  .page-header {
    flex-direction: column;
  }
}

.page-header__left {
  flex: 1;
}

.page-title {
  font-size: 24px;
  font-weight: 600;
  color: #1d1d1f;
  margin-bottom: 4px;
}

.page-subtitle {
  font-size: 14px;
  color: #86868b;
}

.page-header__right {
  display: flex;
  gap: 12px;
  flex-shrink: 0;
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px;
  border-bottom: 1px solid #e5e5ea;
}

.card-header__title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 15px;
  font-weight: 600;
  color: #1d1d1f;
}

.card-body {
  padding: 16px 20px;
  overflow-x: auto;
}

.clone-log {
  height: 360px;
  margin: 0;
  overflow: auto;
  border-radius: 10px;
  padding: 16px;
  font-family: 'SF Mono', Monaco, 'Inconsolata', monospace;
  font-size: 13px;
  line-height: 1.6;
  white-space: pre-wrap;
  color: #1d1d1f;
  background: #f5f5f7;
}

.deploy-modal-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
}

.deploy-modal-project {
  font-size: 14px;
  font-weight: 600;
  color: #1d1d1f;
}
</style>