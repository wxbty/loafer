<template>
  <div class="flex flex-col gap-4">
    <!-- 测试操作 -->
    <el-card shadow="hover" class="section-card">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-semibold">测试操作</span>
          <el-tag v-if="running" type="warning" size="small">测试运行中</el-tag>
        </div>
      </template>

      <div class="flex flex-wrap items-center gap-3">
        <el-select v-model="testType" placeholder="测试类型" style="width: 160px">
          <el-option label="Playwright (E2E)" value="playwright" />
          <el-option label="单元测试 (unit)" value="unit" />
          <el-option label="集成测试 (integration)" value="integration" />
        </el-select>

        <el-button type="primary" :loading="running" :disabled="running" @click="handleRunTest">
          <el-icon class="mr-1"><VideoPlay /></el-icon>
          运行测试
        </el-button>

        <el-button :disabled="running" @click="openSpecDialog">
          <el-icon class="mr-1"><MagicStick /></el-icon>
          生成测试配置
        </el-button>

        <el-button :loading="loadingHistory" @click="loadHistory">
          <el-icon class="mr-1"><Refresh /></el-icon>
          刷新历史
        </el-button>
      </div>

      <!-- 实时通过/失败计数 -->
      <div v-if="running || testOutput" class="flex items-center gap-3 mt-3">
        <el-tag type="success" size="small">通过：{{ liveCounts.pass }}</el-tag>
        <el-tag type="danger" size="small">失败：{{ liveCounts.fail }}</el-tag>
      </div>
    </el-card>

    <!-- 测试流式输出 -->
    <el-card v-if="running || testOutput" shadow="hover" class="section-card">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-semibold">
            测试输出
            <el-icon v-if="running" class="is-loading ml-1"><Loading /></el-icon>
          </span>
          <el-button link size="small" @click="testOutput = ''">清空</el-button>
        </div>
      </template>
      <div ref="outputRef" class="stream-output">{{ testOutput || '等待输出...' }}</div>
    </el-card>

    <!-- 测试历史 -->
    <el-card shadow="hover" class="section-card">
      <template #header>
        <div class="flex items-center justify-between">
          <span class="font-semibold">测试历史</span>
          <span class="text-xs text-slate-400">共 {{ testRuns.length }} 条记录</span>
        </div>
      </template>

      <el-table
        v-loading="loadingHistory"
        :data="testRuns"
        size="small"
        border
        stripe
        empty-text="暂无测试记录"
      >
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column prop="testType" label="类型" width="130">
          <template #default="{ row }">
            <el-tag size="small" type="info">{{ row.testType || '-' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="110">
          <template #default="{ row }">
            <el-tag :type="testStatusTag(row.status)" size="small">
              {{ testStatusLabel(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="passCount" label="通过" width="80">
          <template #default="{ row }">
            <span :class="row.passCount > 0 ? 'text-green-600 font-semibold' : 'text-slate-400'">
              {{ row.passCount }}
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="failCount" label="失败" width="80">
          <template #default="{ row }">
            <span :class="row.failCount > 0 ? 'text-red-600 font-semibold' : 'text-slate-400'">
              {{ row.failCount }}
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="duration" label="耗时" width="100">
          <template #default="{ row }">{{ formatDuration(row.duration) }}</template>
        </el-table-column>
        <el-table-column prop="startedAt" label="开始时间" width="170">
          <template #default="{ row }">{{ formatTime(row.startedAt) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="100" fixed="right">
          <template #default="{ row }">
            <el-button type="primary" link size="small" @click="viewDetail(row)">查看详情</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 生成测试配置对话框 -->
    <el-dialog v-model="specDialogVisible" title="生成测试配置" width="520px" :close-on-click-modal="false">
      <el-form ref="specFormRef" :model="specForm" :rules="specFormRules" label-width="80px">
        <el-form-item label="目标 URL" prop="url">
          <el-input v-model="specForm.url" placeholder="如：http://localhost:40410" />
        </el-form-item>
        <el-form-item label="描述" prop="description">
          <el-input
            v-model="specForm.description"
            type="textarea"
            :rows="3"
            placeholder="测试配置的用途描述（可选）"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="specDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="generatingSpec" @click="submitGenerateSpec">生成</el-button>
      </template>
    </el-dialog>

    <!-- 测试详情对话框 -->
    <el-dialog v-model="detailDialogVisible" title="测试运行详情" width="80%" top="6vh" :close-on-click-modal="true">
      <template v-if="detailRun">
        <el-descriptions :column="3" border size="small" class="mb-3">
          <el-descriptions-item label="运行 ID">{{ detailRun.id }}</el-descriptions-item>
          <el-descriptions-item label="类型">{{ detailRun.testType || '-' }}</el-descriptions-item>
          <el-descriptions-item label="状态">
            <el-tag :type="testStatusTag(detailRun.status)" size="small">
              {{ testStatusLabel(detailRun.status) }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="通过">
            <span class="text-green-600 font-semibold">{{ detailRun.passCount }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="失败">
            <span class="text-red-600 font-semibold">{{ detailRun.failCount }}</span>
          </el-descriptions-item>
          <el-descriptions-item label="耗时">{{ formatDuration(detailRun.duration) }}</el-descriptions-item>
          <el-descriptions-item label="开始时间">{{ formatTime(detailRun.startedAt) }}</el-descriptions-item>
          <el-descriptions-item label="完成时间" :span="2">{{ formatTime(detailRun.completedAt) }}</el-descriptions-item>
        </el-descriptions>
      </template>
      <pre class="stream-output" style="max-height: 50vh">{{ detailOutput || '暂无输出' }}</pre>
      <template #footer>
        <el-button @click="detailDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import { VideoPlay, MagicStick, Refresh, Loading } from '@element-plus/icons-vue'
import { TestApi } from '@/api/infra'
import type { SseCallbacks } from '@/utils/sseStream'

// ---- 类型定义（与后端 model.TestRun 对齐） ----
interface TestRun {
  id: number
  projectId: number
  moduleId?: number | null
  taskId?: number | null
  testType: string
  status: string
  output: string
  passCount: number
  failCount: number
  duration: number
  startedAt?: string | null
  completedAt?: string | null
  createdAt?: string
  updatedAt?: string
}

interface Props {
  projectId: number
}

const props = defineProps<Props>()

// ---- 响应式状态 ----
const testType = ref<string>('playwright')
const testRuns = ref<TestRun[]>([])
const testOutput = ref<string>('')
const outputRef = ref<HTMLElement | null>(null)

const running = ref(false)
const loadingHistory = ref(false)
const generatingSpec = ref(false)

// 实时通过/失败计数（从流式输出解析）
const liveCounts = reactive({ pass: 0, fail: 0 })

// 生成测试配置对话框
const specDialogVisible = ref(false)
const specFormRef = ref<FormInstance>()
const specForm = reactive({
  url: '',
  description: '',
})
const specFormRules: FormRules = {
  url: [{ required: true, message: '请输入目标 URL', trigger: 'blur' }],
}

// 测试详情对话框
const detailDialogVisible = ref(false)
const detailRun = ref<TestRun | null>(null)
const detailOutput = ref<string>('')

// SSE 控制器
let testController: AbortController | null = null

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

const formatDuration = (seconds?: number) => {
  if (!seconds || seconds <= 0) return '-'
  if (seconds < 60) return `${seconds}s`
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return `${m}m ${s}s`
}

const testStatusLabel = (status: string) => {
  const map: Record<string, string> = {
    pending: '等待中',
    running: '运行中',
    passed: '通过',
    failed: '失败',
  }
  return map[status] || status
}

const testStatusTag = (status: string): 'success' | 'danger' | 'primary' | 'info' => {
  const map: Record<string, 'success' | 'danger' | 'primary' | 'info'> = {
    passed: 'success',
    failed: 'danger',
    running: 'primary',
    pending: 'info',
  }
  return map[status] || 'info'
}

/**
 * 从 Playwright 输出文本中解析通过/失败数量。
 * 取最后一组匹配（汇总行通常位于输出末尾），与后端 parseTestResults 行为一致。
 */
const parseTestCounts = (output: string): { pass: number; fail: number } => {
  let pass = 0
  let fail = 0
  const passMatches = [...output.matchAll(/(\d+)\s+passed/g)]
  if (passMatches.length) {
    pass = parseInt(passMatches[passMatches.length - 1][1]) || 0
  }
  const failMatches = [...output.matchAll(/(\d+)\s+failed/g)]
  if (failMatches.length) {
    fail = parseInt(failMatches[failMatches.length - 1][1]) || 0
  }
  return { pass, fail }
}

const updateLiveCounts = () => {
  const counts = parseTestCounts(testOutput.value)
  liveCounts.pass = counts.pass
  liveCounts.fail = counts.fail
}

const scrollToBottom = (el: HTMLElement | null) => {
  if (el) {
    el.scrollTop = el.scrollHeight
  }
}

// ---- 数据加载 ----
const loadHistory = async () => {
  loadingHistory.value = true
  try {
    const res: any = await TestApi.listTests(props.projectId)
    if (res?.success) {
      testRuns.value = (res.data as TestRun[]) || []
    } else {
      testRuns.value = []
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '加载测试历史失败')
  } finally {
    loadingHistory.value = false
  }
}

// ---- 运行测试（SSE 流式） ----
const handleRunTest = () => {
  if (running.value) return
  running.value = true
  testOutput.value = ''
  liveCounts.pass = 0
  liveCounts.fail = 0

  const callbacks: SseCallbacks = {
    onOutput: (payload: string) => {
      testOutput.value += payload
      updateLiveCounts()
      nextTick(() => scrollToBottom(outputRef.value))
    },
    onDone: (payload: string) => {
      running.value = false
      // done 帧的 payload 是 TestRun 的 JSON 序列化字符串
      let result: TestRun | null = null
      try {
        result = payload ? (JSON.parse(payload) as TestRun) : null
      } catch {
        result = null
      }
      if (result) {
        // 用后端返回的权威计数覆盖实时解析值
        liveCounts.pass = result.passCount ?? liveCounts.pass
        liveCounts.fail = result.failCount ?? liveCounts.fail
        if (result.output) {
          testOutput.value = result.output
        }
        ElMessage.success(
          `测试完成：${result.passCount} 通过，${result.failCount} 失败`
        )
      } else {
        updateLiveCounts()
        ElMessage.success('测试完成')
      }
      testController = null
      // 刷新历史列表
      loadHistory()
    },
    onError: (msg: string) => {
      running.value = false
      testOutput.value += `\n[错误] ${msg}\n`
      updateLiveCounts()
      ElMessage.error(msg || '测试运行失败')
      testController = null
      loadHistory()
    },
  }

  testController = TestApi.runTest(
    props.projectId,
    { testType: testType.value },
    callbacks
  )
}

// ---- 生成测试配置 ----
const openSpecDialog = () => {
  specForm.url = ''
  specForm.description = ''
  specDialogVisible.value = true
}

const submitGenerateSpec = async () => {
  if (!specFormRef.value) return
  try {
    await specFormRef.value.validate()
  } catch {
    return
  }
  generatingSpec.value = true
  try {
    const res: any = await TestApi.generateSpec(props.projectId, {
      url: specForm.url,
      description: specForm.description,
    })
    if (res?.success) {
      ElMessage.success('测试配置已生成')
      if (res.data?.path) {
        testOutput.value = `测试配置已生成：${res.data.path}\n`
      }
      specDialogVisible.value = false
    } else {
      ElMessage.error(res?.message || '生成测试配置失败')
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '生成测试配置失败')
  } finally {
    generatingSpec.value = false
  }
}

// ---- 查看详情 ----
const viewDetail = async (row: TestRun) => {
  detailRun.value = row
  detailOutput.value = row.output || ''
  detailDialogVisible.value = true
  // 若行内无完整输出，则单独拉取
  if (!row.output && row.id) {
    try {
      const res: any = await TestApi.getTestRun(row.id)
      if (res?.success && res.data) {
        const full = res.data as TestRun
        detailRun.value = full
        detailOutput.value = full.output || ''
      }
    } catch (e: any) {
      ElMessage.error(e?.message || '加载测试详情失败')
    }
  }
}

// ---- 生命周期 ----
onMounted(() => {
  loadHistory()
})

onBeforeUnmount(() => {
  if (testController) {
    testController.abort()
    testController = null
  }
})
</script>

<style scoped>
.section-card {
  width: 100%;
}

.stream-output {
  max-height: 400px;
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
