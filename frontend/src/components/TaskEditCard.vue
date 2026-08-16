<template>
  <el-card class="task-edit-card" shadow="never">
    <div class="task-edit-header">
      <span class="task-edit-title">{{ title }}</span>
      <el-button v-if="removable" link type="danger" size="small" @click="$emit('remove')">删除</el-button>
    </div>
    <el-form :model="task" label-width="90px" size="small">
      <el-form-item label="任务名称" required>
        <el-input v-model="task.name" placeholder="如：实现登出 API" />
      </el-form-item>
      <el-form-item label="任务描述">
        <el-input v-model="task.description" type="textarea" :rows="2" placeholder="详细描述及验收标准" />
      </el-form-item>
      <el-form-item label="任务分类">
        <el-select v-model="task.category" style="width: 200px">
          <el-option v-for="opt in categoryOptions" :key="opt" :label="opt" :value="opt" />
        </el-select>
      </el-form-item>
      <el-form-item label="依赖任务">
        <el-select
          v-model="task.blockedBy"
          multiple
          filterable
          allow-create
          placeholder="选择依赖的兄弟任务序号（如 M1.1）"
          style="width: 100%"
        >
          <el-option
            v-for="ex in existingTaskOptions"
            :key="ex.sequenceNumber"
            :label="`${ex.sequenceNumber} ${ex.name}`"
            :value="ex.sequenceNumber"
          />
        </el-select>
      </el-form-item>
      <el-collapse>
        <el-collapse-item :title="`步骤（${(task.steps || []).length}）`" name="steps">
          <TaskStepEditor v-model="task.steps" />
        </el-collapse-item>
      </el-collapse>
    </el-form>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import TaskStepEditor from './TaskStepEditor.vue'

interface ExistingTask {
  sequenceNumber: string
  name: string
}

interface TaskData {
  name: string
  description: string
  category: string
  blockedBy: string[]
  steps: any[]
}

const props = defineProps<{
  task: TaskData
  title: string
  removable?: boolean
  existingTasks?: ExistingTask[]
}>()

defineEmits<{ (e: 'remove'): void }>()

const categoryOptions = ['功能', 'bug修复', '代码优化', '性能']

const existingTaskOptions = computed(() =>
  (props.existingTasks || []).filter((t) => t.sequenceNumber && t.name)
)
</script>

<style scoped>
.task-edit-card {
  margin-bottom: 12px;
}
.task-edit-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
  font-weight: 500;
  color: #303133;
}
.task-edit-title {
  font-size: 14px;
}
</style>
