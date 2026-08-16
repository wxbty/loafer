<template>
  <div class="plan-panel">
    <!-- Loading -->
    <div v-if="loading" class="flex items-center justify-center py-10 text-slate-400">
      <el-icon class="is-loading mr-2"><Loading /></el-icon>
      加载执行计划…
    </div>

    <!-- Empty state -->
    <el-card v-else-if="!plan" shadow="never" class="!rounded-xl">
      <el-empty description="当前项目还没有执行计划">
        <el-button type="primary" @click="openGenerateDialog">
          <el-icon class="mr-1"><MagicStick /></el-icon>
          创建执行计划
        </el-button>
      </el-empty>
    </el-card>

    <!-- Plan exists -->
    <el-card v-else shadow="never" class="!rounded-xl">
      <template #header>
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <el-icon class="text-indigo-500"><Document /></el-icon>
            <span class="text-base font-semibold text-slate-800">执行计划</span>
            <el-tag :type="statusTagType(plan.status)" size="small" effect="light">
              {{ statusLabel(plan.status) }}
            </el-tag>
          </div>
          <div class="flex items-center gap-2">
            <span class="text-xs text-slate-400">Plan #{{ plan.id || '-' }}</span>
            <el-button link @click="loadPlan">
              <el-icon><Refresh /></el-icon>
            </el-button>
          </div>
        </div>
      </template>

      <!-- Plan content (Markdown rendered) -->
      <div class="plan-markdown max-h-[500px] overflow-auto rounded-lg bg-white p-6" v-html="renderedPlanContent"></div>

      <!-- Streaming output -->
      <div v-if="streamVisible" class="mt-4">
        <div class="mb-2 flex items-center justify-between">
          <span class="text-sm font-medium text-slate-600">
            <el-icon v-if="streaming" class="is-loading mr-1 align-text-bottom"><Loading /></el-icon>
            {{ currentStreamLabel }}
          </span>
          <el-button v-if="streaming" link type="danger" @click="abortStream">停止</el-button>
        </div>
        <div
          ref="streamBoxRef"
          class="stream-box h-56 overflow-auto rounded-lg bg-gray-900 p-3 font-mono text-xs leading-relaxed text-gray-100"
        >
          <pre class="whitespace-pre-wrap break-words">{{ streamOutput || '等待输出…' }}</pre>
        </div>
      </div>

      <!-- Actions by status -->
      <div class="mt-4 flex flex-wrap items-center justify-end gap-2">
        <!-- draft: 优化 + 确认计划 -->
        <template v-if="plan.status === 'draft'">
          <el-button :disabled="streaming || confirming" @click="openRefineDialog">
            <el-icon class="mr-1"><EditPen /></el-icon>
            优化
          </el-button>
          <el-button type="success" :loading="confirming" :disabled="streaming" @click="handleConfirm">
            <el-icon class="mr-1"><Check /></el-icon>
            确认计划
          </el-button>
        </template>

        <!-- confirmed: 拆解为模块任务 -->
        <template v-else-if="plan.status === 'confirmed'">
          <el-button type="primary" :loading="decomposing" :disabled="streaming" @click="handleDecompose">
            <el-icon class="mr-1"><SetUp /></el-icon>
            拆解为模块任务
          </el-button>
        </template>

        <!-- decomposed: 查看模块任务 -->
        <template v-else-if="plan.status === 'decomposed'">
          <el-button type="primary" @click="emit('decomposed')">
            <el-icon class="mr-1"><View /></el-icon>
            查看模块任务
          </el-button>
        </template>

        <!-- executing: progress indicator -->
        <template v-else-if="plan.status === 'executing'">
          <div class="flex w-full items-center gap-3">
            <el-progress :percentage="100" :indeterminate="true" :show-text="false" :stroke-width="8" class="flex-1" />
            <span class="text-sm text-slate-500">任务执行中…</span>
          </div>
        </template>

        <!-- completed: 查看模块任务 -->
        <template v-else-if="plan.status === 'completed'">
          <el-tag type="success" effect="plain">全部任务已完成</el-tag>
          <el-button type="primary" @click="emit('decomposed')">
            <el-icon class="mr-1"><View /></el-icon>
            查看模块任务
          </el-button>
        </template>
      </div>
    </el-card>

    <!-- Generate dialog -->
    <el-dialog
      v-model="generateDialogVisible"
      title="创建执行计划"
      width="560px"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-input
        v-model="generateRequirement"
        type="textarea"
        :rows="6"
        resize="vertical"
        placeholder="请用自然语言描述你的需求，AI 将据此生成执行计划……"
      />
      <template #footer>
        <div class="flex justify-end gap-2">
          <el-button @click="generateDialogVisible = false">取消</el-button>
          <el-button
            type="primary"
            :loading="generating"
            :disabled="!generateRequirement.trim()"
            @click="handleGenerate"
          >
            生成
          </el-button>
        </div>
      </template>
    </el-dialog>

    <!-- Refine dialog -->
    <el-dialog
      v-model="refineDialogVisible"
      title="优化执行计划"
      width="560px"
      :close-on-click-modal="false"
      destroy-on-close
    >
      <el-input
        v-model="refineFeedback"
        type="textarea"
        :rows="5"
        resize="vertical"
        placeholder="告诉 AI 如何调整这份计划，例如：增加用户权限管理、细化数据库设计……"
      />
      <template #footer>
        <div class="flex justify-end gap-2">
          <el-button @click="refineDialogVisible = false">取消</el-button>
          <el-button
            type="primary"
            :loading="refining"
            :disabled="!refineFeedback.trim()"
            @click="handleRefine"
          >
            提交优化
          </el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  MagicStick,
  Document,
  EditPen,
  Check,
  SetUp,
  View,
  Refresh,
  Loading
} from '@element-plus/icons-vue'
import { PlanApi } from '@/api/plan'
import type { SseCallbacks } from '@/utils/sseStream'
import MarkdownIt from 'markdown-it'
import highlightjs from 'markdown-it-highlightjs'
import hljs from 'highlight.js'
import 'highlight.js/styles/github.css'

