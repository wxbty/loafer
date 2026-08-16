<template>
  <div class="min-h-full">
    <header class="border-b border-slate-200 bg-white px-4 shadow-sm md:px-6">
      <div class="flex h-14 items-center">
        <h1 class="text-lg font-semibold text-slate-900">系统设置</h1>
      </div>
    </header>

    <div class="mx-auto max-w-7xl px-4 py-4 md:px-6 md:py-6">
      <Card class="mx-auto max-w-5xl shadow-sm settings-main-card">
          <CardHeader>
            <CardTitle>系统设置</CardTitle>
            <CardDescription>工程路径与 Claude 运行环境</CardDescription>
          </CardHeader>
          <CardContent class="pt-2">
            <Tabs v-model="settingsTab" class="w-full">
              <TabsList class="mb-6 flex w-full max-w-3xl flex-wrap gap-1 settings-tabs-list">
                <TabsTrigger value="env">环境配置</TabsTrigger>
                <TabsTrigger value="prompts">提示词</TabsTrigger>
                <TabsTrigger value="session-pool">会话池</TabsTrigger>
                <TabsTrigger value="notification">通知设置</TabsTrigger>
              </TabsList>

              <TabsContent value="env" class="mt-0 space-y-6 focus-visible:ring-0">
                <Card>
                  <CardHeader>
                    <CardTitle class="flex items-center gap-2 text-base font-semibold">
                      <el-icon><FolderOpened /></el-icon>
                      工程根目录
                    </CardTitle>
                  </CardHeader>
                  <CardContent>
                    <el-alert
                      type="info"
                      :closable="false"
                      show-icon
                      class="mb-5"
                    >
                      <template #title>
                        设置工程根目录后，所有新建项目的工作目录必须在此目录下。未设置时不做路径限制。
                      </template>
                    </el-alert>

                    <el-form label-width="120px" @submit.prevent>
                      <el-form-item label="根目录路径">
                        <el-input
                          v-model="workspaceRoot"
                          placeholder="请输入绝对路径，如 /home/user/projects"
                          clearable
                          class="max-w-xl"
                        >
                          <template #prefix>
                            <el-icon><Folder /></el-icon>
                          </template>
                        </el-input>
                      </el-form-item>

                      <el-form-item>
                        <div class="flex flex-wrap items-center gap-2">
                          <el-button type="primary" :loading="saving" @click="handleSave">
                            保存设置
                          </el-button>
                          <el-tag v-if="savedValue" type="success" effect="plain">
                            当前值: {{ savedValue }}
                          </el-tag>
                          <el-tag v-if="savedValue && !pathExists" type="danger" effect="plain">
                            目录不存在
                          </el-tag>
                          <el-tag v-if="savedValue && pathExists" type="success" effect="plain">
                            目录有效
                          </el-tag>
                        </div>
                      </el-form-item>
                    </el-form>
                  </CardContent>
                </Card>
              </TabsContent>

              <TabsContent value="prompts" class="mt-0 focus-visible:ring-0">
                <Card>
                  <CardHeader class="flex flex-row items-start justify-between space-y-0 pb-4">
                    <div>
                      <CardTitle class="flex items-center gap-2 text-base font-semibold">
                        <el-icon><ChatDotRound /></el-icon>
                        提示词模板管理
                      </CardTitle>
                      <p class="text-sm text-slate-500 mt-1">管理系统提示词模板，支持变量替换。系统内置模板不可删除但可编辑。</p>
                    </div>
                    <el-button type="primary" size="small" @click="handleAddPrompt">
                      <el-icon><Plus /></el-icon>
                      新增模板
                    </el-button>
                  </CardHeader>
                  <CardContent>
                    <el-table :data="promptTemplates" class="w-full" v-loading="promptLoading">
                      <el-table-column prop="templateKey" label="模板标识" width="180">
                        <template #default="{ row }">
                          <code class="text-sm bg-slate-100 px-2 py-1 rounded">{{ row.templateKey }}</code>
                        </template>
                      </el-table-column>
                      <el-table-column prop="templateName" label="模板名称" width="180" />
                      <el-table-column prop="description" label="描述" show-overflow-tooltip />
                      <el-table-column prop="useCount" label="使用次数" width="100" align="center" />
                      <el-table-column label="状态" width="100" align="center">
                        <template #default="{ row }">
                          <el-tag :type="row.isEnabled === 1 ? 'success' : 'info'" size="small">
                            {{ row.isEnabled === 1 ? '启用' : '禁用' }}
                          </el-tag>
                        </template>
                      </el-table-column>
                      <el-table-column label="类型" width="100" align="center">
                        <template #default="{ row }">
                          <el-tag v-if="row.isSystem === 1" type="warning" size="small">系统</el-tag>
                          <el-tag v-else type="default" size="small">自定义</el-tag>
                        </template>
                      </el-table-column>
                      <el-table-column label="操作" width="200" fixed="right">
                        <template #default="{ row }">
                          <el-button link type="primary" size="small" @click="handleEditPrompt(row)">
                            编辑
                          </el-button>
                          <el-button
                            v-if="row.isSystem !== 1"
                            link
                            :type="row.isEnabled === 1 ? 'warning' : 'success'"
                            size="small"
                            @click="handleTogglePrompt(row)"
                          >
                            {{ row.isEnabled === 1 ? '禁用' : '启用' }}
                          </el-button>
                          <el-button
                            v-if="row.isSystem !== 1"
                            link
                            type="danger"
                            size="small"
                            @click="handleDeletePrompt(row)"
                          >
                            删除
                          </el-button>
                        </template>
                      </el-table-column>
                    </el-table>
                  </CardContent>
                </Card>
              </TabsContent>

              <TabsContent value="session-pool" class="mt-0 space-y-6 focus-visible:ring-0">
                <!-- Session Pool 状态 -->
                <Card>
                  <CardHeader>
                    <CardTitle class="flex items-center gap-2 text-base font-semibold">
                      <el-icon><Monitor /></el-icon>
                      Session Pool 状态
                    </CardTitle>
                    <p class="text-sm text-slate-500 mt-1">
                      Claude Code CLI 会话池容量与占用情况
                    </p>
                  </CardHeader>
                  <CardContent>
                    <div v-if="sessionPoolLoading" class="text-center py-8">
                      <el-icon class="is-loading text-2xl"><Loading /></el-icon>
                      <span class="ml-2">加载中...</span>
                    </div>
                    <div v-else-if="!sessionPoolStatus" class="text-center py-8">
                      <el-empty description="无法获取 Session Pool 状态" />
                    </div>
                    <div v-else class="space-y-4">
                      <div class="grid grid-cols-2 md:grid-cols-5 gap-4">
                        <div class="bg-slate-50 rounded-lg p-3">
                          <div class="text-xs text-slate-500 mb-1">活跃会话</div>
                          <div class="font-medium">
                            <el-tag :type="sessionPoolStatus.isFull ? 'danger' : 'success'" size="small">
                              {{ sessionPoolStatus.activeCount }}
                            </el-tag>
                          </div>
                        </div>
                        <div class="bg-slate-50 rounded-lg p-3">
                          <div class="text-xs text-slate-500 mb-1">待创建</div>
                          <div class="font-medium">{{ sessionPoolStatus.pendingCount }}</div>
                        </div>
                        <div class="bg-slate-50 rounded-lg p-3">
                          <div class="text-xs text-slate-500 mb-1">最大容量</div>
                          <div class="font-medium">{{ sessionPoolStatus.maxPoolSize }}</div>
                        </div>
                        <div class="bg-slate-50 rounded-lg p-3">
                          <div class="text-xs text-slate-500 mb-1">空闲超时</div>
                          <div class="font-medium">{{ sessionPoolStatus.idleTimeoutMinutes }} 分钟</div>
                        </div>
                        <div class="bg-slate-50 rounded-lg p-3">
                          <div class="text-xs text-slate-500 mb-1">池状态</div>
                          <div class="font-medium">
                            <el-tag :type="sessionPoolStatus.isFull ? 'danger' : 'success'" size="small">
                              {{ sessionPoolStatus.isFull ? '已满' : '可用' }}
                            </el-tag>
                          </div>
                        </div>
                      </div>
                      <el-progress
                        :percentage="sessionPoolStatus.maxPoolSize > 0 ? Math.round(sessionPoolStatus.activeCount / sessionPoolStatus.maxPoolSize * 100) : 0"
                        :color="sessionPoolStatus.isFull ? '#F56C6C' : sessionPoolStatus.activeCount / sessionPoolStatus.maxPoolSize > 0.7 ? '#E6A23C' : '#67C23A'"
                        :stroke-width="12"
                      />
                      <div class="flex justify-end gap-2">
                        <el-button type="default" size="small" :loading="sessionPoolLoading" @click="handleCleanupExpired">
                          清理过期会话
                        </el-button>
                        <el-button type="default" size="small" :loading="sessionPoolLoading" @click="loadAllSessionData">
                          强制刷新
                        </el-button>
                      </div>
                    </div>
                  </CardContent>
                </Card>

                <!-- CC 终端会话 -->
                <Card>
                  <CardHeader>
                    <CardTitle class="flex items-center gap-2 text-base font-semibold">
                      <el-icon><Monitor /></el-icon>
                      CC 终端会话
                    </CardTitle>
                    <div class="flex items-center gap-2 mt-1">
                      <p class="text-sm text-slate-500">项目详情页中打开的 Claude Code 终端</p>
                      <el-button type="default" size="small" :loading="ccSessionsLoading" @click="loadCcSessions" class="ml-auto">
                        刷新
                      </el-button>
                    </div>
                  </CardHeader>
                  <CardContent>
                    <el-table :data="ccSessions" v-loading="ccSessionsLoading" size="small" empty-text="暂无活跃的 CC 终端会话">
                      <el-table-column label="会话 ID" width="160">
                        <template #default="{ row }">
                          <el-tooltip :content="row.sessionId" placement="top">
                            <span class="font-mono text-xs">{{ truncateId(row.sessionId) }}</span>
                          </el-tooltip>
                        </template>
                      </el-table-column>
                      <el-table-column prop="projectName" label="项目" width="120" show-overflow-tooltip />
                      <el-table-column label="状态" width="90">
                        <template #default="{ row }">
                          <el-tag :type="row.status === 'RUNNING' ? 'success' : 'danger'" size="small">
                            {{ row.status === 'RUNNING' ? '运行中' : row.status }}
                          </el-tag>
                        </template>
                      </el-table-column>
                      <el-table-column label="进程" width="70">
                        <template #default="{ row }">
                          <el-tag :type="row.isProcessAlive ? 'success' : 'danger'" size="small">
                            {{ row.isProcessAlive ? '存活' : '退出' }}
                          </el-tag>
                        </template>
                      </el-table-column>
                      <el-table-column label="空闲" width="80">
                        <template #default="{ row }">
                          <span class="text-xs">{{ row.idleMinutes }} 分钟</span>
                        </template>
                      </el-table-column>
                      <el-table-column label="创建时间" width="110">
                        <template #default="{ row }">
                          <span class="text-xs">{{ formatDateTime(row.createdAt) }}</span>
                        </template>
                      </el-table-column>
                      <el-table-column label="最后活跃" width="110">
                        <template #default="{ row }">
                          <span class="text-xs">{{ formatDateTime(row.lastActiveAt) }}</span>
                        </template>
                      </el-table-column>
                      <el-table-column label="操作" width="80" fixed="right">
                        <template #default="{ row }">
                          <el-button type="danger" size="small" link @click="handleCloseSession(row.sessionId)">
                            关闭
                          </el-button>
                        </template>
                      </el-table-column>
                    </el-table>
                  </CardContent>
                </Card>
              </TabsContent>

            </Tabs>
          </CardContent>
        </Card>
    </div>

    <!-- 提示词模板对话框 -->
    <el-dialog
      v-model="showPromptDialog"
      :title="isEditingPrompt ? (isSystemPrompt ? '编辑系统提示词模板' : '编辑提示词模板') : '新增提示词模板'"
      width="800px"
      :close-on-click-modal="false"
      class="shadcn-form-dialog"
    >
      <Card class="border-0 shadow-none">
        <CardContent class="pt-0">
      <el-form :model="currentPrompt" label-width="100px">
        <el-form-item label="模板标识" required>
          <el-input
            v-model="currentPrompt.templateKey"
            placeholder="如 task_execute"
            :disabled="isEditingPrompt"
            clearable
          >
            <template #prefix>
              <el-icon><Key /></el-icon>
            </template>
          </el-input>
          <div class="text-xs text-slate-500 mt-1">
            唯一标识，用于代码引用，英文+下划线
            <span v-if="isSystemPrompt" class="block mt-1 text-amber-700">系统内置模板不可修改标识键；可修改名称、描述、内容与变量定义。</span>
          </div>
        </el-form-item>
        <el-form-item label="模板名称" required>
          <el-input
            v-model="currentPrompt.templateName"
            placeholder="如 任务执行提示词"
            clearable
          />
        </el-form-item>
        <el-form-item label="描述">
          <el-input
            v-model="currentPrompt.description"
            placeholder="模板用途说明"
            clearable
            type="textarea"
            :rows="2"
          />
        </el-form-item>
        <el-form-item label="模板内容" required>
          <el-input
            v-model="currentPrompt.templateContent"
            type="textarea"
            :rows="12"
            placeholder="支持变量占位符，使用 {变量名} 格式，如 {taskName}、{userPrompt}"
            class="font-mono text-sm"
          />
          <div class="text-xs text-slate-500 mt-1">
            使用 <code class="bg-slate-100 px-1 rounded">{'{变量名}'}</code> 格式定义变量占位符
          </div>
        </el-form-item>
        <el-form-item label="变量定义">
          <el-input
            v-model="currentPrompt.variablesJson"
            type="textarea"
            :rows="4"
            placeholder="JSON格式，如 [{&quot;name&quot;:&quot;taskName&quot;,&quot;description&quot;:&quot;任务名称&quot;}]"
            class="font-mono text-sm"
          />
          <div class="text-xs text-slate-500 mt-1">定义模板中使用的变量，JSON 数组格式</div>
        </el-form-item>
        <el-form-item v-if="isEditingPrompt" label="状态">
          <el-switch
            v-model="promptEnabled"
            :disabled="isSystemPrompt"
            @change="handlePromptEnabledChange"
          />
          <span class="ml-2 text-sm text-slate-600">
            {{ promptEnabled ? '已启用' : '已禁用' }}
          </span>
        </el-form-item>
        <el-form-item v-if="isEditingPrompt" label="使用统计">
          <el-tag type="info">已使用 {{ currentPrompt.useCount || 0 }} 次</el-tag>
        </el-form-item>
      </el-form>
        </CardContent>
      </Card>
      <template #footer>
        <el-button @click="showPromptDialog = false">取消</el-button>
        <el-button type="primary" :loading="promptDialogLoading" @click="handleSavePrompt">
          保存
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { SettingsApi, PromptTemplateApi, SessionPoolApi } from '@/api/settings'
import type { SessionPoolStatus, CcSessionInfo } from '@/api/settings'
import { Key, Loading, Monitor, FolderOpened, Folder } from '@element-plus/icons-vue'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

