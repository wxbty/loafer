<template>
  <el-row :gutter="20" class="module-task-container">
    <!-- 左侧：模块-任务目录树 -->
    <el-col :span="6">
      <el-card class="module-tree-card" shadow="hover">
        <template #header>
          <div class="flex w-full items-center justify-between">
            <div class="flex items-center">
              <el-icon><Grid /></el-icon>
              <span>模块任务</span>
            </div>
            <div class="flex items-center gap-2">
              <el-button
                v-if="moduleTreeData.length > 0"
                type="danger"
                size="small"
                @click="deleteAllModules"
              >
                删除全部
              </el-button>
              <el-button type="primary" size="small" @click="openDecomposeDialog">
                AI 拆解
              </el-button>
            </div>
          </div>
        </template>

        <div v-if="loading" class="text-center py-4">
          <el-icon class="is-loading"><Loading /></el-icon>
          <span class="ml-2">加载中...</span>
        </div>

        <el-tree
          v-else
          :data="moduleTreeData"
          :props="treeProps"
          node-key="id"
          highlight-current
          :expand-on-click-node="false"
          default-expand-all
          @node-click="handleNodeClick"
        >
          <template #default="scope">
            <span class="tree-node">
              <el-icon
                v-if="scope.data.type === 'module'"
                class="tree-icon tree-icon-expand"
                @click.stop="toggleNodeExpand(scope.node)"
              >
                <FolderOpened v-if="scope.node.expanded" />
                <Folder v-else />
              </el-icon>
              <el-icon v-else class="tree-icon tree-icon-task">
                <Document />
              </el-icon>
              <el-tooltip :content="scope.data.name" placement="top" :show-after="300">
                <span class="tree-label" @click.stop="handleNodeClick(scope.data)">{{ scope.data.name }}</span>
              </el-tooltip>
              <el-tag
                v-if="scope.data.type === 'module' && scope.data.moduleType === 'infrastructure'"
                type="warning"
                size="small"
                effect="plain"
                class="ml-1 flex-shrink-0"
              >
                基础架构
              </el-tag>
              <el-tag
                v-if="scope.data.type === 'module' && scope.data.moduleType !== 'infrastructure'"
                type="success"
                size="small"
                effect="plain"
                class="ml-1 flex-shrink-0"
              >
                业务
              </el-tag>
              <el-tag
                v-if="scope.data.status !== undefined"
                :type="getStatusType(scope.data.status)"
                size="small"
                class="ml-2 flex-shrink-0"
              >
                {{ getStatusText(scope.data.status, scope.data.type) }}
              </el-tag>
              <span v-if="scope.data.type === 'module'" class="task-count ml-2 flex-shrink-0">
                ({{ scope.data.taskCount || 0 }})
              </span>
            </span>
          </template>
        </el-tree>

        <el-empty v-if="!loading && moduleTreeData.length === 0" description="暂无模块" />
      </el-card>
    </el-col>

    <!-- 右侧：详情区 -->
    <el-col :span="18">
      <!-- 访问链接卡片 -->
      <el-card class="access-urls-card" shadow="hover" v-if="accessUrls">
        <template #header>
          <div class="flex items-center">
            <el-icon><Link /></el-icon>
            <span class="ml-2">访问链接</span>
            <el-tag
              v-if="accessUrls.testBaseUrlOverride"
              type="warning"
              size="small"
              effect="plain"
              class="ml-2"
            >
              后端 API 已自定义
            </el-tag>
          </div>
        </template>
        <el-descriptions :column="2" border size="small">
          <el-descriptions-item label="后端 API">
            <div class="flex items-center justify-between gap-2">
              <a :href="accessUrls.backendApi" target="_blank" class="text-blue-600 hover:underline break-all">
                {{ accessUrls.backendApi }}
              </a>
              <el-button size="small" link type="primary" @click="editTestBaseUrl">
                编辑
              </el-button>
            </div>
          </el-descriptions-item>
          <el-descriptions-item label="前端访问">
            <div class="flex items-center justify-between gap-2">
              <a :href="accessUrls.frontendDev" target="_blank" class="text-blue-600 hover:underline break-all">
                {{ accessUrls.frontendDev }}
              </a>
              <el-button size="small" link type="primary" @click="editFrontendUrl">
                编辑
              </el-button>
            </div>
          </el-descriptions-item>
        </el-descriptions>
      </el-card>

      <!-- 模块详情 -->
      <el-card v-if="selectedNode?.type === 'module'" class="detail-card" shadow="hover">
        <template #header>
          <div class="flex w-full items-center justify-between">
            <div class="flex items-center">
              <el-icon><FolderOpened /></el-icon>
              <span class="ml-1">{{ selectedModule?.name || '模块详情' }}</span>
              <el-tag
                v-if="isInfrastructureModule"
                type="warning"
                size="small"
                effect="plain"
                class="ml-2"
              >
                基础架构
              </el-tag>
              <el-tag
                v-else
                type="success"
                size="small"
                effect="plain"
                class="ml-2"
              >
                业务
              </el-tag>
            </div>
            <div class="flex items-center gap-2">
              <el-button size="small" type="primary" plain @click="openAddTaskDialog">
                <el-icon><Plus /></el-icon>
                添加任务
              </el-button>
              <el-button size="small" @click="editModule">编辑</el-button>
              <el-button size="small" type="danger" @click="deleteModule">删除</el-button>
            </div>
          </div>
        </template>

        <el-descriptions :column="2" border>
          <el-descriptions-item label="模块序号">{{ selectedModule?.sequenceNumber }}</el-descriptions-item>
          <el-descriptions-item label="模块类型">
            <el-tag :type="isInfrastructureModule ? 'warning' : 'success'" size="small" effect="plain">
              {{ isInfrastructureModule ? '基础架构（仅构建+启动校验）' : '业务（含API/Web/TDD测试）' }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="模块状态">
            <el-tag :type="getStatusType(selectedModule?.status)">
              {{ getStatusText(selectedModule?.status, 'module') }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="流水线模式">
            <el-tag :type="(selectedModule?.pipelineMode || 'LEGACY') === 'TDD' ? 'success' : 'info'" size="small">
              {{ selectedModule?.pipelineMode || 'LEGACY' }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="simpleMode">
            <el-tag :type="(selectedModule?.simpleMode === 1) ? 'warning' : 'info'" size="small">
              {{ selectedModule?.simpleMode === 1 ? '开（跳过集成测试）' : '关（执行集成测试）' }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="模块描述" :span="2">
            {{ selectedModule?.description || '—' }}
          </el-descriptions-item>
          <el-descriptions-item label="依赖模块" :span="2">
            <template v-if="selectedModule?.blockedBy">
              <el-tag
                v-for="dep in parseBlockedBy(selectedModule.blockedBy)"
                :key="dep"
                size="small"
                class="mr-1"
              >
                {{ dep }}
              </el-tag>
            </template>
            <span v-else class="text-slate-400">无依赖</span>
          </el-descriptions-item>
        </el-descriptions>

        <!-- 流水线模式切换：仅在状态待执行/测试失败/失败时显示；基础设施模块无此切换 -->
        <div class="mt-4" v-if="canSetModulePipelineMode && !isInfrastructureModule">
          <h4 class="text-sm font-medium text-slate-700 mb-2">流水线模式</h4>
          <div class="flex items-center gap-3 flex-wrap">
            <el-radio-group v-model="modulePipelineModeDraft" size="small">
              <el-radio-button label="LEGACY">LEGACY（task 跑完 → API+Web 集成测试）</el-radio-button>
              <el-radio-button label="TDD">TDD（5 阶段断言驱动）</el-radio-button>
            </el-radio-group>
            <el-checkbox
              v-if="modulePipelineModeDraft === 'LEGACY'"
              v-model="moduleSimpleModeDraft"
              size="small"
              title="勾选后所有 task 完成即视为模块完成，不跑 API/Web 集成测试。"
            >
              simpleMode：仅跑 task，不跑集成测试
            </el-checkbox>
            <el-button
              size="small"
              type="primary"
              :loading="modulePipelineSaving"
              :disabled="!modulePipelineDirty"
              @click="saveModulePipelineMode"
            >
              保存模式
            </el-button>
          </div>
          <p class="text-xs text-slate-400 mt-1">
            仅在「待执行 / 测试失败 / 失败」状态下允许切换；TDD 模式忽略 simpleMode。
          </p>
        </div>

        <!-- 基础架构模块验证面板：仅做构建校验 + 启动验证 -->
        <div v-if="isInfrastructureModule" class="mt-4">
          <el-alert
            type="info"
            :closable="false"
            show-icon
            title="基础架构模块"
            description="非业务模块，仅做构建校验和启动验证；不涉及 API/Web/TDD 集成测试。"
            class="mb-3"
          />
          <el-descriptions :column="2" border size="small">
            <el-descriptions-item label="构建校验">
              <el-tag :type="infraBuildStatus === 'passed' ? 'success' : (infraBuildStatus === 'failed' ? 'danger' : 'info')" size="small">
                {{ infraBuildStatusText }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="启动验证">
              <el-tag :type="infraStartupStatus === 'passed' ? 'success' : (infraStartupStatus === 'failed' ? 'danger' : 'info')" size="small">
                {{ infraStartupStatusText }}
              </el-tag>
            </el-descriptions-item>
          </el-descriptions>
          <div class="flex items-center gap-2 mt-3 flex-wrap">
            <el-button
              size="small"
              type="primary"
              plain
              :loading="infraVerifyLoading"
              @click="runInfraVerification"
            >
              <el-icon><VideoPlay /></el-icon>
              <span class="ml-1">运行构建 + 启动验证</span>
            </el-button>
            <span v-if="infraVerifyMessage" class="text-xs" :class="infraVerifyFailed ? 'text-red-600' : 'text-emerald-600'">
              {{ infraVerifyMessage }}
            </span>
          </div>
          <pre
            v-if="infraVerifyOutput"
            class="text-xs text-slate-500 mt-2 max-h-64 overflow-auto whitespace-pre-wrap bg-slate-50 p-2 rounded"
          >{{ infraVerifyOutput }}</pre>
        </div>

        <!-- 集成测试：仅业务模块显示 -->
        <div class="mt-4" v-if="!isInfrastructureModule">
          <div class="flex items-center justify-between mb-2">
            <h4 class="text-sm font-medium text-slate-700">集成测试</h4>
            <div class="flex items-center gap-2">
              <!-- LEGACY 的 API/Web 集成测试编辑器无单独保存入口，这里提供「保存测试」持久化编辑 -->
              <el-button
                v-if="modulePipelineModeDraft === 'LEGACY'"
                size="small"
                type="success"
                :loading="testSaving"
                :disabled="testGenLoading"
                @click="saveTestSpec"
              >
                保存测试
              </el-button>
              <el-button
                size="small"
                type="primary"
                plain
                :loading="testGenLoading && testGenMode === 'LEGACY'"
                :disabled="testGenLoading"
                @click="openGenerateTestDialog('LEGACY')"
              >
                AI 生成 API+Web 用例
              </el-button>
              <el-button
                size="small"
                plain
                :loading="testGenLoading && testGenMode === 'TDD'"
                :disabled="testGenLoading"
                @click="openGenerateTestDialog('TDD')"
              >
                AI 生成 TDD 验收标准
              </el-button>
            </div>
          </div>
          <el-tabs v-model="activeTestTab" type="border-card" class="test-tabs">
            <!-- LEGACY 模式：显示 API 和 Web 集成测试 -->
            <template v-if="modulePipelineModeDraft === 'LEGACY'">
              <el-tab-pane label="API集成测试" name="api">
                <IntegrationTestEditor
                  v-model="apiIntegrationTestText"
                  :module-id="selectedModule?.id"
                  :project-id="props.projectId"
                  scenario-type="api"
                  @module-updated="onModuleSpecUpdated"
                />
              </el-tab-pane>
              <el-tab-pane label="Web集成测试" name="web">
                <IntegrationTestEditor
                  v-model="webIntegrationTestText"
                  :module-id="selectedModule?.id"
                  :project-id="props.projectId"
                  scenario-type="web"
                  @module-updated="onModuleSpecUpdated"
                />
              </el-tab-pane>
            </template>
            <!-- TDD 模式：只显示 TDD 验收标准 -->
            <template v-else>
              <el-tab-pane label="TDD 验收标准 (acceptanceCriteria)" name="tdd">
              <!-- 工具栏 -->
              <div class="flex items-center gap-2 mb-3 flex-wrap">
                <el-button
                  v-if="parsedTddSpec?.acceptanceCriteria?.length && isTddTestSpecSaved"
                  type="success"
                  size="small"
                  :loading="tddRunAllLoading"
                  :disabled="tddFixAllLoading"
                  @click="runAllTddAssertions"
                >
                  <el-icon><VideoPlay /></el-icon>
                  <span class="ml-1">运行全部测试</span>
                </el-button>
                <el-button
                  v-if="parsedTddSpec?.acceptanceCriteria?.length && isTddTestSpecSaved"
                  type="warning"
                  size="small"
                  :loading="tddFixAllLoading"
                  :disabled="tddRunAllLoading || tddRedAssertionCount === 0"
                  @click="fixAllRedAssertions"
                >
                  <el-icon><Tools /></el-icon>
                  <span class="ml-1">修复全部{{ tddRedAssertionCount > 0 ? `（${tddRedAssertionCount} 红）` : '' }}</span>
                </el-button>
                <el-button
                  v-if="parsedTddSpec?.acceptanceCriteria?.length"
                  size="small"
                  @click="openFixHistoryDialog"
                >
                  <el-icon><Clock /></el-icon>
                  <span class="ml-1">修复记录</span>
                </el-button>
                <el-button
                  v-if="!isTddTestSpecSaved"
                  type="primary"
                  size="small"
                  :disabled="!isTddTestSpecDirty"
                  @click="saveTestSpec"
                >保存测试</el-button>
                <span v-if="isTddTestSpecDirty" class="text-xs text-amber-600">草稿未保存</span>
                <span v-else-if="isTddTestSpecSaved" class="text-xs text-emerald-600">
                  已保存{{ tddAssertionsCount > 0 ? ` · 已编译 ${tddAssertionsCount} 条断言` : '（首次「运行全部」会调 TestAuthor 编译断言）' }}
                </span>
                <span class="flex-1"></span>
                <el-button size="small" link @click="showTddRawEditor = !showTddRawEditor">
                  {{ showTddRawEditor ? '收起原始 JSON' : '编辑原始 JSON' }}
                </el-button>
              </div>

              <!-- 结构化卡片 -->
              <div v-if="parsedTddSpec?.acceptanceCriteria?.length" class="tdd-criteria-list">
                <div
                  v-for="(c, ci) in parsedTddSpec.acceptanceCriteria"
                  :key="(c.id || '') + '-' + ci"
                  class="tdd-criterion-card"
                  :class="tddCardClass(c.id)"
                >
                  <div class="tdd-criterion-header">
                    <el-tag size="small" effect="dark">{{ c.id || '—' }}</el-tag>
                    <el-tag :type="tddBadgeType(c.id)" size="small">{{ tddBadgeText(c.id) }}</el-tag>
                    <el-tooltip
                      :content="executorTooltip(c)"
                      placement="top"
                    >
                      <el-tag :type="executorTagType(c)" size="small" effect="plain">
                        {{ executorTagLabel(c) }}
                      </el-tag>
                    </el-tooltip>
                    <el-tooltip
                      v-if="missingHttpMainAssertion(c)"
                      content="When 子句里有 HTTP 动词，但已编译的断言只检查了文件存在，没有真实调用接口。请点「运行全部测试」让 TestAuthor 重编译（后端会按 when 兜底注入 http_status 主断言）。"
                      placement="top"
                    >
                      <el-tag type="danger" size="small" effect="dark">⚠ 缺 HTTP 主断言</el-tag>
                    </el-tooltip>
                    <span class="tdd-criterion-behavior">{{ c.behavior || '（未填写 behavior）' }}</span>
                    <span class="flex-1"></span>
                    <el-select
                      :model-value="effectiveTaskRefs(c)"
                      size="small"
                      placeholder="关联任务（可多选）"
                      class="tdd-task-select"
                      :disabled="tddTaskRefSaving === c.id"
                      :loading="tddTaskRefSaving === c.id"
                      multiple
                      collapse-tags
                      collapse-tags-tooltip
                      filterable
                      clearable
                      @change="(v) => updateCriterionTaskRefs(c.id, v as string[])"
                    >
                      <el-option
                        v-for="t in moduleTasks"
                        :key="t.id"
                        :label="`${t.sequenceNumber || ''} ${t.name || ''}`.trim()"
                        :value="t.sequenceNumber || ''"
                      />
                    </el-select>
                    <el-button
                      size="small"
                      link
                      :disabled="!hasAssertionFor(c.id) || tddRunSingleLoading === c.id || tddRunAllLoading || tddFixAllLoading || tddFixSingleLoading === c.id || tddRegenerateLoading === c.id"
                      :loading="tddRunSingleLoading === c.id"
                      @click="runSingleTddAssertion(c.id)"
                    >重跑</el-button>
                    <el-button
                      size="small"
                      link
                      :disabled="tddRunAllLoading || tddFixAllLoading || tddRunSingleLoading === c.id || tddFixSingleLoading === c.id || tddRegenerateLoading === c.id"
                      @click="openRegenerateCriterionDialog(c.id)"
                    >重新生成</el-button>
                    <el-button
                      v-if="criterionIsRed(c.id)"
                      size="small"
                      type="warning"
                      link
                      :disabled="tddFixAllLoading || tddRunAllLoading || tddRunSingleLoading === c.id || tddFixSingleLoading === c.id || tddRegenerateLoading === c.id"
                      :loading="tddFixSingleLoading === c.id"
                      @click="fixSingleRedAssertion(c.id)"
                    >修复</el-button>
                  </div>
                  <div class="tdd-criterion-body">
                    <div><span class="tdd-criterion-label">Given:</span> {{ c.given || '—' }}</div>
                    <div><span class="tdd-criterion-label">When:</span> {{ c.when || '—' }}</div>
                    <div><span class="tdd-criterion-label">Then:</span> {{ c.then || '—' }}</div>
                    <div v-if="assertionExecutableFor(c.id)" class="tdd-criterion-detail">
                      <span class="tdd-criterion-label">编译为:</span>
                      <pre class="tdd-criterion-detail-pre">{{ assertionExecutableFor(c.id) }}</pre>
                    </div>
                    <div v-if="(c.dependsOn || []).length" class="tdd-criterion-detail">
                      <span class="tdd-criterion-label">前置:</span>
                      <code class="text-xs">{{ (c.dependsOn || []).join(' → ') }} (setup)</code>
                    </div>
                    <div v-if="lastDetailFor(c.id)" class="tdd-criterion-detail">
                      <span class="tdd-criterion-label">最近输出:</span>
                      <pre class="tdd-criterion-detail-pre">{{ lastDetailFor(c.id) }}</pre>
                    </div>
                    <!-- v3: 自动重试关联任务已下线；taskRef 仅保留作为参考信息（顶部下拉框可手动改） -->
                    <div v-if="effectiveTaskRefs(c).length" class="tdd-criterion-footer">
                      <span class="text-xs text-slate-500">
                        参考关联任务: {{ effectiveTaskRefs(c).join('、') }}
                        <span class="text-slate-400 ml-1">（修复请点上方「修复」按钮，不再自动触发任务重试）</span>
                      </span>
                    </div>
                  </div>
                </div>
              </div>

              <!-- 空态 -->
              <el-empty
                v-else-if="!tddTestSpecText?.trim()"
                :image-size="80"
                description="请先点击「AI 生成 TDD 验收标准」生成结构化测试用例"
              />

              <!-- JSON 解析失败兜底 -->
              <el-alert
                v-else-if="!parsedTddSpec"
                type="warning"
                :closable="false"
                show-icon
                title="无法解析 TDD JSON，请在下方原始 JSON 编辑器中修复或重新生成。"
              />

              <!-- 原始 JSON 编辑器（默认收起，解析失败时强制展开） -->
              <div v-if="showTddRawEditor || !parsedTddSpec" class="mt-3">
                <el-input
                  v-model="tddTestSpecText"
                  type="textarea"
                  :rows="14"
                  placeholder='{"acceptanceCriteria":[{"id":"A1","taskRef":"T1.2","behavior":"...","given":"...","when":"...","then":"..."}]}'
                />
              </div>

              <!-- 运行日志输出 -->
              <pre
                v-if="tddRunOutput"
                class="text-xs text-slate-500 mt-2 max-h-64 overflow-auto whitespace-pre-wrap bg-slate-50 p-2 rounded"
              >{{ tddRunOutput }}</pre>

              <!-- 修复日志输出（独立栏，避免与运行日志混淆） -->
              <div v-if="tddFixOutput" class="mt-2">
                <div class="text-xs text-amber-700 font-medium mb-1">修复 Agent 日志</div>
                <pre class="text-xs text-slate-600 max-h-64 overflow-auto whitespace-pre-wrap bg-amber-50 p-2 rounded">{{ tddFixOutput }}</pre>
              </div>

              <!-- 重新生成日志输出 -->
              <div v-if="tddRegenerateOutput" class="mt-2">
                <div class="text-xs text-blue-700 font-medium mb-1">重新生成日志</div>
                <pre class="text-xs text-slate-600 max-h-64 overflow-auto whitespace-pre-wrap bg-blue-50 p-2 rounded">{{ tddRegenerateOutput }}</pre>
              </div>
              </el-tab-pane>
            </template>
          </el-tabs>
          <pre
            v-if="testGenOutput"
            class="text-xs text-slate-500 mt-2 max-h-48 overflow-auto whitespace-pre-wrap bg-slate-50 p-2 rounded"
          >{{ testGenOutput }}</pre>
        </div>
      </el-card>

      <!-- 任务详情 -->
      <el-card v-else-if="selectedNode?.type === 'task'" class="detail-card" shadow="hover">
        <template #header>
          <div class="flex w-full items-center justify-between">
            <div class="flex items-center">
              <el-icon><Document /></el-icon>
              <span>{{ selectedTask?.name || '任务详情' }}</span>
            </div>
            <div class="flex items-center gap-2">
              <el-button
                v-if="selectedTask?.status === 0"
                size="small"
                type="success"
                @click="startTask"
              >
                启动
              </el-button>
              <el-button
                v-if="selectedTask?.status === 1"
                size="small"
                type="warning"
                @click="pauseTask"
              >
                暂停
              </el-button>
              <el-button
                v-if="selectedTask?.status === 4"
                size="small"
                type="success"
                @click="resumeTask"
              >
                恢复
              </el-button>
              <el-checkbox
                v-if="selectedTask?.status === 5 || selectedTask?.status === 3"
                v-model="retrySkipPassed"
                size="small"
                title="勾选后，重试时跳过上一次已通过的步骤，只重跑未通过/未执行的步骤"
              >
                跳过已通过步骤
              </el-checkbox>
              <el-button
                v-if="selectedTask?.status === 5 || selectedTask?.status === 3"
                size="small"
                type="warning"
                @click="retryTask"
              >
                重试
              </el-button>
              <el-button size="small" @click="editTask">编辑</el-button>
            </div>
          </div>
        </template>

        <el-descriptions :column="2" border>
          <el-descriptions-item label="任务序号">{{ selectedTask?.sequenceNumber }}</el-descriptions-item>
          <el-descriptions-item label="任务状态">
            <el-tag :type="getTaskStatusType(selectedTask?.status)">
              {{ getTaskStatusText(selectedTask?.status) }}
            </el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="所属模块">{{ selectedModule?.name || '—' }}</el-descriptions-item>
          <el-descriptions-item label="任务分类">{{ selectedTask?.category || '—' }}</el-descriptions-item>
          <el-descriptions-item label="任务描述" :span="2">
            {{ selectedTask?.description || '—' }}
          </el-descriptions-item>
          <el-descriptions-item label="依赖任务" :span="2">
            <template v-if="selectedTask?.blockedBy">
              <el-tag
                v-for="dep in parseBlockedBy(selectedTask.blockedBy)"
                :key="dep"
                size="small"
                class="mr-1"
              >
                {{ dep }}
              </el-tag>
            </template>
            <span v-else class="text-slate-400">无依赖</span>
          </el-descriptions-item>
        </el-descriptions>

        <!-- 步骤详情（任务级仅 Dev → Deploy，功能验证已上提到模块层） -->
        <div class="mt-4" v-if="taskSteps.length > 0">
          <h4 class="text-sm font-medium text-slate-700 mb-2">执行步骤</h4>
          <el-timeline>
            <el-timeline-item
              v-for="(step, index) in taskSteps"
              :key="index"
              :type="getStepStatusType(step.status)"
              :hollow="step.status !== 'completed' && step.status !== 'passed'"
            >
              <div class="step-item">
                <div class="step-header">
                  <span class="step-seq">步骤 {{ step.seq || index + 1 }}</span>
                  <el-tag :type="getStepStatusType(step.status)" size="small">
                    {{ step.status || '待执行' }}
                  </el-tag>
                  <el-tooltip
                    v-if="step.manualOverride"
                    :content="`人工介入 ${step.manualOverrideAt || ''}：${step.manualNote || ''}`"
                    placement="top"
                  >
                    <el-tag type="info" size="small" effect="plain">人工通过</el-tag>
                  </el-tooltip>
                  <span class="step-header-spacer"></span>
                  <!-- 编辑/删除按钮：仅待办/暂停/失败状态可操作 -->
                  <el-button
                    v-if="canEditStep()"
                    size="small"
                    type="primary"
                    link
                    @click="openEditStepDialog(step, index)"
                  >
                    编辑
                  </el-button>
                  <el-button
                    v-if="canEditStep()"
                    size="small"
                    type="danger"
                    link
                    @click="deleteStep(step, index)"
                  >
                    删除
                  </el-button>
                </div>
                <div class="step-action">{{ step.action }}</div>
                <div v-if="step.planExcerpt || step.plan_excerpt" class="step-plan-excerpt">
                  <span class="text-amber-600 font-medium">Plan 引用：</span>
                  <blockquote class="plan-excerpt-content">{{ step.planExcerpt || step.plan_excerpt }}</blockquote>
                </div>
                <div v-if="step.files" class="step-files">
                  <span class="text-slate-500">涉及文件：</span>
                  <code>{{ step.files.join(', ') }}</code>
                </div>
                <div v-if="step.migrationFile || step.migration_file" class="step-migration">
                  <span class="text-slate-500">迁移文件：</span>
                  <code>{{ step.migrationFile || step.migration_file }}</code>
                </div>
                <div v-if="step.validation" class="step-validation">
                  <span class="text-slate-500">验证标准：</span>
                  {{ step.validation }}
                </div>
                <div v-if="step.devSummary" class="step-validation">
                  <span class="text-slate-500">Dev 摘要：</span>
                  {{ step.devSummary }}
                </div>
              </div>
            </el-timeline-item>
          </el-timeline>
        </div>

        <!-- 执行摘要 -->
        <div class="mt-4" v-if="selectedTask?.status === 1 || taskState?.executionSummary || selectedTask?.status === 3 || selectedTask?.status === 5">
          <div class="flex items-center justify-between mb-2">
            <h4 class="text-sm font-medium text-slate-700">
              执行摘要
              <span v-if="executionSummaryUpdatedAt" class="text-xs text-slate-400 font-normal ml-2">
                最近刷新 {{ executionSummaryUpdatedAt }}
              </span>
            </h4>
            <el-button
              v-if="selectedTask?.status === 1"
              size="small"
              :loading="executionSummaryRefreshing"
              @click="refreshExecutionSummary"
            >
              <el-icon v-if="!executionSummaryRefreshing"><Refresh /></el-icon>
              <span class="ml-1">刷新</span>
            </el-button>
          </div>
          <div v-if="taskState?.executionSummary">
            <div
              ref="executionSummaryRef"
              class="execution-summary-pre"
              :class="{
                'border-green-300': selectedTask?.status === 3,
                'border-red-300': selectedTask?.status === 5,
                'border-blue-300': selectedTask?.status === 1
              }"
            >
              <template v-for="(seg, idx) in executionSummarySegments" :key="idx">
                <pre v-if="seg.type === 'text'" class="exec-summary-text">{{ seg.text }}</pre>
                <div v-else class="deploy-detail-block">
                  <div
                    class="deploy-detail-header"
                    :class="{ 'is-fail': seg.status === 'fail' }"
                    @click="toggleDeployBlock(idx)"
                  >
                    <span class="deploy-caret">{{ deployExpanded.has(idx) ? '▼' : '▶' }}</span>
                    Deploy 详情 · step {{ seg.step }} · 第 {{ seg.attempt }} 轮
                    <span class="deploy-meta">（{{ seg.lines.length }} 行）</span>
                    <span v-if="seg.badges.length" class="deploy-badges">
                      <span
                        v-for="(b, bi) in seg.badges"
                        :key="bi"
                        class="deploy-badge"
                        :class="b.kind"
                      >{{ b.text }}</span>
                    </span>
                    <span class="deploy-hint">Ctrl+O 全部展开/收起</span>
                  </div>
                  <pre v-show="deployExpanded.has(idx)" class="deploy-detail-body">{{ seg.lines.join('\n') }}</pre>
                </div>
              </template>
            </div>
          </div>
          <div v-else-if="selectedTask?.status === 1" class="text-slate-400 text-sm">
            任务执行中，暂无执行摘要，请点击右上角「刷新」按钮获取最新进度
          </div>
          <div v-else class="text-slate-400 text-sm">
            暂无执行摘要
          </div>
        </div>

        <!-- 当前聚焦 -->
        <div class="mt-4" v-if="taskState?.currentFocus && taskState.currentFocus.trim()">
          <h4 class="text-sm font-medium text-slate-700 mb-2">当前聚焦</h4>
          <el-tag type="primary" size="small">{{ taskState.currentFocus }}</el-tag>
        </div>

      </el-card>

      <!-- 未选中 -->
      <el-card v-else class="detail-card" shadow="hover">
        <el-empty description="请选择模块或任务查看详情" />
      </el-card>
    </el-col>
  </el-row>

  <!-- 拆解对话框 -->
  <el-dialog
    v-model="decomposeDialogVisible"
    title="AI 模块化拆解"
    width="700px"
    destroy-on-close
  >
    <el-form :model="decomposeForm" label-width="100px">
      <el-form-item label="需求描述" required>
        <el-input
          v-model="decomposeForm.requirementDescription"
          type="textarea"
          :rows="4"
          placeholder="请输入需求描述（与需求文档二选一）"
          :disabled="decomposeForm.selectedPlanFiles.length > 0"
        />
      </el-form-item>
      <el-form-item label="需求文档">
        <div class="plan-files-container">
          <el-select
            v-model="decomposeForm.selectedPlanFiles"
            multiple
            filterable
            placeholder="选择需求文档（与需求描述二选一）"
            style="width: 100%"
            :loading="planFilesLoading"
            :disabled="!!decomposeForm.requirementDescription"
          >
            <el-option
              v-for="doc in planFiles"
              :key="doc.path"
              :label="doc.path"
              :value="doc.absolutePath"
            >
              <div class="plan-option">
                <span class="plan-name">{{ doc.name }}</span>
                <span class="plan-path">{{ doc.path }}</span>
              </div>
            </el-option>
          </el-select>
          <el-button
            type="primary"
            link
            :loading="planFilesLoading"
            :disabled="!!decomposeForm.requirementDescription"
            @click="loadPlanFiles"
          >
            <el-icon><Refresh /></el-icon>
            刷新
          </el-button>
        </div>
        <div v-if="decomposeForm.selectedPlanFiles.length > 0" class="selected-files">
          <el-tag
            v-for="path in decomposeForm.selectedPlanFiles"
            :key="path"
            closable
            size="small"
            class="mr-1 mb-1"
            @close="removePlanFile(path)"
          >
            {{ getFileName(path) }}
          </el-tag>
        </div>
      </el-form-item>
      <el-form-item>
        <div class="text-amber-500 text-sm">
          <el-icon><Warning /></el-icon>
          <span>需求描述和需求文档必须填写其中一项，不能同时为空或同时填写</span>
        </div>
      </el-form-item>
      <el-form-item label="拆解选项">
        <el-checkbox v-model="decomposeForm.generateApiTest">生成API集成测试</el-checkbox>
        <el-checkbox v-model="decomposeForm.generateWebTest">生成Web集成测试</el-checkbox>
        <el-checkbox v-model="decomposeForm.autoConfig">自动配置测试环境</el-checkbox>
      </el-form-item>
    </el-form>

    <!-- 拆解输出 -->
    <div v-if="decomposeOutput" class="decompose-output">
      <el-collapse>
        <el-collapse-item title="执行日志" name="log">
          <pre class="output-log">{{ decomposeOutput }}</pre>
        </el-collapse-item>
      </el-collapse>
    </div>

    <template #footer>
      <el-button @click="decomposeDialogVisible = false">取消</el-button>
      <el-button
        type="primary"
        :loading="decomposeLoading"
        :disabled="!decomposeForm.requirementDescription.trim() && decomposeForm.selectedPlanFiles.length === 0"
        @click="startDecompose"
      >
        开始拆解
      </el-button>
    </template>
  </el-dialog>

  <!-- 预览编辑对话框 -->
  <DecomposePreviewDialog
    v-model="previewDialogVisible"
    :project-id="projectId"
    :decompose-result="decomposeResult"
    @saved="onDecomposeSaved"
  />

  <!-- 给模块追加任务 -->
  <AddTaskDialog
    v-model="addTaskDialogVisible"
    :module-id="selectedModule?.id ?? null"
    :module-name="selectedModule?.name"
    :module-sequence="selectedModule?.sequenceNumber"
    :project-id="projectId"
    :existing-tasks="siblingTaskOptions"
    @saved="onTasksAppended"
  />

  <!-- 编辑单个步骤对话框 -->
  <el-dialog
    v-model="editStepDialogVisible"
    :title="`编辑步骤 ${editingStepData?.seq || editingStepIndex + 1}`"
    width="760px"
    destroy-on-close
  >
    <el-form v-if="editingStepData" label-width="80px" size="small">
      <el-form-item label="操作描述" required>
        <el-input v-model="editingStepData.action" placeholder="如：创建 AuthController" />
      </el-form-item>
      <el-form-item label="Plan 引用">
        <el-input
          v-model="editingStepData.planExcerpt"
          type="textarea"
          :rows="3"
          placeholder="从 Plan.md 中提取的相关需求片段原文"
        />
      </el-form-item>
      <el-form-item label="涉及文件">
        <el-select
          v-model="editingStepData.files"
          multiple
          filterable
          allow-create
          default-first-option
          placeholder="输入文件路径后回车"
        />
      </el-form-item>
      <el-form-item label="迁移文件">
        <el-input v-model="editingStepData.migrationFile" placeholder="数据库迁移文件路径（可选）" />
      </el-form-item>
      <el-form-item label="验证标准">
        <el-input v-model="editingStepData.validation" placeholder="如：编译通过，API 可访问" />
      </el-form-item>
      <el-form-item label="事实校验">
        <div class="fact-checks-block">
          <div v-if="!editingStepData.factChecks || editingStepData.factChecks.length === 0" class="fact-checks-empty">
            未规划事实校验
          </div>
          <ul v-else class="fact-checks-list">
            <li v-for="(fc, fcIdx) in editingStepData.factChecks" :key="fcIdx" class="fact-checks-item">
              <el-tag size="small" :type="getFactCheckTagType(fc.type)">
                {{ fc.type }}
              </el-tag>
              <span class="fact-check-label">{{ formatFactCheck(fc) }}</span>
              <el-button size="small" link type="primary" @click="editFactCheck(fcIdx)">编辑</el-button>
              <el-button size="small" link type="danger" @click="removeFactCheck(fcIdx)">删除</el-button>
            </li>
          </ul>
          <el-button size="small" type="primary" link @click="addFactCheck">添加校验项</el-button>
        </div>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="editStepDialogVisible = false">取消</el-button>
      <el-button type="primary" :loading="savingStep" @click="saveEditedStep">保存</el-button>
    </template>
  </el-dialog>

  <!-- 事实校验项编辑对话框 -->
  <el-dialog
    v-model="factCheckDialogVisible"
    title="编辑事实校验项"
    width="500px"
    destroy-on-close
  >
    <el-form :model="factCheckDraft" label-width="100px" size="small">
      <el-form-item label="类型">
        <el-select v-model="factCheckDraft.type" placeholder="选择校验类型">
          <el-option label="文件存在 (file_exists)" value="file_exists" />
          <el-option label="Shell命令 (shell)" value="shell" />
          <el-option label="HTTP状态 (http_status)" value="http_status" />
          <el-option label="Git提交 (git_commit)" value="git_commit" />
        </el-select>
      </el-form-item>
      <!-- file_exists -->
      <el-form-item v-if="factCheckDraft.type === 'file_exists'" label="文件路径">
        <el-input v-model="factCheckDraft.path" placeholder="绝对路径，如 /abs/path/file.java" />
      </el-form-item>
      <!-- shell -->
      <el-form-item v-if="factCheckDraft.type === 'shell'" label="Shell命令">
        <el-input v-model="factCheckDraft.command" type="textarea" :rows="2" placeholder="如 mvn test -Dtest=xxx" />
      </el-form-item>
      <el-form-item v-if="factCheckDraft.type === 'shell'" label="期望退出码">
        <el-input-number v-model="factCheckDraft.expectedExitCode" :min="0" :max="255" />
      </el-form-item>
      <!-- http_status -->
      <el-form-item v-if="factCheckDraft.type === 'http_status'" label="HTTP方法">
        <el-select v-model="factCheckDraft.method">
          <el-option label="GET" value="GET" />
          <el-option label="POST" value="POST" />
          <el-option label="PUT" value="PUT" />
          <el-option label="DELETE" value="DELETE" />
        </el-select>
      </el-form-item>
      <el-form-item v-if="factCheckDraft.type === 'http_status'" label="URL">
        <el-input v-model="factCheckDraft.url" placeholder="如 http://localhost:8080/api/test" />
      </el-form-item>
      <el-form-item v-if="factCheckDraft.type === 'http_status'" label="期望状态码">
        <el-input-number v-model="factCheckDraft.expectedStatus" :min="100" :max="599" />
      </el-form-item>
      <!-- git_commit -->
      <el-form-item v-if="factCheckDraft.type === 'git_commit'" label="Commit Hash">
        <el-input v-model="factCheckDraft.commitHash" placeholder="可选，Tester执行时会填入" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="factCheckDialogVisible = false">取消</el-button>
      <el-button type="primary" @click="saveFactCheck">保存</el-button>
    </template>
  </el-dialog>

  <!-- AI 生成测试用例 -->
  <el-dialog
    v-model="testGenDialogVisible"
    :title="testGenDialogMode === 'TDD' ? 'AI 生成 TDD 验收标准' : 'AI 生成 API+Web 集成测试用例'"
    width="480px"
    :close-on-click-modal="false"
    append-to-body
  >
    <div class="text-xs text-slate-500">
      AI 会阅读该模块已实现代码（后端路由/handler、前端页面与 api 调用）后生成测试用例。
      用例将覆盖模块全部核心 API 的正常与边界情况。
    </div>

    <template #footer>
      <el-button @click="testGenDialogVisible = false">取消</el-button>
      <el-button
        type="primary"
        :loading="testGenLoading"
        @click="confirmGenerateModuleTest"
      >
        开始生成
      </el-button>
    </template>
  </el-dialog>

  <!-- 修复记录弹框（模块 TDD 手动修复历史） -->
  <el-dialog
    v-model="fixHistoryDialogVisible"
    title="修复记录"
    width="720px"
    destroy-on-close
  >
    <div v-loading="fixHistoryLoading" style="min-height: 200px">
      <el-empty v-if="!fixHistoryLoading && fixHistoryList.length === 0"
                description="暂无修复记录" :image-size="80" />
      <el-table v-else :data="fixHistoryList" size="small" stripe>
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column label="范围" width="80">
          <template #default="{ row }">
            <el-tag :type="row.scope === 'ALL' ? 'warning' : 'info'" size="small">
              {{ row.scope === 'ALL' ? '全部' : '单条' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="涉及 criterion" min-width="180">
          <template #default="{ row }">
            <span class="text-xs">{{ formatCriteriaIds(row.criteriaIds) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="fixStatusTag(row.status)" size="small">{{ fixStatusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="开始时间" width="160">
          <template #default="{ row }">
            <span class="text-xs">{{ formatDateTime(row.startedAt) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="80">
          <template #default="{ row }">
            <el-button size="small" link @click="openFixHistoryDetail(row)">详情</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>
    <template #footer>
      <el-button @click="fixHistoryDialogVisible = false">关闭</el-button>
      <el-button type="primary" :loading="fixHistoryLoading" @click="reloadFixHistory">刷新</el-button>
    </template>
  </el-dialog>

  <!-- 修复记录详情弹框 -->
  <el-dialog
    v-model="fixHistoryDetailVisible"
    :title="`修复记录 #${fixHistoryDetail?.id || ''}`"
    width="560px"
    destroy-on-close
  >
    <div v-if="fixHistoryDetail" class="space-y-2 text-sm">
      <div>
        <span class="text-slate-500 mr-2">范围:</span>
        <el-tag :type="fixHistoryDetail.scope === 'ALL' ? 'warning' : 'info'" size="small">
          {{ fixHistoryDetail.scope === 'ALL' ? '全部红用例' : '单条 criterion' }}
        </el-tag>
      </div>
      <div>
        <span class="text-slate-500 mr-2">涉及 criterion id:</span>
        <code class="text-xs">{{ formatCriteriaIds(fixHistoryDetail.criteriaIds) || '—' }}</code>
      </div>
      <div>
        <span class="text-slate-500 mr-2">状态:</span>
        <el-tag :type="fixStatusTag(fixHistoryDetail.status)" size="small">{{ fixStatusLabel(fixHistoryDetail.status) }}</el-tag>
      </div>
      <div>
        <span class="text-slate-500 mr-2">开始:</span>
        <span class="text-xs">{{ formatDateTime(fixHistoryDetail.startedAt) }}</span>
      </div>
      <div>
        <span class="text-slate-500 mr-2">结束:</span>
        <span class="text-xs">{{ formatDateTime(fixHistoryDetail.completedAt) || '（进行中）' }}</span>
      </div>
      <div v-if="fixHistoryDetail.fixSummary">
        <div class="text-slate-500 mb-1">修复摘要:</div>
        <pre class="text-xs text-green-700 bg-green-50 p-2 rounded whitespace-pre-wrap">{{ fixHistoryDetail.fixSummary }}</pre>
      </div>
      <div v-if="fixHistoryDetail.errorMessage">
        <div class="text-slate-500 mb-1">错误信息:</div>
        <pre class="text-xs text-red-600 bg-red-50 p-2 rounded whitespace-pre-wrap">{{ fixHistoryDetail.errorMessage }}</pre>
      </div>
      <div class="text-xs text-slate-400 mt-2">
        提示：本表只记录修复范围与时间。Claude 的完整提示词与输出可在 LLM 调用日志页面按 callType=MODULE_INTEGRATION_TEST_FIX 查询。
      </div>
    </div>
    <template #footer>
      <el-button @click="fixHistoryDetailVisible = false">关闭</el-button>
    </template>
  </el-dialog>

  <!-- 单条 criterion 重新生成弹框：提示词变量选择 -->
  <el-dialog
    v-model="regenerateCriterionDialogVisible"
    :title="`重新生成 ${regenerateCriterionId || ''}`"
    width="720px"
    :close-on-click-modal="false"
    append-to-body
  >
    <div class="text-xs text-slate-500 mb-3">
      勾选要注入到提示词的变量；AI 会根据原有 criterion 内容和这些真实值重新生成该条验收标准。
    </div>

    <!-- 部署访问 URL（只读预览） -->
    <el-descriptions
      :column="1"
      size="small"
      border
      class="mb-3"
      v-loading="regenerateCriterionAccessUrlsLoading"
    >
      <template #title>
        <span class="text-sm font-medium">将注入的部署访问 URL（只读）</span>
      </template>
      <el-descriptions-item label="前端 URL">
        <code class="text-xs">{{ regenerateCriterionAccessUrls?.frontendDev || '（未配置）' }}</code>
      </el-descriptions-item>
      <el-descriptions-item label="后端 URL">
        <code class="text-xs">{{ regenerateCriterionAccessUrls?.backendApi || '（未配置）' }}</code>
      </el-descriptions-item>
    </el-descriptions>

    <!-- 项目提示词变量选择 -->
    <div class="flex items-center justify-between mb-2">
      <span class="text-sm font-medium">项目提示词变量</span>
      <div class="flex items-center gap-2">
        <el-button
          size="small"
          link
          :disabled="!regenerateCriterionPromptVars.length"
          @click="toggleSelectAllRegeneratePromptVars(true)"
        >全选</el-button>
        <el-button
          size="small"
          link
          :disabled="!regenerateCriterionPromptVars.length"
          @click="toggleSelectAllRegeneratePromptVars(false)"
        >全不选</el-button>
      </div>
    </div>
    <el-empty
      v-if="!regenerateCriterionPromptVarsLoading && !regenerateCriterionPromptVars.length"
      :image-size="60"
      description="项目暂无提示词变量"
    />
    <el-table
      v-else
      v-loading="regenerateCriterionPromptVarsLoading"
      :data="regenerateCriterionPromptVars"
      max-height="320"
      size="small"
      class="w-full"
    >
      <el-table-column width="50">
        <template #header>
          <el-checkbox
            :model-value="allRegeneratePromptVarsSelected"
            :indeterminate="someRegeneratePromptVarsSelected && !allRegeneratePromptVarsSelected"
            @change="toggleSelectAllRegeneratePromptVars($event)"
          />
        </template>
        <template #default="scope">
          <el-checkbox v-model="scope.row.selected" />
        </template>
      </el-table-column>
      <el-table-column label="Key" prop="key" min-width="160">
        <template #default="scope">
          <code class="text-xs">{{ scope.row.key }}</code>
        </template>
      </el-table-column>
      <el-table-column label="Value" min-width="280">
        <template #default="scope">
          <div class="flex items-center gap-2">
            <code class="text-xs break-all flex-1">
              {{ scope.row.showValue ? scope.row.value : maskPromptVarValue(scope.row.value) }}
            </code>
            <el-button
              size="small"
              link
              @click="scope.row.showValue = !scope.row.showValue"
            >
              {{ scope.row.showValue ? '隐藏' : '显示' }}
            </el-button>
          </div>
        </template>
      </el-table-column>
      <el-table-column label="备注" prop="remark" min-width="180">
        <template #default="scope">
          <span class="text-xs text-slate-600">{{ scope.row.remark || '-' }}</span>
        </template>
      </el-table-column>
    </el-table>

    <template #footer>
      <el-button @click="regenerateCriterionDialogVisible = false">取消</el-button>
      <el-button
        type="primary"
        :loading="tddRegenerateLoading"
        @click="confirmRegenerateCriterion"
      >
        开始生成
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Grid, FolderOpened, Folder, Document, Loading, Refresh, Warning, Link, Plus, VideoPlay, Tools, Clock } from '@element-plus/icons-vue'
import { ModuleApi, moduleDecomposeStream, moduleGenerateTestStream, moduleTddRunAllStream, moduleTddFixAllStream, moduleTddFixSingleStream, moduleTddRegenerateCriterionStream } from '@/api/module'
import { TaskApi, taskExecuteStream } from '@/api/task'
import { ProjectApi } from '@/api/project'
import { RuntimeStatusApi } from '@/api/project'
import { TaskStateApi, SliceHistoryApi } from '@/api/monitoring'
import DecomposePreviewDialog from './DecomposePreviewDialog.vue'
import IntegrationTestEditor from './IntegrationTestEditor.vue'
import AddTaskDialog from './AddTaskDialog.vue'

const props = defineProps<{
  projectId: number
}>()

const router = useRouter()
const route = useRoute()

/**
 * 兜底剥离 ```json``` / ``` 围栏，并裁剪到首个 [ 与最后一个 ] 之间的 JSON 数组。
 * 后端 ModuleDecomposeService.extractJsonArray 已经做过一次剥离，这里是双保险。
 */
function escapeHtml(s: string): string {
  if (!s) return ''
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

function stripJsonFence(raw: string): string {
  if (!raw) return raw
  let s = raw.trim()
  if (s.includes('```json')) {
    const start = s.indexOf('```json') + 7
    const end = s.indexOf('```', start)
    if (end > start) s = s.slice(start, end).trim()
  } else if (s.startsWith('```')) {
    const start = 3
    const end = s.indexOf('```', start)
    if (end > start) s = s.slice(start, end).trim()
  }
  const arrStart = s.indexOf('[')
  const arrEnd = s.lastIndexOf(']')
  if (arrStart >= 0 && arrEnd > arrStart) {
    return s.slice(arrStart, arrEnd + 1)
  }
  return s
}

// 状态
const loading = ref(false)
const modules = ref<any[]>([])
const tasks = ref<any[]>([])
const selectedNode = ref<any>(null)
const selectedModule = ref<any>(null)
const selectedTask = ref<any>(null)
const integrationTestSpecText = ref('')
const apiIntegrationTestText = ref('')
const webIntegrationTestText = ref('')
const tddTestSpecText = ref('')
const testGenLoading = ref(false)
const testSaving = ref(false)
const testGenMode = ref<'LEGACY' | 'TDD' | null>(null)
const testGenOutput = ref('')
let testGenController: AbortController | null = null
const activeTestTab = ref<'api' | 'web' | 'tdd'>('api')

const isTddTestSpecSaved = computed(() => {
  const current = (tddTestSpecText.value || '').trim()
  const saved = (selectedModule.value?.tddTestSpecJson || '').trim()
  return current !== '' && current === saved
})

const isTddTestSpecDirty = computed(() => {
  const current = (tddTestSpecText.value || '').trim()
  const saved = (selectedModule.value?.tddTestSpecJson || '').trim()
  return current !== '' && current !== saved
})
const accessUrls = ref<any>(null)

// 任务执行状态（实时）
const taskState = ref<any>(null)
/** 失败重试时是否跳过上一次已通过的步骤（默认开启） */
const retrySkipPassed = ref(true)
/** 任务执行/重试时的实时输出缓冲，执行期间优先于 taskState.executionSummary 展示 */
const liveExecOutput = ref('')

// 模块流水线模式编辑草稿
const modulePipelineModeDraft = ref<'LEGACY' | 'TDD'>('LEGACY')
const moduleSimpleModeDraft = ref(false)
const modulePipelineSaving = ref(false)
const canSetModulePipelineMode = computed(() => {
  const s = selectedModule.value?.status
  return s === 0 || s === 5 || s === 6
})
const modulePipelineDirty = computed(() => {
  if (!selectedModule.value) return false
  const curMode = (selectedModule.value.pipelineMode || 'LEGACY') as 'LEGACY' | 'TDD'
  const curSimple = selectedModule.value.simpleMode === 1
  return modulePipelineModeDraft.value !== curMode || moduleSimpleModeDraft.value !== curSimple
})

// ===== 基础架构模块（infrastructure）相关状态 =====
/** 当前选中的模块是否为基础架构模块（仅做构建+启动验证） */
const isInfrastructureModule = computed(() => {
  return (selectedModule.value?.moduleType || 'business') === 'infrastructure'
})
/** 基础架构验证状态：pending / running / passed / failed */
const infraBuildStatus = ref<'pending' | 'running' | 'passed' | 'failed'>('pending')
const infraStartupStatus = ref<'pending' | 'running' | 'passed' | 'failed'>('pending')
const infraVerifyLoading = ref(false)
const infraVerifyOutput = ref('')
const infraVerifyMessage = ref('')
const infraVerifyFailed = ref(false)
const infraBuildStatusText = computed(() => {
  switch (infraBuildStatus.value) {
    case 'passed': return '通过'
    case 'failed': return '失败'
    case 'running': return '运行中…'
    default: return '未执行'
  }
})
const infraStartupStatusText = computed(() => {
  switch (infraStartupStatus.value) {
    case 'passed': return '通过'
    case 'failed': return '失败'
    case 'running': return '运行中…'
    default: return '未执行'
  }
})

/** 运行基础架构模块验证：构建 + 启动 + 端口可达 */
async function runInfraVerification() {
  if (!selectedModule.value?.id) {
    ElMessage.error('请先选择模块')
    return
  }
  infraVerifyLoading.value = true
  infraVerifyFailed.value = false
  infraVerifyOutput.value = ''
  infraVerifyMessage.value = ''
  infraBuildStatus.value = 'running'
  infraStartupStatus.value = 'pending'
  try {
    // SSE 流式执行：与 ExecuteModuleTasksStream 等接口一致，逐行接收输出
    const projectId = selectedModule.value.projectId
    const moduleId = selectedModule.value.id
    const url = `${import.meta.env.VITE_API_BASE_URL || '/api'}/modules/${moduleId}/infra-verify-stream?projectId=${projectId}`
    const resp = await fetch(url, { method: 'POST' })
    if (!resp.ok || !resp.body) {
      throw new Error(`HTTP ${resp.status}`)
    }
    const reader = resp.body.getReader()
    const decoder = new TextDecoder('utf-8')
    let buffer = ''
    while (true) {
      const { value, done } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''
      for (const line of lines) {
        const trimmed = line.trim()
        if (!trimmed) continue
        if (trimmed.startsWith('data:')) {
          const payload = trimmed.slice(5).trim()
          try {
            const evt = JSON.parse(payload)
            if (evt.type === 'output' && evt.data) {
              infraVerifyOutput.value += evt.data
            } else if (evt.type === 'build_status') {
              infraBuildStatus.value = evt.status
            } else if (evt.type === 'startup_status') {
              infraStartupStatus.value = evt.status
            } else if (evt.type === 'done') {
              infraVerifyMessage.value = evt.message || '验证完成'
              infraVerifyFailed.value = !!evt.failed
              if (evt.buildStatus) infraBuildStatus.value = evt.buildStatus
              if (evt.startupStatus) infraStartupStatus.value = evt.startupStatus
            } else if (evt.type === 'error') {
              infraVerifyMessage.value = evt.message || '验证失败'
              infraVerifyFailed.value = true
              if (infraBuildStatus.value === 'running') infraBuildStatus.value = 'failed'
              if (infraStartupStatus.value === 'running') infraStartupStatus.value = 'failed'
            }
          } catch {
            // 非 JSON 行直接追加
            infraVerifyOutput.value += payload + '\n'
          }
        }
      }
    }
  } catch (e: any) {
    infraVerifyFailed.value = true
    infraVerifyMessage.value = '验证请求失败: ' + (e?.message || String(e))
    if (infraBuildStatus.value === 'running') infraBuildStatus.value = 'failed'
    if (infraStartupStatus.value === 'running') infraStartupStatus.value = 'failed'
  } finally {
    infraVerifyLoading.value = false
  }
}

// 切换模块时重置 infra 验证状态
watch(selectedModule, (cur, prev) => {
  if (!cur || (prev && cur.id !== prev.id)) {
    infraBuildStatus.value = 'pending'
    infraStartupStatus.value = 'pending'
    infraVerifyOutput.value = ''
    infraVerifyMessage.value = ''
    infraVerifyFailed.value = false
    // 已完成的模块默认标记为通过
    if (cur?.status === 2) {
      infraBuildStatus.value = 'passed'
      infraStartupStatus.value = 'passed'
    }
  }
})
/** 执行摘要手动刷新的 loading 标记 */
const executionSummaryRefreshing = ref(false)
/** 执行摘要最近一次刷新的时间显示 */
const executionSummaryUpdatedAt = ref('')
const sliceHistories = ref<any[]>([])
const executionSummaryRef = ref<HTMLElement | null>(null)
let pollingTimer: number | null = null

// 执行摘要内容变化时自动滚动到底部（流式效果）
watch(() => taskState.value?.executionSummary, async () => {
  await nextTick()
  if (executionSummaryRef.value) {
    executionSummaryRef.value.scrollTop = executionSummaryRef.value.scrollHeight
  }
})

// 流水线模式变化时自动切换集成测试 tab
watch(modulePipelineModeDraft, (newMode) => {
  if (newMode === 'TDD') {
    activeTestTab.value = 'tdd'
  } else if (activeTestTab.value === 'tdd') {
    // 从 TDD 切回 LEGACY 时，如果当前是 tdd tab 则切到 api
    activeTestTab.value = 'api'
  }
})

// ───── 执行摘要 Deploy 详情折叠面板 ─────
// 后端把 deploy 末尾输出/警告/版本校验结果包成下面这种标记块写入 executionSummary：
//   <<<DEPLOY_DETAIL step=2 attempt=1 status=ok>>>
//   ...任意多行...
//   <<<END_DEPLOY_DETAIL>>>
// 这里把字符串拆成 text/deploy 两类段，渲染时 deploy 段折叠，Ctrl+O 全部展开/收起。
interface DeployBadge { kind: 'warn' | 'ok' | 'err'; text: string }
interface DeploySeg {
  type: 'deploy'
  step: string
  attempt: string
  status: 'ok' | 'fail'
  lines: string[]
  badges: DeployBadge[]
}
interface TextSeg { type: 'text'; text: string }
type ExecSeg = TextSeg | DeploySeg

// 后端 updateExecutionSummary 会在每段新内容首行加 [HH:MM:SS] 前缀，
// 这里用可选前缀吞掉它，避免上一段 text 末尾留孤悬时间戳。
const DEPLOY_BLOCK_RE = /(?:\[\d{2}:\d{2}:\d{2}\]\s*)?<<<DEPLOY_DETAIL\s+([^>]*)>>>\n([\s\S]*?)\n<<<END_DEPLOY_DETAIL>>>/g

function parseDeployAttrs(attrStr: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const m of attrStr.matchAll(/(\w+)=(\S+)/g)) out[m[1]] = m[2]
  return out
}

function buildBadges(lines: string[]): DeployBadge[] {
  const badges: DeployBadge[] = []
  const joined = lines.join('\n')
  if (joined.includes('⚠️')) badges.push({ kind: 'warn', text: '⚠️ 警告' })
  if (/版本指纹校验通过/.test(joined)) badges.push({ kind: 'ok', text: '✓ 版本一致' })
  else if (/版本指纹不匹配/.test(joined)) badges.push({ kind: 'err', text: '✗ 版本不匹配' })
  else if (/未发现版本端点/.test(joined)) badges.push({ kind: 'warn', text: '无版本端点' })
  return badges
}

const executionSummarySegments = computed<ExecSeg[]>(() => {
  const raw = liveExecOutput.value || (taskState.value?.executionSummary ?? '')
  if (!raw) return []
  const segs: ExecSeg[] = []
  let lastIdx = 0
  // 重置全局 regex 状态
  DEPLOY_BLOCK_RE.lastIndex = 0
  let m: RegExpExecArray | null
  while ((m = DEPLOY_BLOCK_RE.exec(raw)) !== null) {
    if (m.index > lastIdx) {
      const txt = raw.slice(lastIdx, m.index).replace(/\n+$/, '')
      if (txt) segs.push({ type: 'text', text: txt })
    }
    const attrs = parseDeployAttrs(m[1])
    const lines = m[2].split('\n')
    segs.push({
      type: 'deploy',
      step: attrs.step ?? '?',
      attempt: attrs.attempt ?? '?',
      status: (attrs.status === 'fail' ? 'fail' : 'ok'),
      lines,
      badges: buildBadges(lines),
    })
    lastIdx = m.index + m[0].length
  }
  if (lastIdx < raw.length) {
    const txt = raw.slice(lastIdx).replace(/^\n+/, '')
    if (txt) segs.push({ type: 'text', text: txt })
  }
  return segs
})

// 已展开的 deploy 段索引集合（key 为段在 segments 中的下标）
const deployExpanded = ref<Set<number>>(new Set())

function toggleDeployBlock(idx: number) {
  const next = new Set(deployExpanded.value)
  if (next.has(idx)) next.delete(idx)
  else next.add(idx)
  deployExpanded.value = next
}

function toggleAllDeployBlocks() {
  const all = executionSummarySegments.value
    .map((s, i) => (s.type === 'deploy' ? i : -1))
    .filter(i => i >= 0)
  if (!all.length) return
  const allExpanded = all.every(i => deployExpanded.value.has(i))
  deployExpanded.value = allExpanded ? new Set() : new Set(all)
}

// Ctrl+O：全局监听；仅当本组件可见且存在 deploy 段时才接管按键，避免误吞浏览器快捷键
function onExecutionSummaryKeydown(ev: KeyboardEvent) {
  if (!(ev.ctrlKey && !ev.shiftKey && !ev.altKey && !ev.metaKey)) return
  if (ev.key !== 'o' && ev.key !== 'O') return
  const hasDeploy = executionSummarySegments.value.some(s => s.type === 'deploy')
  if (!hasDeploy) return
  // 执行摘要容器在 DOM 中（v-if 渲染过）才认为本 tab 可见
  if (!executionSummaryRef.value || !document.body.contains(executionSummaryRef.value)) return
  ev.preventDefault()
  toggleAllDeployBlocks()
}

// Plan 文件列表
const planFiles = ref<any[]>([])
const planFilesLoading = ref(false)

// 拆解相关
const decomposeDialogVisible = ref(false)
const decomposeForm = ref({
  requirementDescription: '',
  selectedPlanFiles: [] as string[],
  generateApiTest: true,
  generateWebTest: true,
  autoConfig: true
})
const decomposeLoading = ref(false)
const decomposeOutput = ref('')
const decomposeController = ref<AbortController | null>(null)

// 预览对话框
const previewDialogVisible = ref(false)
const decomposeResult = ref<any[]>([])

// 添加任务对话框
const addTaskDialogVisible = ref(false)

// 同模块内任务的「序号 + 名称」列表，作为新任务 blockedBy 候选
const siblingTaskOptions = computed(() => {
  return moduleTasks.value
    .filter((t) => t.sequenceNumber)
    .map((t) => ({ sequenceNumber: t.sequenceNumber, name: t.name }))
})

const openAddTaskDialog = () => {
  if (!selectedModule.value?.id) {
    ElMessage.error('请先选择一个模块')
    return
  }
  addTaskDialogVisible.value = true
}

const onTasksAppended = async () => {
  await loadData()
  // 重新选中原模块，让用户看到新加的任务
  if (selectedModule.value?.id) {
    const m = modules.value.find((x) => x.id === selectedModule.value.id)
    if (m) selectedModule.value = m
  }
}

// 树配置
const treeProps = {
  children: 'children',
  label: 'name'
}

// 计算树数据
const moduleTreeData = computed(() => {

  return modules.value.map(module => {
    // 使用宽松比较（==）处理 Long 和 Number 类型差异
    const moduleTasks = tasks.value.filter(t => t.moduleId == module.id)

    return {
      id: `module-${module.id}`,
      type: 'module',
      name: `${module.sequenceNumber || 'M?'} ${module.name}`,
      status: module.status,
      moduleType: module.moduleType || 'business',
      taskCount: moduleTasks.length,
      data: module,
      children: moduleTasks.map(task => ({
        id: `task-${task.id}`,
        type: 'task',
        name: `${task.sequenceNumber || 'T?'} ${task.name}`,
        status: task.status,
        data: task
      }))
    }
  })
})

// 模块任务
const moduleTasks = computed(() => {
  if (!selectedModule.value) return []
  // 使用宽松比较（==）处理 Long 和 Number 类型差异
  return tasks.value.filter(t => t.moduleId == selectedModule.value.id)
})

// 任务步骤（合并任务定义的 stepsJson 和实时 stepStatusJson）
// 兼容两种格式：
//   1. 字符串数组 ["步骤1", "步骤2"] → 转换为对象数组 {seq, action, status}
//   2. 对象数组 [{seq, status, action, ...}] → 直接使用
const taskSteps = computed(() => {
  // 优先使用实时 stepStatusJson（执行中的验证结果）
  if (taskState.value?.stepStatusJson) {
    try {
      const steps = JSON.parse(taskState.value.stepStatusJson)
      if (Array.isArray(steps) && steps.length > 0) {
        return normalizeSteps(steps)
      }
    } catch { /* ignore */ }
  }
  // 回退到定义时的 stepsJson
  if (!selectedTask.value?.stepsJson) return []
  try {
    const steps = JSON.parse(selectedTask.value.stepsJson)
    if (Array.isArray(steps)) {
      return normalizeSteps(steps)
    }
    return []
  } catch {
    return []
  }
})

/** 将步骤数组统一为对象格式，兼容字符串数组和对象数组 */
const normalizeSteps = (steps: any[]): any[] => {
  if (steps.length === 0) return []
  // 如果第一个元素是字符串，说明是字符串数组格式
  if (typeof steps[0] === 'string') {
    return steps.map((s, i) => ({
      seq: i + 1,
      action: s,
      status: 'pending',
    }))
  }
  return steps
}

// 加载数据
const loadData = async () => {
  if (!props.projectId) return
  loading.value = true
  try {
    const [modulesRes, tasksRes] = await Promise.all([
      ModuleApi.getProjectModules(props.projectId),
      TaskApi.getTaskList()
    ])


    if (modulesRes?.success) {
      modules.value = modulesRes.data || []
    } else {
      console.warn('[ModuleTaskTab] modulesRes.success 为 false 或不存在:', modulesRes)
      modules.value = []
    }
    if (tasksRes) {
      tasks.value = (tasksRes || []).filter((t: any) => t.projectId === props.projectId)
    }

    // 加载访问链接
    try {
      const statusRes = await RuntimeStatusApi.getRuntimeStatus(props.projectId)
      if (statusRes?.accessUrls) {
        accessUrls.value = statusRes.accessUrls
      }
    } catch (e) {
      console.error('Failed to load access URLs:', e)
    }

    // 调试：检查数据

    // 检查 moduleId 匹配情况
    if (modules.value.length > 0 && tasks.value.length > 0) {
      const firstModuleId = modules.value[0].id
      const tasksWithModule = tasks.value.filter((t: any) => t.moduleId === firstModuleId)
      const tasksWithModuleNum = tasks.value.filter((t: any) => t.moduleId == firstModuleId)
    }
  } catch (e) {
    console.error('Failed to load modules and tasks:', e)
  } finally {
    loading.value = false
  }
}

// 切换节点展开/收缩（仅图标点击触发）
const toggleNodeExpand = (node: any) => {
  if (node.expanded) {
    node.collapse()
  } else {
    node.expand()
  }
}

// 节点点击
const handleNodeClick = async (data: any) => {
  selectedNode.value = data
  // 切换节点时停止轮询
  stopPolling()
  taskState.value = null
  sliceHistories.value = []

  if (data.type === 'module') {
    selectedModule.value = data.data
    selectedTask.value = null
    integrationTestSpecText.value = data.data?.integrationTestSpec || ''
    apiIntegrationTestText.value = data.data?.apiIntegrationTest || ''
    webIntegrationTestText.value = data.data?.webIntegrationTest || ''
    tddTestSpecText.value = data.data?.tddTestSpecJson || ''
    modulePipelineModeDraft.value = (data.data?.pipelineMode || 'LEGACY') as 'LEGACY' | 'TDD'
    moduleSimpleModeDraft.value = data.data?.simpleMode === 1
    // 根据流水线模式设置默认的集成测试 tab
    activeTestTab.value = modulePipelineModeDraft.value === 'TDD' ? 'tdd' : 'api'
    testGenOutput.value = ''
    testGenMode.value = null
    if (testGenController) {
      try { testGenController.abort() } catch { /* ignore */ }
      testGenController = null
      testGenLoading.value = false
    }
  } else if (data.type === 'task') {
    // 重新拉取任务详情，避免使用任务树里的陈旧状态（重试/启动后树数据未刷新，
    // 否则切走再切回会把正在执行的任务误显示为"失败"）。
    let detail = data.data
    try {
      const fresh = await TaskApi.getTaskDetail(data.data.id)
      if (fresh) {
        detail = fresh
        syncTaskToTree(fresh)
      }
    } catch { /* 拉取失败时回退到树里的数据 */ }
    selectedTask.value = detail
    selectedNode.value = { ...selectedNode.value, data: detail, status: detail.status }
    const moduleId = detail?.moduleId
    selectedModule.value = modules.value.find(m => m.id === moduleId) || null
    // 加载任务实时状态和重试信息
    loadTaskState(detail.id)
  }
}

/** 加载任务实时状态 */
const loadTaskState = async (taskId: number) => {
  try {
    const state = await TaskStateApi.getByTaskId(taskId)
    if (state) {
      taskState.value = state
      executionSummaryUpdatedAt.value = formatRefreshTime(new Date())
    }
    try {
      const histories = await SliceHistoryApi.getByTaskId(taskId)
      sliceHistories.value = histories || []
    } catch { /* ignore */ }
  } catch { /* ignore */ }
}

/** 手动刷新执行摘要：拉取 task / task_state，覆盖当前展示 */
const refreshExecutionSummary = async () => {
  if (!selectedTask.value?.id || executionSummaryRefreshing.value) return
  executionSummaryRefreshing.value = true
  try {
    // 同时刷新任务主表，避免状态已变（完成/失败）但页面还在显示执行中
    try {
      const detail = await TaskApi.getTaskDetail(selectedTask.value.id)
      if (detail) {
        selectedTask.value = detail
        selectedNode.value = { ...selectedNode.value, data: detail, status: detail.status }
        syncTaskToTree(detail)
      }
    } catch { /* ignore */ }
    await loadTaskState(selectedTask.value.id)
    ElMessage.success('已刷新')
  } catch (e: any) {
    ElMessage.error(e?.message || '刷新失败')
  } finally {
    executionSummaryRefreshing.value = false
  }
}

const formatRefreshTime = (d: Date): string => {
  const pad = (n: number) => n.toString().padStart(2, '0')
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

// 状态样式
const getStatusType = (status: number | undefined) => {
  const types: Record<number, string> = {
    0: 'info',
    1: 'primary',
    2: 'warning',
    3: 'success',
    4: 'info',
    5: 'danger',
    6: 'danger'
  }
  return types[status ?? 0] || 'info'
}

const getStatusText = (status: number | undefined, type?: string) => {
  if (type === 'module') {
    const texts: Record<number, string> = {
      0: '待执行',
      1: '执行中',
      2: '待测试',
      3: '测试中',
      4: '完成',
      5: '测试失败',
      6: '失败'
    }
    return texts[status ?? 0] || '未知'
  }
  const texts: Record<number, string> = {
    0: '待办',
    1: '执行中',
    2: '审查中',
    3: '完成',
    4: '已暂停',
    5: '失败'
  }
  return texts[status ?? 0] || '未知'
}

const getTaskStatusType = (status: number | undefined) => {
  const types: Record<number, string> = {
    0: 'info',
    1: 'primary',
    2: 'warning',
    3: 'success',
    4: 'info',
    5: 'danger'
  }
  return types[status ?? 0] || 'info'
}

const getTaskStatusText = (status: number | undefined) => {
  const texts: Record<number, string> = {
    0: '待办',
    1: '执行中',
    2: '审查中',
    3: '完成',
    4: '已暂停',
    5: '失败'
  }
  return texts[status ?? 0] || '未知'
}

const getStepStatusType = (status: string | undefined) => {
  const types: Record<string, string> = {
    pending: 'info',
    running: 'primary',
    in_progress: 'primary',
    completed: 'success',
    passed: 'success',
    failed: 'danger'
  }
  return types[status || 'pending'] || 'info'
}

// 任务操作
/** 将最新任务详情合并回共享列表 tasks.value，让左侧任务树状态实时同步
 * （避免重试/启动后树仍显示陈旧"失败"，切走再切回误以为任务回退） */
const syncTaskToTree = (detail: any) => {
  if (!detail?.id) return
  const idx = tasks.value.findIndex(t => t.id === detail.id)
  if (idx >= 0) tasks.value[idx] = { ...tasks.value[idx], ...detail }
}

const startTask = async () => {
  if (!selectedTask.value?.id) {
    ElMessage.error('任务ID不存在')
    return
  }
  try {
    await TaskApi.startTask(selectedTask.value.id)
    ElMessage.success('任务已启动')
    // 刷新任务详情
    const detail = await TaskApi.getTaskDetail(selectedTask.value.id)
    selectedTask.value = detail
    selectedNode.value = { ...selectedNode.value, data: detail, status: detail.status }
    syncTaskToTree(detail)
    // 立即加载任务实时状态（获取初始 executionSummary）
    await loadTaskState(selectedTask.value.id)
    // 启动轮询：每1秒刷新任务状态和执行信息
    startPolling(selectedTask.value.id)
    // 触发实际执行（execute-stream），实时输出显示在执行摘要区域
    runTaskExecuteStream(selectedTask.value.id, false)
  } catch (e: any) {
    ElMessage.error(e?.message || '启动失败')
  }
}

/** 启动状态轮询 */
const startPolling = (taskId: number) => {
  stopPolling()
  pollingTimer = window.setInterval(async () => {
    try {
      // 刷新任务主表
      const detail = await TaskApi.getTaskDetail(taskId)
      selectedTask.value = detail
      selectedNode.value = { ...selectedNode.value, data: detail, status: detail.status }
      syncTaskToTree(detail)
    } catch { /* ignore task detail error */ }

    try {
      // 刷新任务实时状态
      const state = await TaskStateApi.getByTaskId(taskId)
      if (state) {
        taskState.value = state
        executionSummaryUpdatedAt.value = formatRefreshTime(new Date())
      }
    } catch { /* ignore task state error */ }

    if (sliceHistories.value.length === 0) {
      try {
        const histories = await SliceHistoryApi.getByTaskId(taskId)
        sliceHistories.value = histories || []
      } catch { /* ignore */ }
    }

    // 终态：停止轮询
    if (selectedTask.value?.status === 3 || selectedTask.value?.status === 5 || selectedTask.value?.status === 4) {
      try {
        const histories = await SliceHistoryApi.getByTaskId(taskId)
        sliceHistories.value = histories || []
      } catch { /* ignore */ }
      stopPolling()
    }
  }, 1000)
}

/** 停止轮询 */
const stopPolling = () => {
  if (pollingTimer !== null) {
    clearInterval(pollingTimer)
    pollingTimer = null
  }
}

const pauseTask = async () => {
  if (!selectedTask.value?.id) return
  try {
    await TaskApi.pauseTask(selectedTask.value.id)
    ElMessage.success('任务已暂停')
    loadData()
  } catch (e: any) {
    ElMessage.error(e?.message || '暂停失败')
  }
}

const resumeTask = async () => {
  if (!selectedTask.value?.id) return
  try {
    await TaskApi.resumeTask(selectedTask.value.id)
    ElMessage.success('任务已恢复')
    // 恢复后重新轮询
    const detail = await TaskApi.getTaskDetail(selectedTask.value.id)
    selectedTask.value = detail
    selectedNode.value = { ...selectedNode.value, data: detail, status: detail.status }
    syncTaskToTree(detail)
    startPolling(selectedTask.value.id)
  } catch (e: any) {
    ElMessage.error(e?.message || '恢复失败')
  }
}

const retryTask = async () => {
  if (!selectedTask.value?.id) return
  try {
    await TaskApi.startTask(selectedTask.value.id, retrySkipPassed.value)
    ElMessage.success(retrySkipPassed.value
      ? '任务重试中（跳过已通过的步骤）'
      : '任务重试中（全量重跑）')
    sliceHistories.value = []
    const detail = await TaskApi.getTaskDetail(selectedTask.value.id)
    selectedTask.value = detail
    selectedNode.value = { ...selectedNode.value, data: detail, status: detail.status }
    syncTaskToTree(detail)
    startPolling(selectedTask.value.id)
    // 触发实际执行（execute-stream），实时输出显示在执行摘要区域
    runTaskExecuteStream(selectedTask.value.id, retrySkipPassed.value)
  } catch (e: any) {
    ElMessage.error(e?.message || '重试失败')
    // 失败时同步一次真实状态，避免停留在线程/树里的陈旧"失败"上（如后端返回"正在执行中"）
    try {
      const fresh = await TaskApi.getTaskDetail(selectedTask.value.id)
      if (fresh) {
        selectedTask.value = fresh
        selectedNode.value = { ...selectedNode.value, data: fresh, status: fresh.status }
        syncTaskToTree(fresh)
      }
    } catch { /* ignore */ }
  }
}

/**
 * 调用 execute-stream SSE 执行任务，实时输出追加到 liveExecOutput，
 * 在执行摘要区域展示；流结束后清空 liveExecOutput 并刷新 taskState。
 */
const runTaskExecuteStream = (taskId: number, skipPassed: boolean) => {
  liveExecOutput.value = ''
  taskExecuteStream(
    taskId,
    (line) => {
      liveExecOutput.value += line + '\n'
    },
    async () => {
      // 执行完成：清空实时输出，刷新任务状态与摘要
      liveExecOutput.value = ''
      try {
        const detail = await TaskApi.getTaskDetail(taskId)
        selectedTask.value = detail
        selectedNode.value = { ...selectedNode.value, data: detail, status: detail.status }
        await loadTaskState(taskId)
      } catch { /* ignore */ }
    },
    (msg) => {
      liveExecOutput.value = ''
      ElMessage.error(msg || '执行失败')
    },
    () => {
      liveExecOutput.value = ''
    },
    skipPassed
  )
}

/** 判断是否可以编辑/删除步骤（仅待办/暂停/失败状态） */
const canEditStep = () => {
  const taskStatus = selectedTask.value?.status
  return taskStatus === 0 || taskStatus === 4 || taskStatus === 5
}

// 编辑步骤对话框状态
const editStepDialogVisible = ref(false)
const editingStepIndex = ref<number>(-1)
const editingStepData = ref<any>(null)
const savingStep = ref(false)

/** 打开编辑步骤对话框 */
const openEditStepDialog = (step: any, index: number) => {
  editingStepIndex.value = index
  // 复制步骤数据用于编辑
  editingStepData.value = {
    seq: step.seq || index + 1,
    action: step.action || '',
    files: Array.isArray(step.files) ? [...step.files] : [],
    planExcerpt: step.planExcerpt || step.plan_excerpt || '',
    migrationFile: step.migrationFile || step.migration_file || '',
    validation: step.validation || '',
    factChecks: Array.isArray(step.factChecks) ? [...step.factChecks] : []
  }
  editStepDialogVisible.value = true
}

/** 删除步骤 */
const deleteStep = async (step: any, index: number) => {
  const taskId = selectedTask.value?.id
  if (!taskId) return
  try {
    await ElMessageBox.confirm(
      `确定删除步骤 ${step.seq || index + 1} 吗？删除后剩余步骤序号将自动重排。`,
      '确认删除步骤',
      { type: 'warning' }
    )
    const res: any = await TaskApi.deleteStep(taskId, index)
    if (res?.success === false) {
      ElMessage.error(res.message || '删除失败')
      return
    }
    ElMessage.success('步骤已删除')
    // 刷新任务数据
    const detail = await TaskApi.getTaskDetail(taskId)
    if (detail) {
      selectedTask.value = detail
      selectedNode.value = { ...selectedNode.value, data: detail, status: detail.status }
    }
    await loadTaskState(taskId)
  } catch (e: any) {
    if (e === 'cancel' || e === 'close') return
    ElMessage.error(e?.response?.data?.message || e?.message || '删除失败')
  }
}

/** 保存编辑的步骤 */
const saveEditedStep = async () => {
  const taskId = selectedTask.value?.id
  const stepIndex = editingStepIndex.value
  if (!taskId || stepIndex < 0) return

  // 校验必填字段
  if (!editingStepData.value?.action?.trim()) {
    ElMessage.error('操作描述不能为空')
    return
  }

  savingStep.value = true
  try {
    // 构建提交数据（去掉前端临时seq）
    const payload = {
      action: editingStepData.value.action,
      files: editingStepData.value.files,
      planExcerpt: editingStepData.value.planExcerpt,
      migrationFile: editingStepData.value.migrationFile,
      validation: editingStepData.value.validation,
      factChecks: editingStepData.value.factChecks
    }
    const res: any = await TaskApi.updateStep(taskId, stepIndex, payload)
    if (res?.success === false) {
      ElMessage.error(res.message || '保存失败')
      return
    }
    ElMessage.success('步骤已更新')
    editStepDialogVisible.value = false
    // 刷新任务数据
    const detail = await TaskApi.getTaskDetail(taskId)
    if (detail) {
      selectedTask.value = detail
      selectedNode.value = { ...selectedNode.value, data: detail, status: detail.status }
    }
    await loadTaskState(taskId)
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '保存失败')
  } finally {
    savingStep.value = false
  }
}

// 事实校验编辑相关
const factCheckDialogVisible = ref(false)
const factCheckDraftIndex = ref<number>(-1)
const factCheckDraft = ref<any>({
  type: 'file_exists',
  path: '',
  command: '',
  expectedExitCode: 0,
  method: 'GET',
  url: '',
  expectedStatus: 200,
  commitHash: ''
})

const getFactCheckTagType = (type: string): string => {
  switch (type) {
    case 'file_exists': return 'info'
    case 'shell': return 'warning'
    case 'http_status': return 'success'
    case 'git_commit': return 'primary'
    default: return ''
  }
}

const formatFactCheck = (fc: any): string => {
  switch (fc?.type) {
    case 'file_exists':
      return fc.path || '(空 path)'
    case 'shell':
      return fc.command || '(空 command)'
    case 'http_status':
      return `${fc.method || 'GET'} ${fc.url || '(空 url)'} → ${fc.expectedStatus ?? 200}`
    case 'git_commit':
      return fc.commitHash ? `commit ${fc.commitHash}` : '(Tester 填入)'
    default:
      return JSON.stringify(fc)
  }
}

const addFactCheck = () => {
  factCheckDraftIndex.value = -1
  factCheckDraft.value = {
    type: 'file_exists',
    path: '',
    command: '',
    expectedExitCode: 0,
    method: 'GET',
    url: '',
    expectedStatus: 200,
    commitHash: ''
  }
  factCheckDialogVisible.value = true
}

const editFactCheck = (index: number) => {
  if (!editingStepData.value?.factChecks) return
  const fc = editingStepData.value.factChecks[index]
  factCheckDraftIndex.value = index
  factCheckDraft.value = {
    type: fc.type || 'file_exists',
    path: fc.path || '',
    command: fc.command || '',
    expectedExitCode: fc.expectedExitCode ?? 0,
    method: fc.method || 'GET',
    url: fc.url || '',
    expectedStatus: fc.expectedStatus ?? 200,
    commitHash: fc.commitHash || ''
  }
  factCheckDialogVisible.value = true
}

const removeFactCheck = (index: number) => {
  if (!editingStepData.value?.factChecks) return
  editingStepData.value.factChecks.splice(index, 1)
}

const saveFactCheck = () => {
  const draft = factCheckDraft.value
  if (!draft.type) {
    ElMessage.error('请选择校验类型')
    return
  }

  // 根据类型构建factCheck对象
  let fc: any = { type: draft.type }
  switch (draft.type) {
    case 'file_exists':
      if (!draft.path?.trim()) {
        ElMessage.error('文件路径不能为空')
        return
      }
      fc.path = draft.path.trim()
      break
    case 'shell':
      if (!draft.command?.trim()) {
        ElMessage.error('Shell命令不能为空')
        return
      }
      fc.command = draft.command.trim()
      fc.expectedExitCode = draft.expectedExitCode ?? 0
      break
    case 'http_status':
      if (!draft.url?.trim()) {
        ElMessage.error('URL不能为空')
        return
      }
      fc.method = draft.method || 'GET'
      fc.url = draft.url.trim()
      fc.expectedStatus = draft.expectedStatus ?? 200
      break
    case 'git_commit':
      fc.commitHash = draft.commitHash?.trim() || ''
      break
  }

  // 添加或更新
  if (!editingStepData.value.factChecks) {
    editingStepData.value.factChecks = []
  }
  if (factCheckDraftIndex.value >= 0) {
    editingStepData.value.factChecks[factCheckDraftIndex.value] = fc
  } else {
    editingStepData.value.factChecks.push(fc)
  }
  factCheckDialogVisible.value = false
}

const editFrontendUrl = async () => {
  if (!props.projectId) return
  const current = accessUrls.value?.frontendUrlOverride || ''
  try {
    const { value } = await ElMessageBox.prompt(
      '设置「前端访问」的自定义 URL。留空表示恢复默认（按服务器 IP + 前端端口拼接）。',
      '自定义前端访问',
      {
        confirmButtonText: '保存',
        cancelButtonText: '取消',
        inputValue: current,
        inputPlaceholder: '如 http://frontend.example.com:5173，留空恢复默认',
        inputValidator: (v: string) => {
          const t = (v || '').trim()
          if (!t) return true
          if (t.startsWith('http://') || t.startsWith('https://')) return true
          return 'URL 必须以 http:// 或 https:// 开头'
        }
      }
    )
    const trimmed = (value || '').trim()
    // PortAllocationApi removed - frontend URL override no longer supported
    ElMessage.success(trimmed ? '已更新前端访问' : '已恢复默认前端访问')
    const statusRes: any = await RuntimeStatusApi.getRuntimeStatus(props.projectId)
    if (statusRes?.accessUrls) accessUrls.value = statusRes.accessUrls
  } catch (e: any) {
    if (e === 'cancel' || e === 'close') return
    ElMessage.error(e?.response?.data?.message || e?.message || '保存失败')
  }
}

const editTestBaseUrl = async () => {
  if (!props.projectId) return
  const current = accessUrls.value?.testBaseUrlOverride || ''
  try {
    const { value } = await ElMessageBox.prompt(
      '设置「后端 API」的自定义 URL，作为任务/模块测试的 base URL。留空表示恢复默认（按服务器 IP + 后端端口拼接）。',
      '自定义后端 API',
      {
        confirmButtonText: '保存',
        cancelButtonText: '取消',
        inputValue: current,
        inputPlaceholder: '如 http://api.example.com:8080，留空恢复默认',
        inputValidator: (v: string) => {
          const t = (v || '').trim()
          if (!t) return true
          if (t.startsWith('http://') || t.startsWith('https://')) return true
          return 'URL 必须以 http:// 或 https:// 开头'
        }
      }
    )
    const trimmed = (value || '').trim()
    // PortAllocationApi removed - test base URL override no longer supported
    ElMessage.success(trimmed ? '已更新后端 API' : '已恢复默认后端 API')
    const statusRes: any = await RuntimeStatusApi.getRuntimeStatus(props.projectId)
    if (statusRes?.accessUrls) accessUrls.value = statusRes.accessUrls
  } catch (e: any) {
    if (e === 'cancel' || e === 'close') return
    ElMessage.error(e?.response?.data?.message || e?.message || '保存失败')
  }
}

const editTask = () => {
  ElMessage.info('编辑任务功能开发中')
}

// 解析依赖
const parseBlockedBy = (blockedBy: string | null) => {
  if (!blockedBy) return []
  try {
    return JSON.parse(blockedBy)
  } catch {
    return []
  }
}

// 模块操作
const editModule = () => {
  ElMessage.info('编辑模块功能开发中')
}

const deleteModule = async () => {
  try {
    await ElMessageBox.confirm('确定删除该模块吗？任务的模块关联将被解除。', '确认删除')
    await ModuleApi.deleteModule(selectedModule.value.id)
    ElMessage.success('删除成功')
    loadData()
  } catch (e) {
    // 用户取消
  }
}

// 删除全部模块和任务
const deleteAllModules = async () => {
  try {
    await ElMessageBox.confirm(
      '确定删除所有模块和任务吗？此操作不可恢复！',
      '确认删除全部',
      { type: 'warning' }
    )
    await ModuleApi.deleteAllByProjectId(props.projectId)
    ElMessage.success('删除成功')
    selectedNode.value = null
    selectedModule.value = null
    selectedTask.value = null
    loadData()
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error('删除失败')
    }
  }
}

// 保存模块流水线模式
const saveModulePipelineMode = async () => {
  if (!selectedModule.value?.id) return
  modulePipelineSaving.value = true
  try {
    await ModuleApi.setPipelineMode(selectedModule.value.id, {
      pipelineMode: modulePipelineModeDraft.value,
      simpleMode: moduleSimpleModeDraft.value ? 1 : 0,
    })
    ElMessage.success(`已设置为 ${modulePipelineModeDraft.value}${
      modulePipelineModeDraft.value === 'LEGACY' && moduleSimpleModeDraft.value ? ' + simpleMode' : ''
    }`)
    // 刷新模块缓存
    selectedModule.value = {
      ...selectedModule.value,
      pipelineMode: modulePipelineModeDraft.value,
      simpleMode: moduleSimpleModeDraft.value ? 1 : 0,
    }
    loadData()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || e?.message || '保存失败')
  } finally {
    modulePipelineSaving.value = false
  }
}

// 测试规范编辑
const saveTestSpec = async () => {
  testSaving.value = true
  try {
    const res = await ModuleApi.updateModule(selectedModule.value.id, {
      ...selectedModule.value,
      integrationTestSpec: integrationTestSpecText.value,
      apiIntegrationTest: apiIntegrationTestText.value,
      webIntegrationTest: webIntegrationTestText.value,
      tddTestSpecJson: tddTestSpecText.value,
      // 保存新测试规格时清空旧断言结果，避免显示过时的红/绿卡片
      tddAssertionsJson: '[]',
    })
    const saved = (res as any)?.data
    if (saved && saved.id) {
      selectedModule.value = saved
      tddTestSpecText.value = saved.tddTestSpecJson || ''
    } else {
      selectedModule.value = {
        ...selectedModule.value,
        tddTestSpecJson: tddTestSpecText.value,
      }
    }
    ElMessage.success('保存成功')
    loadData()
  } catch (e) {
    ElMessage.error('保存失败')
  } finally {
    testSaving.value = false
  }
}

// ═══════════════ TDD 验收标准：结构化渲染 + 手动执行 ═══════════════

/** 默认收起的「原始 JSON 编辑器」展开状态。 */
const showTddRawEditor = ref(false)
/** 模块级「运行全部」SSE 状态。 */
const tddRunAllLoading = ref(false)
let tddRunController: AbortController | null = null
/** 单条「重跑」当前 loading 的 criterion id（同时只允许重跑一条，避免错乱）。 */
const tddRunSingleLoading = ref<string | null>(null)
/** 运行日志，append-only。 */
const tddRunOutput = ref('')

/** 「修复全部」SSE 状态 */
const tddFixAllLoading = ref(false)
let tddFixController: AbortController | null = null
/** 「修复单条」当前 loading 的 criterion id */
const tddFixSingleLoading = ref<string | null>(null)
/** 修复日志，append-only */
const tddFixOutput = ref('')

/** 「重新生成单条 criterion」SSE 状态 */
const tddRegenerateLoading = ref<string | null>(null)
const tddRegenerateOutput = ref('')
let tddRegenerateController: AbortController | null = null

/** 修复历史弹框状态 */
const fixHistoryDialogVisible = ref(false)
const fixHistoryLoading = ref(false)
const fixHistoryList = ref<any[]>([])
const fixHistoryDetailVisible = ref(false)
const fixHistoryDetail = ref<any | null>(null)

/** 单条 criterion 重新生成弹框状态 */
const regenerateCriterionDialogVisible = ref(false)
const regenerateCriterionId = ref<string | null>(null)
type RegeneratePromptVarRow = { key: string; value: string; remark: string; selected: boolean; showValue: boolean }
const regenerateCriterionPromptVars = ref<RegeneratePromptVarRow[]>([])
const regenerateCriterionPromptVarsLoading = ref(false)
const regenerateCriterionAccessUrls = ref<{ frontendDev?: string; backendApi?: string } | null>(null)
const regenerateCriterionAccessUrlsLoading = ref(false)

const allRegeneratePromptVarsSelected = computed(() =>
  regenerateCriterionPromptVars.value.length > 0 && regenerateCriterionPromptVars.value.every(r => r.selected),
)
const someRegeneratePromptVarsSelected = computed(() => regenerateCriterionPromptVars.value.some(r => r.selected))

/** 解析 tddTestSpecText 为对象；解析失败返回 null，UI 走原始 JSON 兜底。 */
const parsedTddSpec = computed<any | null>(() => {
  const text = (tddTestSpecText.value || '').trim()
  if (!text) return null
  try {
    const obj = JSON.parse(text)
    return obj && typeof obj === 'object' ? obj : null
  } catch {
    return null
  }
})

/** 从 selectedModule.tddAssertionsJson 解析出 criteriaId → assertion[] 的查找表（同 criterion 可有多条 sub-assertion）。 */
const tddAssertionMap = computed<Record<string, any[]>>(() => {
  const map: Record<string, any[]> = {}
  const raw = (selectedModule.value as any)?.tddAssertionsJson
  if (typeof raw !== 'string' || !raw.trim()) return map
  try {
    const arr = JSON.parse(raw)
    if (Array.isArray(arr)) {
      for (const a of arr) {
        const key = a?.criteriaId || a?.id
        if (!key) continue
        const k = String(key)
        if (!map[k]) map[k] = []
        map[k].push(a)
      }
    }
  } catch { /* 旧数据格式异常时静默忽略，UI 走「未编译」分支 */ }
  return map
})

/** 已编译的 assertion 总条数（用于「已编译 N 条断言」徽章）。 */
const tddAssertionsCount = computed(() =>
  Object.values(tddAssertionMap.value).reduce((sum, list) => sum + list.length, 0))

/** taskRef（sequenceNumber，如 "T1.2"）→ 任务对象。优先用当前模块的任务，缺失时退回全项目任务。 */
const taskRefMap = computed<Record<string, any>>(() => {
  const map: Record<string, any> = {}
  for (const t of (moduleTasks.value || [])) {
    if (t?.sequenceNumber) map[String(t.sequenceNumber)] = t
  }
  for (const t of (tasks.value || [])) {
    if (t?.sequenceNumber && !map[String(t.sequenceNumber)]) map[String(t.sequenceNumber)] = t
  }
  return map
})

const taskRefLabel = (ref: string | null | undefined): string => {
  if (!ref) return ''
  const t = taskRefMap.value[String(ref)]
  if (!t) return String(ref)
  return `${ref} ${t.name || ''}`.trim()
}

/** 按 criteriaId 聚合后的红绿判定：任一 RED → 红；全部 GREEN → 绿；其余 → warning。 */
const tddBadgeType = (criterionId: string | null | undefined): 'success' | 'danger' | 'warning' | 'info' => {
  if (!criterionId) return 'info'
  const list = tddAssertionMap.value[String(criterionId)]
  if (!list || list.length === 0) return 'info'
  if (list.some(a => a?.lastStatus === 'RED')) return 'danger'
  if (list.every(a => a?.lastStatus === 'GREEN')) return 'success'
  return 'warning'
}

const tddBadgeText = (criterionId: string | null | undefined): string => {
  if (!criterionId) return '未编译'
  const list = tddAssertionMap.value[String(criterionId)]
  if (!list || list.length === 0) return '未编译'
  const total = list.length
  const reds = list.filter(a => a?.lastStatus === 'RED').length
  const greens = list.filter(a => a?.lastStatus === 'GREEN').length
  if (reds > 0) return total === 1 ? '✗ 红' : `✗ 红 ${reds}/${total}`
  if (greens === total) return total === 1 ? '✓ 绿' : `✓ 绿 ${total}/${total}`
  return total === 1 ? '待运行' : `待运行 ${total - reds - greens}/${total}`
}

const tddCardClass = (criterionId: string | null | undefined): string => {
  const t = tddBadgeType(criterionId)
  if (t === 'success') return 'is-green'
  if (t === 'danger') return 'is-red'
  return ''
}

/** 该 criterion 是否含至少一条红色断言（驱动「修复」按钮显隐）。 */
const criterionIsRed = (criterionId: string | null | undefined): boolean => {
  if (!criterionId) return false
  const list = tddAssertionMap.value[String(criterionId)]
  return !!(list && list.some(a => a?.lastStatus === 'RED'))
}

/** 当前模块的红色断言总数（按 criterion 聚合一次再计数，避免同 criterion 多条断言重复计入）。 */
const tddRedAssertionCount = computed(() => {
  const map = tddAssertionMap.value
  let count = 0
  for (const list of Object.values(map)) {
    if (list.some((a: any) => a?.lastStatus === 'RED')) count++
  }
  return count
})

const hasAssertionFor = (criterionId: string | null | undefined): boolean => {
  if (!criterionId) return false
  const list = tddAssertionMap.value[String(criterionId)]
  return !!(list && list.length > 0)
}

/** 同 criterion 多条 assertion 的 lastDetail 拼接展示，每条带「断言 id · 状态」前缀。 */
const lastDetailFor = (criterionId: string | null | undefined): string => {
  if (!criterionId) return ''
  const list = tddAssertionMap.value[String(criterionId)]
  if (!list || list.length === 0) return ''
  const parts: string[] = []
  for (const a of list) {
    const detail = a?.lastDetail
    if (!detail) continue
    const status = a?.lastStatus === 'GREEN' ? '✓ 绿'
                  : a?.lastStatus === 'RED' ? '✗ 红'
                  : (a?.lastStatus || '待运行')
    const header = list.length > 1 ? `[${a?.id || '?'} · ${status}]` : ''
    parts.push(header ? `${header} ${String(detail)}` : String(detail))
  }
  const merged = parts.join('\n\n')
  return merged.length > 600 ? merged.slice(0, 600) + '\n…（已截断，完整内容请查看 LLM 调用日志）' : merged
}

/** 同 criterion 多条 assertion 的「编译为」展示，每条一行：type :: executable。 */
const assertionExecutableFor = (criterionId: string | null | undefined): string => {
  if (!criterionId) return ''
  const list = tddAssertionMap.value[String(criterionId)]
  if (!list || list.length === 0) return ''
  const lines = list
    .filter(a => a?.executable)
    .map(a => `${a.type || 'shell'} :: ${a.executable}`)
  return lines.join('\n')
}

/**
 * 兜底告警：criterion.when 含 HTTP 动词，但已编译的 assertion 列表里没有 http_status —— 即「行为是 HTTP 调用但实际只测了文件存在」。
 * 后端 ensureHttpMainAssertion 已在编译时兜底注入；本告警仅用于覆盖历史脏数据 + LLM 偶发漏注的兜底提示。
 */
const HTTP_VERB_RE = /\b(GET|POST|PUT|DELETE|PATCH)\b/i
const missingHttpMainAssertion = (c: any): boolean => {
  if (!c?.id) return false
  const when = String(c?.when || '')
  if (!when || !HTTP_VERB_RE.test(when)) return false
  const list = tddAssertionMap.value[String(c.id)]
  if (!list || list.length === 0) return false
  return !list.some(a => a?.type === 'http_status')
}

/** executor 字段徽章：programmatic（纯 Java 跑）/ llm_fallback（C 类，运行时调 TestAuthor）。 */
const isLlmFallback = (c: any): boolean => {
  const e = String(c?.executor || '').toLowerCase()
  if (e === 'llm_fallback' || e === 'llm') return true
  // 未声明 + 缺 assertion → 视为兑底
  return !c?.assertion || !c?.assertion?.type
}
const executorTagLabel = (c: any): string => isLlmFallback(c) ? 'LLM 兑底' : '程序'
const executorTagType = (c: any): 'warning' | 'info' => isLlmFallback(c) ? 'warning' : 'info'
const executorTooltip = (c: any): string => {
  return isLlmFallback(c)
    ? 'C 类用例（数据携带链路 / 复杂前置）。运行时调 TestAuthor LLM 探测环境并编译断言。'
    : 'A/B 类用例。AI 在生成时已写明可执行 assertion，运行时纯 Java 直接跑。'
}

/** 取一条 criterion 当前生效的 taskRef 列表（合并新 taskRefs 数组 + 旧 taskRef 单字段，去重去空白）。 */
const effectiveTaskRefs = (c: any): string[] => {
  const out = new Set<string>()
  if (Array.isArray(c?.taskRefs)) {
    for (const r of c.taskRefs) {
      if (typeof r === 'string' && r.trim()) out.add(r.trim())
    }
  }
  if (typeof c?.taskRef === 'string' && c.taskRef.trim()) {
    out.add(c.taskRef.trim())
  }
  return Array.from(out)
}

/** 计算 criterion 实际生效的 autoRetryTask 值：显式 true/false 优先；null 则默认有 taskRef 时 true。 */
const effectiveAutoRetry = (c: any): boolean => {
  if (c?.autoRetryTask === true) return effectiveTaskRefs(c).length > 0
  if (c?.autoRetryTask === false) return false
  return effectiveTaskRefs(c).length > 0
}

/** 当前正在保存的 criterion id（按字段分开，避免一个 in-flight 卡住所有交互）。 */
const tddAutoRetrySaving = ref<string | null>(null)
const tddTaskRefSaving = ref<string | null>(null)

/**
 * 通用 helper：parse tddTestSpecText → 在 acceptanceCriteria 里找到指定 criterion → 调 patcher 改字段
 * → JSON.stringify → PUT /modules/{id}，并把回包写回 selectedModule + tddTestSpecText。
 */
const patchCriterionAndSave = async (
  criterionId: string,
  patcher: (item: any) => void,
  savingFlag: { value: string | null },
): Promise<boolean> => {
  if (!criterionId || !selectedModule.value?.id) return false
  if (savingFlag.value) return false
  let spec: any
  try {
    spec = JSON.parse(tddTestSpecText.value || '{}')
  } catch {
    ElMessage.error('TDD JSON 解析失败，无法编辑；请检查原始 JSON')
    return false
  }
  if (!Array.isArray(spec?.acceptanceCriteria)) {
    ElMessage.error('TDD JSON 缺少 acceptanceCriteria 数组')
    return false
  }
  const item = spec.acceptanceCriteria.find((x: any) => x?.id === criterionId)
  if (!item) {
    ElMessage.error('未找到 criterion: ' + criterionId)
    return false
  }
  patcher(item)
  const nextText = JSON.stringify(spec, null, 2)
  savingFlag.value = String(criterionId)
  try {
    const res: any = await ModuleApi.updateModule(selectedModule.value.id, {
      ...selectedModule.value,
      tddTestSpecJson: nextText,
    })
    const saved = res?.data
    if (saved && saved.id) {
      selectedModule.value = saved
      tddTestSpecText.value = saved.tddTestSpecJson || nextText
    } else {
      selectedModule.value = { ...selectedModule.value, tddTestSpecJson: nextText }
      tddTestSpecText.value = nextText
    }
    return true
  } catch (e: any) {
    ElMessage.error('保存失败：' + (e?.message || e))
    return false
  } finally {
    savingFlag.value = null
  }
}

/** 切换某条 criterion 的 autoRetryTask 字段，并立即写回。 */
const toggleAutoRetryTask = async (criterionId: string | null | undefined, next: boolean) => {
  if (!criterionId) return
  if (!c_hasTaskRef(criterionId)) {
    ElMessage.warning('该用例没有 taskRef，无法自动触发任务重试；请先在上方下拉里选一个关联任务')
    return
  }
  await patchCriterionAndSave(String(criterionId), (item) => {
    item.autoRetryTask = next
  }, tddAutoRetrySaving)
}

/**
 * 更改某条 criterion 关联的 taskRefs（多选数组），并立即写回。
 * 同步清掉旧单字段 taskRef 避免「新 taskRefs + 旧 taskRef」双源数据飘移。
 */
const updateCriterionTaskRefs = async (criterionId: string | null | undefined, taskRefs: string[]) => {
  if (!criterionId) return
  const normalized = (Array.isArray(taskRefs) ? taskRefs : [])
    .map(r => (r || '').trim())
    .filter(r => r.length > 0)
  // 去重，保持选择顺序
  const dedup: string[] = []
  const seen = new Set<string>()
  for (const r of normalized) {
    if (!seen.has(r)) { seen.add(r); dedup.push(r) }
  }
  const ok = await patchCriterionAndSave(String(criterionId), (item) => {
    item.taskRefs = dedup
    // 旧字段统一清掉，避免新旧并存
    if ('taskRef' in item) delete item.taskRef
  }, tddTaskRefSaving)
  if (ok) {
    ElMessage.success(dedup.length
      ? `已关联到 ${dedup.length} 个任务：${dedup.join('、')}`
      : '已清除任务关联')
  }
}

/** 内部辅助：当前 tddTestSpecText 中指定 criterion 是否有任意 taskRef。 */
const c_hasTaskRef = (criterionId: string): boolean => {
  const list = parsedTddSpec.value?.acceptanceCriteria || []
  const c = list.find((x: any) => x?.id === criterionId)
  return effectiveTaskRefs(c).length > 0
}

/** 模块级「运行全部测试」：调 TestAuthor 编译断言 + 跑一遍 + 写回。 */
const runAllTddAssertions = () => {
  if (!selectedModule.value?.id) {
    ElMessage.error('请先选择模块')
    return
  }
  if (tddRunAllLoading.value) {
    ElMessage.warning('已有运行任务在进行中')
    return
  }
  if (!isTddTestSpecSaved.value) {
    ElMessage.warning('请先点击「保存测试」持久化当前 TDD 验收标准')
    return
  }
  tddRunAllLoading.value = true
  tddRunOutput.value = ''
  tddRunController = moduleTddRunAllStream(
    selectedModule.value.id,
    (line) => { tddRunOutput.value += line + '\n' },
    (moduleJson) => {
      tddRunAllLoading.value = false
      tddRunController = null
      try {
        const updated = JSON.parse(moduleJson)
        if (updated && updated.id) {
          selectedModule.value = updated
        }
        ElMessage.success('TDD 测试运行完成')
      } catch (e) {
        ElMessage.error('解析运行结果失败：' + (e as Error).message)
      }
    },
    (err) => {
      tddRunAllLoading.value = false
      tddRunController = null
      tddRunOutput.value += `\n[系统] ✗ 运行失败：${err}\n`
      ElMessage.error('运行失败：' + err)
    },
  )
}

/** 单条「重跑」：基于已编译的 tddAssertionsJson 重跑一条。 */
const runSingleTddAssertion = async (criterionId: string | null | undefined) => {
  if (!criterionId) return
  if (!selectedModule.value?.id) return
  if (!hasAssertionFor(criterionId)) {
    ElMessage.warning('该用例尚未编译为可执行断言，请先点击「运行全部测试」')
    return
  }
  if (tddRunAllLoading.value || tddRunSingleLoading.value) return
  tddRunSingleLoading.value = String(criterionId)
  try {
    const res: any = await ModuleApi.tddRunSingleAssertion(selectedModule.value.id, String(criterionId))
    if (res?.success) {
      if (res.data?.id) selectedModule.value = res.data
      // 从更新后的 module 中收集该 criterion 下所有 assertion（同 criteriaId/id 全部展示）
      const raw = (selectedModule.value as any)?.tddAssertionsJson
      const matched: any[] = []
      if (raw) {
        try {
          const arr = JSON.parse(raw)
          if (Array.isArray(arr)) {
            for (const a of arr) {
              const cid = a?.criteriaId || a?.id
              if (cid === criterionId) matched.push(a)
            }
          }
        } catch {}
      }

      // 顶部汇总：N 条全绿 / X 红 Y 绿 等
      const total = matched.length
      const reds = matched.filter(a => a?.lastStatus === 'RED').length
      const greens = matched.filter(a => a?.lastStatus === 'GREEN').length
      const summary = total === 0
        ? '未找到匹配断言'
        : (reds > 0
            ? `✗ ${reds} 红 / ${greens} 绿 / 共 ${total} 条`
            : (greens === total ? `✓ 全部 ${total} 条绿` : `${greens} 绿 / ${total - reds - greens} 待运行 / 共 ${total} 条`))

      const lines: string[] = []
      lines.push(`Criterion: ${criterionId}`)
      lines.push(`聚合状态: ${summary}`)
      lines.push('')

      matched.forEach((assertion, idx) => {
        if (idx > 0) lines.push('')
        lines.push(`── 断言 ${idx + 1}/${total} ──`)
        const status = assertion?.lastStatus || '?'
        const isGreen = status === 'GREEN'
        const statusIcon = isGreen ? '✓' : (status === 'RED' ? '✗' : '·')
        const statusLabel = isGreen ? '绿' : (status === 'RED' ? '红' : status)
        lines.push(`断言 ID: ${assertion?.id || '?'}`)
        lines.push(`状态: ${statusIcon} ${statusLabel}`)
        if (assertion?.type) lines.push(`类型: ${assertion.type}`)
        if (assertion?.executable) lines.push(`执行命令: ${assertion.executable}`)
        if (assertion?.description) lines.push(`描述: ${assertion.description}`)
        if (assertion?.lastRunAt) {
          const runTime = new Date(assertion.lastRunAt).toLocaleString('zh-CN')
          lines.push(`执行时间: ${runTime}`)
        }
        if (assertion?.lastDetail) {
          lines.push('详细输出:')
          lines.push(assertion.lastDetail)
        }
      })

      // 自动重试编排日志：与「运行全部」一致的 [自动重试] 行；reds=0 时后端不返回
      const retryNotices: string[] = Array.isArray(res.retryNotices) ? res.retryNotices : []
      if (retryNotices.length > 0) {
        lines.push('')
        lines.push('── 自动触发任务重试 ──')
        for (const n of retryNotices) lines.push(n)
        lines.push('')
        lines.push('提示：任务已在后台异步执行，等任务跑完后再次点击「重跑」或「运行全部」复查状态。')
      } else if (reds > 0) {
        lines.push('')
        lines.push('── 自动触发任务重试 ──')
        lines.push('（本次有红用例，但没有关联任务或 autoRetryTask=false，未触发重试。可在用例下方勾选「红了自动触发任务...重试」。）')
      }

      ElMessageBox.alert(lines.join('\n'), `${criterionId} 重跑完成（${total} 条断言）`, {
        confirmButtonText: '确定',
        customClass: 'tdd-rerun-detail-dialog',
        dangerouslyUseHTMLString: false,
      })
    } else {
      ElMessage.error(res?.message || '重跑失败')
    }
  } catch (e: any) {
    ElMessage.error('重跑失败：' + (e?.message || e))
  } finally {
    tddRunSingleLoading.value = null
  }
}

// ═════════════ 修复 agent（手动触发，无自动重试） ═════════════

/** 触发「修复全部红用例」：扫当前模块红 assertion → 单次 Claude 调用 → 写入 module_fix_history */
const fixAllRedAssertions = () => {
  if (!selectedModule.value?.id) { ElMessage.error('请先选择模块'); return }
  if (tddFixAllLoading.value || tddRunAllLoading.value) return
  if (tddRedAssertionCount.value === 0) {
    ElMessage.warning('当前模块没有红色用例，无需修复')
    return
  }
  tddFixAllLoading.value = true
  tddFixOutput.value = ''
  tddFixController = moduleTddFixAllStream(
    selectedModule.value.id,
    (line) => { tddFixOutput.value += line + '\n' },
    (data) => {
      tddFixAllLoading.value = false
      tddFixController = null
      try {
        const payload = JSON.parse(data)
        if (payload?.module?.id) selectedModule.value = payload.module
        ElMessage.success('修复完成，请点击「运行全部测试」复查')
      } catch (e) {
        ElMessage.warning('修复完成（done 帧解析异常: ' + (e as Error).message + '）')
      }
    },
    (err) => {
      tddFixAllLoading.value = false
      tddFixController = null
      tddFixOutput.value += `\n[系统] ✗ 修复失败：${err}\n`
      ElMessage.error('修复失败：' + err)
    },
  )
}

/** 触发「修复单条 criterion」 */
const fixSingleRedAssertion = (criterionId: string | null | undefined) => {
  if (!criterionId) return
  if (!selectedModule.value?.id) return
  if (tddFixAllLoading.value || tddFixSingleLoading.value || tddRunAllLoading.value) return
  if (!criterionIsRed(criterionId)) {
    ElMessage.warning('该 criterion 当前不是红色，无需修复')
    return
  }
  tddFixSingleLoading.value = String(criterionId)
  tddFixOutput.value = ''
  tddFixController = moduleTddFixSingleStream(
    selectedModule.value.id,
    String(criterionId),
    (line) => { tddFixOutput.value += line + '\n' },
    (data) => {
      tddFixSingleLoading.value = null
      tddFixController = null
      try {
        const payload = JSON.parse(data)
        if (payload?.module?.id) selectedModule.value = payload.module
        ElMessage.success(`${criterionId} 修复完成，请点击「运行全部测试」或单条「重跑」复查`)
      } catch (e) {
        ElMessage.warning('修复完成（done 帧解析异常: ' + (e as Error).message + '）')
      }
    },
    (err) => {
      tddFixSingleLoading.value = null
      tddFixController = null
      tddFixOutput.value += `\n[系统] ✗ 修复失败：${err}\n`
      ElMessage.error('修复失败：' + err)
    },
  )
}

// ═════════════ 修复记录弹框 ═════════════

const openFixHistoryDialog = async () => {
  if (!selectedModule.value?.id) { ElMessage.error('请先选择模块'); return }
  fixHistoryDialogVisible.value = true
  await reloadFixHistory()
}

const reloadFixHistory = async () => {
  if (!selectedModule.value?.id) return
  fixHistoryLoading.value = true
  try {
    const res: any = await ModuleApi.getFixHistory(selectedModule.value.id)
    if (res?.success) {
      fixHistoryList.value = Array.isArray(res.data) ? res.data : []
    } else {
      ElMessage.error(res?.message || '加载修复记录失败')
    }
  } catch (e: any) {
    ElMessage.error('加载修复记录失败：' + (e?.message || e))
  } finally {
    fixHistoryLoading.value = false
  }
}

const openFixHistoryDetail = async (row: any) => {
  if (!row?.id) return
  try {
    const res: any = await ModuleApi.getFixHistoryDetail(row.id)
    if (res?.success) {
      fixHistoryDetail.value = res.data
      fixHistoryDetailVisible.value = true
    } else {
      ElMessage.error(res?.message || '加载详情失败')
    }
  } catch (e: any) {
    ElMessage.error('加载详情失败：' + (e?.message || e))
  }
}

const formatCriteriaIds = (raw: string | null | undefined): string => {
  if (!raw) return ''
  try {
    const arr = JSON.parse(raw)
    if (Array.isArray(arr)) return arr.join(', ')
  } catch { /* ignore */ }
  return String(raw)
}

const fixStatusTag = (status: string | null | undefined): 'success' | 'danger' | 'warning' | 'info' => {
  switch (status) {
    case 'SUCCESS': return 'success'
    case 'FAILED': return 'danger'
    case 'RUNNING': return 'warning'
    default: return 'info'
  }
}

const fixStatusLabel = (status: string | null | undefined): string => {
  switch (status) {
    case 'SUCCESS': return '成功'
    case 'FAILED': return '失败'
    case 'RUNNING': return '进行中'
    default: return status || '—'
  }
}

const formatDateTime = (dt: string | null | undefined): string => {
  if (!dt) return ''
  try { return new Date(dt).toLocaleString('zh-CN') }
  catch { return String(dt) }
}

// ═════════════ 单条 criterion 重新生成 ═════════════

/** 打开「重新生成 criterion」弹框 */
const openRegenerateCriterionDialog = async (criterionId: string | null | undefined) => {
  if (!criterionId) return
  if (!selectedModule.value?.id) {
    ElMessage.error('请先选择模块')
    return
  }
  if (tddRegenerateLoading.value || testGenLoading.value) {
    ElMessage.warning('已有生成任务在进行中')
    return
  }
  regenerateCriterionId.value = criterionId
  regenerateCriterionDialogVisible.value = true
  regenerateCriterionPromptVars.value = []
  regenerateCriterionAccessUrls.value = null

  // 并发加载项目提示词变量 + 部署 URL
  regenerateCriterionPromptVarsLoading.value = true
  regenerateCriterionAccessUrlsLoading.value = true
  try {
    const [proj, urls] = await Promise.all([
      ProjectApi.getProjectDetail(props.projectId).catch(() => null),
      RuntimeStatusApi.getAccessUrls(props.projectId).catch(() => null),
    ])
    // 解析 promptVarsJson
    let rows: RegeneratePromptVarRow[] = []
    const raw = (proj as any)?.promptVarsJson
    if (typeof raw === 'string' && raw.trim()) {
      try {
        const arr = JSON.parse(raw)
        if (Array.isArray(arr)) {
          rows = arr
            .filter((e: any) => e && typeof e.key === 'string' && e.key.trim())
            .map((e: any) => ({
              key: String(e.key),
              value: e.value == null ? '' : String(e.value),
              remark: e.remark == null ? '' : String(e.remark),
              selected: false,
              showValue: false,
            }))
        }
      } catch (e) {
        console.warn('promptVarsJson 解析失败', e)
      }
    }
    regenerateCriterionPromptVars.value = rows
    regenerateCriterionAccessUrls.value = urls as any
  } finally {
    regenerateCriterionPromptVarsLoading.value = false
    regenerateCriterionAccessUrlsLoading.value = false
  }
}

const toggleSelectAllRegeneratePromptVars = (checked: boolean | string | number) => {
  const v = Boolean(checked)
  regenerateCriterionPromptVars.value.forEach(r => { r.selected = v })
}

/** 弹框确认 → 发起 SSE 重新生成 */
const confirmRegenerateCriterion = () => {
  if (!regenerateCriterionId.value) return
  if (!selectedModule.value?.id) {
    ElMessage.error('请先选择模块')
    return
  }
  const promptVarKeys = regenerateCriterionPromptVars.value.filter(r => r.selected).map(r => r.key)

  tddRegenerateLoading.value = regenerateCriterionId.value
  tddRegenerateOutput.value = ''
  regenerateCriterionDialogVisible.value = false

  tddRegenerateController = moduleTddRegenerateCriterionStream(
    selectedModule.value.id,
    regenerateCriterionId.value,
    promptVarKeys,
    (line) => {
      tddRegenerateOutput.value += line + '\n'
    },
    (moduleJson) => {
      tddRegenerateLoading.value = null
      tddRegenerateController = null
      try {
        const updated = JSON.parse(moduleJson)
        if (updated && updated.id) {
          selectedModule.value = updated
          tddTestSpecText.value = updated.tddTestSpecJson || ''
        }
        ElMessage.success('重新生成并执行完成')
      } catch (e) {
        ElMessage.error('解析生成结果失败：' + (e as Error).message)
      }
    },
    (err) => {
      tddRegenerateLoading.value = null
      tddRegenerateController = null
      tddRegenerateOutput.value += `\n[系统] ✗ 重新生成失败：${err}\n`
      ElMessage.error('重新生成失败：' + err)
    },
  )
}

/** AI 生成测试弹框：状态 */
const testGenDialogVisible = ref(false)
const testGenDialogMode = ref<'LEGACY' | 'TDD' | null>(null)
const maskPromptVarValue = (raw: string) => {
  if (!raw) return ''
  if (raw.length <= 4) return '***'
  return raw.slice(0, 2) + '***' + raw.slice(-2)
}

/** 打开「AI 生成测试用例」弹框。 */
const openGenerateTestDialog = (mode: 'LEGACY' | 'TDD') => {
  if (!selectedModule.value?.id) {
    ElMessage.error('请先选择模块')
    return
  }
  if (testGenLoading.value) {
    ElMessage.warning('已有生成任务在进行中')
    return
  }
  testGenDialogMode.value = mode
  testGenDialogVisible.value = true
}

/** 弹框确认 → 发起 SSE 生成。LEGACY 已存在用例时确认覆盖，并传 force=1 强制重新生成。 */
const confirmGenerateModuleTest = async () => {
  if (!testGenDialogMode.value) return
  if (!selectedModule.value?.id) {
    ElMessage.error('请先选择模块')
    return
  }
  const mode = testGenDialogMode.value

  const force = mode === 'LEGACY'
  if (force && (apiIntegrationTestText.value || webIntegrationTestText.value)) {
    try {
      await ElMessageBox.confirm(
        '将重新生成并覆盖现有 API/Web 集成测试用例，确定继续吗？',
        '确认重新生成',
        { confirmButtonText: '重新生成', cancelButtonText: '取消', type: 'warning' },
      )
    } catch {
      return
    }
  }

  testGenLoading.value = true
  testGenMode.value = mode
  testGenOutput.value = ''
  testGenDialogVisible.value = false

  // 清空旧的验收标准和断言结果，避免显示过时内容
  if (mode === 'TDD') {
    tddTestSpecText.value = ''
    if (selectedModule.value) {
      (selectedModule.value as any).tddAssertionsJson = '[]'
    }
  }

  testGenController = moduleGenerateTestStream(
    selectedModule.value.id,
    mode,
    [],
    (line) => {
      testGenOutput.value += line + '\n'
    },
    (payload) => {
      testGenLoading.value = false
      try {
        if (mode === 'TDD') {
          // 后端返回的是 AI 生成的验收标准 JSON（草稿，未写库）。
          // 回灌到 TDD 文本框，pretty-print 一下方便人工核对；不更新 selectedModule.tddTestSpecJson，
          // 这样 isTddTestSpecDirty 为 true，「保存测试」按钮会出现，用户点击后才真正落库。
          let formatted = payload
          try {
            formatted = JSON.stringify(JSON.parse(payload), null, 2)
          } catch { /* AI 返回非合法 JSON 时退回原样，编辑器仍可手工修复 */ }
          tddTestSpecText.value = formatted
          // 清空旧的断言结果，避免显示过时的红/绿卡片
          if (selectedModule.value) {
            (selectedModule.value as any).tddAssertionsJson = '[]'
          }
          activeTestTab.value = 'tdd'
          testGenOutput.value += '\n[系统] ✓ 已回灌到 TDD 草稿，请在「TDD 验收标准」tab 点击「保存测试」按钮持久化。\n'
          ElMessage.success('TDD 验收标准已生成（草稿），请点击「保存测试」持久化')
        } else {
          const updated = JSON.parse(payload)
          if (updated && updated.id) {
            selectedModule.value = updated
            apiIntegrationTestText.value = updated.apiIntegrationTest || ''
            webIntegrationTestText.value = updated.webIntegrationTest || ''
            tddTestSpecText.value = updated.tddTestSpecJson || ''
            testGenOutput.value += '\n[系统] ✓ 生成完成，已保存到模块。\n'
            ElMessage.success('LEGACY 测试用例已生成并保存到模块')
          } else {
            ElMessage.warning('生成完成，但未收到更新后的模块数据，请手动刷新')
          }
          loadData()
        }
      } catch (e) {
        ElMessage.error('解析生成结果失败：' + (e as Error).message)
      } finally {
        testGenMode.value = null
        testGenDialogMode.value = null
        testGenController = null
      }
    },
    (err) => {
      testGenLoading.value = false
      testGenMode.value = null
      testGenDialogMode.value = null
      testGenController = null
      testGenOutput.value += `\n[系统] ✗ 生成失败：${err}\n`
      ElMessage.error('生成失败：' + err)
    },
    force,
  )
}

/** 集成测试编辑器内部触发了 AI 加场景 / 运行场景后回写模块。 */
const onModuleSpecUpdated = (updated: any) => {
  if (!updated || !updated.id) return
  selectedModule.value = updated
  apiIntegrationTestText.value = updated.apiIntegrationTest || ''
  webIntegrationTestText.value = updated.webIntegrationTest || ''
}

// 打开拆解对话框
const openDecomposeDialog = () => {
  decomposeForm.value = {
    requirementDescription: '',
    selectedPlanFiles: [],
    generateTestSpec: true,
    autoConfig: true
  }
  decomposeOutput.value = ''
  decomposeDialogVisible.value = true
  loadPlanFiles()
}

// 加载 Plan 文件列表
const loadPlanFiles = async () => {
  if (!props.projectId) return
  planFilesLoading.value = true
  try {
    const res = await ProjectApi.getProjectDocs(props.projectId)
    // request 拦截器直接返回 data，不包装 success 字段
    if (Array.isArray(res)) {
      planFiles.value = res
    } else {
      planFiles.value = []
    }
  } catch (e) {
    console.error('Failed to load plan files:', e)
    planFiles.value = []
  } finally {
    planFilesLoading.value = false
  }
}

// 获取文件名
const getFileName = (path: string) => {
  return path.split('/').pop() || path
}

// 移除选中的 Plan 文件
const removePlanFile = (path: string) => {
  const index = decomposeForm.value.selectedPlanFiles.indexOf(path)
  if (index > -1) {
    decomposeForm.value.selectedPlanFiles.splice(index, 1)
  }
}

// 开始拆解
const startDecompose = () => {
  if (!props.projectId) {
    ElMessage.error('项目ID不存在')
    return
  }

  // 校验：需求描述和需求文档二选一
  const hasDescription = !!decomposeForm.value.requirementDescription.trim()
  const hasPlanFiles = decomposeForm.value.selectedPlanFiles.length > 0

  if (!hasDescription && !hasPlanFiles) {
    ElMessage.error('请填写需求描述或选择需求文档')
    return
  }

  if (hasDescription && hasPlanFiles) {
    ElMessage.error('需求描述和需求文档只能选择一项，请清空其中一项')
    return
  }

  decomposeLoading.value = true
  decomposeOutput.value = ''

  // 检测 URL 中是否有 mock=true 参数
  const mockMode = route.query.mock === 'true'

  decomposeController.value = moduleDecomposeStream(
    decomposeForm.value.requirementDescription.trim(),  // 需求描述
    props.projectId,
    decomposeForm.value.selectedPlanFiles,
    (line) => {
      decomposeOutput.value += line + '\n'
    },
    (result) => {
      decomposeLoading.value = false
      const len = result?.length ?? 0
      decomposeOutput.value += `\n[done] 收到 SSE done 帧，payload ${len} 字节\n`
      try {
        const stripped = stripJsonFence(result)
        decomposeResult.value = JSON.parse(stripped)
        const moduleCount = Array.isArray(decomposeResult.value) ? decomposeResult.value.length : 0
        decomposeOutput.value += `[done] JSON 解析成功，共 ${moduleCount} 个模块\n`
        decomposeDialogVisible.value = false
        previewDialogVisible.value = true
      } catch (e) {
        const msg = (e as Error).message
        const head = result?.slice(0, 800) ?? ''
        const tail = result && result.length > 1600 ? '\n...\n' + result.slice(-800) : ''
        console.error('[ModuleTaskTab] 解析失败:', e, 'payload preview:', result?.slice(0, 200))
        decomposeOutput.value += `[done] JSON 解析失败：${msg}\n---- payload 预览 ----\n${head}${tail}\n----------------------\n`
        ElMessageBox.alert(
          `<div style="word-break:break-all;white-space:pre-wrap;max-height:300px;overflow:auto;">${escapeHtml(msg)}\n\n常见原因：AI 在 command/playwrightCode 字段里写了未转义的双引号。请展开「执行日志」查看 payload 后重试。</div>`,
          '解析拆解结果失败',
          { dangerouslyUseHTMLString: true, type: 'error', confirmButtonText: '我知道了' }
        ).catch(() => {})
      }
    },
    (error) => {
      decomposeLoading.value = false
      console.error('[ModuleTaskTab] SSE 出错:', error)
      decomposeOutput.value += `\n[error] ${error}\n`
      ElMessageBox.alert(
        `<div style="word-break:break-all;white-space:pre-wrap;">${escapeHtml(error)}</div>`,
        'AI 拆解失败',
        { dangerouslyUseHTMLString: true, type: 'error', confirmButtonText: '我知道了' }
      ).catch(() => {})
    },
    mockMode
  )
}

// 拆解保存完成
const onDecomposeSaved = () => {
  loadData()
}

// 监听 projectId 变化
watch(() => props.projectId, () => {
  stopPolling()
  loadData()
}, { immediate: true })

onMounted(() => {
  loadData()
  window.addEventListener('keydown', onExecutionSummaryKeydown)
})

// 组件卸载时清理
onUnmounted(() => {
  stopPolling()
  window.removeEventListener('keydown', onExecutionSummaryKeydown)
})
</script>

<style scoped>
.module-task-container {
  min-height: 400px;
}

.access-urls-card {
  margin-bottom: 16px;
}

.module-tree-card {
  height: 100%;
}

.module-tree-card :deep(.el-tree-node__content) {
  overflow: hidden;
}

.module-tree-card :deep(.el-tree-node__content > .el-tree-node__children) {
  overflow: visible;
}

.tree-node {
  display: flex;
  align-items: center;
  gap: 4px;
  width: 100%;
  overflow: hidden;
}

.tree-icon {
  color: #409eff;
  flex-shrink: 0;
}

.tree-icon-expand {
  cursor: pointer;
  padding: 2px;
  border-radius: 4px;
  transition: background-color 0.2s;
}

.tree-icon-expand:hover {
  background-color: rgba(64, 158, 255, 0.1);
}

.tree-icon-task {
  color: #67c23a;
  cursor: default;
}

.tree-label {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.task-count {
  color: #909399;
  font-size: 12px;
}

.detail-card {
  min-height: 400px;
}

.step-item {
  padding: 4px 0;
}

.step-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}

.step-header-spacer {
  flex: 1;
}

.step-seq {
  font-weight: 500;
  color: #303133;
}

.step-action {
  color: #606266;
  margin-bottom: 4px;
}

.step-files,
.step-validation,
.step-migration {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
}

.step-files code {
  background: #f5f7fa;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 12px;
}

.step-migration code {
  background: #ecfdf5;
  padding: 2px 6px;
  border-radius: 4px;
  color: #065f46;
  font-size: 12px;
}

.step-plan-excerpt {
  margin-top: 6px;
  padding: 8px 12px;
  background: #fffbeb;
  border-left: 3px solid #f59e0b;
  border-radius: 4px;
  font-size: 13px;
}

.plan-excerpt-content {
  margin: 4px 0 0 0;
  padding: 0;
  color: #92400e;
  font-style: italic;
  white-space: pre-wrap;
}

.step-evidence {
  margin-top: 6px;
  font-size: 12px;
  color: #909399;
}

.evidence-list {
  list-style: none;
  margin: 4px 0 0 0;
  padding: 8px 12px;
  background: #f8fafc;
  border-left: 3px solid #64748b;
  border-radius: 4px;
}

.evidence-item {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 6px;
  padding: 3px 0;
  line-height: 1.6;
  border-bottom: 1px dashed #e2e8f0;
}

.evidence-item:last-child {
  border-bottom: none;
}

.evidence-type {
  color: #475569;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 11px;
}

.evidence-label {
  color: #334155;
  word-break: break-all;
}

.evidence-label-has-tooltip {
  cursor: help;
  text-decoration: underline dotted #94a3b8;
  text-underline-offset: 2px;
}

.evidence-detail {
  color: #64748b;
  font-size: 11px;
  margin-left: 4px;
  flex-basis: 100%;
  padding-left: 6px;
  border-left: 2px solid #cbd5e1;
}

.evidence-detail-failed {
  color: #b91c1c;
  border-left-color: #ef4444;
}

.evidence-full-command {
  flex-basis: 100%;
  margin: 4px 0 0 0;
  padding: 6px 10px;
  background: #fef2f2;
  border-left: 2px solid #ef4444;
  border-radius: 3px;
  color: #991b1b;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 11px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-all;
}


.decompose-output {
  margin-top: 16px;
  padding: 12px;
  background: #f5f7fa;
  border-radius: 4px;
  max-height: 300px;
  overflow: auto;
}

.decompose-output pre {
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
  font-size: 12px;
}

.plan-files-container {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}

.plan-files-container .el-select {
  flex: 1;
}

.plan-option {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.plan-name {
  font-size: 14px;
  color: #303133;
}

.plan-path {
  font-size: 12px;
  color: #909399;
}

.selected-files {
  margin-top: 8px;
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.output-log {
  max-height: 200px;
  overflow: auto;
  font-size: 12px;
  line-height: 1.5;
}

.retry-info-card {
  padding: 12px;
  background: #f9fafb;
  border-radius: 6px;
  border: 1px solid #e5e7eb;
}

.retry-item {
  padding: 4px 0;
}

.retry-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 4px;
}

.retry-label {
  font-weight: 500;
  color: #303133;
  font-size: 13px;
}

.retry-error,
.retry-cause,
.retry-fix,
.retry-reason {
  font-size: 12px;
  margin-top: 2px;
}

.retry-error {
  color: #e74c3c;
}

.retry-cause {
  color: #606266;
}

.retry-fix {
  color: #065f46;
}

.retry-reason {
  color: #909399;
}

.execution-summary-pre {
  margin: 0;
  padding: 12px;
  max-height: 360px;
  overflow-y: auto;
  background-color: #0f172a;
  color: #e2e8f0;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.5;
  border: 1px solid #334155;
  border-radius: 4px;
}
.exec-summary-text {
  margin: 0;
  padding: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  white-space: pre-wrap;
  word-break: break-all;
}
.deploy-detail-block {
  margin: 4px 0;
  border: 1px solid #334155;
  border-radius: 3px;
  background-color: #1e293b;
}
.deploy-detail-header {
  padding: 4px 8px;
  cursor: pointer;
  user-select: none;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  color: #cbd5e1;
}
.deploy-detail-header:hover {
  background-color: #273449;
}
.deploy-detail-header.is-fail {
  border-left: 3px solid #f87171;
}
.deploy-caret {
  display: inline-block;
  width: 12px;
  color: #94a3b8;
}
.deploy-meta {
  color: #94a3b8;
}
.deploy-badges {
  display: inline-flex;
  gap: 4px;
}
.deploy-badge {
  font-size: 11px;
  padding: 1px 6px;
  border-radius: 2px;
  border: 1px solid transparent;
}
.deploy-badge.warn {
  background-color: #422006;
  border-color: #92400e;
  color: #fcd34d;
}
.deploy-badge.ok {
  background-color: #052e16;
  border-color: #166534;
  color: #86efac;
}
.deploy-badge.err {
  background-color: #450a0a;
  border-color: #991b1b;
  color: #fca5a5;
}
.deploy-hint {
  margin-left: auto;
  font-size: 11px;
  color: #64748b;
}
.deploy-detail-body {
  margin: 0;
  padding: 8px 8px 8px 24px;
  background-color: #0b1220;
  border-top: 1px solid #334155;
  color: #cbd5e1;
  font: inherit;
  white-space: pre-wrap;
  word-break: break-all;
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
  padding: 4px 0;
}

.fact-check-label {
  color: #303133;
  flex: 1;
  min-width: 0;
}

/* TDD 验收标准结构化卡片 */
.tdd-criteria-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.tdd-criterion-card {
  border: 1px solid #e4e7ed;
  border-radius: 6px;
  background: #fff;
  padding: 8px 10px;
  transition: border-color 0.15s ease;
}
.tdd-criterion-card.is-green {
  border-color: #67c23a;
  background: #f6fff0;
}
.tdd-criterion-card.is-red {
  border-color: #f56c6c;
  background: #fff3f3;
}
.tdd-criterion-header {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.tdd-criterion-behavior {
  font-size: 13px;
  font-weight: 600;
  color: #303133;
  margin-left: 4px;
}
.tdd-criterion-body {
  margin-top: 6px;
  font-size: 12px;
  color: #606266;
  line-height: 1.6;
}
.tdd-criterion-body > div {
  word-break: break-word;
}
.tdd-criterion-label {
  display: inline-block;
  min-width: 48px;
  color: #909399;
  font-weight: 500;
}
.tdd-criterion-detail {
  margin-top: 4px;
}
.tdd-criterion-detail-pre {
  display: block;
  margin: 4px 0 0 0;
  background: #f5f7fa;
  border-radius: 4px;
  padding: 6px 8px;
  font-size: 11px;
  max-height: 180px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
}
.tdd-criterion-footer {
  display: flex;
  align-items: center;
  margin-top: 6px;
  padding-top: 6px;
  border-top: 1px dashed #e4e7ed;
}
.tdd-task-select {
  width: 200px;
}
.tdd-task-select :deep(.el-input__wrapper) {
  font-size: 12px;
}
</style>

<!-- el-tooltip popper 通过 teleport 渲染到 body，无法被 scoped 选择器命中，
     用全局 style 才能给"事实校验"hover 弹层定制样式 -->
<style>
.evidence-tooltip-popper {
  max-width: 560px;
}
.evidence-tooltip-content {
  display: flex;
  flex-direction: column;
  gap: 6px;
  line-height: 1.55;
  font-size: 12px;
  word-break: break-word;
  white-space: pre-wrap;
}
.evidence-tooltip-content .ev-tt-row {
  display: flex;
  gap: 4px;
  align-items: flex-start;
}
.evidence-tooltip-content .ev-tt-key {
  flex-shrink: 0;
  color: #94a3b8;
}
.evidence-tooltip-content .ev-tt-value {
  color: #f1f5f9;
}
/* TDD 断言重跑详情弹窗样式 */
.tdd-rerun-detail-dialog {
  max-width: 700px;
}
.tdd-rerun-detail-dialog .el-message-box__content p {
  white-space: pre-wrap;
  font-family: ui-monospace, monospace;
  font-size: 13px;
  line-height: 1.5;
  max-height: 400px;
  overflow-y: auto;
  background: #f8fafc;
  padding: 12px;
  border-radius: 6px;
  margin: 0;
}
</style>
