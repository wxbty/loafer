<template>
  <div class="task-step-editor">
    <div class="step-list">
      <el-card v-for="(step, index) in steps" :key="index" class="step-card" shadow="hover">
        <div class="step-header">
          <span class="step-seq">步骤 {{ index + 1 }}</span>
          <div class="step-actions">
            <el-button size="small" link @click="moveStep(index, -1)" :disabled="index === 0">
              上移
            </el-button>
            <el-button size="small" link @click="moveStep(index, 1)" :disabled="index === steps.length - 1">
              下移
            </el-button>
            <el-button size="small" link type="danger" @click="removeStep(index)">
              删除
            </el-button>
          </div>
        </div>
        <el-form label-width="80px" size="small">
          <el-form-item label="操作描述">
            <el-input v-model="step.action" placeholder="如：创建 AuthController" />
          </el-form-item>
          <el-form-item label="Plan 引用" class="plan-excerpt-item">
            <el-input
              v-model="step.planExcerpt"
              type="textarea"
              :rows="3"
              placeholder="从 Plan.md 中提取的相关需求片段原文，帮助 AI 执行时理解上下文"
            />
          </el-form-item>
          <el-form-item label="涉及文件">
            <el-select
              v-model="step.files"
              multiple
              filterable
              allow-create
              default-first-option
              placeholder="输入文件路径后回车"
            />
          </el-form-item>
          <el-form-item label="迁移文件">
            <el-input v-model="step.migrationFile" placeholder="数据库迁移文件路径（可选）" />
          </el-form-item>
          <el-form-item label="验证标准">
            <el-input v-model="step.validation" placeholder="如：编译通过，API 可访问" />
          </el-form-item>
          <el-form-item label="事实校验">
            <div class="fact-checks-block">
              <div v-if="!step.factChecks || step.factChecks.length === 0" class="fact-checks-empty">
                未规划事实校验（Tester 执行时会按"验证标准"自行决定 evidence）
              </div>
              <ul v-else class="fact-checks-list">
                <li v-for="(fc, fcIdx) in step.factChecks" :key="fcIdx" class="fact-checks-item">
                  <el-tag size="small" :type="factCheckTagType(fc.type)" class="fact-check-tag">
                    {{ fc.type || 'unknown' }}
                  </el-tag>
                  <span class="fact-check-label">{{ formatFactCheckLabel(fc) }}</span>
                  <el-button size="small" link type="primary" @click="openSingleFactCheckEditor(index, fcIdx)">
                    编辑
                  </el-button>
                  <el-button size="small" link type="danger" @click="removeFactCheck(index, fcIdx)">
                    删除
                  </el-button>
                </li>
              </ul>
              <div class="fact-checks-actions">
                <el-button size="small" link type="primary" @click="openSingleFactCheckEditor(index, -1)">
                  添加校验项
                </el-button>
                <el-button size="small" link type="info" @click="openFactChecksEditor(index)">
                  编辑 JSON
                </el-button>
              </div>
            </div>
          </el-form-item>
        </el-form>
      </el-card>
    </div>
    <el-button type="primary" size="small" class="add-step-btn" @click="addStep">
      <el-icon><Plus /></el-icon>
      添加步骤
    </el-button>

    <el-dialog
      v-model="factChecksDialogVisible"
      title="编辑 factChecks JSON"
      width="640px"
      :close-on-click-modal="false"
    >
      <div class="fact-checks-dialog-hint">
        factChecks 数组按以下 4 种 type：
        <code>file_exists</code> / <code>shell</code> / <code>http_status</code> / <code>git_commit</code>。
        path / command 中的文件路径请使用绝对路径。
      </div>
      <el-input
        v-model="factChecksDraft"
        type="textarea"
        :rows="14"
        placeholder='[{"type":"file_exists","path":"/abs/path/foo.java"}]'
        :class="{ 'fact-checks-error': !!factChecksError }"
      />
      <div v-if="factChecksError" class="fact-checks-error-tip">{{ factChecksError }}</div>
      <template #footer>
        <el-button @click="factChecksDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveFactChecks">保存</el-button>
      </template>
    </el-dialog>

    <!-- 单个事实校验项编辑对话框 -->
    <el-dialog
      v-model="singleFactCheckDialogVisible"
      :title="singleFactCheckIndex >= 0 ? '编辑事实校验项' : '添加事实校验项'"
      width="500px"
      destroy-on-close
    >
      <el-form :model="singleFactCheckDraft" label-width="100px" size="small">
        <el-form-item label="类型">
          <el-select v-model="singleFactCheckDraft.type" placeholder="选择校验类型">
            <el-option label="文件存在 (file_exists)" value="file_exists" />
            <el-option label="Shell命令 (shell)" value="shell" />
            <el-option label="HTTP状态 (http_status)" value="http_status" />
            <el-option label="Git提交 (git_commit)" value="git_commit" />
          </el-select>
        </el-form-item>
        <!-- file_exists -->
        <el-form-item v-if="singleFactCheckDraft.type === 'file_exists'" label="文件路径">
          <el-input v-model="singleFactCheckDraft.path" placeholder="绝对路径，如 /abs/path/file.java" />
        </el-form-item>
        <!-- shell -->
        <el-form-item v-if="singleFactCheckDraft.type === 'shell'" label="Shell命令">
          <el-input v-model="singleFactCheckDraft.command" type="textarea" :rows="2" placeholder="如 mvn test -Dtest=xxx" />
        </el-form-item>
        <el-form-item v-if="singleFactCheckDraft.type === 'shell'" label="期望退出码">
          <el-input-number v-model="singleFactCheckDraft.expectedExitCode" :min="0" :max="255" />
        </el-form-item>
        <!-- http_status -->
        <el-form-item v-if="singleFactCheckDraft.type === 'http_status'" label="HTTP方法">
          <el-select v-model="singleFactCheckDraft.method">
            <el-option label="GET" value="GET" />
            <el-option label="POST" value="POST" />
            <el-option label="PUT" value="PUT" />
            <el-option label="DELETE" value="DELETE" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="singleFactCheckDraft.type === 'http_status'" label="URL">
          <el-input v-model="singleFactCheckDraft.url" placeholder="如 http://localhost:8080/api/test" />
        </el-form-item>
        <el-form-item v-if="singleFactCheckDraft.type === 'http_status'" label="期望状态码">
          <el-input-number v-model="singleFactCheckDraft.expectedStatus" :min="100" :max="599" />
        </el-form-item>
        <!-- git_commit -->
        <el-form-item v-if="singleFactCheckDraft.type === 'git_commit'" label="Commit Hash">
          <el-input v-model="singleFactCheckDraft.commitHash" placeholder="可选，Tester执行时会填入" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="singleFactCheckDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveSingleFactCheck">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { Plus } from '@element-plus/icons-vue'

