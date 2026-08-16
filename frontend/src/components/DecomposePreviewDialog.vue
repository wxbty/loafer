<template>
  <el-dialog
    v-model="visible"
    title="模块化拆解预览"
    width="90%"
    top="5vh"
    destroy-on-close
    :close-on-click-modal="false"
    @close="handleClose"
  >
    <div class="decompose-preview-container">
      <!-- 左侧：目录树 -->
      <div class="preview-tree">
        <div class="tree-header">
          <span>模块任务列表</span>
          <div class="tree-actions">
            <el-button
              v-if="checkedNodes.length > 0"
              size="small"
              type="danger"
              @click="batchDelete"
            >
              删除选中 ({{ checkedNodes.length }})
            </el-button>
            <el-button size="small" @click="expandAll">全部展开</el-button>
            <el-button size="small" @click="collapseAll">全部折叠</el-button>
          </div>
        </div>
        <el-tree
          ref="treeRef"
          :data="treeData"
          :props="treeProps"
          node-key="id"
          highlight-current
          default-expand-all
          draggable
          show-checkbox
          @node-click="handleNodeClick"
          @node-drop="handleNodeDrop"
          @check="handleCheck"
        >
          <template #default="scope">
            <span class="tree-node">
              <el-icon v-if="scope.data.type === 'module'" class="tree-icon">
                <FolderOpened v-if="scope.node.expanded" />
                <Folder v-else />
              </el-icon>
              <el-icon v-else class="tree-icon tree-icon-task">
                <Document />
              </el-icon>
              <el-input
                v-if="editingNode?.id === scope.data.id"
                v-model="scope.data.name"
                size="small"
                class="edit-input"
                @blur="editingNode = null"
                @keyup.enter="editingNode = null"
              />
              <span v-else class="tree-label">{{ scope.data.name }}</span>
              <el-button
                v-if="!editingNode || editingNode?.id !== scope.data.id"
                size="small"
                link
                @click.stop="startEdit(scope.data)"
              >
                编辑
              </el-button>
              <el-button
                v-if="scope.data.type === 'module'"
                size="small"
                link
                type="primary"
                @click.stop="addTaskToModule(scope.data)"
              >
                添加任务
              </el-button>
              <el-button
                size="small"
                link
                type="danger"
                @click.stop="removeNode(scope.data, scope.node)"
              >
                删除
              </el-button>
            </span>
          </template>
        </el-tree>
        <el-empty v-if="treeData.length === 0" description="暂无拆解结果" />
      </div>

      <!-- 右侧：详情编辑 -->
      <div class="preview-detail">
        <template v-if="selectedNode">
          <!-- 模块详情 -->
          <template v-if="selectedNode.type === 'module'">
            <h3 class="detail-title">模块详情</h3>
            <el-form :model="selectedNode" label-width="100px">
              <el-form-item label="模块名称">
                <el-input v-model="selectedNode.name" />
              </el-form-item>
              <el-form-item label="模块序号">
                <el-input v-model="selectedNode.sequenceNumber" />
              </el-form-item>
              <el-form-item label="模块描述">
                <el-input v-model="selectedNode.description" type="textarea" :rows="3" />
              </el-form-item>
              <el-form-item label="依赖模块">
                <el-select v-model="selectedNode.blockedByArr" multiple placeholder="选择依赖模块">
                  <el-option
                    v-for="m in otherModules"
                    :key="m.id"
                    :label="m.sequenceNumber + ' ' + m.name"
                    :value="m.sequenceNumber"
                  />
                </el-select>
              </el-form-item>
              <el-form-item label="集成测试">
                <el-tabs type="border-card" class="test-tabs">
                  <el-tab-pane label="API集成测试">
                    <IntegrationTestEditor v-model="selectedNode.apiIntegrationTestText" />
                  </el-tab-pane>
                </el-tabs>
              </el-form-item>
            </el-form>
          </template>

          <!-- 任务详情 -->
          <template v-else-if="selectedNode.type === 'task'">
            <h3 class="detail-title">任务详情</h3>
            <el-form :model="selectedNode" label-width="100px">
              <el-form-item label="任务名称">
                <el-input v-model="selectedNode.name" />
              </el-form-item>
              <el-form-item label="任务序号">
                <el-input v-model="selectedNode.sequenceNumber" />
              </el-form-item>
              <el-form-item label="任务描述">
                <el-input v-model="selectedNode.description" type="textarea" :rows="3" />
              </el-form-item>
              <el-form-item label="任务分类">
                <el-select v-model="selectedNode.category">
                  <el-option label="功能" value="功能" />
                  <el-option label="bug修复" value="bug修复" />
                  <el-option label="代码优化" value="代码优化" />
                  <el-option label="性能" value="性能" />
                </el-select>
              </el-form-item>
              <el-form-item label="依赖任务">
                <el-select v-model="selectedNode.blockedByArr" multiple placeholder="选择依赖任务">
                  <el-option
                    v-for="t in otherTasks"
                    :key="t.id"
                    :label="t.sequenceNumber + ' ' + t.name"
                    :value="t.sequenceNumber"
                  />
                </el-select>
              </el-form-item>
              <el-form-item label="执行步骤">
                <TaskStepEditor v-model="selectedNode.steps" />
              </el-form-item>
            </el-form>
          </template>
        </template>
        <el-empty v-else description="请选择模块或任务查看详情" />
      </div>
    </div>

    <template #footer>
      <div class="dialog-footer">
        <el-button @click="handleClose">取消</el-button>
        <el-button type="primary" :loading="saving" @click="handleSave">
          确认保存
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { FolderOpened, Folder, Document } from '@element-plus/icons-vue'
import type { ElTree } from 'element-plus'
import { ModuleApi } from '@/api/module'
import TaskStepEditor from './TaskStepEditor.vue'
import IntegrationTestEditor from './IntegrationTestEditor.vue'

