<template>
  <el-dialog
    v-model="visible"
    :title="`为模块 ${moduleSequence || ''} ${moduleName || ''} 添加任务`"
    width="860px"
    destroy-on-close
    @closed="onClosed"
  >
    <el-tabs v-model="activeTab">
      <el-tab-pane label="表单添加" name="form">
        <div v-for="(t, idx) in formTasks" :key="`f-${idx}`">
          <TaskEditCard
            :task="t"
            :title="`任务 ${idx + 1}`"
            :removable="formTasks.length > 1"
            :existing-tasks="existingTaskOptions"
            @remove="formTasks.splice(idx, 1)"
          />
        </div>
        <el-button type="primary" size="small" plain @click="addFormTask">
          <el-icon><Plus /></el-icon>
          再加一个任务
        </el-button>
      </el-tab-pane>

      <el-tab-pane label="AI 添加" name="ai">
        <el-form :model="aiForm" label-width="100px">
          <el-form-item label="需求描述">
            <el-input
              v-model="aiForm.description"
              type="textarea"
              :rows="3"
              placeholder="描述要追加的功能（与需求文档二选一）"
              :disabled="aiForm.planFiles.length > 0"
            />
          </el-form-item>
          <el-form-item label="需求文档">
            <div class="plan-files-row">
              <el-select
                v-model="aiForm.planFiles"
                multiple
                filterable
                placeholder="选择 Plan 文件（与需求描述二选一）"
                :disabled="!!aiForm.description.trim()"
                :loading="planFilesLoading"
                style="width: 100%"
              >
                <el-option
                  v-for="f in planFiles"
                  :key="f.path"
                  :label="f.path"
                  :value="f.absolutePath"
                />
              </el-select>
              <el-button link type="primary" :loading="planFilesLoading" @click="loadPlanFiles">
                <el-icon><Refresh /></el-icon>
                刷新
              </el-button>
            </div>
          </el-form-item>
        </el-form>

        <div class="ai-context-hint">
          <el-icon><InfoFilled /></el-icon>
          已有任务 {{ existingTaskOptions.length }} 个，将作为上下文喂给 AI，避免重复生成。
        </div>

        <div class="ai-actions">
          <el-button type="primary" :loading="aiLoading" :disabled="!canStartAi" @click="startAi">
            {{ aiResultTasks.length > 0 ? '重新生成' : '开始生成' }}
          </el-button>
          <el-button v-if="aiLoading" @click="cancelAi">取消生成</el-button>
        </div>

        <el-collapse v-if="aiOutput" v-model="logCollapseOpen" class="mt-3">
          <el-collapse-item title="执行日志" name="log">
            <pre class="ai-log">{{ aiOutput }}</pre>
          </el-collapse-item>
        </el-collapse>

        <div v-if="aiResultTasks.length > 0" class="mt-3">
          <el-divider>AI 生成结果（可编辑，保存即落库）</el-divider>
          <div v-for="(t, idx) in aiResultTasks" :key="`ai-${idx}`">
            <TaskEditCard
              :task="t"
              :title="`AI 任务 ${idx + 1}`"
              :removable="true"
              :existing-tasks="existingTaskOptions"
              @remove="aiResultTasks.splice(idx, 1)"
            />
          </div>
        </div>
      </el-tab-pane>
    </el-tabs>

    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="saving" :disabled="!canSave" @click="save">
        保存
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus, Refresh, InfoFilled } from '@element-plus/icons-vue'
import { ModuleApi, appendTasksToModuleStream } from '@/api/module'
import { ProjectApi } from '@/api/project'
import TaskEditCard from './TaskEditCard.vue'

interface ExistingTask {
  sequenceNumber: string
  name: string
}

const props = defineProps<{
  modelValue: boolean
  moduleId: number | null
  moduleName?: string
  moduleSequence?: string
  projectId: number | null
  existingTasks?: ExistingTask[]
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'saved'): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})

const activeTab = ref<'form' | 'ai'>('form')

// 表单模式：可同时新增多个任务
const blankTask = () => ({
  name: '',
  description: '',
  category: '功能',
  blockedBy: [] as string[],
  steps: [] as any[]
})
const formTasks = ref([blankTask()])
const addFormTask = () => formTasks.value.push(blankTask())

// AI 模式
const aiForm = ref({
  description: '',
  planFiles: [] as string[]
})
const aiLoading = ref(false)
const aiOutput = ref('')
const aiResultTasks = ref<any[]>([])
const aiController = ref<AbortController | null>(null)
const logCollapseOpen = ref<string[]>([])

// Plan 文件列表
const planFiles = ref<any[]>([])
const planFilesLoading = ref(false)

const saving = ref(false)

const existingTaskOptions = computed<ExistingTask[]>(() => props.existingTasks || [])

