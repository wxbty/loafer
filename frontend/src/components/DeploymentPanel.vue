<template>
  <div class="flex flex-col gap-4">
    <!-- 部署状态 -->
    <el-card shadow="hover" class="section-card">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-semibold">部署状态</span>
          <el-tag :type="statusTagType" size="small">{{ statusLabel }}</el-tag>
        </div>
      </template>

      <div v-loading="loadingDeployment">
        <!-- 未部署 -->
        <template v-if="!isDeployed">
          <el-empty description="项目尚未部署" :image-size="80">
            <el-button type="primary" :loading="deploying" @click="handleDeploy">
              <el-icon class="mr-1"><Promotion /></el-icon>
              部署项目
            </el-button>
          </el-empty>
        </template>

        <!-- 已部署 -->
        <template v-else>
          <el-descriptions :column="2" border size="small">
            <el-descriptions-item label="访问地址">
              <el-link
                v-if="deployment?.accessUrl"
                type="primary"
                :href="deployment.accessUrl"
                target="_blank"
                rel="noopener"
              >
                <el-icon class="mr-1"><Link /></el-icon>
                {{ deployment.accessUrl }}
              </el-link>
              <span v-else class="text-slate-400">-</span>
            </el-descriptions-item>
            <el-descriptions-item label="部署状态">
              <el-tag :type="statusTagType" size="small">{{ statusLabel }}</el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="前端端口">
              {{ deployment?.frontendPort || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="后端端口">
              {{ deployment?.backendPort || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="Nginx 配置路径" :span="2">
              <span class="font-mono text-xs break-all">{{ deployment?.nginxConfigPath || '-' }}</span>
            </el-descriptions-item>
            <el-descriptions-item v-if="deployment?.lastDeployedAt" label="最近部署时间" :span="2">
              {{ formatTime(deployment.lastDeployedAt) }}
            </el-descriptions-item>
          </el-descriptions>

          <div class="flex items-center gap-2 mt-4">
            <el-button type="danger" :loading="undeploying" @click="handleUndeploy">
              <el-icon class="mr-1"><Delete /></el-icon>
              卸载
            </el-button>
            <el-button :loading="loadingLogs" @click="toggleLogs">
              <el-icon class="mr-1"><View /></el-icon>
              查看日志
            </el-button>
            <el-button :loading="loadingDeployment" @click="loadDeployment">
              <el-icon class="mr-1"><Refresh /></el-icon>
              刷新
            </el-button>
          </div>
        </template>
      </div>
    </el-card>

    <!-- 部署流式输出 -->
    <el-card v-if="deploying || deployOutput" shadow="hover" class="section-card">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-semibold">
            部署输出
            <el-icon v-if="deploying" class="is-loading ml-1"><Loading /></el-icon>
          </span>
          <el-tag v-if="deploying" type="warning" size="small">部署中</el-tag>
        </div>
      </template>
      <div ref="deployOutputRef" class="stream-output">{{ deployOutput || '等待输出...' }}</div>
    </el-card>

    <!-- 端口分配 -->
    <el-card shadow="hover" class="section-card">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-semibold">端口分配</span>
          <div class="flex items-center gap-2">
            <el-tag v-if="portRange" type="info" size="small">
              端口段：{{ portRange.start }} - {{ portRange.end }}
            </el-tag>
            <el-button size="small" type="primary" @click="openPortDialog">
              <el-icon class="mr-1"><Plus /></el-icon>
              分配端口
            </el-button>
          </div>
        </div>
      </template>

      <el-table
        v-loading="loadingPorts"
        :data="ports"
        size="small"
        border
        stripe
        empty-text="暂无已分配端口"
      >
        <el-table-column prop="port" label="端口" width="100" />
        <el-table-column prop="portType" label="类型" width="120">
          <template #default="{ row }">
            <el-tag size="small" :type="portTypeTag(row.portType)">{{ row.portType || '-' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="描述" show-overflow-tooltip />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag size="small" :type="row.status === 'allocated' ? 'success' : 'info'">
              {{ row.status === 'allocated' ? '已分配' : '已释放' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="allocatedAt" label="分配时间" width="170">
          <template #default="{ row }">{{ formatTime(row.allocatedAt) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="90" fixed="right">
          <template #default="{ row }">
            <el-button
              type="danger"
              link
              size="small"
              :disabled="row.status !== 'allocated'"
              @click="handleReleasePort(row.port)"
            >
              释放
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 数据库供给 -->
    <el-card shadow="hover" class="section-card">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-semibold">数据库供给</span>
          <el-tag v-if="database" :type="dbStatusTagType" size="small">{{ dbStatusLabel }}</el-tag>
        </div>
      </template>

      <div v-loading="loadingDatabase">
        <template v-if="database && database.status === 'ready'">
          <el-descriptions :column="2" border size="small">
            <el-descriptions-item label="数据库名">
              <span class="font-mono">{{ database.dbName }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="状态">
              <el-tag :type="dbStatusTagType" size="small">{{ dbStatusLabel }}</el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="主机">
              <span class="font-mono">{{ database.dbHost }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="端口">
              {{ database.dbPort }}
            </el-descriptions-item>
            <el-descriptions-item label="用户名">
              <span class="font-mono">{{ database.dbUsername }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="创建时间">
              {{ formatTime(database.createdAt) }}
            </el-descriptions-item>
          </el-descriptions>

          <div class="flex items-center gap-2 mt-4">
            <el-button type="danger" :loading="droppingDb" @click="handleDropDatabase">
              <el-icon class="mr-1"><Delete /></el-icon>
              删除数据库
            </el-button>
          </div>
        </template>

        <template v-else>
          <el-empty :description="database ? '数据库已删除，可重新创建' : '项目尚未创建数据库'" :image-size="80">
            <el-button type="primary" :loading="provisioningDb" @click="handleProvisionDatabase">
              <el-icon class="mr-1"><Plus /></el-icon>
              创建数据库
            </el-button>
          </el-empty>
        </template>
      </div>
    </el-card>

    <!-- 部署日志（可折叠） -->
    <el-card v-if="logsVisible" shadow="hover" class="section-card">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-semibold">部署日志</span>
          <el-button link size="small" @click="logsVisible = false">收起</el-button>
        </div>
      </template>
      <pre ref="logPreRef" class="stream-output">{{ deployLog || '暂无日志' }}</pre>
    </el-card>

    <!-- 分配端口对话框 -->
    <el-dialog v-model="portDialogVisible" title="分配端口" width="440px" :close-on-click-modal="false">
      <el-form ref="portFormRef" :model="portForm" :rules="portFormRules" label-width="80px">
        <el-form-item label="端口类型" prop="portType">
          <el-select v-model="portForm.portType" placeholder="请选择端口类型" class="w-full">
            <el-option label="前端 (frontend)" value="frontend" />
            <el-option label="后端 (backend)" value="backend" />
            <el-option label="其他 (other)" value="other" />
          </el-select>
        </el-form-item>
        <el-form-item label="描述" prop="description">
          <el-input
            v-model="portForm.description"
            type="textarea"
            :rows="2"
            placeholder="端口用途描述（可选）"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="portDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="allocatingPort" @click="submitAllocatePort">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onBeforeUnmount, watch, nextTick } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import {
  Promotion,
  Delete,
  View,
  Refresh,
  Plus,
  Link,
  Loading,
} from '@element-plus/icons-vue'
import { DeployApi } from '@/api/deploy'
import { PortApi, DatabaseApi } from '@/api/infra'
import type { SseCallbacks } from '@/utils/sseStream'

// ---- 类型定义（与后端 model 对齐） ----
interface ProjectDeployment {
  id?: number
  projectId: number
  frontendPort: number
  backendPort: number
  nginxConfigPath: string
  buildDir: string
  backendBinary: string
  status: string
  accessUrl: string
  deployLog: string
  lastDeployedAt?: string | null
  createdAt?: string
  updatedAt?: string
}

interface PortAllocation {
  id: number
  projectId: number
  port: number
  portType: string
  description: string
  allocatedAt: string
  releasedAt?: string | null
  status: string
}

interface ProjectDatabase {
  id?: number
  projectId: number
  dbName: string
  dbHost: string
  dbPort: number
  dbUsername: string
  dbPassword: string
  status: string
  createdAt?: string
  updatedAt?: string
}

interface Props {
  projectId: number
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'status-change', status: string): void
}>()

// ---- 响应式状态 ----
const deployment = ref<ProjectDeployment | null>(null)
const status = ref<string>('not_deployed')
const ports = ref<PortAllocation[]>([])
const portRange = ref<{ start: number; end: number } | null>(null)
const database = ref<ProjectDatabase | null>(null)
const deployLog = ref<string>('')

const deploying = ref(false)
const undeploying = ref(false)
const loadingDeployment = ref(false)
const loadingPorts = ref(false)
const loadingDatabase = ref(false)
const loadingLogs = ref(false)
const provisioningDb = ref(false)
const droppingDb = ref(false)
const allocatingPort = ref(false)

const deployOutput = ref<string>('')
const deployOutputRef = ref<HTMLElement | null>(null)
const logPreRef = ref<HTMLElement | null>(null)
const logsVisible = ref(false)

// 端口对话框
const portDialogVisible = ref(false)
const portFormRef = ref<FormInstance>()
const portForm = reactive({
  portType: 'frontend',
  description: '',
})
const portFormRules: FormRules = {
  portType: [{ required: true, message: '请选择端口类型', trigger: 'change' }],
}

// SSE 控制器与轮询定时器
let deployController: AbortController | null = null
let statusPollTimer: ReturnType<typeof setInterval> | null = null

// ---- 计算属性 ----
const isDeployed = computed(() => {
  return (
    !!deployment.value &&
    status.value !== 'not_deployed' &&
    status.value !== 'undeployed'
  )
})

const statusLabel = computed(() => {
  const map: Record<string, string> = {
    not_deployed: '未部署',
    deploying: '部署中',
    running: '运行中',
    stopped: '已停止',
    failed: '部署失败',
    undeployed: '已卸载',
    pending: '等待中',
  }
  return map[status.value] || status.value
})

const statusTagType = computed<'primary' | 'success' | 'warning' | 'danger' | 'info'>(() => {
  const map: Record<string, 'primary' | 'success' | 'warning' | 'danger' | 'info'> = {
    running: 'success',
    deploying: 'primary',
    pending: 'primary',
    stopped: 'warning',
    failed: 'danger',
    not_deployed: 'info',
    undeployed: 'info',
  }
  return map[status.value] || 'info'
})

const dbStatusLabel = computed(() => {
  const map: Record<string, string> = {
    pending: '创建中',
    ready: '就绪',
    dropped: '已删除',
  }
  return database.value ? map[database.value.status] || database.value.status : ''
})

const dbStatusTagType = computed<'success' | 'warning' | 'info'>(() => {
  const map: Record<string, 'success' | 'warning' | 'info'> = {
    ready: 'success',
    pending: 'warning',
    dropped: 'info',
  }
  return database.value ? map[database.value.status] || 'info' : 'info'
})

// ---- 工具方法 ----
const formatTime = (timeStr?: string | null) => {
  if (!timeStr) return '-'
  const date = new Date(timeStr)
  if (isNaN(date.getTime())) return '-'
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

const portTypeTag = (type: string): 'success' | 'warning' | 'info' => {
  if (type === 'frontend') return 'success'
  if (type === 'backend') return 'warning'
  return 'info'
}

const scrollToBottom = (el: HTMLElement | null) => {
  if (el) {
    el.scrollTop = el.scrollHeight
  }
}

// ---- 数据加载 ----
const loadDeployment = async () => {
  loadingDeployment.value = true
  try {
    const [depRes, statusRes] = await Promise.all([
      DeployApi.getDeployment(props.projectId),
      DeployApi.getStatus(props.projectId),
    ])
    const depData: any = depRes
    const statusData: any = statusRes
    // 后端统一返回 { success, data }；getDeployment 无记录时返回 success:false
    deployment.value = depData?.success ? (depData.data as ProjectDeployment) : null
    status.value = statusData?.success ? statusData.data?.status || 'not_deployed' : 'not_deployed'
    if (deployment.value?.deployLog) {
      deployLog.value = deployment.value.deployLog
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '加载部署信息失败')
  } finally {
    loadingDeployment.value = false
  }
}

const loadPorts = async () => {
  loadingPorts.value = true
  try {
    const [rangeRes, portsRes] = await Promise.all([
      PortApi.getPortRange(),
      PortApi.getProjectPorts(props.projectId),
    ])
    const rangeData: any = rangeRes
    const portsData: any = portsRes
    portRange.value = rangeData?.success ? rangeData.data : null
    ports.value = portsData?.success ? (portsData.data as PortAllocation[]) : []
  } catch (e: any) {
    ElMessage.error(e?.message || '加载端口信息失败')
  } finally {
    loadingPorts.value = false
  }
}

const loadDatabase = async () => {
  loadingDatabase.value = true
  try {
    const res: any = await DatabaseApi.get(props.projectId)
    database.value = res?.success ? (res.data as ProjectDatabase) : null
  } catch (e: any) {
    ElMessage.error(e?.message || '加载数据库信息失败')
  } finally {
    loadingDatabase.value = false
  }
}

const loadLogs = async () => {
  loadingLogs.value = true
  try {
    const res: any = await DeployApi.getLogs(props.projectId)
    deployLog.value = res?.success ? res.data || '' : ''
    logsVisible.value = true
    await nextTick()
    scrollToBottom(logPreRef.value)
  } catch (e: any) {
    ElMessage.error(e?.message || '加载日志失败')
  } finally {
    loadingLogs.value = false
  }
}

const toggleLogs = () => {
  if (!logsVisible.value) {
    loadLogs()
  } else {
    logsVisible.value = false
  }
}

const refreshAll = async () => {
  await Promise.all([loadDeployment(), loadPorts(), loadDatabase()])
}

// ---- 部署 / 卸载 ----
const handleDeploy = () => {
  if (deploying.value) return
  deploying.value = true
  deployOutput.value = ''

  const callbacks: SseCallbacks = {
    onOutput: (payload: string) => {
      deployOutput.value += payload + '\n'
      nextTick(() => scrollToBottom(deployOutputRef.value))
    },
    onDone: (payload: string) => {
      deploying.value = false
      // done 帧的 payload 是 ProjectDeployment 的 JSON 序列化字符串
      let result: ProjectDeployment | null = null
      try {
        result = payload ? (JSON.parse(payload) as ProjectDeployment) : null
      } catch {
        result = null
      }
      if (result) {
        deployment.value = result
        status.value = result.status || 'running'
        deployLog.value = result.deployLog || deployLog.value
      }
      ElMessage.success('部署完成')
      emit('status-change', status.value)
      deployController = null
      // 刷新端口与数据库（部署过程会自动分配端口与创建数据库）
      loadPorts()
      loadDatabase()
    },
    onError: (msg: string) => {
      deploying.value = false
      deployOutput.value += `\n[错误] ${msg}\n`
      ElMessage.error(msg || '部署失败')
      emit('status-change', 'failed')
      deployController = null
      // 刷新状态以同步真实结果
      loadDeployment()
    },
  }

  deployController = DeployApi.deploy(props.projectId, callbacks)
}

const handleUndeploy = async () => {
  try {
    await ElMessageBox.confirm(
      '确认卸载该项目部署？将停止后端服务、移除 Nginx 配置、释放端口并删除数据库，操作不可恢复。',
      '卸载确认',
      {
        confirmButtonText: '卸载',
        cancelButtonText: '取消',
        type: 'warning',
      }
    )
  } catch (e) {
    // 用户取消
    return
  }

  undeploying.value = true
  try {
    const res: any = await DeployApi.undeploy(props.projectId)
    if (res?.success) {
      ElMessage.success('已卸载')
      status.value = 'undeployed'
      deployment.value = null
      deployLog.value = ''
      emit('status-change', 'undeployed')
      // 卸载会释放端口与删除数据库，刷新两者
      loadPorts()
      loadDatabase()
    } else {
      ElMessage.error(res?.message || '卸载失败')
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '卸载失败')
  } finally {
    undeploying.value = false
  }
}

// ---- 端口管理 ----
const openPortDialog = () => {
  portForm.portType = 'frontend'
  portForm.description = ''
  portDialogVisible.value = true
}

const submitAllocatePort = async () => {
  if (!portFormRef.value) return
  try {
    await portFormRef.value.validate()
  } catch {
    return
  }
  allocatingPort.value = true
  try {
    const res: any = await PortApi.allocatePort(props.projectId, {
      portType: portForm.portType,
      description: portForm.description,
    })
    if (res?.success) {
      ElMessage.success(`端口分配成功：${res.data?.port ?? ''}`)
      portDialogVisible.value = false
      loadPorts()
    } else {
      ElMessage.error(res?.message || '分配端口失败')
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '分配端口失败')
  } finally {
    allocatingPort.value = false
  }
}

const handleReleasePort = async (port: number) => {
  try {
    await ElMessageBox.confirm(`确认释放端口 ${port}？`, '释放确认', {
      confirmButtonText: '释放',
      cancelButtonText: '取消',
      type: 'warning',
    })
  } catch {
    return
  }
  try {
    const res: any = await PortApi.releasePort(port)
    if (res?.success) {
      ElMessage.success('端口已释放')
      loadPorts()
    } else {
      ElMessage.error(res?.message || '释放端口失败')
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '释放端口失败')
  }
}

// ---- 数据库管理 ----
const handleProvisionDatabase = async () => {
  provisioningDb.value = true
  try {
    const res: any = await DatabaseApi.provision(props.projectId)
    if (res?.success) {
      ElMessage.success('数据库创建成功')
      database.value = res.data as ProjectDatabase
    } else {
      ElMessage.error(res?.message || '创建数据库失败')
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '创建数据库失败')
  } finally {
    provisioningDb.value = false
  }
}

const handleDropDatabase = async () => {
  try {
    await ElMessageBox.confirm(
      '确认删除该项目数据库？数据将全部丢失且不可恢复。',
      '删除确认',
      {
        confirmButtonText: '删除',
        cancelButtonText: '取消',
        type: 'warning',
      }
    )
  } catch {
    return
  }
  droppingDb.value = true
  try {
    const res: any = await DatabaseApi.drop(props.projectId)
    if (res?.success) {
      ElMessage.success('数据库已删除')
      if (database.value) {
        database.value = { ...database.value, status: 'dropped' }
      }
      loadDatabase()
    } else {
      ElMessage.error(res?.message || '删除数据库失败')
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '删除数据库失败')
  } finally {
    droppingDb.value = false
  }
}

// ---- 状态轮询：running 时每 10 秒刷新 ----
const startPolling = () => {
  stopPolling()
  statusPollTimer = setInterval(async () => {
    try {
      const res: any = await DeployApi.getStatus(props.projectId)
      if (res?.success) {
        const newStatus = res.data?.status || 'not_deployed'
        if (newStatus !== status.value) {
          status.value = newStatus
          emit('status-change', newStatus)
          // 状态变化时同步部署记录
          if (newStatus === 'stopped' || newStatus === 'failed') {
            loadDeployment()
          }
        }
      }
    } catch {
      // 轮询失败静默处理，下次重试
    }
  }, 10000)
}

const stopPolling = () => {
  if (statusPollTimer) {
    clearInterval(statusPollTimer)
    statusPollTimer = null
  }
}

watch(
  () => status.value,
  (newStatus) => {
    if (newStatus === 'running') {
      startPolling()
    } else {
      stopPolling()
    }
  }
)

// ---- 生命周期 ----
onMounted(() => {
  refreshAll()
})

onBeforeUnmount(() => {
  stopPolling()
  if (deployController) {
    deployController.abort()
    deployController = null
  }
})
</script>

<style scoped>
.section-card {
  width: 100%;
}

.stream-output {
  max-height: 360px;
  min-height: 160px;
  overflow: auto;
  margin: 0;
  padding: 12px;
  border: 1px solid #1e293b;
  border-radius: 6px;
  background: #0f172a;
  color: #e2e8f0;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', monospace;
  font-size: 12px;
  line-height: 1.6;
}

.stream-output:empty::before {
  content: '等待输出...';
  color: #64748b;
}
</style>