const props = defineProps<{
  modelValue: any[]
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: any[]): void
}>()

const steps = ref<any[]>([])

// 监听外部值变化
watch(() => props.modelValue, (val) => {
  if (val) {
    steps.value = val
  }
}, { immediate: true, deep: true })

// 监听内部值变化
watch(steps, (val) => {
  emit('update:modelValue', val)
}, { deep: true })

// 添加步骤
const addStep = () => {
  steps.value.push({
    seq: steps.value.length + 1,
    action: '',
    files: [],
    planExcerpt: '',
    migrationFile: '',
    validation: '',
    factChecks: []
  })
}

// factChecks JSON 编辑
const factChecksDialogVisible = ref(false)
const factChecksDraft = ref('')
const factChecksError = ref('')
const factChecksEditingIndex = ref<number>(-1)

const openFactChecksEditor = (stepIndex: number) => {
  factChecksEditingIndex.value = stepIndex
  const current = steps.value[stepIndex]?.factChecks ?? []
  factChecksDraft.value = JSON.stringify(current, null, 2)
  factChecksError.value = ''
  factChecksDialogVisible.value = true
}

const saveFactChecks = () => {
  const raw = factChecksDraft.value.trim()
  let parsed: any = []
  if (raw !== '') {
    try {
      parsed = JSON.parse(raw)
    } catch (e: any) {
      factChecksError.value = 'JSON 解析失败：' + (e?.message || e)
      return
    }
    if (!Array.isArray(parsed)) {
      factChecksError.value = 'factChecks 必须是数组'
      return
    }
    const allowed = ['file_exists', 'shell', 'http_status', 'git_commit']
    for (let i = 0; i < parsed.length; i++) {
      const t = parsed[i]?.type
      if (!t || !allowed.includes(t)) {
        factChecksError.value = `第 ${i + 1} 条 type 非法（必须是 ${allowed.join(' / ')}）`
        return
      }
    }
  }
  const idx = factChecksEditingIndex.value
  if (idx >= 0 && idx < steps.value.length) {
    steps.value[idx].factChecks = parsed
  }
  factChecksDialogVisible.value = false
}

const factCheckTagType = (type: string): string => {
  switch (type) {
    case 'file_exists': return 'info'
    case 'shell': return 'warning'
    case 'http_status': return 'success'
    case 'git_commit': return 'primary'
    default: return ''
  }
}

// 单个事实校验项编辑
const singleFactCheckDialogVisible = ref(false)
const singleFactCheckStepIndex = ref<number>(-1)
const singleFactCheckIndex = ref<number>(-1)
const singleFactCheckDraft = ref<any>({
  type: 'file_exists',
  path: '',
  command: '',
  expectedExitCode: 0,
  method: 'GET',
  url: '',
  expectedStatus: 200,
  commitHash: ''
})