const route = useRoute()
const router = useRouter()

// 当前用户 ID（从 localStorage 获取）
const currentUserId = ref<number>(1)
try {
  const userId = localStorage.getItem('userId')
  if (userId) {
    currentUserId.value = parseInt(userId, 10)
  }
} catch {
  // ignore
}

/** 设置页 Tab：env | prompts | session-pool | notification */
const validTabs = ['env', 'prompts', 'session-pool', 'notification'] as const
type SettingsTab = typeof validTabs[number]

const getInitialTab = (): SettingsTab => {
  const tabFromQuery = route.query.tab as string
  return validTabs.includes(tabFromQuery as SettingsTab) ? (tabFromQuery as SettingsTab) : 'env'
}

const settingsTab = ref<SettingsTab>(getInitialTab())

const workspaceRoot = ref('')
const savedValue = ref('')
const pathExists = ref(false)
const saving = ref(false)

// 提示词模板相关
const promptTemplates = ref<any[]>([])
const promptLoading = ref(false)
const showPromptDialog = ref(false)
const isEditingPrompt = ref(false)
const isSystemPrompt = ref(false)
const currentPrompt = ref<any>({
  templateKey: '',
  templateName: '',
  templateContent: '',
  description: '',
  variablesJson: '',
  isEnabled: 1,
  isSystem: 0,
  useCount: 0
})
const promptEnabled = ref(true)
const promptDialogLoading = ref(false)

