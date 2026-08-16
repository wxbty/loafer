<template>
  <div class="integration-test-editor">
    <!-- 测试场景 -->
    <el-card class="section-card" shadow="hover">
      <template #header>
        <div class="flex items-center justify-between">
          <span>测试场景</span>
          <div class="flex items-center gap-2">
            <el-button
              size="small"
              type="warning"
              :loading="fullRunning"
              :disabled="!moduleId || fullRunning || runningIndex !== null"
              @click="runAllScenarios"
            >
              <el-icon><VideoPlay /></el-icon>
              全量测试
            </el-button>
            <el-button
              size="small"
              :disabled="fullRunning"
              @click="addScenario"
            >
              添加场景
            </el-button>
            <el-button
              size="small"
              type="primary"
              :disabled="!moduleId || fullRunning"
              @click="openAiAddDialog"
            >
              AI 添加场景
            </el-button>
          </div>
        </div>
      </template>
      <el-empty v-if="!spec.testScenarios?.length" description="暂无测试场景" />
      <el-collapse v-else v-model="activeScenarios">
        <el-collapse-item
          v-for="(scenario, index) in spec.testScenarios"
          :key="index"
          :name="index"
        >
          <template #title>
            <div class="scenario-title">
              <div class="flex items-center gap-2 flex-1 min-w-0">
                <span class="truncate">{{ scenario.name || `场景 ${index + 1}` }}</span>
                <el-tag
                  v-if="scenario.lastRunAt"
                  :type="scenario.lastSuccess ? 'success' : 'danger'"
                  size="small"
                >
                  {{ scenario.lastSuccess ? '上次通过' : '上次失败' }}
                </el-tag>
              </div>
              <div class="flex items-center gap-1">
                <el-button
                  type="primary"
                  link
                  size="small"
                  :loading="runningIndex === index"
                  :disabled="!moduleId || runningIndex !== null || fullRunning"
                  @click.stop="runScenario(index)"
                >
                  {{ scenario.lastRunAt ? '▶ 重试' : '▶ 运行' }}
                </el-button>
                <el-button
                  type="danger"
                  link
                  size="small"
                  @click.stop="removeScenario(index)"
                >
                  删除
                </el-button>
              </div>
            </div>
          </template>

          <el-form label-width="80px" size="small">
            <el-form-item label="场景名称">
              <el-input v-model="scenario.name" placeholder="如：登录成功" />
            </el-form-item>
            <el-form-item label="测试步骤">
              <div class="scenario-steps">
                <div v-for="(step, stepIndex) in scenario.steps" :key="stepIndex" class="step-item">
                  <div class="step-header">
                    <span>步骤 {{ stepIndex + 1 }}</span>
                    <el-button type="danger" link size="small" @click="removeScenarioStep(index, stepIndex)">
                      删除
                    </el-button>
                  </div>
                  <el-form label-width="60px" size="small">
                    <el-form-item label="操作">
                      <el-input v-model="step.action" placeholder="操作描述" />
                    </el-form-item>
                    <el-form-item label="命令">
                      <el-input v-model="step.command" type="textarea" :rows="2" placeholder="执行命令" />
                    </el-form-item>
                    <el-form-item label="期望">
                      <el-input v-model="step.expected" placeholder="期望结果（输出包含的子串）" />
                    </el-form-item>
                  </el-form>
                </div>
                <el-button size="small" @click="addScenarioStep(index)">添加步骤</el-button>
              </div>
            </el-form-item>
            <el-form-item label="失败处理">
              <el-input v-model="scenario.onFailure" placeholder="continue 或 stop" />
            </el-form-item>
          </el-form>

          <!-- 上次运行结果（折叠展示 step 明细） -->
          <div v-if="scenario.lastRunAt" class="last-run-panel">
            <div class="flex items-center justify-between mb-1">
              <span class="text-xs text-slate-500">
                上次运行：{{ formatRunTime(scenario.lastRunAt) }} ·
                <span :class="scenario.lastSuccess ? 'text-green-600' : 'text-red-600'">
                  {{ scenario.lastSummary || (scenario.lastSuccess ? '通过' : '失败') }}
                </span>
              </span>
            </div>
            <div v-if="scenario.screenshot || scenario.errorScreenshot" class="last-screenshots">
              <figure v-if="scenario.screenshot" class="shot-item">
                <el-image
                  :src="screenshotUrl(scenario.screenshot)"
                  :preview-src-list="[screenshotUrl(scenario.screenshot)]"
                  fit="contain"
                  class="shot-img"
                  lazy
                />
                <figcaption class="shot-caption">终态截图</figcaption>
              </figure>
              <figure v-if="scenario.errorScreenshot" class="shot-item">
                <el-image
                  :src="screenshotUrl(scenario.errorScreenshot)"
                  :preview-src-list="[screenshotUrl(scenario.errorScreenshot)]"
                  fit="contain"
                  class="shot-img"
                  lazy
                />
                <figcaption class="shot-caption">失败截图</figcaption>
              </figure>
            </div>
            <el-collapse>
              <el-collapse-item :title="`步骤明细（${scenario.lastSteps?.length || 0}）`" :name="`steps-${index}`">
                <div v-if="!scenario.lastSteps?.length" class="text-xs text-slate-400">无步骤记录</div>
                <div
                  v-for="(s, i) in scenario.lastSteps"
                  :key="i"
                  class="last-step-item"
                  :class="{ 'last-step-fail': !s.ok }"
                >
                  <div class="flex items-center justify-between">
                    <span class="text-xs font-medium">
                      {{ i + 1 }}. {{ s.action || '(无描述)' }}
                    </span>
                    <el-tag :type="s.ok ? 'success' : 'danger'" size="small">
                      {{ s.ok ? 'PASS' : 'FAIL' }}
                    </el-tag>
                  </div>
                  <pre v-if="s.command" class="last-step-pre">$ {{ s.command }}</pre>
                  <pre v-if="s.output" class="last-step-pre">{{ s.output }}</pre>
                  <div v-if="s.error" class="text-xs text-red-600 mt-1">错误：{{ s.error }}</div>
                </div>
              </el-collapse-item>
            </el-collapse>
          </div>
        </el-collapse-item>
      </el-collapse>
    </el-card>

    <!-- 重试策略 -->
    <el-card class="section-card" shadow="hover">
      <template #header>
        <span>重试策略</span>
      </template>
      <el-form label-width="100px" size="small">
        <el-form-item label="最大重试次数">
          <el-input-number v-model="spec.retryStrategy.maxRetries" :min="0" :max="10" />
        </el-form-item>
        <el-form-item label="重试条件">
          <el-select v-model="spec.retryStrategy.retryOn" multiple placeholder="选择重试条件">
            <el-option label="断言失败" value="断言失败" />
            <el-option label="超时" value="超时" />
            <el-option label="连接失败" value="连接失败" />
            <el-option label="服务异常" value="服务异常" />
          </el-select>
        </el-form-item>
        <el-form-item label="自修正提示">
          <div class="correction-hints">
            <el-tag
              v-for="(hint, index) in spec.retryStrategy.selfCorrectionHints"
              :key="index"
              closable
              @close="removeCorrectionHint(index)"
              class="mr-1 mb-1"
            >
              {{ hint }}
            </el-tag>
            <el-input
              v-model="newHint"
              size="small"
              placeholder="输入提示后回车"
              class="hint-input"
              @keyup.enter="addCorrectionHint"
            />
          </div>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 运行 / AI 拆解输出 -->
    <pre v-if="streamOutput" class="stream-output">{{ streamOutput }}</pre>

    <!-- AI 添加场景弹框 -->
    <el-dialog
      v-model="aiDialogVisible"
      title="AI 添加 API 测试场景"
      width="640px"
      :close-on-click-modal="false"
      append-to-body
    >
      <div class="text-xs text-slate-500 mb-3">
        用一句话描述要测的功能，AI 会拆解成 curl 步骤并追加到「测试场景」列表底部。
        若需用到真实第三方 key/token，请在下方勾选项目环境变量。
      </div>
      <el-form label-width="90px" size="small">
        <el-form-item label="场景描述">
          <el-input
            v-model="aiDescription"
            type="textarea"
            :rows="3"
            placeholder="如：新用户注册后立刻用同样邮箱登录，应该返回 200 并带上 JWT"
          />
        </el-form-item>
        <el-form-item label="环境变量">
          <el-empty
            v-if="!aiEnvVars.length && !aiEnvVarsLoading"
            :image-size="40"
            description="项目暂无环境变量"
          />
          <el-table
            v-else
            v-loading="aiEnvVarsLoading"
            :data="aiEnvVars"
            size="small"
            max-height="180"
            class="w-full"
          >
            <el-table-column width="40">
              <template #default="scope">
                <el-checkbox v-model="scope.row.selected" />
              </template>
            </el-table-column>
            <el-table-column label="Key" prop="key" min-width="120" />
            <el-table-column label="Value" min-width="220">
              <template #default="scope">
                <div class="flex items-center gap-1">
                  <code class="text-xs break-all flex-1">
                    {{ scope.row.showValue ? scope.row.value : maskValue(scope.row.value) }}
                  </code>
                  <el-button link size="small" @click="scope.row.showValue = !scope.row.showValue">
                    {{ scope.row.showValue ? '隐藏' : '显示' }}
                  </el-button>
                </div>
              </template>
            </el-table-column>
          </el-table>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="aiDialogVisible = false">取消</el-button>
        <el-button
          type="primary"
          :loading="aiLoading"
          :disabled="!aiDescription.trim()"
          @click="submitAiAddScenario"
        >
          开始生成
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { VideoPlay } from '@element-plus/icons-vue'
import { ProjectApi } from '@/api/project'
import { moduleGenerateScenarioStream, moduleRunAllStream, moduleRunScenarioStream } from '@/api/module'

