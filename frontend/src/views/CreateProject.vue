<template>
  <div class="create-project-page min-h-screen w-full flex flex-col items-center bg-slate-50">
    <!-- Hero -->
    <div class="w-full max-w-3xl px-6 pt-16 pb-8 text-center">
      <h1 class="text-4xl font-bold tracking-tight text-slate-900">AI 自动开发平台</h1>
      <p class="mt-3 text-base text-slate-500">描述你的需求，AI 自动生成完整网站</p>
    </div>

    <!-- Input card -->
    <div class="w-full max-w-3xl px-6">
      <el-card shadow="always" class="!rounded-2xl">
        <div class="flex flex-col gap-4">
          <div>
            <label class="mb-1 block text-sm font-medium text-slate-700">需求描述</label>
            <el-input
              v-model="requirement"
              type="textarea"
              :rows="6"
              resize="vertical"
              placeholder="例如：帮我做一个待办事项网站，支持注册登录、创建任务、设置截止日期、分类标签、日历视图……"
            />
          </div>
          <div>
            <label class="mb-1 block text-sm font-medium text-slate-700">项目名称</label>
            <el-input
              v-model="projectName"
              placeholder="给项目起个名字（可选，留空将根据需求自动生成）"
              clearable
            />
          </div>
          <div class="flex items-center justify-between">
            <span class="text-xs text-slate-400">
              {{ generating ? '正在生成执行计划，请稍候…' : '生成的计划可继续优化、确认并拆解为模块任务' }}
            </span>
            <el-button
              type="primary"
              size="large"
              :loading="generating"
              :disabled="!requirement.trim() || streaming"
              @click="handleGenerate"
            >
              <el-icon class="mr-1"><MagicStick /></el-icon>
              生成执行计划
            </el-button>
          </div>
        </div>
      </el-card>
    </div>

    <!-- Streaming output -->
    <div v-if="streamVisible" class="w-full max-w-3xl px-6 mt-6">
      <div class="mb-2 flex items-center justify-between">
        <span class="text-sm font-medium text-slate-600">
          <el-icon v-if="streaming" class="is-loading mr-1 align-text-bottom"><Loading /></el-icon>
          {{ currentStreamLabel }}
        </span>
        <el-button v-if="streaming" link type="danger" @click="abortStream">停止</el-button>
      </div>
      <div
        ref="streamBoxRef"
        class="stream-box h-72 overflow-auto rounded-xl bg-gray-900 p-4 font-mono text-sm leading-relaxed text-gray-100"
      >
        <pre class="whitespace-pre-wrap break-words">{{ streamOutput || '等待输出…' }}</pre>
      </div>
    </div>

    <!-- Plan card -->
    <div v-if="plan" class="w-full max-w-3xl px-6 mt-6 pb-16">
      <el-card shadow="always" class="!rounded-2xl">
        <template #header>
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <el-icon class="text-indigo-500"><Document /></el-icon>
              <span class="text-base font-semibold text-slate-800">执行计划</span>
              <el-tag v-if="plan.status" :type="statusTagType(plan.status)" size="small" effect="light">
                {{ statusLabel(plan.status) }}
              </el-tag>
            </div>
            <span class="text-xs text-slate-400">Plan #{{ plan.id || '-' }}</span>
          </div>
        </template>

        <pre class="max-h-[480px] overflow-auto whitespace-pre-wrap break-words rounded-lg bg-slate-50 p-4 text-sm leading-relaxed text-slate-800">{{ plan.planContent }}</pre>

        <!-- Inline refine area -->
        <div v-if="refineMode" class="mt-4 rounded-lg border border-indigo-200 bg-indigo-50/40 p-3">
          <label class="mb-1 block text-sm font-medium text-slate-700">优化反馈</label>
          <el-input
            v-model="refineFeedback"
            type="textarea"
            :rows="3"
            placeholder="告诉 AI 如何调整这份计划，例如：增加用户权限管理、去掉日历视图、细化数据库设计……"
          />
          <div class="mt-2 flex justify-end gap-2">
            <el-button @click="cancelRefine">取消</el-button>
            <el-button
              type="primary"
              :loading="refining"
              :disabled="!refineFeedback.trim()"
              @click="handleRefine"
            >
              提交优化
            </el-button>
          </div>
        </div>

        <!-- Actions -->
        <div class="mt-4 flex flex-wrap justify-end gap-2">
          <el-button :disabled="streaming || confirming || decomposing" @click="openRefine">
            <el-icon class="mr-1"><EditPen /></el-icon>
            优化计划
          </el-button>
          <el-button
            type="success"
            :loading="confirming"
            :disabled="streaming || plan.status === 'confirmed'"
            @click="handleConfirm"
          >
            <el-icon class="mr-1"><Check /></el-icon>
            确认计划
          </el-button>
          <el-button
            type="primary"
            :loading="decomposing"
            :disabled="streaming"
            @click="handleDecompose"
          >
            <el-icon class="mr-1"><SetUp /></el-icon>
            拆解为模块任务
          </el-button>
        </div>
      </el-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { MagicStick, Document, EditPen, Check, SetUp, Loading } from '@element-plus/icons-vue'