const openSingleFactCheckEditor = (stepIndex: number, factCheckIndex: number) => {
  singleFactCheckStepIndex.value = stepIndex
  singleFactCheckIndex.value = factCheckIndex

  if (factCheckIndex >= 0 && steps.value[stepIndex]?.factChecks?.[factCheckIndex]) {
    // 编辑已有项
    const fc = steps.value[stepIndex].factChecks[factCheckIndex]
    singleFactCheckDraft.value = {
      type: fc.type || 'file_exists',
      path: fc.path || '',
      command: fc.command || '',
      expectedExitCode: fc.expectedExitCode ?? 0,
      method: fc.method || 'GET',
      url: fc.url || '',
      expectedStatus: fc.expectedStatus ?? 200,
      commitHash: fc.commitHash || ''
    }
  } else {
    // 新增
    singleFactCheckDraft.value = {
      type: 'file_exists',
      path: '',
      command: '',
      expectedExitCode: 0,
      method: 'GET',
      url: '',
      expectedStatus: 200,
      commitHash: ''
    }
  }
  singleFactCheckDialogVisible.value = true
}

const removeFactCheck = (stepIndex: number, factCheckIndex: number) => {
  if (steps.value[stepIndex]?.factChecks) {
    steps.value[stepIndex].factChecks.splice(factCheckIndex, 1)
  }
}

const saveSingleFactCheck = () => {
  const draft = singleFactCheckDraft.value
  if (!draft.type) {
    return
  }

  // 根据类型构建factCheck对象
  let fc: any = { type: draft.type }
  switch (draft.type) {
    case 'file_exists':
      fc.path = draft.path || ''
      break
    case 'shell':
      fc.command = draft.command || ''
      fc.expectedExitCode = draft.expectedExitCode ?? 0
      break
    case 'http_status':
      fc.method = draft.method || 'GET'
      fc.url = draft.url || ''
      fc.expectedStatus = draft.expectedStatus ?? 200
      break
    case 'git_commit':
      fc.commitHash = draft.commitHash || ''
      break
  }

  const stepIdx = singleFactCheckStepIndex.value
  if (stepIdx >= 0 && stepIdx < steps.value.length) {
    if (!steps.value[stepIdx].factChecks) {
      steps.value[stepIdx].factChecks = []
    }
    if (singleFactCheckIndex.value >= 0) {
      // 更新
      steps.value[stepIdx].factChecks[singleFactCheckIndex.value] = fc
    } else {
      // 新增
      steps.value[stepIdx].factChecks.push(fc)
    }
  }
  singleFactCheckDialogVisible.value = false
}

const formatFactCheckLabel = (fc: any): string => {
  switch (fc?.type) {
    case 'file_exists':
      return fc.path || '(空 path)'
    case 'shell': {
      const cmd = fc.command || '(空 command)'
      return cmd.length > 80 ? cmd.slice(0, 80) + '…' : cmd
    }
    case 'http_status':
      return `${fc.method || 'GET'} ${fc.url || '(空 url)'} → ${fc.expectedStatus ?? '?'}`
    case 'git_commit':
      return fc.commitHash ? `commit ${fc.commitHash}` : '(Tester 在执行时填入 commit hash)'
    default:
      return JSON.stringify(fc)
  }
}

// 删除步骤
const removeStep = (index: number) => {
  steps.value.splice(index, 1)
  // 更新序号
  steps.value.forEach((s, i) => {
    s.seq = i + 1
  })
}

// 移动步骤
const moveStep = (index: number, direction: number) => {
  const newIndex = index + direction
  if (newIndex < 0 || newIndex >= steps.value.length) return

  const temp = steps.value[index]
  steps.value[index] = steps.value[newIndex]
  steps.value[newIndex] = temp

  // 更新序号
  steps.value.forEach((s, i) => {
    s.seq = i + 1
  })
}
</script>

<style scoped>
.task-step-editor {
  width: 100%;
}

.step-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-height: 400px;
  overflow-y: auto;
}

.step-card {
  margin-bottom: 0;
}

.step-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}

.step-seq {
  font-weight: 500;
  color: #303133;
}

.step-actions {
  display: flex;
  gap: 8px;
}

.add-step-btn {
  margin-top: 12px;
}

.plan-excerpt-item :deep(.el-textarea__inner) {
  background-color: #fffbeb;
  border-color: #fbbf24;
}

.plan-excerpt-item :deep(.el-textarea__inner:focus) {
  border-color: #f59e0b;
}

.fact-checks-block {
  width: 100%;
}

.fact-checks-empty {
  color: #909399;
  font-size: 12px;
  margin-bottom: 6px;
}

.fact-checks-list {
  list-style: none;
  padding: 0;
  margin: 0 0 6px;
}

.fact-checks-item {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  line-height: 1.6;
  word-break: break-all;
}

.fact-checks-actions {
  display: flex;
  gap: 8px;
  margin-top: 4px;
}

.fact-check-tag {
  flex-shrink: 0;
}

.fact-check-label {
  color: #303133;
}

.fact-checks-dialog-hint {
  font-size: 12px;
  color: #606266;
  margin-bottom: 8px;
}

.fact-checks-dialog-hint code {
  background: #f5f7fa;
  padding: 1px 4px;
  border-radius: 3px;
}

.fact-checks-error :deep(.el-textarea__inner) {
  border-color: #f56c6c;
}

.fact-checks-error-tip {
  color: #f56c6c;
  font-size: 12px;
  margin-top: 6px;
}
</style>