// Session Pool 追踪相关
const sessionPoolStatus = ref<SessionPoolStatus | null>(null)
const sessionPoolLoading = ref(false)
const ccSessions = ref<CcSessionInfo[]>([])
const ccSessionsLoading = ref(false)

const loadWorkspaceRoot = async () => {
  try {
    const res: any = await SettingsApi.getWorkspaceRoot()
    savedValue.value = res.value || ''
    pathExists.value = res.exists || false
    workspaceRoot.value = savedValue.value
  } catch {
    console.error('加载工程根目录配置失败')
  }
}

const handleSave = async () => {
  saving.value = true
  try {
    const res: any = await SettingsApi.setWorkspaceRoot(workspaceRoot.value)
    if (res.success) {
      ElMessage.success('工程根目录已保存')
      await loadWorkspaceRoot()
    } else {
      ElMessage.error(res.message || '保存失败')
    }
  } catch (err: any) {
    const msg = err?.response?.data?.message || err?.message || '保存失败'
    ElMessage.error(msg)
  } finally {
    saving.value = false
  }
}

// 提示词模板相关方法
const loadPromptTemplates = async () => {
  promptLoading.value = true
  try {
    const res: any = await PromptTemplateApi.getTemplates()
    promptTemplates.value = res || []
  } catch (err: any) {
    console.error('加载提示词模板失败:', err)
    ElMessage.error('加载提示词模板失败')
  } finally {
    promptLoading.value = false
  }
}