const canStartAi = computed(() => {
  if (aiLoading.value) return false
  const hasDesc = !!aiForm.value.description.trim()
  const hasPlan = aiForm.value.planFiles.length > 0
  return (hasDesc || hasPlan) && !(hasDesc && hasPlan)
})

const canSave = computed(() => {
  const tasks = activeTab.value === 'form' ? formTasks.value : aiResultTasks.value
  if (tasks.length === 0) return false
  return tasks.every((t) => !!t.name && t.name.trim())
})

const loadPlanFiles = async () => {
  if (!props.projectId) return
  planFilesLoading.value = true
  try {
    const res = await ProjectApi.getProjectDocs(props.projectId)
    planFiles.value = Array.isArray(res) ? res : []
  } catch (e) {
    planFiles.value = []
  } finally {
    planFilesLoading.value = false
  }
}

const startAi = () => {
  if (!props.moduleId) {
    ElMessage.error('moduleId 缺失')
    return
  }
  aiLoading.value = true
  aiOutput.value = ''
  aiResultTasks.value = []
  logCollapseOpen.value = ['log']

  aiController.value = appendTasksToModuleStream(
    props.moduleId,
    aiForm.value.description.trim(),
    aiForm.value.planFiles,
    (line) => {
      aiOutput.value += line + '\n'
    },
    (result) => {
      aiLoading.value = false
      try {
        const parsed = JSON.parse(result)
        if (Array.isArray(parsed)) {
          aiResultTasks.value = parsed.map((t: any) => ({
            name: t.name || '',
            description: t.description || '',
            category: t.category || '功能',
            blockedBy: parseBlockedBy(t.blockedBy),
            steps: parseSteps(t.stepsJson)
          }))
          if (aiResultTasks.value.length === 0) {
            ElMessage.warning('AI 未生成任何任务')
          } else {
            ElMessage.success(`AI 生成 ${aiResultTasks.value.length} 个任务，请编辑后保存`)
          }
        } else {
          ElMessage.error('AI 返回格式不是数组')
        }
      } catch (e) {
        ElMessage.error('解析 AI 结果失败')
      }
    },
    (err) => {
      aiLoading.value = false
      ElMessage.error(err)
    }
  )
}

const cancelAi = () => {
  aiController.value?.abort()
  aiController.value = null
  aiLoading.value = false
}

const parseBlockedBy = (raw: any): string[] => {
  if (!raw) return []
  if (Array.isArray(raw)) return raw
  if (typeof raw === 'string') {
    try {
      const arr = JSON.parse(raw)
      return Array.isArray(arr) ? arr : []
    } catch {
      return []
    }
  }
  return []
}

const parseSteps = (raw: any): any[] => {
  if (!raw) return []
  if (Array.isArray(raw)) return raw
  if (typeof raw === 'string') {
    try {
      const arr = JSON.parse(raw)
      return Array.isArray(arr) ? arr : []
    } catch {
      return []
    }
  }
  return []
}

const save = async () => {
  if (!props.moduleId) {
    ElMessage.error('moduleId 缺失')
    return
  }
  const tasks = activeTab.value === 'form' ? formTasks.value : aiResultTasks.value
  // 校验
  for (const t of tasks) {
    if (!t.name || !t.name.trim()) {
      ElMessage.error('任务名称不能为空')
      return
    }
  }

  const payload = tasks.map((t) => ({
    name: t.name.trim(),
    description: t.description || '',
    category: t.category || '功能',
    blockedBy: t.blockedBy && t.blockedBy.length > 0 ? JSON.stringify(t.blockedBy) : null,
    stepsJson: t.steps && t.steps.length > 0 ? JSON.stringify(t.steps) : null
  }))

  saving.value = true
  try {
    const res: any = await ModuleApi.appendTasks(props.moduleId, payload)
    if (res?.success === false) {
      ElMessage.error(res.message || '保存失败')
      return
    }
    ElMessage.success(`已新增 ${tasks.length} 个任务`)
    emit('saved')
    visible.value = false
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '保存失败')
  } finally {
    saving.value = false
  }
}

const onClosed = () => {
  // 关闭后重置表单状态
  formTasks.value = [blankTask()]
  aiForm.value = { description: '', planFiles: [] }
  aiOutput.value = ''
  aiResultTasks.value = []
  aiController.value?.abort()
  aiController.value = null
  aiLoading.value = false
  activeTab.value = 'form'
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      loadPlanFiles()
    }
  }
)
</script>

<style scoped>
.plan-files-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}
.ai-context-hint {
  display: flex;
  align-items: center;
  gap: 4px;
  margin: 4px 0 12px;
  font-size: 12px;
  color: #909399;
}
.ai-actions {
  display: flex;
  gap: 8px;
  margin-top: 4px;
}
.ai-log {
  max-height: 240px;
  overflow: auto;
  margin: 0;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
}
.mt-3 {
  margin-top: 12px;
}
</style>