import { ProjectApi } from '@/api/project'
import { PlanApi } from '@/api/plan'
import type { SseCallbacks } from '@/utils/sseStream'

defineOptions({ name: 'CreateProject' })

const router = useRouter()

interface PlanInfo {
  id: number
  planContent: string
  status: string
}

const requirement = ref('')
const projectName = ref('')

const generating = ref(false)
const refining = ref(false)
const confirming = ref(false)
const decomposing = ref(false)

const streamOutput = ref('')
const streamBoxRef = ref<HTMLElement | null>(null)
let currentController: AbortController | null = null

const plan = ref<PlanInfo | null>(null)
const projectId = ref<number | null>(null)

const refineMode = ref(false)
const refineFeedback = ref('')

const streaming = computed(() => generating.value || refining.value || decomposing.value)
const streamVisible = computed(() => streaming.value || streamOutput.value.length > 0)
const currentStreamLabel = computed(() => {
  if (generating.value) return '生成执行计划中…'
  if (refining.value) return '优化计划中…'
  if (decomposing.value) return '拆解模块任务中…'
  return '输出'
})

const statusTagType = (status: string): 'info' | 'success' | 'warning' | 'primary' | 'danger' | '' => {
  switch (status) {
    case 'draft':
      return 'info'
    case 'confirmed':
      return 'success'
    case 'decomposed':
      return 'warning'
    case 'executing':
      return 'primary'
    case 'completed':
      return 'success'
    default:
      return ''
  }
}

const statusLabel = (status: string) => {
  const map: Record<string, string> = {
    draft: '草稿',
    confirmed: '已确认',
    decomposed: '已拆解',
    executing: '执行中',
    completed: '已完成'
  }
  return map[status] || status
}

const scrollToBottom = () => {
  nextTick(() => {
    const el = streamBoxRef.value
    if (el) el.scrollTop = el.scrollHeight
  })
}

const abortStream = () => {
  currentController?.abort()
  currentController = null
  generating.value = false
  refining.value = false
  decomposing.value = false
}

const buildCallbacks = (opts: {
  onDone: (payload: string) => void
  onError?: (msg: string) => void
}): SseCallbacks => {
  return {
    onOutput: (payload: string) => {
      streamOutput.value += payload
      scrollToBottom()
    },
    onDone: opts.onDone,
    onError: (msg: string) => {
      if (opts.onError) opts.onError(msg)
      else ElMessage.error(msg || '操作失败')
    }
  }
}

/** 解析 SSE done 帧的 payload 为 PlanInfo，兼容裸 JSON / { data: {...} } / 纯文本 */
const parsePlanPayload = (payload: string): PlanInfo => {
  if (!payload) return { id: 0, planContent: '', status: 'draft' }
  try {
    const obj = JSON.parse(payload)
    const inner = obj?.data ?? obj
    return {
      id: Number(inner?.id ?? inner?.planId ?? 0),
      planContent: String(inner?.planContent ?? inner?.content ?? payload),
      status: String(inner?.status ?? 'draft')
    }
  } catch {
    return { id: 0, planContent: payload, status: 'draft' }
  }
}