const handleAddPrompt = () => {
  currentPrompt.value = {
    templateKey: '',
    templateName: '',
    templateContent: '',
    description: '',
    variablesJson: '',
    isEnabled: 1,
    isSystem: 0,
    useCount: 0
  }
  promptEnabled.value = true
  isEditingPrompt.value = false
  isSystemPrompt.value = false
  showPromptDialog.value = true
}

const handleEditPrompt = (row: any) => {
  currentPrompt.value = { ...row }
  promptEnabled.value = row.isEnabled === 1
  isEditingPrompt.value = true
  isSystemPrompt.value = row.isSystem === 1
  showPromptDialog.value = true
}

const handleSavePrompt = async () => {
  if (!currentPrompt.value.templateKey || !currentPrompt.value.templateName || !currentPrompt.value.templateContent) {
    ElMessage.error('请填写完整信息：模板标识、名称和内容不能为空')
    return
  }
  promptDialogLoading.value = true
  try {
    let res: any
    if (isEditingPrompt.value) {
      res = await PromptTemplateApi.updateTemplate(currentPrompt.value.id, currentPrompt.value)
    } else {
      res = await PromptTemplateApi.createTemplate(currentPrompt.value)
    }
    if (res.success) {
      ElMessage.success(isEditingPrompt.value ? '模板更新成功' : '模板创建成功')
      showPromptDialog.value = false
      await loadPromptTemplates()
    } else {
      ElMessage.error(res.message || '保存失败')
    }
  } catch (err: any) {
    console.error('保存提示词模板失败:', err)
    ElMessage.error('保存失败')
  } finally {
    promptDialogLoading.value = false
  }
}