const props = defineProps<{
  modelValue: string | null
  moduleId?: number | null
  projectId?: number | null
  scenarioType?: 'api' | 'web'
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'module-updated', module: any): void
}>()

const defaultSpec = {
  testScenarios: [] as any[],
  retryStrategy: {
    maxRetries: 3,
    retryOn: ['断言失败', '超时'],
    selfCorrectionHints: ['检查API路径', '检查参数格式']
  }
}

const spec = ref<any>(JSON.parse(JSON.stringify(defaultSpec)))
const activeScenarios = ref<number[]>([])
const newHint = ref('')
const runningIndex = ref<number | null>(null)
const fullRunning = ref(false)
const streamOutput = ref('')

const apiBase = import.meta.env.VITE_API_BASE_URL || '/api'
/** 把结果 JSON 里的相对截图路径转成静态路由 URL（取末段文件名，后端按模块截图目录解析）。 */
const screenshotUrl = (rel: string): string => {
  if (!rel || !props.moduleId) return ''
  const file = rel.split('/').pop() || ''
  if (!file) return ''
  return `${apiBase}/module-screenshots/${props.moduleId}/${encodeURIComponent(file)}`
}

watch(() => props.modelValue, (val) => {
  if (val) {
    try {
      const parsed = JSON.parse(val)
      delete parsed.preconditions
      if (!parsed.testScenarios) parsed.testScenarios = []
      if (!parsed.retryStrategy) parsed.retryStrategy = JSON.parse(JSON.stringify(defaultSpec.retryStrategy))
      spec.value = parsed
    } catch {
      spec.value = JSON.parse(JSON.stringify(defaultSpec))
    }
  } else {
    spec.value = JSON.parse(JSON.stringify(defaultSpec))
  }
}, { immediate: true })