const props = defineProps<{
  modelValue: boolean
  projectId: number
  decomposeResult: any[]
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'saved'): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val)
})

// 监听对话框显示

const treeRef = ref<InstanceType<typeof ElTree>>()
const treeData = ref<any[]>([])
const selectedNode = ref<any>(null)
const editingNode = ref<any>(null)
const saving = ref(false)
const checkedNodes = ref<any[]>([])

const treeProps = {
  children: 'tasks',
  label: 'name'
}

// 其他模块（排除当前选中的）
const otherModules = computed(() => {
  return treeData.value.filter((m: any) => m.id !== selectedNode.value?.id)
})

// 其他任务（排除当前选中的）
const otherTasks = computed(() => {
  const tasks: any[] = []
  treeData.value.forEach((m: any) => {
    if (m.tasks) {
      m.tasks.forEach((t: any) => {
        if (t.id !== selectedNode.value?.id) {
          tasks.push(t)
        }
      })
    }
  })
  return tasks
})

// 监听拆解结果变化
watch(() => props.decomposeResult, (val) => {
  if (val && val.length > 0) {
    treeData.value = convertToTreeData(val)
  }
}, { immediate: true })

// 转换为树数据
const convertToTreeData = (data: any[]) => {
  return data.map((module, mIndex) => ({
    id: `module-${mIndex}`,
    type: 'module',
    name: module.name,
    description: module.description,
    sequenceNumber: module.sequenceNumber || `M${mIndex + 1}`,
    blockedBy: module.blockedBy ? (typeof module.blockedBy === 'string' ? module.blockedBy : JSON.stringify(module.blockedBy)) : null,
    blockedByArr: module.blockedBy ? (typeof module.blockedBy === 'string' ? JSON.parse(module.blockedBy) : module.blockedBy) : [],
    integrationTestSpecText: module.integrationTestSpec
      ? (typeof module.integrationTestSpec === 'string'
          ? module.integrationTestSpec
          : JSON.stringify(module.integrationTestSpec, null, 2))
      : '',
    apiIntegrationTestText: module.apiIntegrationTest
      ? (typeof module.apiIntegrationTest === 'string'
          ? module.apiIntegrationTest
          : JSON.stringify(module.apiIntegrationTest, null, 2))
      : '',
    webIntegrationTestText: module.webIntegrationTest
      ? (typeof module.webIntegrationTest === 'string'
          ? module.webIntegrationTest
          : JSON.stringify(module.webIntegrationTest, null, 2))
      : '',
    tasks: (module.tasks || []).map((task: any, tIndex: number) => ({
      id: `task-${mIndex}-${tIndex}`,
      type: 'task',
      name: task.name,
      description: task.description,
      sequenceNumber: task.sequenceNumber || `M${mIndex + 1}.${tIndex + 1}`,
      category: task.category || '功能',
      blockedBy: task.blockedBy ? (typeof task.blockedBy === 'string' ? task.blockedBy : JSON.stringify(task.blockedBy)) : null,
      blockedByArr: task.blockedBy ? (typeof task.blockedBy === 'string' ? JSON.parse(task.blockedBy) : task.blockedBy) : [],
      steps: task.steps || (task.stepsJson ? JSON.parse(task.stepsJson) : []),
      moduleId: `module-${mIndex}`
    }))
  }))
}

// 节点点击
const handleNodeClick = (data: any) => {
  selectedNode.value = data
}

// 开始编辑
const startEdit = (node: any) => {
  editingNode.value = node
}

// 添加任务到模块
const addTaskToModule = (module: any) => {
  if (!module.tasks) {
    module.tasks = []
  }
  const newTask = {
    id: `task-new-${Date.now()}`,
    type: 'task',
    name: '新任务',
    description: '',
    sequenceNumber: `${module.sequenceNumber}.${module.tasks.length + 1}`,
    category: '功能',
    blockedByArr: [],
    steps: [],
    moduleId: module.id
  }
  module.tasks.push(newTask)
  selectedNode.value = newTask
}

// 删除节点
const removeNode = async (node: any, treeNode: any) => {
  try {
    await ElMessageBox.confirm('确定删除该项吗？', '确认删除')
    const parent = treeNode.parent
    const children = parent.data.tasks || parent.data
    const index = children.findIndex((c: any) => c.id === node.id)
    if (index > -1) {
      children.splice(index, 1)
    }
    if (selectedNode.value?.id === node.id) {
      selectedNode.value = null
    }
  } catch {
    // 用户取消
  }
}