const handleGenerate = async () => {
  if (!requirement.value.trim()) {
    ElMessage.warning('请先描述你的需求')
    return
  }

  // reset
  plan.value = null
  projectId.value = null
  streamOutput.value = ''
  refineMode.value = false
  refineFeedback.value = ''

  generating.value = true
  try {
    // 1. create project first
    const name =
      projectName.value.trim() ||
      Array.from(requirement.value.trim()).slice(0, 20).join('') ||
      `项目-${Date.now()}`
    const createRes: any = await ProjectApi.createProject({
      name,
      description: requirement.value
      // devLanguage 和 workDir 由后端自动设置，前端无需传递
    })
    if (!createRes || createRes.success === false) {
      generating.value = false
      ElMessage.error(createRes?.message || '创建项目失败')
      return
    }
    const newProjectId =
      createRes?.id ?? createRes?.data?.id ?? createRes?.data?.project?.id
    if (!newProjectId) {
      generating.value = false
      ElMessage.error('创建项目失败：未获取到项目 ID')
      return
    }
    projectId.value = newProjectId

    // 2. generate plan (SSE)
    const callbacks = buildCallbacks({
      onDone: (payload) => {
        generating.value = false
        currentController = null
        const parsed = parsePlanPayload(payload)
        if (parsed.id) {
          plan.value = parsed
        } else if (parsed.planContent) {
          // 后端未回传 id 时，保留内容并尝试用项目 ID 上下文（仍允许后续操作）
          plan.value = parsed
        }
        ElMessage.success('执行计划生成完成')
      },
      onError: (msg) => {
        generating.value = false
        currentController = null
        ElMessage.error(msg || '生成执行计划失败')
      }
    })
    currentController = PlanApi.generatePlan(newProjectId, requirement.value, callbacks)
  } catch (e: any) {
    generating.value = false
    ElMessage.error(e?.message || '生成执行计划失败')
  }
}

const openRefine = () => {
  if (!plan.value) return
  refineMode.value = true
  refineFeedback.value = ''
}

const cancelRefine = () => {
  refineMode.value = false
  refineFeedback.value = ''
}

const handleRefine = () => {
  if (!plan.value || !refineFeedback.value.trim()) return
  if (!plan.value.id) {
    ElMessage.warning('计划 ID 缺失，无法优化')
    return
  }
  refineMode.value = false
  refining.value = true
  streamOutput.value = ''
  const callbacks = buildCallbacks({
    onDone: (payload) => {
      refining.value = false
      currentController = null
      const parsed = parsePlanPayload(payload)
      if (parsed.id) {
        plan.value = parsed
      } else if (plan.value) {
        plan.value = { ...plan.value, planContent: parsed.planContent || plan.value.planContent }
      }
      ElMessage.success('计划已优化')
    },
    onError: (msg) => {
      refining.value = false
      currentController = null
      ElMessage.error(msg || '优化计划失败')
    }
  })
  currentController = PlanApi.refinePlan(plan.value.id, refineFeedback.value, callbacks)
}

const handleConfirm = async () => {
  if (!plan.value || !plan.value.id) {
    ElMessage.warning('计划 ID 缺失，无法确认')
    return
  }
  confirming.value = true
  try {
    await PlanApi.confirmPlan(plan.value.id)
    plan.value = { ...plan.value, status: 'confirmed' }
    ElMessage.success('计划已确认')
  } catch (e: any) {
    ElMessage.error(e?.message || '确认计划失败')
  } finally {
    confirming.value = false
  }
}

const handleDecompose = () => {
  if (!plan.value || !plan.value.id) {
    ElMessage.warning('计划 ID 缺失，无法拆解')
    return
  }
  decomposing.value = true
  streamOutput.value = ''
  const callbacks = buildCallbacks({
    onDone: () => {
      decomposing.value = false
      currentController = null
      ElMessage.success('已拆解为模块任务，即将跳转到项目详情…')
      const id = projectId.value
      if (id) {
        setTimeout(() => router.push(`/projects/${id}`), 600)
      }
    },
    onError: (msg) => {
      decomposing.value = false
      currentController = null
      ElMessage.error(msg || '拆解失败')
    }
  })
  currentController = PlanApi.decomposePlan(plan.value.id, callbacks)
}
</script>

<style scoped>
.stream-box pre {
  margin: 0;
}
</style>