watch(spec, (val) => {
  emit('update:modelValue', JSON.stringify(val, null, 2))
}, { deep: true })

const addScenario = () => {
  spec.value.testScenarios.push({ name: '', steps: [], onFailure: 'continue' })
  activeScenarios.value.push(spec.value.testScenarios.length - 1)
}
const removeScenario = (i: number) => { spec.value.testScenarios.splice(i, 1) }
const addScenarioStep = (i: number) => {
  spec.value.testScenarios[i].steps.push({ action: '', command: '', expected: '' })
}
const removeScenarioStep = (i: number, j: number) => {
  spec.value.testScenarios[i].steps.splice(j, 1)
}
const addCorrectionHint = () => {
  if (newHint.value.trim()) {
    spec.value.retryStrategy.selfCorrectionHints.push(newHint.value.trim())
    newHint.value = ''
  }
}
const removeCorrectionHint = (i: number) => {
  spec.value.retryStrategy.selfCorrectionHints.splice(i, 1)
}

// ==== AI 添加场景 ====
type EnvVarRow = { key: string; value: string; selected: boolean; showValue: boolean }
const aiDialogVisible = ref(false)
const aiDescription = ref('')
const aiLoading = ref(false)
const aiEnvVars = ref<EnvVarRow[]>([])
const aiEnvVarsLoading = ref(false)
const maskValue = (raw: string) => {
  if (!raw) return ''
  if (raw.length <= 4) return '***'
  return raw.slice(0, 2) + '***' + raw.slice(-2)
}

const openAiAddDialog = async () => {
  if (!props.moduleId) {
    ElMessage.error('请先选择模块')
    return
  }
  aiDescription.value = ''
  aiEnvVars.value = []
  aiDialogVisible.value = true
  if (!props.projectId) return
  aiEnvVarsLoading.value = true
  try {
    const proj: any = await ProjectApi.getProjectDetail(props.projectId).catch(() => null)
    const raw = proj?.envVarsJson
    if (typeof raw === 'string' && raw.trim()) {
      try {
        const arr = JSON.parse(raw)
        if (Array.isArray(arr)) {
          aiEnvVars.value = arr
            .filter((e: any) => e && typeof e.key === 'string' && e.key.trim())
            .map((e: any) => ({
              key: String(e.key),
              value: e.value == null ? '' : String(e.value),
              selected: false,
              showValue: false,
            }))
        }
      } catch { /* ignore */ }
    }
  } finally {
    aiEnvVarsLoading.value = false
  }
}