// Markdown 渲染器（与 ProjectDetail.vue 文件预览保持一致）
const md = MarkdownIt({
  html: true,
  linkify: true,
  typographer: true,
  breaks: true,
}).use(highlightjs, { hljs })

defineOptions({ name: 'PlanPanel' })

const props = defineProps<{
  projectId: number
}>()

const emit = defineEmits(['decomposed'])

interface PlanInfo {
  id: number
  planContent: string
  status: string
}

const loading = ref(false)
const plan = ref<PlanInfo | null>(null)

/** 将计划内容渲染为 Markdown HTML */
const renderedPlanContent = computed(() => {
  if (!plan.value?.planContent) {
    return '<p style="color:#94a3b8;">（计划内容为空）</p>'
  }
  return md.render(plan.value.planContent)
})

const generating = ref(false)
const refining = ref(false)
const confirming = ref(false)
const decomposing = ref(false)

const streamOutput = ref('')
const streamBoxRef = ref<HTMLElement | null>(null)
let currentController: AbortController | null = null

const generateDialogVisible = ref(false)
const generateRequirement = ref('')

const refineDialogVisible = ref(false)
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

/** 归一化后端返回的计划对象为 PlanInfo */
const normalizePlan = (raw: any): PlanInfo | null => {
  if (!raw) return null
  const inner = raw?.data ?? raw
  if (!inner || (!inner.id && !inner.planId)) return null
  return {
    id: Number(inner.id ?? inner.planId ?? 0),
    planContent: String(inner.planContent ?? inner.content ?? ''),
    status: String(inner.status ?? 'draft')
  }
}

/** 解析 SSE done 帧的 payload 为 PlanInfo */
const parsePlanPayload = (payload: string): PlanInfo | null => {
  if (!payload) return null
  try {
    const obj = JSON.parse(payload)
    return normalizePlan(obj)
  } catch {
    return null
  }
}

const loadPlan = async () => {
  loading.value = true
  try {
    const res: any = await PlanApi.getPlan(props.projectId)
    plan.value = normalizePlan(res)
  } catch {
    plan.value = null
  } finally {
    loading.value = false
  }
}

const openGenerateDialog = () => {
  generateRequirement.value = ''
  generateDialogVisible.value = true
}

const handleGenerate = () => {
  if (!generateRequirement.value.trim()) return
  generateDialogVisible.value = false
  generating.value = true
  streamOutput.value = ''
  const callbacks = buildCallbacks({
    onDone: (payload) => {
      generating.value = false
      currentController = null
      const parsed = parsePlanPayload(payload)
      if (parsed) {
        plan.value = parsed
      } else {
        // 回退：重新拉取一次计划
        loadPlan()
      }
      ElMessage.success('执行计划生成完成')
    },
    onError: (msg) => {
      generating.value = false
      currentController = null
      ElMessage.error(msg || '生成执行计划失败')
    }
  })
  currentController = PlanApi.generatePlan(props.projectId, generateRequirement.value, callbacks)
}

const openRefineDialog = () => {
  if (!plan.value || !plan.value.id) {
    ElMessage.warning('计划 ID 缺失，无法优化')
    return
  }
  refineFeedback.value = ''
  refineDialogVisible.value = true
}