const handleTogglePrompt = async (row: any) => {
  try {
    const newEnabled = row.isEnabled === 1 ? false : true
    const res: any = await PromptTemplateApi.toggleEnabled(row.id, newEnabled)
    if (res.success) {
      ElMessage.success(newEnabled ? '模板已启用' : '模板已禁用')
      await loadPromptTemplates()
    } else {
      ElMessage.error(res.message || '操作失败')
    }
  } catch (err: any) {
    console.error('切换模板状态失败:', err)
    ElMessage.error('操作失败')
  }
}

const handleDeletePrompt = async (row: any) => {
  try {
    await ElMessageBox.confirm(`确定要删除模板「${row.templateName}」吗？`, '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })
    const res: any = await PromptTemplateApi.deleteTemplate(row.id)
    if (res.success) {
      ElMessage.success('模板删除成功')
      await loadPromptTemplates()
    } else {
      ElMessage.error(res.message || '删除失败')
    }
  } catch (err: any) {
    if (err !== 'cancel') {
      console.error('删除提示词模板失败:', err)
      ElMessage.error('删除失败')
    }
  }
}

const handlePromptEnabledChange = async (val: boolean) => {
  if (!isEditingPrompt.value || isSystemPrompt.value) return
  try {
    const res: any = await PromptTemplateApi.toggleEnabled(currentPrompt.value.id, val)
    if (res.success) {
      ElMessage.success(val ? '模板已启用' : '模板已禁用')
      await loadPromptTemplates()
    } else {
      ElMessage.error(res.message || '操作失败')
      promptEnabled.value = !val
    }
  } catch (err: any) {
    console.error('切换模板状态失败:', err)
    ElMessage.error('操作失败')
    promptEnabled.value = !val
  }
}

// Session Pool 追踪相关方法
const loadSessionPoolStatus = async () => {
  sessionPoolLoading.value = true
  try {
    const res: any = await SessionPoolApi.getStatus()
    if (res.success) {
      sessionPoolStatus.value = res as SessionPoolStatus
    }
  } catch (err: any) {
    console.error('加载 Session Pool 状态失败:', err)
  } finally {
    sessionPoolLoading.value = false
  }
}

const loadCcSessions = async () => {
  ccSessionsLoading.value = true
  try {
    const res: any = await SessionPoolApi.getCcSessions()
    if (res.success) {
      ccSessions.value = res.sessions || []
    }
  } catch (err: any) {
    console.error('加载 CC 终端会话列表失败:', err)
  } finally {
    ccSessionsLoading.value = false
  }
}

const loadAllSessionData = async () => {
  await Promise.all([loadSessionPoolStatus(), loadCcSessions()])
}

const handleCleanupExpired = async () => {
  try {
    const res: any = await SessionPoolApi.cleanup()
    if (res.success) {
      ElMessage.success(res.message || '清理完成')
      await loadAllSessionData()
    } else {
      ElMessage.error(res.message || '清理失败')
    }
  } catch (err: any) {
    ElMessage.error('清理过期会话失败')
  }
}

const handleCloseSession = async (sessionId: string) => {
  try {
    await ElMessageBox.confirm('确定要关闭该会话吗？', '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })
    const res: any = await SessionPoolApi.closeSession(sessionId)
    if (res.success) {
      ElMessage.success('会话已关闭')
      await loadAllSessionData()
    } else {
      ElMessage.error(res.message || '关闭失败')
    }
  } catch (err: any) {
    if (err !== 'cancel') {
      ElMessage.error('关闭会话失败')
    }
  }
}

const formatDateTime = (dt: string | null) => {
  if (!dt) return '-'
  try {
    const d = new Date(dt)
    return d.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
  } catch {
    return dt
  }
}

const truncateId = (id: string) => {
  if (!id) return '-'
  return id.length > 20 ? id.substring(0, 20) + '...' : id
}

// 监听 tab 变化，同步到 URL query 参数
watch(settingsTab, (newTab) => {
  const currentQuery = { ...route.query }
  if (currentQuery.tab !== newTab) {
    router.replace({ query: { ...currentQuery, tab: newTab } })
  }
})

onMounted(() => {
  loadWorkspaceRoot()
  loadPromptTemplates()
  loadAllSessionData()
})
</script>

<style scoped>
.shadcn-form-dialog :deep(.el-dialog__body) {
  padding-top: 8px;
}

.settings-main-card {
  border-radius: 12px;
}

.settings-tabs-list {
  position: sticky;
  top: 0;
  z-index: 10;
  border-radius: 10px;
  padding: 8px;
  @apply bg-white/95 backdrop-blur border border-slate-200;
}
</style>