const submitAiAddScenario = () => {
  if (!props.moduleId) return
  if (!aiDescription.value.trim()) return
  const keys = aiEnvVars.value.filter(r => r.selected).map(r => r.key)
  aiLoading.value = true
  streamOutput.value = ''
  moduleGenerateScenarioStream(
    props.moduleId,
    'api',
    aiDescription.value.trim(),
    keys,
    (line) => { streamOutput.value += line + '\n' },
    (moduleJson) => {
      aiLoading.value = false
      aiDialogVisible.value = false
      try {
        const updated = JSON.parse(moduleJson)
        emit('module-updated', updated)
        ElMessage.success('AI 已追加新场景')
      } catch (e) {
        ElMessage.error('解析返回失败: ' + (e as Error).message)
      }
    },
    (err) => {
      aiLoading.value = false
      ElMessage.error('生成失败：' + err)
    },
  )
}

// ==== 单场景手动运行 ====
const runScenario = (index: number) => {
  if (!props.moduleId) {
    ElMessage.error('请先保存模块')
    return
  }
  runningIndex.value = index
  streamOutput.value = ''
  moduleRunScenarioStream(
    props.moduleId,
    props.scenarioType ?? 'api',
    index,
    (line) => { streamOutput.value += line + '\n' },
    (moduleJson) => {
      runningIndex.value = null
      try {
        const updated = JSON.parse(moduleJson)
        emit('module-updated', updated)
        ElMessage.success('运行完成')
      } catch (e) {
        ElMessage.error('解析返回失败: ' + (e as Error).message)
      }
    },
    (err) => {
      runningIndex.value = null
      ElMessage.error('运行失败：' + err)
    },
  )
}

// ==== 全量测试：顺序执行该类型全部场景，失败时后端驱动修复→部署→重测（≤3轮） ====
const runAllScenarios = () => {
  if (!props.moduleId) {
    ElMessage.error('请先保存模块')
    return
  }
  if (!props.scenarioType) return
  fullRunning.value = true
  streamOutput.value = ''
  moduleRunAllStream(
    props.moduleId,
    props.scenarioType,
    (line) => { streamOutput.value += line + '\n' },
    (moduleJson) => {
      fullRunning.value = false
      try {
        const updated = JSON.parse(moduleJson)
        emit('module-updated', updated)
        ElMessage.success('全量测试完成，详见各场景步骤明细')
      } catch (e) {
        ElMessage.error('解析返回失败: ' + (e as Error).message)
      }
    },
    (err) => {
      fullRunning.value = false
      ElMessage.error('全量测试失败：' + err)
    },
  )
}

const formatRunTime = (iso: string) => {
  if (!iso) return ''
  try {
    const d = new Date(iso)
    return d.toLocaleString()
  } catch {
    return iso
  }
}
</script>

<style scoped>
.integration-test-editor {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.section-card {
  margin-bottom: 0;
}

.scenario-title {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex: 1;
  padding-right: 8px;
  gap: 8px;
}

.scenario-steps {
  width: 100%;
}

.step-item {
  padding: 12px;
  margin-bottom: 12px;
  border: 1px solid #ebeef5;
  border-radius: 4px;
}

.step-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.last-run-panel {
  margin-top: 8px;
  padding: 8px;
  background: #fafbfc;
  border: 1px solid #ebeef5;
  border-radius: 4px;
}

.last-step-item {
  padding: 6px 8px;
  margin-bottom: 6px;
  border-left: 3px solid #67c23a;
  background: #fff;
}

.last-step-fail {
  border-left-color: #f56c6c;
}

.last-step-pre {
  margin: 4px 0 0;
  padding: 4px 6px;
  background: #f5f7fa;
  font-size: 11px;
  white-space: pre-wrap;
  max-height: 160px;
  overflow: auto;
}

.correction-hints {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
}

.hint-input {
  width: 150px;
}

.stream-output {
  margin: 0;
  padding: 8px;
  background: #f5f7fa;
  font-size: 11px;
  max-height: 200px;
  overflow: auto;
  white-space: pre-wrap;
  border: 1px solid #ebeef5;
  border-radius: 4px;
}

.last-screenshots {
  display: flex;
  gap: 12px;
  margin: 6px 0;
  flex-wrap: wrap;
}
.shot-item {
  margin: 0;
}
.shot-img {
  width: 240px;
  height: 150px;
  border: 1px solid #e2e8f0;
  border-radius: 4px;
  background: #f8fafc;
}
.shot-caption {
  font-size: 12px;
  color: #94a3b8;
  text-align: center;
  margin-top: 2px;
}
</style>