// 拖拽完成
const handleNodeDrop = () => {
  // 更新序号
  updateSequenceNumbers()
}

// 更新序号
const updateSequenceNumbers = () => {
  treeData.value.forEach((module, mIndex) => {
    module.sequenceNumber = `M${mIndex + 1}`
    if (module.tasks) {
      module.tasks.forEach((task: any, tIndex: number) => {
        task.sequenceNumber = `${module.sequenceNumber}.${tIndex + 1}`
      })
    }
  })
}

// 全部展开
const expandAll = () => {
  const nodes = treeRef.value?.store?.nodesMap
  if (nodes) {
    Object.values(nodes).forEach((node: any) => {
      node.expanded = true
    })
  }
}

// 全部折叠
const collapseAll = () => {
  const nodes = treeRef.value?.store?.nodesMap
  if (nodes) {
    Object.values(nodes).forEach((node: any) => {
      node.expanded = false
    })
  }
}

// 处理勾选
const handleCheck = () => {
  checkedNodes.value = treeRef.value?.getCheckedNodes() || []
}

// 批量删除
const batchDelete = async () => {
  if (checkedNodes.value.length === 0) return

  try {
    await ElMessageBox.confirm(`确定删除选中的 ${checkedNodes.value.length} 项吗？`, '确认删除')

    // 收集要删除的节点 ID
    const idsToDelete = new Set(checkedNodes.value.map(n => n.id))

    // 递归删除
    const removeById = (list: any[], ids: Set<string>) => {
      for (let i = list.length - 1; i >= 0; i--) {
        const item = list[i]
        if (ids.has(item.id)) {
          list.splice(i, 1)
        } else if (item.tasks) {
          removeById(item.tasks, ids)
        }
      }
    }

    removeById(treeData.value, idsToDelete)

    // 清空选择
    treeRef.value?.setCheckedKeys([])
    checkedNodes.value = []

    // 清空详情
    if (idsToDelete.has(selectedNode.value?.id)) {
      selectedNode.value = null
    }

    ElMessage.success('删除成功')
  } catch {
    // 用户取消
  }
}

// 保存
const handleSave = async () => {

  saving.value = true
  try {
    // 转换为层级结构，确保任务关联到模块
    const modulesWithTasks: any[] = []

    treeData.value.forEach((module) => {
      const moduleData = {
        name: module.name,
        description: module.description,
        sequenceNumber: module.sequenceNumber,
        blockedBy: module.blockedByArr.length > 0 ? JSON.stringify(module.blockedByArr) : null,
        integrationTestSpec: module.integrationTestSpecText || null,
        apiIntegrationTest: module.apiIntegrationTestText || null,
        webIntegrationTest: module.webIntegrationTestText || null,
        projectId: props.projectId,
        status: 0,
        tasks: (module.tasks || []).map((task: any) => ({
          name: task.name,
          description: task.description,
          sequenceNumber: task.sequenceNumber,
          category: task.category,
          blockedBy: task.blockedByArr && task.blockedByArr.length > 0
            ? JSON.stringify(task.blockedByArr) : null,
          stepsJson: task.steps && task.steps.length > 0
            ? JSON.stringify(task.steps) : null,
          projectId: props.projectId,
          status: 0
        }))
      }
      modulesWithTasks.push(moduleData)
    })

    const payload = { projectId: props.projectId, modulesWithTasks }

    const res = await ModuleApi.batchSave(props.projectId, modulesWithTasks)

    ElMessage.success('保存成功')
    emit('saved')
    handleClose()
  } catch (e) {
    console.error('[DecomposePreviewDialog] 保存失败:', e)
    ElMessage.error('保存失败')
  } finally {
    saving.value = false
  }
}

// 关闭
const handleClose = () => {
  visible.value = false
  selectedNode.value = null
  editingNode.value = null
}
</script>

<style scoped>
.decompose-preview-container {
  display: flex;
  gap: 16px;
  height: 70vh;
}

.preview-tree {
  width: 400px;
  border: 1px solid #e4e7ed;
  border-radius: 4px;
  overflow: auto;
}

.tree-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px;
  border-bottom: 1px solid #e4e7ed;
  font-weight: 500;
}

.tree-actions {
  display: flex;
  gap: 8px;
}

.tree-node {
  display: flex;
  align-items: center;
  gap: 4px;
  flex: 1;
}

.tree-icon {
  color: #409eff;
}

.tree-icon-task {
  color: #67c23a;
}

.tree-label {
  flex: 1;
}

.edit-input {
  width: 200px;
}

.preview-detail {
  flex: 1;
  border: 1px solid #e4e7ed;
  border-radius: 4px;
  padding: 16px;
  overflow: auto;
}

.detail-title {
  margin: 0 0 16px 0;
  padding-bottom: 8px;
  border-bottom: 1px solid #e4e7ed;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.test-tabs {
  width: 100%;
}
</style>