const handleRefine = () => {
  if (!plan.value || !plan.value.id || !refineFeedback.value.trim()) return
  refineDialogVisible.value = false
  refining.value = true
  streamOutput.value = ''
  const callbacks = buildCallbacks({
    onDone: (payload) => {
      refining.value = false
      currentController = null
      const parsed = parsePlanPayload(payload)
      if (parsed && parsed.id) {
        plan.value = parsed
      } else if (plan.value) {
        plan.value = { ...plan.value, planContent: parsed?.planContent || plan.value.planContent }
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
    ElMessage.success('计划已确认')
    await loadPlan()
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
    onDone: async () => {
      decomposing.value = false
      currentController = null
      ElMessage.success('已拆解为模块任务')
      await loadPlan()
      emit('decomposed')
    },
    onError: (msg) => {
      decomposing.value = false
      currentController = null
      ElMessage.error(msg || '拆解失败')
    }
  })
  currentController = PlanApi.decomposePlan(plan.value.id, callbacks)
}

onMounted(() => {
  loadPlan()
})
</script>

<style scoped>
.stream-box pre {
  margin: 0;
}
</style>

<!-- Global styles for v-html rendered Markdown (scoped styles cannot penetrate v-html) -->
<style>
/* ========================
   Plan Markdown — Typora-like typography
   ======================== */
.plan-markdown {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Roboto', 'Oxygen', 'Ubuntu', 'Cantarell', 'Helvetica Neue', Arial, 'PingFang SC', 'Noto Sans', 'Microsoft YaHei', sans-serif;
  font-size: 15px;
  line-height: 1.8;
  color: #2c3e50;
  word-wrap: break-word;
  overflow-wrap: break-word;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}

/* ---- Headings ---- */
.plan-markdown h1,
.plan-markdown h2,
.plan-markdown h3,
.plan-markdown h4,
.plan-markdown h5,
.plan-markdown h6 {
  margin-top: 28px;
  margin-bottom: 14px;
  font-weight: 600;
  line-height: 1.35;
  color: #1a1a1a;
}
.plan-markdown h1 {
  font-size: 28px;
  font-weight: 700;
  padding-bottom: 12px;
  margin-top: 0;
  border-bottom: 2px solid #eaecef;
}
.plan-markdown h2 {
  font-size: 23px;
  padding-bottom: 10px;
  border-bottom: 1px solid #eaecef;
}
.plan-markdown h3 {
  font-size: 19px;
}
.plan-markdown h4 {
  font-size: 16px;
}
.plan-markdown h5 {
  font-size: 15px;
  color: #444;
}
.plan-markdown h6 {
  font-size: 14px;
  color: #666;
}

/* ---- Paragraphs ---- */
.plan-markdown p {
  margin: 12px 0;
  letter-spacing: 0.01em;
}

/* ---- Links ---- */
.plan-markdown a {
  color: #0366d6;
  text-decoration: none;
  border-bottom: 1px solid transparent;
  transition: border-color 0.15s;
}
.plan-markdown a:hover {
  border-bottom-color: #0366d6;
}

/* ---- Inline code ---- */
.plan-markdown code {
  background: rgba(175, 184, 193, 0.12);
  padding: 2px 6px;
  border-radius: 4px;
  font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', 'Fira Code', 'Cascadia Code', monospace;
  font-size: 0.88em;
  color: #c7254e;
}

/* ---- Code blocks ---- */
.plan-markdown pre {
  position: relative;
  background: #f6f8fa;
  padding: 16px 18px;
  border-radius: 8px;
  overflow-x: auto;
  margin: 16px 0;
  border: 1px solid #e8e8e8;
  line-height: 1.65;
}
.plan-markdown pre code {
  background: transparent;
  padding: 0;
  border-radius: 0;
  color: inherit;
  font-size: 13px;
  font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', 'Fira Code', monospace;
}

/* ---- Blockquote ---- */
.plan-markdown blockquote {
  border-left: 4px solid #0366d6;
  padding: 8px 16px;
  margin: 16px 0;
  color: #57606a;
  background: rgba(3, 102, 214, 0.04);
  border-radius: 0 6px 6px 0;
}
.plan-markdown blockquote p:last-child {
  margin-bottom: 0;
}

/* ---- Tables ---- */
.plan-markdown table {
  width: 100%;
  border-collapse: collapse;
  margin: 16px 0;
  font-size: 14px;
}
.plan-markdown th,
.plan-markdown td {
  padding: 8px 12px;
  border: 1px solid #d0d7de;
  text-align: left;
}
.plan-markdown th {
  background: #f6f8fa;
  font-weight: 600;
  color: #24292e;
}
.plan-markdown tr:nth-child(even) td {
  background: #fafbfc;
}

/* ---- Lists ---- */
.plan-markdown ul,
.plan-markdown ol {
  margin: 12px 0;
  padding-left: 26px;
}
.plan-markdown li {
  margin: 4px 0;
  line-height: 1.75;
}
.plan-markdown li > ul,
.plan-markdown li > ol {
  margin: 4px 0;
}

/* ---- Horizontal rule ---- */
.plan-markdown hr {
  border: none;
  height: 2px;
  background: #eaecef;
  margin: 28px 0;
}

/* ---- Images ---- */
.plan-markdown img {
  max-width: 100%;
  height: auto;
  border-radius: 6px;
  margin: 12px 0;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.08);
}

/* ---- Strikethrough ---- */
.plan-markdown del {
  color: #999;
}
</style>
