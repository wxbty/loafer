<template>
  <div class="h-full flex flex-col">
    <header class="border-b border-slate-200 bg-white shadow-sm">
      <div class="flex h-14 items-center justify-between gap-3">
        <el-button type="primary" @click="goBack">
          <el-icon><ArrowLeft /></el-icon>
          返回项目列表
        </el-button>
        <h1 class="min-w-0 flex-1 truncate text-center text-lg font-semibold text-slate-900">
          {{ project.name }}
        </h1>
        <div class="flex shrink-0 items-center gap-2">
          <el-button @click="editProject">
            <el-icon><Edit /></el-icon>
            编辑
          </el-button>
          <el-button type="danger" @click="deleteProject">
            <el-icon><Delete /></el-icon>
            删除
          </el-button>
        </div>
      </div>
    </header>

    <div class="flex-1 overflow-auto">
        <el-tabs v-model="detailActiveTab" class="detail-tabs-clean w-full">
          <el-tab-pane label="工程目录" name="work">
            <el-row :gutter="20">
              <!-- 左侧：工程目录树 -->
              <el-col :span="12">
                <el-card class="file-tree-card file-tree-card--half" shadow="hover">
                  <template #header>
                    <div class="file-tree-header">
                      <div
                        class="file-tree-title file-tree-title--clickable"
                        :class="{ 'is-git-mode': workTreeGitMode }"
                        role="button"
                        tabindex="0"
                        :aria-pressed="workTreeGitMode"
                        :title="workTreeGitMode ? '点击切换为完整工程目录' : '点击仅显示 Git 未提交的文件与目录'"
                        @click="toggleWorkTreeGitMode"
                        @keydown.enter.prevent="toggleWorkTreeGitMode"
                      >
                        <el-icon><FolderOpened /></el-icon>
                        <span v-if="workTreeGitLoading" class="text-slate-400">加载中…</span>
                        <span v-else>{{ workTreeGitMode ? 'Git 未提交' : '工程目录' }}</span>
                      </div>
                      <div class="file-tree-header-actions">
                        <el-button
                          v-if="workTreeGitMode"
                          size="small"
                          type="primary"
                          :loading="gitPushLoading"
                          @click="openGitPushDialog"
                          @dblclick.stop="handleAiGitPush"
                        >
                          Git Push
                        </el-button>
                        <el-button
                          v-if="workTreeGitMode"
                          size="small"
                          :loading="gitHistoryLoading"
                          @click="openGitHistoryDialog"
                        >
                          History
                        </el-button>
                        <span class="file-tree-path file-tree-path--clickable" :title="'点击执行 git pull'" @click="handleGitPull">{{ project.workDir }}</span>
                      </div>
                    </div>
                  </template>

                  <el-empty v-if="!project.workDir" description="未设置工作目录" />
                  <el-tree
                    v-else
                    :key="workTreeVersion"
                    :props="workTreeProps"
                    :load="loadTreeNode"
                    lazy
                    node-key="path"
                    highlight-current
                    :expand-on-click-node="true"
                    @node-click="handleWorkNodeClick"
                    @node-contextmenu="onWorkDirNodeContextMenu"
                  >
                    <template #default="scope">
                      <span v-if="scope?.node && scope?.data" class="tree-node">
                        <el-icon v-if="scope.data.isDirectory" class="tree-icon tree-icon-folder">
                          <FolderOpened v-if="scope.node.expanded" />
                          <Folder v-else />
                        </el-icon>
                        <el-icon v-else class="tree-icon tree-icon-file">
                          <Document />
                        </el-icon>
                        <span
                          class="tree-label"
                          :class="{
                            'tree-label-previewable':
                              !scope.data.isDirectory && isPreviewable(scope.data.name)
                          }"
                        >
                          {{ scope.data.name }}
                        </span>
                        <span class="tree-created">{{
                          formatFileTime(scope.data.creationTime ?? scope.data.lastModified)
                        }}</span>
                        <span v-if="!scope.data.isDirectory" class="tree-size">{{
                          formatFileSize(scope.data.size)
                        }}</span>
                        <el-button
                          v-if="!scope.data.isDirectory && scope.data.path"
                          link
                          type="primary"
                          size="small"
                          class="tree-download-btn"
                          @click.stop="downloadWorkDirFile(scope.data.path, scope.data.name)"
                        >
                          下载
                        </el-button>
                      </span>
                    </template>
                  </el-tree>
                </el-card>
              </el-col>

              <!-- 右侧：服务状态卡片 -->
              <el-col :span="12">
                <el-card class="service-status-card glass-card" shadow="hover">
                  <template #header>
                    <div class="flex items-center justify-between">
                      <div class="flex items-center">
                        <el-icon><Monitor /></el-icon>
                        <span class="ml-2">服务状态</span>
                      </div>
                      <el-button size="small" :loading="runtimeStatusLoading" @click="loadRuntimeStatus">
                        <el-icon><Refresh /></el-icon>
                      </el-button>
                    </div>
                  </template>

                  <!-- 后端服务 -->
                  <div class="service-status-item">
                    <div class="service-status-header">
                      <span class="service-status-label">后端服务</span>
                      <el-tag :type="runtimeStatus.backend?.status === 'running' ? 'success' : 'danger'" size="small">
                        {{ runtimeStatus.backend?.status === 'running' ? '运行中' : '已停止' }}
                      </el-tag>
                    </div>
                    <div class="service-status-info">
                      <span v-if="runtimeStatus.backend?.pid" class="service-status-time">
                        PID: {{ runtimeStatus.backend.pid }}
                      </span>
                      <span v-if="runtimeStatus.backend?.startTime" class="service-status-time ml-2">
                        开始: {{ runtimeStatus.backend.startTime }}
                      </span>
                      <span v-if="runtimeStatus.backend?.port" class="service-status-time ml-2">
                        端口: {{ runtimeStatus.backend.port }}
                      </span>
                      <span v-if="!runtimeStatus.backend?.pid && !runtimeStatus.backend?.port" class="service-status-time text-slate-400">-</span>
                    </div>
                    <div v-if="runtimeStatus.accessUrls?.backendApi" class="service-status-info mt-1">
                      <a :href="runtimeStatus.accessUrls.backendApi" target="_blank" class="service-status-link">
                        <el-icon size="12"><Link /></el-icon>
                        {{ runtimeStatus.accessUrls.backendApi }}
                      </a>
                    </div>
                  </div>

                  <!-- 前端服务 -->
                  <div v-if="runtimeStatus.accessUrls?.frontendDev" class="service-status-item">
                    <div class="service-status-header">
                      <span class="service-status-label">前端服务</span>
                    </div>
                    <div class="service-status-info">
                      <a :href="runtimeStatus.accessUrls.frontendDev" target="_blank" class="service-status-link">
                        <el-icon size="12"><Link /></el-icon>
                        {{ runtimeStatus.accessUrls.frontendDev }}
                      </a>
                    </div>
                  </div>

                  <!-- 前端构建 -->
                  <div class="service-status-item">
                    <div class="service-status-header">
                      <span class="service-status-label">前端构建</span>
                      <el-tag :type="runtimeStatus.frontend?.frontendDistDirExists ? 'success' : 'warning'" size="small">
                        {{ runtimeStatus.frontend?.frontendDistDirExists ? '已部署' : '未部署' }}
                      </el-tag>
                    </div>
                    <div class="service-status-info">
                      <span class="service-status-time text-slate-500">源码: {{ project.workDir }}/frontend</span>
                    </div>
                    <div class="service-status-info mt-1">
                      <span class="service-status-time text-slate-500">发布: {{ runtimeStatus.frontend?.frontendDistDir || '-' }}</span>
                    </div>
                    <div v-if="runtimeStatus.frontend?.frontendDistLastModified" class="service-status-info mt-1">
                      <span class="service-status-time">更新: {{ runtimeStatus.frontend.frontendDistLastModified }}</span>
                    </div>
                  </div>

                  <!-- 操作按钮 - 三层布局 -->
                  <div class="action-grid">
                    <!-- 第一层：核心操作 -->
                    <div class="action-row action-row--primary">
                      <button class="action-card action-card--primary" @click="openDeployDialog">
                        <el-icon :size="28" class="action-card-icon"><VideoPlay /></el-icon>
                        <span class="action-card-label">部署</span>
                      </button>
                      <button class="action-card action-card--primary" @click="interactiveTerminalVisible = true">
                        <el-icon :size="28" class="action-card-icon"><Monitor /></el-icon>
                        <span class="action-card-label">CC终端</span>
                      </button>
<el-popover
                        placement="bottom"
                        :width="320"
                        trigger="click"
                        v-model:visible="sessionRecoveryPopoverVisible"
                        @show="loadClaudeHistorySessions"
                        popper-class="glass-popover"
                      >
                        <template #reference>
                          <button class="action-card action-card--primary">
                            <el-icon :size="28" class="action-card-icon"><RefreshRight /></el-icon>
                            <span class="action-card-label">CC会话</span>
                          </button>
                        </template>
                        <div v-loading="sessionRecoveryLoading" class="session-recovery-list">
                          <div
                            v-for="session in claudeHistorySessions"
                            :key="session.claudeSessionId"
                            class="session-recovery-item"
                            @click="handleResumeSession(session)"
                          >
                            <div class="session-recovery-item-main">
                              <span class="session-recovery-item-summary" :title="session.summary || session.claudeSessionId">
                                {{ session.summary || session.claudeSessionId.substring(0, 8) + '...' }}
                              </span>
                              <span class="session-recovery-item-time">{{ session.lastModified }}</span>
                            </div>
                            <span class="session-recovery-item-size">{{ formatSessionSize(session.jsonlSize) }}</span>
                          </div>
                          <div v-if="!sessionRecoveryLoading && claudeHistorySessions.length === 0" class="session-recovery-empty">
                            暂无历史会话
                          </div>
                        </div>
                      </el-popover>
                      <button
                        v-if="miniProgramDetect?.detected"
                        class="action-card action-card--primary"
                        :disabled="miniProgramUploadLoading"
                        @click="handlePublishMiniProgram"
                      >
                        <el-icon :size="28" class="action-card-icon"><Promotion /></el-icon>
                        <span class="action-card-label">发布小程序</span>
                      </button>
                    </div>

                    <!-- 底部：更多 -->
                    <div class="action-row action-row--footer" v-if="showMoreActions">
                      <button class="action-card" @click="moreActionsDialogVisible = true">
                        <el-icon :size="24" class="action-card-icon"><List /></el-icon>
                        <span class="action-card-label">更多</span>
                      </button>
                    </div>
                  </div>
                </el-card>
              </el-col>
            </el-row>
          </el-tab-pane>



          <el-tab-pane label="模块任务" name="moduleTasks" lazy>
            <ModuleTaskTab v-if="project.id" :project-id="project.id" />
          </el-tab-pane>

          <el-tab-pane label="环境变量" name="envVars" lazy>
            <el-row :gutter="20">
              <el-col :span="24">
                <el-card class="tasks-list" shadow="hover">
                  <template #header>
                    <div class="flex w-full items-center justify-between">
                      <div class="flex items-center">
                        <el-icon><Setting /></el-icon>
                        <span>环境变量管理</span>
                      </div>
                      <div class="flex items-center gap-2">
                        <el-button
                          type="warning"
                          :disabled="envVarsSelected.size === 0"
                          @click="copyEnvVarsToPromptVars"
                        >
                          复制到提示词
                        </el-button>
                        <el-button
                          :disabled="envVars.length === 0"
                          @click="selectAllEnvVars"
                        >
                          全选
                        </el-button>
                        <el-button
                          :disabled="envVarsSelected.size === 0"
                          @click="clearEnvVarsSelection"
                        >
                          清空选择
                        </el-button>
                        <el-button type="primary" @click="addEnvVar">新增环境变量</el-button>
                        <el-button type="success" :loading="envVarsSaving" @click="saveEnvVars">保存</el-button>
                      </div>
                    </div>
                  </template>

                  <el-empty v-if="!envVars.length" description="暂无环境变量，点击「新增环境变量」添加" />
                  <el-table v-else :data="envVars" class="w-full">
                    <el-table-column width="55">
                      <template #default="scope">
                        <el-checkbox
                          :model-value="envVarsSelected.has(scope.$index)"
                          @update:model-value="(v: boolean) => toggleEnvVarSelect(scope.$index, v)"
                          @click.stop
                        />
                      </template>
                    </el-table-column>
                    <el-table-column label="#" width="60">
                      <template #default="scope">
                        {{ (scope.$index || 0) + 1 }}
                      </template>
                    </el-table-column>
                    <el-table-column label="变量名 (Key)" min-width="200">
                      <template #default="scope">
                        <el-input
                          v-model="scope.row.key"
                          placeholder="如: JAVA_OPTS"
                          :class="{ 'is-error': scope.row.keyError }"
                          @input="scope.row.keyError = false"
                        />
                        <span v-if="scope.row.keyError" class="text-xs text-red-500">格式无效</span>
                      </template>
                    </el-table-column>
                    <el-table-column label="变量值 (Value)" min-width="300">
                      <template #default="scope">
                        <el-input
                          v-model="scope.row.value"
                          placeholder="如: -Xms512m -Xmx1024m"
                          type="textarea"
                          :rows="2"
                        />
                      </template>
                    </el-table-column>
                    <el-table-column label="操作" width="120">
                      <template #default="scope">
                        <el-button type="danger" link @click="removeEnvVar(scope.$index)">删除</el-button>
                      </template>
                    </el-table-column>
                  </el-table>

                  <div class="mt-4 p-3 bg-slate-50 rounded text-sm text-slate-600">
                    <div class="font-medium mb-2">说明：</div>
                    <ul class="list-disc list-inside space-y-1">
                      <li>环境变量将在部署后端时注入到 JVM 参数中</li>
                      <li>变量名只允许字母、数字、下划线，且不能以数字开头</li>
                      <li>变量值会在部署时作为 <code class="bg-slate-200 px-1 rounded">-Dkey=value</code> 传递给 JVM</li>
                      <li>选中记录后点击「复制到提示词」可将环境变量复制到提示词变量表格中</li>
                    </ul>
                  </div>
                </el-card>
              </el-col>
            </el-row>

            <!-- 提示词变量 -->
            <el-row :gutter="20" class="mt-4">
              <el-col :span="24">
                <el-card class="tasks-list" shadow="hover">
                  <template #header>
                    <div class="flex w-full items-center justify-between">
                      <div class="flex items-center">
                        <el-icon><Document /></el-icon>
                        <span>提示词变量管理</span>
                      </div>
                      <div class="flex items-center gap-2">
                        <el-button type="primary" @click="addPromptVar">新增提示词变量</el-button>
                        <el-button type="success" :loading="promptVarsSaving" @click="savePromptVars">保存</el-button>
                      </div>
                    </div>
                  </template>

                  <el-empty v-if="!promptVars.length" description="暂无提示词变量，点击「新增提示词变量」添加" />
                  <el-table v-else :data="promptVars" class="w-full">
                    <el-table-column label="#" width="60">
                      <template #default="scope">
                        {{ (scope.$index || 0) + 1 }}
                      </template>
                    </el-table-column>
                    <el-table-column label="变量名 (Key)" min-width="200">
                      <template #default="scope">
                        <el-input
                          v-model="scope.row.key"
                          placeholder="如: API_KEY"
                          :class="{ 'is-error': scope.row.keyError }"
                          :disabled="scope.row.readonly"
                          @input="scope.row.keyError = false"
                        />
                        <span v-if="scope.row.keyError" class="text-xs text-red-500">格式无效</span>
                      </template>
                    </el-table-column>
                    <el-table-column label="变量值 (Value)" min-width="300">
                      <template #default="scope">
                        <el-input
                          v-model="scope.row.value"
                          placeholder="如: sk-xxx"
                          type="textarea"
                          :rows="2"
                          :disabled="scope.row.readonly"
                        />
                      </template>
                    </el-table-column>
                    <el-table-column label="备注" min-width="200">
                      <template #default="scope">
                        <el-input
                          v-model="scope.row.remark"
                          placeholder="用途说明，如：第三方 API Key"
                          :disabled="scope.row.readonly"
                        />
                      </template>
                    </el-table-column>
                    <el-table-column label="操作" width="120">
                      <template #default="scope">
                        <el-button type="danger" link @click="removePromptVar(scope.$index)">删除</el-button>
                      </template>
                    </el-table-column>
                  </el-table>

                  <div class="mt-4 p-3 bg-slate-50 rounded text-sm text-slate-600">
                    <div class="font-medium mb-2">说明：</div>
                    <ul class="list-disc list-inside space-y-1">
                      <li>提示词变量用于模块任务生成测试用例时注入到 AI 提示词中</li>
                      <li>变量名只允许字母、数字、下划线，且不能以数字开头</li>
                      <li>在「AI 生成测试用例」弹框中勾选需要注入的变量</li>
                      <li>从环境变量复制的记录为只读状态，如需修改请在环境变量表格中修改后重新复制</li>
                    </ul>
                  </div>
                </el-card>
              </el-col>
            </el-row>
          </el-tab-pane>

          <el-tab-pane label="执行计划" name="plan" lazy>
            <PlanPanel v-if="project.id" :project-id="project.id" @decomposed="handlePlanDecomposed" />
          </el-tab-pane>

          <el-tab-pane label="部署运维" name="deploy" lazy>
            <DeploymentPanel v-if="project.id" :project-id="project.id" @status-change="handleDeployStatusChange" />
          </el-tab-pane>

          <el-tab-pane label="测试管理" name="test" lazy>
            <TestPanel v-if="project.id" :project-id="project.id" />
          </el-tab-pane>

        </el-tabs>
    </div>



    <!-- 更多操作弹框 -->
    <el-dialog
      v-model="moreActionsDialogVisible"
      title="更多操作"
      width="520px"
      destroy-on-close
      class="shadcn-form-dialog"
    >
      <div class="more-actions-grid">
        <el-button size="default" :loading="envCheckLoading" @click="() => { handleEnvCheck(); moreActionsDialogVisible = false }">
          <el-icon><Monitor /></el-icon>
          环境检测
        </el-button>
        <el-button size="default" type="primary" :loading="installDepsLoading" @click="() => { handleInstallDeps(); moreActionsDialogVisible = false }">
          <el-icon><Download /></el-icon>
          安装依赖
        </el-button>
        <el-button size="default" type="primary" plain :loading="updateFeatureListLoading" @click="() => { handleUpdateFeatureList(); moreActionsDialogVisible = false }">
          <el-icon><List /></el-icon>
          更新任务清单
        </el-button>
        <el-button size="default" type="warning" :loading="rebuildVenvLoading" @click="() => { handleRebuildPythonVenv(); moreActionsDialogVisible = false }">
          <el-icon><RefreshRight /></el-icon>
          重建venv(3.10)
        </el-button>
        <el-button size="default" @click="() => { openCliOsUserDialog(); moreActionsDialogVisible = false }">
          <el-icon><User /></el-icon>
          CLI 系统用户
        </el-button>
        <el-button size="default" type="success" :loading="initDeployScriptLoading" @click="() => { handleInitDeployScript(); moreActionsDialogVisible = false }">
          <el-icon><Document /></el-icon>
          初始化部署脚本
        </el-button>
        <el-button size="default" type="primary" @click="() => { openDeployDialog(); moreActionsDialogVisible = false }">
          <el-icon><Upload /></el-icon>
          部署到服务器
        </el-button>
      </div>
      <div v-if="project.cliOsUser" class="more-actions-user-badge">
        <el-tag type="info" size="small">执行身份: {{ project.cliOsUser }}</el-tag>
      </div>
    </el-dialog>

    <!-- 编辑项目对话框 -->
    <el-dialog
      v-model="editVisible"
      title="编辑项目"
      width="50%"
      destroy-on-close
      class="shadcn-form-dialog"
    >
      <Card class="border-0 shadow-none">
        <CardHeader class="pb-2">
          <CardTitle class="text-lg">项目信息</CardTitle>
          <CardDescription>修改名称、路径与状态</CardDescription>
        </CardHeader>
        <CardContent>
      <el-form :model="editForm" :rules="editRules" ref="editFormRef" label-width="100px">
        <el-form-item label="项目名称" prop="name">
          <el-input v-model="editForm.name" placeholder="请输入项目名称" />
        </el-form-item>
        <el-form-item label="项目描述" prop="description">
          <el-input v-model="editForm.description" type="textarea" :rows="3" placeholder="请输入项目描述" />
        </el-form-item>
        <el-form-item label="Git地址" prop="gitUrl">
          <el-input v-model="editForm.gitUrl" placeholder="可选，输入Git仓库地址" />
        </el-form-item>
        <el-form-item label="开发语言" prop="devLanguage">
          <el-select v-model="editForm.devLanguage" placeholder="请选择开发语言" clearable>
            <el-option label="java" value="java" />
            <el-option label="python" value="python" />
            <el-option label="go" value="go" />
          </el-select>
        </el-form-item>
        <el-form-item label="工作目录" prop="workDir">
          <el-input v-if="workspaceRoot" v-model="editWorkDirRel" placeholder="请输入子目录名">
            <template #prepend>{{ workspaceRoot }}/</template>
          </el-input>
          <el-input v-else v-model="editForm.workDir" placeholder="请输入项目工作目录（绝对路径）" />
        </el-form-item>
        <el-form-item label="数据目录" prop="dataDir">
          <el-input v-if="dataRoot" v-model="editDataDirRel" placeholder="可选，输入子目录名">
            <template #prepend>{{ dataRoot }}/</template>
          </el-input>
          <el-input v-else v-model="editForm.dataDir" placeholder="请先配置数据目录根路径" disabled />
        </el-form-item>
        <el-form-item label="CLI系统用户" prop="cliOsUser">
          <el-input
            v-model="editForm.cliOsUser"
            placeholder="Linux 可选，与工具栏「CLI 系统用户」一致；留空为 JVM 用户执行"
            clearable
          />
        </el-form-item>
        <el-form-item label="模型方案" prop="claudeProfileId">
          <el-select v-model="editForm.claudeProfileId" placeholder="选择默认模型方案（可选）" clearable style="width: 100%">
            <el-option
              v-for="p in claudeProfiles"
              :key="p.id"
              :label="p.name + ' (' + p.model + ')'"
              :value="p.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="项目状态" prop="status">
          <el-select v-model="editForm.status" placeholder="请选择项目状态">
            <el-option label="待启动" :value="0" />
            <el-option label="进行中" :value="1" />
            <el-option label="已暂停" :value="2" />
            <el-option label="已完成" :value="3" />
          </el-select>
        </el-form-item>
      </el-form>
        </CardContent>
      </Card>
      <template #footer>
        <el-button @click="editVisible = false">取消</el-button>
        <el-button type="primary" @click="submitEdit">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="cliOsUserDialogVisible"
      title="关联 Linux CLI 系统用户"
      width="480px"
      destroy-on-close
      class="shadcn-form-dialog"
    >
      <p class="mb-3 text-sm text-slate-600 leading-relaxed">
        设置后，本项目的工程目录环境检测、依赖安装、Git Pull、重建 venv、测试用例代码步骤及「AI 拆解」Claude 进程，在 Linux 上将以
        <code class="rounded bg-slate-100 px-1">sudo -u &lt;用户&gt; -H</code>
        执行。需保证运行后端的账户对该用户有无密码 sudo 权限，且该用户对工程目录、数据目录具备读写权限。
      </p>
      <el-input
        v-model="cliOsUserInput"
        placeholder="小写字母/数字/下划线/连字符，如 deploy（留空可清除）"
        clearable
        :disabled="cliOsUserSaving"
      />
      <template #footer>
        <el-button :disabled="cliOsUserSaving" @click="cliOsUserDialogVisible = false">取消</el-button>
        <el-button :disabled="cliOsUserSaving" @click="submitClearCliOsUser">清除关联</el-button>
        <el-button type="primary" :loading="cliOsUserSaving" @click="submitCliOsUser">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="runStreamVisible"
      :title="runStreamTitle"
      width="900px"
      destroy-on-close
      :close-on-click-modal="false"
      :show-close="!runStreamRunning"
    >
      <div class="run-stream-meta">
        <span>{{ runStreamRunning ? '运行中...' : '运行结束' }}</span>
      </div>
      <pre class="run-stream-log">{{ runStreamLog || '等待日志输出...' }}</pre>
      <template #footer>
        <el-button :disabled="runStreamRunning" @click="runStreamVisible = false">关闭</el-button>
      </template>
    </el-dialog>

    <!-- 文件预览对话框 -->
    <el-dialog
      v-model="previewVisible"
      :title="previewFileName"
      width="80%"
      top="3vh"
      destroy-on-close
      class="file-preview-dialog"
      @close="handlePreviewDialogClose"
    >
      <div class="preview-toolbar">
        <span class="preview-path">{{ previewFilePath }}</span>
        <div class="preview-toolbar-actions">
          <span class="preview-meta">{{ previewLanguage }} &middot; {{ formatFileSize(previewFileSize) }}</span>
          <template v-if="!previewEditing">
            <el-button size="small" @click="startPreviewEdit">
              编辑
            </el-button>
            <el-button size="small" @click="copyPreviewContent">
              {{ previewCopied ? '☑️' : '复制' }}
            </el-button>
            <el-button size="small" type="primary" @click="downloadPreviewFile">
              下载
            </el-button>
          </template>
          <template v-else>
            <el-button size="small" @click="cancelPreviewEdit">取消</el-button>
            <el-button size="small" type="primary" @click="savePreviewEdit" :loading="previewSaving">保存</el-button>
          </template>
        </div>
      </div>
      <div class="preview-content" v-loading="previewLoading">
        <!-- 编辑模式 -->
        <div v-if="previewEditing" class="preview-editor-wrapper">
          <textarea
            v-model="previewEditContent"
            class="preview-editor"
            :placeholder="'在此编辑 ' + previewFileName"
          ></textarea>
        </div>
        <!-- 预览模式 -->
        <template v-else>
          <!-- 图片预览 -->
          <div v-if="previewIsImage" class="image-preview-container">
            <img :src="previewImageUrl" :alt="previewFileName" class="preview-image" @error="handleImageError" />
          </div>
          <div v-else-if="previewIsMarkdown && previewMarkdownHtml" class="markdown-preview" v-html="previewMarkdownHtml"></div>
          <pre v-else-if="previewCode"><code v-html="previewCode"></code></pre>
          <el-empty v-if="!previewLoading && !previewIsImage && !previewCode && !previewMarkdownHtml" description="无法加载文件内容" />
        </template>
      </div>
    </el-dialog>




    <!-- Git Push 对话框 -->
    <el-dialog
      v-model="gitPushDialogVisible"
      title="Git Push"
      width="400px"
      :close-on-click-modal="false"
    >
      <el-form>
        <el-form-item label="提交信息">
          <el-input
            v-model="gitPushCommitMsg"
            type="textarea"
            :rows="3"
            placeholder="请输入 commit message"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="gitPushDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="gitPushLoading" @click="handleGitPush">确定</el-button>
      </template>
    </el-dialog>

    <!-- Git History 对话框 -->
    <el-dialog
      v-model="gitHistoryDialogVisible"
      title="Git 提交记录"
      width="700px"
    >
      <el-table
        v-loading="gitHistoryLoading"
        :data="gitHistoryCommits"
        row-key="hash"
        :expand-row-keys="gitHistoryExpandedHashes"
        class="w-full"
        highlight-current-row
        @row-click="handleGitHistoryRowClick"
        @expand-change="handleGitHistoryExpandChange"
      >
        <el-table-column type="expand">
          <template #default="{ row }">
            <div class="px-4 py-2 bg-slate-50">
              <el-skeleton v-if="row.filesLoading" :rows="2" animated />
              <el-empty v-else-if="row.filesError" :description="row.filesError" :image-size="60" />
              <template v-else-if="row.files && row.files.length">
                <div class="text-sm font-medium text-slate-700 mb-1">修改的文件：</div>
                <ul class="list-disc list-inside text-sm text-slate-600 space-y-1">
                  <li v-for="file in row.files" :key="file">{{ file }}</li>
                </ul>
              </template>
              <el-empty v-else description="该提交未修改文件" :image-size="60" />
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="hash" label="Hash" width="100">
          <template #default="{ row }">
            <code class="text-xs bg-slate-100 px-1 rounded">{{ row.hash }}</code>
          </template>
        </el-table-column>
        <el-table-column prop="author" label="作者" width="120" />
        <el-table-column prop="date" label="日期" width="160" />
        <el-table-column prop="message" label="提交信息" show-overflow-tooltip />
      </el-table>
      <el-empty v-if="!gitHistoryLoading && gitHistoryCommits.length === 0" description="暂无提交记录" />
      <template #footer>
        <el-button type="primary" @click="gitHistoryDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>

    <!-- 部署对话框 -->
    <el-dialog
      v-model="deployDialogVisible"
      title="部署与服务管理"
      width="550px"
      :close-on-click-modal="false"
    >
      <el-form label-width="80px">
        <el-form-item label="目标服务器" required>
          <el-select v-model="selectedServerId" placeholder="选择服务器" filterable class="w-full">
            <el-option
              v-for="server in serverList"
              :key="server.id"
              :label="`${server.description || server.ip} (${server.ip}:${server.port || 22})`"
              :value="server.id"
            />
          </el-select>
          <div class="mt-2">
            <el-button size="small" @click="loadServerList">
              <el-icon><Refresh /></el-icon> 刷新列表
            </el-button>
            <router-link to="/settings?tab=servers" class="ml-2">
              <el-button size="small" type="primary" link>
                <el-icon><Setting /></el-icon> 管理服务器
              </el-button>
            </router-link>
          </div>
        </el-form-item>
        <el-divider content-position="left">操作类型</el-divider>
        <!-- K8s 部署模式 -->
        <template v-if="isK8sServer(selectedServerId)">
          <el-form-item label="">
            <el-alert type="warning" :closable="false" show-icon>
              <template #title>
                <span class="font-medium">K8s 容器化部署</span>
              </template>
              本机构建产物 → 打包 Docker 镜像 → 推送 → kubectl apply
            </el-alert>
          </el-form-item>
        </template>
        <!-- SSH 部署模式 -->
        <template v-else>
          <el-form-item label="">
            <el-radio-group v-model="deployMode" class="deploy-radio-group">
              <el-radio-button value="all">全部部署</el-radio-button>
              <el-radio-button value="backend">仅后端</el-radio-button>
              <el-radio-button value="frontend">仅前端</el-radio-button>
            </el-radio-group>
          </el-form-item>
        </template>
        <template v-if="isLocalServer(selectedServerId)">
          <el-divider content-position="left">服务管理</el-divider>
          <el-form-item label="">
            <el-button-group class="deploy-btn-group">
              <el-button :type="deployMode === 'start' ? 'success' : 'default'" @click="deployMode = 'start'">
                <el-icon><VideoPlay /></el-icon> 启动
              </el-button>
              <el-button :type="deployMode === 'stop' ? 'warning' : 'default'" @click="deployMode = 'stop'">
                <el-icon><VideoPause /></el-icon> 停止
              </el-button>
              <el-button :type="deployMode === 'restart' ? 'primary' : 'default'" @click="deployMode = 'restart'">
                <el-icon><Refresh /></el-icon> 重启
              </el-button>
              <el-button :type="deployMode === 'status' ? 'info' : 'default'" @click="deployMode = 'status'">
                <el-icon><View /></el-icon> 状态
              </el-button>
            </el-button-group>
          </el-form-item>
          <el-divider v-if="['all', 'backend'].includes(deployMode)" content-position="left">部署选项</el-divider>
          <el-form-item v-if="['all', 'backend'].includes(deployMode)" label="初始化Nginx">
            <el-switch v-model="deployInitNginx" />
            <span class="ml-2 text-slate-500 text-sm">部署时同步更新 nginx 配置</span>
          </el-form-item>
        </template>
        <el-alert v-if="['start', 'stop', 'restart', 'status'].includes(deployMode)" type="info" :closable="false" show-icon class="mt-3">
          <template #title>
            <span v-if="deployMode === 'start'">启动应用（不拉代码不构建）</span>
            <span v-else-if="deployMode === 'stop'">停止应用</span>
            <span v-else-if="deployMode === 'restart'">重启应用（不拉代码不构建）</span>
            <span v-else-if="deployMode === 'status'">查看应用运行状态</span>
          </template>
        </el-alert>
      </el-form>
      <template #footer>
        <el-button @click="deployDialogVisible = false">取消</el-button>
        <el-button
          :type="deployMode === 'stop' ? 'warning' : 'primary'"
          :loading="deployLoading"
          :disabled="!selectedServerId"
          @click="handleDeploy"
        >
          {{ isK8sServer(selectedServerId) ? 'K8s 部署' : (['start', 'stop', 'restart', 'status'].includes(deployMode) ? '执行' : '开始部署') }}
        </el-button>
      </template>
    </el-dialog>

    <!-- 部署日志对话框 -->
    <el-dialog
      v-model="deployLogDialogVisible"
      :title="getDeployModeLabel(deployMode) + ' - 执行日志'"
      width="800px"
      :close-on-click-modal="false"
      :show-close="!deployRunning"
    >
      <div class="flex items-center justify-end mb-2">
        <el-button size="small" :disabled="!deployLogText" @click="copyDeployLogText">
          {{ deployLogCopied ? '已复制' : '复制日志' }}
        </el-button>
      </div>
      <pre class="run-stream-log">{{ deployLogText || '等待日志输出...' }}</pre>
      <!-- 部署完成后的端口信息 -->
      <div v-if="deployResultUrls && !deployRunning" class="mt-4 p-4 bg-green-50 rounded border border-green-200">
        <h4 class="font-semibold text-green-800 mb-2">部署成功！访问地址：</h4>
        <div v-if="deployResultUrls.frontendUrl" class="mb-2">
          <span class="text-gray-600">前端：</span>
          <a :href="deployResultUrls.frontendUrl" target="_blank" class="text-blue-600 hover:underline">
            {{ deployResultUrls.frontendUrl }}
          </a>
        </div>
        <div v-if="deployResultUrls.backendUrl" class="mb-2">
          <span class="text-gray-600">后端：</span>
          <a :href="deployResultUrls.backendUrl" target="_blank" class="text-blue-600 hover:underline">
            {{ deployResultUrls.backendUrl }}
          </a>
        </div>
        <el-alert type="info" :closable="false" class="mt-3">
          <template #title>
            <span class="text-sm">请确保deploy-local.sh脚本中正确使用了 FRONTEND_PORT 和 BACKEND_PORT 环境变量</span>
          </template>
        </el-alert>
      </div>
      <template #footer>
        <el-button v-if="!deployRunning" type="primary" @click="deployLogDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>


    <!-- 数据目录树：右键菜单（重命名 / 删除） -->
    <Teleport to="body">
      <div
        v-if="workDirContextMenu.visible && !workDirContextMenu.isRoot"
        class="fixed z-[4000] min-w-[132px] rounded-md border border-slate-200 bg-white py-1 text-sm text-slate-800 shadow-md"
        :style="{ left: workDirContextMenu.x + 'px', top: workDirContextMenu.y + 'px' }"
        @mousedown.stop
      >
        <div
          class="cursor-pointer px-4 py-2 leading-normal text-rose-600 select-none hover:bg-rose-50 hover:text-rose-600"
          @click="onWorkContextMenuDelete"
        >
          删除
        </div>
      </div>
    </Teleport>

    <!-- 交互式 Claude Code Shell -->
    <InteractiveClaudeTerminal
      v-model="interactiveTerminalVisible"
      :project-id="route.params.id"
      :project-name="project.name"
      :work-dir="project.workDir"
      :resume-session-uuid="resumeSessionUuid"
    />


  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { useRouter, useRoute, onBeforeRouteLeave } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { UploadRequestOptions } from 'element-plus'
import { Loading, Monitor, RefreshRight, Setting, ArrowDown, ArrowUp, Search, VideoPlay, VideoPause, Refresh, View, Link, Folder, Upload, MagicStick, Warning, List, Download, Promotion, User } from '@element-plus/icons-vue'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ProjectApi, RuntimeStatusApi } from '@/api/project'
import { SettingsApi, PromptTemplateApi, DeployApi } from '@/api/settings'
import { ClaudeApi } from '@/api/claude'
import type Node from 'element-plus/es/components/tree/src/model/node'
import hljs from 'highlight.js'
import 'highlight.js/styles/github.css'
import MarkdownIt from 'markdown-it'
import highlightjs from 'markdown-it-highlightjs'

const md = MarkdownIt({
  html: true,
  linkify: true,
  typographer: true,
  breaks: true,
}).use(highlightjs, { hljs })
import InteractiveClaudeTerminal from '@/components/InteractiveClaudeTerminal.vue'
import ModuleTaskTab from '@/components/ModuleTaskTab.vue'
import PlanPanel from '@/components/PlanPanel.vue'
import DeploymentPanel from '@/components/DeploymentPanel.vue'
import TestPanel from '@/components/TestPanel.vue'

const router = useRouter()
const route = useRoute()

/** 详情页 Tab：work | data | database | tasks | claude | agents | logs */
const detailActiveTab = ref('work')

// 计划拆解完成后切换到模块任务 tab
const handlePlanDecomposed = () => {
  detailActiveTab.value = 'moduleTasks'
}

// 部署状态变更处理
const handleDeployStatusChange = () => {
  // 可扩展：刷新运行状态等
}

/** 从 URL query 或 localStorage 恢复 tab 状态 */
const restoreTabFromUrl = () => {
  const validTabs = ['work', 'moduleTasks', 'claude', 'envVars', 'plan', 'deploy', 'test']
  // 优先从 URL query 读取
  const urlTab = route.query.tab as string
  if (urlTab && validTabs.includes(urlTab)) {
    detailActiveTab.value = urlTab
    return
  }
  // 回退到 localStorage
  try {
    const id = Number(route.params.id)
    const key = Number.isFinite(id) && id > 0 ? `project-detail-active-tab:${id}` : 'project-detail-active-tab:default'
    const savedTab = localStorage.getItem(key)
    if (savedTab && validTabs.includes(savedTab)) {
      detailActiveTab.value = savedTab
    }
  } catch {
    // 忽略
  }
}

const project = ref<any>({})
const workspaceRoot = ref('')
const dataRoot = ref('')
const editWorkDirRel = ref('')
const editDataDirRel = ref('')
const workTreeVersion = ref(0)
/** 工程目录树：false 完整目录；true 仅 Git 工作区未提交路径 */
const workTreeGitMode = ref(false)
const workTreeGitLoading = ref(false)
const gitChangedPaths = ref<string[]>([])
/** 更多操作弹框显示状态 */
const moreActionsDialogVisible = ref(false)
/** 是否显示更多按钮（由系统设置控制） */
const showMoreActions = ref(true)
/** Claude 模型方案列表（项目级默认方案选择用） */
const claudeProfiles = ref<{ id: string; name: string; model: string }[]>([])
/** 小程序目录检测结果 */
const miniProgramDetect = ref<{ detected: boolean; projectPath?: string; type?: string; typeLabel?: string; message?: string } | null>(null)
/** 小程序发布按钮 loading */
const miniProgramUploadLoading = ref(false)

const loadShowMoreActions = async () => {
  try {
    const res = await SettingsApi.getShowMoreActions()
    showMoreActions.value = res.value === true
  } catch {
    console.error('加载显示更多操作按钮设置失败')
  }
}

const loadClaudeProfiles = async () => {
  try {
    const res: any = await SettingsApi.getClaudeProfiles()
    if (res && res.success !== false) {
      claudeProfiles.value = Array.isArray(res.profiles) ? res.profiles : []
    }
  } catch (err: any) {
    console.error('加载 Claude 模型方案列表失败:', err)
  }
}

const gitChangedPathSet = computed(() => {
  const s = new Set<string>()
  for (const p of gitChangedPaths.value) {
    if (typeof p === 'string' && p.length) {
      s.add(p.replace(/\\/g, '/'))
    }
  }
  return s
})

function childVisibleInGitMode(relPosix: string): boolean {
  const set = gitChangedPathSet.value
  if (set.size === 0) {
    return false
  }
  const rel = relPosix.replace(/\\/g, '/')
  if (set.has(rel)) {
    return true
  }
  const prefix = rel + '/'
  for (const p of set) {
    if (p.startsWith(prefix)) {
      return true
    }
  }
  return false
}

/** 目录绝对路径 -> 相对工程根目录 workDir 的 posix 路径（无首尾斜杠） */
function workDirRelativePath(dirAbsolute: string, workRoot: string): string {
  const a = normalizePathStr(dirAbsolute)
  const r = normalizePathStr(workRoot)
  if (!r) {
    return ''
  }
  if (a === r) {
    return ''
  }
  if (a.startsWith(r + '/')) {
    return a.slice(r.length + 1)
  }
  return ''
}

async function toggleWorkTreeGitMode() {
  const id = Number(route.params.id)
  const root = (project.value?.workDir || '').trim()
  if (!root) {
    ElMessage.warning('未设置工作目录')
    return
  }
  if (workTreeGitMode.value) {
    workTreeGitMode.value = false
    gitChangedPaths.value = []
    workTreeVersion.value += 1
    return
  }
  workTreeGitLoading.value = true
  try {
    const res: any = await ProjectApi.getWorkDirGitChangedPaths(id)
    if (!res?.success) {
      ElMessage.error(res?.message || '获取 Git 变更失败')
      return
    }
    if (res.isGitRepo === false) {
      ElMessage.warning(res?.message || '当前目录不是 Git 仓库')
      return
    }
    gitChangedPaths.value = Array.isArray(res.paths) ? res.paths : []
    workTreeGitMode.value = true
    workTreeVersion.value += 1
    if (gitChangedPaths.value.length === 0) {
      ElMessage.info('工作区无未提交变更')
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '请求失败')
  } finally {
    workTreeGitLoading.value = false
  }
}
const dataTreeVersion = ref(0)
/** 数据目录：批量删除多选（路径集合，整 Set 替换以保证响应式） */
const dataDirBulkSelected = ref<Set<string>>(new Set())
const dataDirBulkDeleteLoading = ref(false)
/** 数据目录：当前浏览路径（点击文件夹进入） */
const dataTreeCwd = ref('')
const selectedDataDirPath = ref('')
/** 并发上传计数，避免多文件时 loading 提前结束 */
const dataDirUploadPending = ref(0)
/** 数据目录：上传进度 (fileName -> percent) */
const dataDirUploadProgress = ref<Record<string, number>>({})
/** 数据目录根路径是否有效（存在且可访问） */
const dataDirRootValid = ref(true)
/** 数据目录根路径错误信息 */
const dataDirRootError = ref('')
/** 数据目录树右键菜单（仅子目录：非数据根） */
const dataDirContextMenu = ref({
  visible: false,
  x: 0,
  y: 0,
  path: '',
  isRoot: false
})
let dataDirContextMenuClickCleanup: (() => void) | null = null
const workDirContextMenu = ref({
  visible: false,
  x: 0,
  y: 0,
  path: '',
  isRoot: false,
  isDirectory: false
})
let workDirContextMenuClickCleanup: (() => void) | null = null


// 环境变量相关
interface EnvVarItem {
  key: string
  value: string
  keyError?: boolean
}
const envVars = ref<EnvVarItem[]>([])
const envVarsSaving = ref(false)
const envVarsSelected = ref<Set<number>>(new Set())

// 提示词变量相关
interface PromptVarItem {
  key: string
  value: string
  remark?: string
  keyError?: boolean
  readonly?: boolean  // 从环境变量复制的记录为只读
}
const promptVars = ref<PromptVarItem[]>([])
const promptVarsSaving = ref(false)

const runtimeStatus = ref<any>({
  backend: null,
  frontend: null,
  accessUrls: null,
  system: null
})
const runtimeStatusLoading = ref(false)

const loadRuntimeStatus = async () => {
  const id = Number(route.params.id)
  if (!id) return
  runtimeStatusLoading.value = true
  try {
    const res: any = await RuntimeStatusApi.getRuntimeStatus(id)
    if (res.success) {
      runtimeStatus.value = {
        projectId: res.projectId,
        projectName: res.projectName,
        projectWorkDir: res.projectWorkDir,
        backend: res.backend,
        frontend: res.frontend,
        accessUrls: res.accessUrls,
        system: res.system
      }
    }
  } catch (e) {
    console.error('加载运行状态失败', e)
    ElMessage.error('加载运行状态失败')
  } finally {
    runtimeStatusLoading.value = false
  }
}

const runStreamVisible = ref(false)
const runStreamRunning = ref(false)
const runStreamLog = ref('')
const runStreamTitle = ref('Claude 命令执行日志')

const envCheckLoading = ref(false)
const installDepsLoading = ref(false)
const rebuildVenvLoading = ref(false)
const gitPullLoading = ref(false)
const gitPushLoading = ref(false)
const gitPushDialogVisible = ref(false)
const gitPushCommitMsg = ref('')
const gitHistoryLoading = ref(false)
const gitHistoryDialogVisible = ref(false)
const gitHistoryCommits = ref<any[]>([])
const gitHistoryExpandedHashes = ref<string[]>([])
const updateFeatureListLoading = ref(false)
const initDeployScriptLoading = ref(false)
// 本机服务器 ID（根据 IP 动态查找）
const LOCAL_HOST_IPS = ['127.0.0.1', 'localhost', '::1', '::ffff:127.0.0.1']
const getLocalServerId = () => {
  const localServer = serverList.value.find(s => LOCAL_HOST_IPS.includes(s.ip))
  return localServer?.id ?? null
}
const isLocalServer = (serverId: number | null) => {
  if (serverId === null) return false
  return serverId === getLocalServerId()
}
const isK8sServer = (serverId: number | null) => {
  if (serverId === null) return false
  const server = serverList.value.find(s => s.id === serverId)
  return server?.deployType === 'k8s'
}
const deployDialogVisible = ref(false)
const deployMode = ref<'all' | 'backend' | 'frontend' | 'start' | 'stop' | 'restart' | 'status'>('all')
const deployInitNginx = ref(false)  // 是否初始化 nginx 配置
const deployLoading = ref(false)
const deployLogDialogVisible = ref(false)
const deployLogText = ref('')
const deployLogCopied = ref(false)
const deployRunning = ref(false)
let deployPollTimer: ReturnType<typeof setTimeout> | null = null
let deployLogReadLen = 0  // 已读取的日志长度（用于增量追加）
let deployTaskId = ''     // 当前部署任务ID
let deployPollErrorCount = 0  // 连续轮询失败计数（服务重启期间容错）
// 部署完成后的端口信息
const deployResultUrls = ref<{ frontendUrl?: string; backendUrl?: string } | null>(null)
// 服务器选择相关状态
const selectedServerId = ref<number | null>(null)  // 默认本机服务器（动态获取）
const serverList = ref<{ id: number; ip: string; port: number; username: string; description?: string }[]>([])
// 远程部署相关状态
const remoteDeployStarting = ref(false)
let remoteDeployAbort: AbortController | null = null
const projectClaudeConfigLoading = ref(false)
const projectClaudeConfigSaving = ref(false)
const projectClaudeConfig = ref({
  claudeMd: '',
  claudeSettings: '',
  skillRelativePath: '',
  skillContent: ''
})

// Skill 自动探测相关
const availableSkills = ref<any[]>([])
const skillsLoading = ref(false)
const selectedSkill = ref<string>('')
const globalSkillsPath = ref('')

function normalizePathStr(p: string) {
  return p.trim().replace(/\\/g, '/').replace(/\/+$/, '')
}

function pathsEqual(a: string, b: string) {
  return normalizePathStr(a) === normalizePathStr(b)
}

function parentDirPath(p: string): string {
  const s = normalizePathStr(p)
  const i = s.lastIndexOf('/')
  return i <= 0 ? '' : s.slice(0, i)
}

/** child 是否落在 root 目录之下或等于 root */
function isPathUnderOrEqualRoot(child: string, root: string): boolean {
  const c = normalizePathStr(child)
  const r = normalizePathStr(root)
  if (!r) return true
  return c === r || c.startsWith(r + '/')
}

const canDataTreeGoUp = computed(() => {
  const base = (project.value.dataDir || '').trim()
  const cwd = (dataTreeCwd.value || base).trim()
  return !!base && !!cwd && !pathsEqual(cwd, base)
})

// 工程目录：传统懒加载树（目录可展开）
const workTreeProps = {
  label: 'name',
  children: 'children',
  isLeaf: (data: any) => !data.isDirectory,
}
// 数据目录：目录视为 leaf，用点击进入子目录，避免与钻取浏览冲突
const dataTreeProps = {
  label: 'name',
  children: 'children',
  isLeaf: () => true,
}

const loadTreeChildren = async (
  rootPath: string,
  node: Node,
  resolve: (data: any[]) => void,
  applyGitFilter = false
) => {
  const dirPath = node.level === 0 ? rootPath : node.data.path
  if (!dirPath) {
    resolve([])
    return
  }
  try {
    const res: any = await ProjectApi.listDirectory(dirPath)
    // 后端 OKWithData 返回 {success:true, data:{children:[...]}}
    // 兼容两种格式：直接返回 {children:...} 或包裹在 data 中
    const payload = res?.data ?? res
    let children = (res?.success === true || payload?.success === true) && payload?.children ? payload.children : []
    if (applyGitFilter && workTreeGitMode.value) {
      const relBase = workDirRelativePath(dirPath, rootPath)
      children = children.filter((ch: any) => {
        const name = ch?.name != null ? String(ch.name) : ''
        const rel = relBase ? `${relBase}/${name}` : name
        return childVisibleInGitMode(rel)
      })
    }
    resolve(children)
  } catch {
    resolve([])
  }
}

const loadTreeNode = async (node: Node, resolve: (data: any[]) => void) => {
  await loadTreeChildren(project.value.workDir, node, resolve, true)
}

const loadDataTreeNode = async (node: Node, resolve: (data: any[]) => void) => {
  const root = (dataTreeCwd.value || project.value.dataDir || '').trim()
  if (!root) {
    resolve([])
    return
  }
  try {
    const res: any = await ProjectApi.listDirectory(root)
    // 后端 OKWithData 返回 {success:true, data:{children:[...]}}
    const payload = res?.data ?? res
    const success = res?.success === true || payload?.success === true
    if (node.level === 0) {
      // 根目录加载结果
      if (success) {
        dataDirRootValid.value = true
        dataDirRootError.value = ''
        resolve(payload.children || [])
      } else {
        dataDirRootValid.value = false
        dataDirRootError.value = payload?.message || res?.message || '无法访问数据目录'
        resolve([])
      }
    } else {
      // 子目录加载
      resolve(success && payload?.children ? payload.children : [])
    }
  } catch {
    if (node.level === 0) {
      dataDirRootValid.value = false
      dataDirRootError.value = '无法访问数据目录'
    }
    resolve([])
  }
}

watch(dataTreeVersion, () => {
  dataDirBulkSelected.value = new Set()
})

function toggleDataDirBulkSelect(path: string, checked: boolean) {
  const p = (path || '').trim()
  if (!p) return
  const next = new Set(dataDirBulkSelected.value)
  if (checked) next.add(p)
  else next.delete(p)
  dataDirBulkSelected.value = next
}

async function selectAllInDataDirCwd() {
  const cwd = (dataTreeCwd.value || project.value.dataDir || '').trim()
  if (!cwd) {
    ElMessage.warning('未配置数据目录')
    return
  }
  try {
    const res: any = await ProjectApi.listDirectory(cwd)
    const payload = res?.data ?? res
    const children = (res?.success === true || payload?.success === true) && payload?.children ? payload.children : []
    const next = new Set<string>()
    for (const ch of children) {
      const p = (ch?.path || '').trim()
      if (p) next.add(p)
    }
    dataDirBulkSelected.value = next
    if (next.size === 0) {
      ElMessage.info('当前目录为空')
    }
  } catch {
    ElMessage.error('加载目录列表失败')
  }
}

function clearDataDirBulkSelect() {
  dataDirBulkSelected.value = new Set()
}

async function batchDeleteDataDirSelection() {
  const paths = [...dataDirBulkSelected.value]
  if (paths.length === 0) {
    ElMessage.warning('请先勾选要删除的项')
    return
  }
  const base = (project.value.dataDir || '').trim()
  for (const p of paths) {
    if (base && !isPathUnderOrEqualRoot(p, base)) {
      ElMessage.error('只能删除当前项目数据目录下的项')
      return
    }
  }
  try {
    await ElMessageBox.confirm(
      `确认删除已选的 ${paths.length} 项？删除后不可恢复。若包含文件夹，将递归删除其中的全部文件与子目录。`,
      '批量删除',
      {
        confirmButtonText: '删除',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error(e?.message || '批量删除已中断')
    }
    return
  }

  dataDirBulkDeleteLoading.value = true
  const errors: string[] = []
  for (const p of paths) {
    try {
      const res: any = await ProjectApi.deleteFileOrDirectory(p)
      if (!res?.success) {
        const label = p.split(/[/\\]/).pop() || p
        errors.push(`${label}: ${res?.message || '失败'}`)
      }
    } catch {
      const label = p.split(/[/\\]/).pop() || p
      errors.push(`${label}: 请求失败`)
    }
  }
  dataDirBulkDeleteLoading.value = false

  for (const p of paths) {
    if (previewVisible.value && pathsEqual(previewFilePath.value, p)) {
      previewVisible.value = false
    }
  }

  dataDirBulkSelected.value = new Set()
  dataTreeVersion.value += 1

  if (errors.length === 0) {
    ElMessage.success('批量删除成功')
  } else {
    const head = errors.slice(0, 3).join('；')
    const more = errors.length > 3 ? ` 等共 ${errors.length} 条失败` : ''
    ElMessage.warning(`部分删除失败（${errors.length}/${paths.length}）：${head}${more}`)
  }
}

const formatFileSize = (bytes: number) => {
  if (!bytes || bytes === 0) return ''
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}

/** 目录列表接口返回的 creationTime / lastModified（毫秒） */
const formatFileTime = (ms: number | null | undefined) => {
  const n = ms == null ? NaN : Number(ms)
  if (Number.isNaN(n) || n <= 0) return ''
  const d = new Date(n)
  const pad = (x: number) => String(x).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

// 可预览的文件扩展名
const PREVIEWABLE_EXTS = new Set([
  'txt', 'java', 'py', 'go', 'sh', 'bash', 'zsh',
  'js', 'ts', 'jsx', 'tsx', 'vue', 'svelte',
  'html', 'htm', 'css', 'scss', 'less', 'sass',
  'json', 'yml', 'yaml', 'xml', 'toml', 'ini', 'cfg', 'conf',
  'md', 'markdown', 'rst',
  'sql', 'graphql', 'gql',
  'c', 'cpp', 'h', 'hpp', 'cs', 'rs', 'rb', 'php', 'kt', 'kts', 'swift',
  'gradle', 'groovy', 'scala',
  'dockerfile', 'makefile', 'cmake',
  'env', 'properties', 'gitignore', 'editorconfig',
  'proto', 'lua', 'r', 'dart', 'tf', 'hcl',
  // 图片文件
  'png', 'jpg', 'jpeg', 'gif', 'svg', 'webp', 'ico', 'bmp',
])

// 图片扩展名集合
const IMAGE_EXTS = new Set(['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp', 'ico', 'bmp'])

const getFileExt = (name: string) => {
  const lower = name.toLowerCase()
  // 统一识别 .env / .env.production / .env.local 等环境配置文件为 env 类型
  if (lower.startsWith('.env') && (lower.length === 4 || lower.charAt(4) === '.')) {
    return 'env'
  }
  if (!lower.includes('.')) return lower
  return lower.substring(lower.lastIndexOf('.') + 1)
}

const isPreviewable = (name: string) => {
  return PREVIEWABLE_EXTS.has(getFileExt(name))
}

// 编辑项目状态
const editVisible = ref(false)
const editFormRef = ref()
const editForm = ref({
  name: '',
  description: '',
  gitUrl: '',
  devLanguage: '',
  workDir: '',
  dataDir: '',
  cliOsUser: '',
  status: 0
})

const editRules = {
  name: [
    { required: true, message: '请输入项目名称', trigger: 'blur' },
    { min: 1, max: 50, message: '长度在 1 到 50 个字符', trigger: 'blur' }
  ],
  description: [
    { max: 200, message: '长度不能超过 200 个字符', trigger: 'blur' }
  ],
  workDir: [
    { max: 500, message: '长度不能超过 500 个字符', trigger: 'blur' }
  ],
  dataDir: [
    { max: 500, message: '长度不能超过 500 个字符', trigger: 'blur' }
  ],
  cliOsUser: [
    { max: 64, message: '长度不能超过 64 个字符', trigger: 'blur' }
  ],
  status: [
    { required: true, message: '请选择项目状态', trigger: 'change' }
  ]
}

const cliOsUserDialogVisible = ref(false)
const cliOsUserInput = ref('')
const cliOsUserSaving = ref(false)

// 交互式 Claude Code Shell状态
const interactiveTerminalVisible = ref(false)
const resumeSessionUuid = ref('')
const sessionRecoveryPopoverVisible = ref(false)
const sessionRecoveryLoading = ref(false)

// 打开本地终端（stub - ServerApi deleted）
const openLocalTerminal = async () => {
  ElMessage.warning('本地终端功能暂不可用')
}

const handlePublishMiniProgram = async () => {
  const id = Number(route.params.id)
  if (!id) return
  if (!miniProgramDetect.value?.detected) {
    ElMessage.warning('未检测到小程序目录')
    return
  }
  try {
    await ElMessageBox.confirm(
      `检测到小程序目录：${miniProgramDetect.value.projectPath}\n确认上传开发版本？\n上传后需到微信公众平台「版本管理」手动设为体验版。`,
      '发布小程序',
      { confirmButtonText: '确认上传', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }
  if (miniProgramUploadLoading.value) return
  miniProgramUploadLoading.value = true
  runStreamVisible.value = true
  runStreamRunning.value = true
  runStreamTitle.value = '发布小程序日志'
  runStreamLog.value = '开始上传小程序开发版本...'
  try {
    const res: any = await ProjectApi.uploadMiniProgram(id)
    runStreamRunning.value = false
    if (res?.success) {
      runStreamLog.value = appendRunLog(runStreamLog.value, res.data?.log || '')
      runStreamLog.value = appendRunLog(runStreamLog.value, `\n发布成功: ${res.message || ''}`)
      ElMessage.success(res.message || '发布成功')
    } else {
      runStreamLog.value = appendRunLog(runStreamLog.value, res?.data?.log || '')
      runStreamLog.value = appendRunLog(runStreamLog.value, `\n发布失败: ${res?.message || '未知错误'}`)
      ElMessage.error(res?.message || '发布失败')
    }
  } catch (e: any) {
    runStreamRunning.value = false
    runStreamLog.value = appendRunLog(runStreamLog.value, `\n发布异常: ${e?.message || '未知错误'}`)
    ElMessage.error(e?.message || '发布异常')
  } finally {
    miniProgramUploadLoading.value = false
  }
}

const claudeHistorySessions = ref<any[]>([])

// 会话恢复：懒加载历史会话列表
const loadClaudeHistorySessions = async () => {
  sessionRecoveryLoading.value = true
  try {
    const res: any = await ClaudeApi.listClaudeSessions(String(route.params.id))
    if (res?.data) {
      claudeHistorySessions.value = res.data.slice(0, 20)
    }
  } catch (e) {
    console.error('Failed to load Claude history sessions:', e)
  } finally {
    sessionRecoveryLoading.value = false
  }
}

// 会话恢复：选择一个历史会话后恢复
const handleResumeSession = (session: any) => {
  if (interactiveTerminalVisible.value) {
    ElMessage.warning('请先关闭当前终端再恢复会话')
    return
  }
  resumeSessionUuid.value = session.claudeSessionId
  sessionRecoveryPopoverVisible.value = false
  interactiveTerminalVisible.value = true
}

// 会话恢复：格式化文件大小
const formatSessionSize = (bytes: number) => {
  if (!bytes) return '0 B'
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}

// 终端关闭时清空 resumeSessionUuid，下次打开走正常流程
watch(interactiveTerminalVisible, (val) => {
  if (!val) {
    resumeSessionUuid.value = ''
  }
})

// 文件预览状态
const previewVisible = ref(false)
const previewLoading = ref(false)
const previewFileName = ref('')
const previewFilePath = ref('')
const previewFileSize = ref(0)
const previewLanguage = ref('')
const previewCode = ref('')
const previewRawContent = ref('')
const previewCopied = ref(false)
const previewIsMarkdown = ref(false)
const previewMarkdownHtml = ref('')
let previewCopiedTimer: number | null = null
// 文件编辑状态
const previewEditing = ref(false)
const previewEditContent = ref('')
const previewSaving = ref(false)
// 图片预览状态
const previewIsImage = ref(false)
const previewImageUrl = ref('')

const goDataTreeUp = () => {
  const base = (project.value.dataDir || '').trim()
  const cwd = (dataTreeCwd.value || base).trim()
  if (!base || pathsEqual(cwd, base)) return
  const parent = parentDirPath(cwd)
  if (!parent || !isPathUnderOrEqualRoot(parent, base)) {
    dataTreeCwd.value = base
  } else {
    dataTreeCwd.value = parent
  }
  selectedDataDirPath.value = dataTreeCwd.value
  dataTreeVersion.value += 1
}

const handleWorkNodeClick = async (data: any) => {
  if (data.isDirectory) return
  if (!isPreviewable(data.name)) {
    ElMessage.info('该文件类型不支持预览')
    return
  }

  previewFileName.value = data.name
  previewFilePath.value = data.path
  previewFileSize.value = data.size || 0
  previewCode.value = ''
  previewRawContent.value = ''
  previewCopied.value = false
  previewIsMarkdown.value = false
  previewMarkdownHtml.value = ''
  previewEditing.value = false
  previewEditContent.value = ''
  previewIsImage.value = false
  previewImageUrl.value = ''
  if (previewCopiedTimer) {
    window.clearTimeout(previewCopiedTimer)
    previewCopiedTimer = null
  }
  previewLanguage.value = ''
  previewLoading.value = true
  previewVisible.value = true

  const ext = getFileExt(data.name)

  // 判断是否为图片文件
  if (IMAGE_EXTS.has(ext)) {
    previewIsImage.value = true
    previewLanguage.value = 'image'
    // 构造图片预览 URL（使用 inline 方式的专用接口，浏览器直接渲染）
    const apiBase = import.meta.env.VITE_API_BASE_URL || ''
    previewImageUrl.value = `${apiBase}/api/files/image-preview?path=${encodeURIComponent(data.path)}`
    previewLoading.value = false
    return
  }

  try {
    const res: any = await ProjectApi.readFileContent(data.path)
    if (res.success) {
      previewLanguage.value = res.language || 'plaintext'
      previewFileSize.value = res.size || 0
      previewRawContent.value = res.content || ''

      // 判断是否为 Markdown 文件
      const lang = res.language || 'plaintext'
      const isMd = lang === 'markdown' || ext === 'md' || ext === 'markdown'

      if (isMd) {
        // 使用 markdown-it 渲染 Markdown
        previewIsMarkdown.value = true
        previewMarkdownHtml.value = md.render(res.content || '')
        previewCode.value = ''
      } else if (lang !== 'plaintext' && hljs.getLanguage(lang)) {
        previewCode.value = hljs.highlight(res.content, { language: lang }).value
        previewIsMarkdown.value = false
      } else {
        previewCode.value = hljs.highlightAuto(res.content).value
        previewIsMarkdown.value = false
      }
    } else {
      ElMessage.error(res.message || '读取文件失败')
      previewVisible.value = false
    }
  } catch {
    ElMessage.error('读取文件失败')
    previewVisible.value = false
  } finally {
    previewLoading.value = false
  }
}

const downloadDataDirFile = async (path: string, fileName?: string) => {
  const p = (path || '').trim()
  if (!p) {
    ElMessage.warning('路径无效')
    return
  }
  const name = (fileName || p.split(/[/\\]/).pop() || 'download').trim()
  // 使用浏览器原生下载，避免 axios 超时和 blob 内存问题
  const baseUrl = import.meta.env.VITE_API_BASE_URL || '/api'
  const url = `${baseUrl}/files/download?path=${encodeURIComponent(p)}`
  const a = document.createElement('a')
  a.href = url
  a.download = name
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  ElMessage.success('下载已开始')
}

/** 图片加载错误处理 */
const handleImageError = () => {
  ElMessage.error('图片加载失败')
  previewIsImage.value = false
}

/** 下载工程目录下的文件 */
const downloadWorkDirFile = async (path: string, fileName?: string) => {
  const p = (path || '').trim()
  if (!p) {
    ElMessage.warning('路径无效')
    return
  }
  const name = (fileName || p.split(/[/\\]/).pop() || 'download').trim()
  // 使用浏览器原生下载，避免 axios 超时和 blob 内存问题
  const baseUrl = import.meta.env.VITE_API_BASE_URL || '/api'
  const url = `${baseUrl}/files/download?path=${encodeURIComponent(p)}`
  const a = document.createElement('a')
  a.href = url
  a.download = name
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  ElMessage.success('下载已开始')
}

const deleteDataDirFile = async (path: string, displayName?: string) => {
  const p = (path || '').trim()
  if (!p) {
    ElMessage.warning('路径无效')
    return
  }
  const base = (project.value.dataDir || '').trim()
  if (base && !isPathUnderOrEqualRoot(p, base)) {
    ElMessage.error('只能删除当前项目数据目录下的文件')
    return
  }
  const label = (displayName || p.split(/[/\\]/).pop() || '').trim()
  try {
    await ElMessageBox.confirm(
      label ? `确认删除「${label}」？删除后不可恢复。` : '确认删除该文件？删除后不可恢复。',
      '删除确认',
      {
        confirmButtonText: '删除',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
    const res: any = await ProjectApi.deleteFileOrDirectory(p)
    if (res?.success) {
      ElMessage.success('删除成功')
      if (previewVisible.value && pathsEqual(previewFilePath.value, p)) {
        previewVisible.value = false
      }
      dataTreeVersion.value += 1
    } else {
      ElMessage.error(res?.message || '删除失败')
    }
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error('删除失败')
    }
  }
}

const downloadPreviewFile = async () => {
  await downloadDataDirFile(previewFilePath.value, previewFileName.value)
}

const copyPreviewContent = async () => {
  if (!previewRawContent.value) {
    ElMessage.warning('当前没有可复制内容')
    return
  }
  const markCopied = () => {
    previewCopied.value = true
    if (previewCopiedTimer) {
      window.clearTimeout(previewCopiedTimer)
    }
    previewCopiedTimer = window.setTimeout(() => {
      previewCopied.value = false
      previewCopiedTimer = null
    }, 1500)
  }

  const copyWithExecCommand = (text: string) => {
    const textarea = document.createElement('textarea')
    textarea.value = text
    textarea.setAttribute('readonly', 'true')
    textarea.style.position = 'fixed'
    textarea.style.top = '-9999px'
    textarea.style.left = '-9999px'
    document.body.appendChild(textarea)
    textarea.focus()
    textarea.select()
    let success = false
    try {
      success = document.execCommand('copy')
    } finally {
      document.body.removeChild(textarea)
    }
    return success
  }

  try {
    if (window.isSecureContext && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(previewRawContent.value)
      markCopied()
      return
    }
  } catch {
    // Clipboard API 失败时，继续尝试降级方案
  }

  if (copyWithExecCommand(previewRawContent.value)) {
    markCopied()
    return
  }

  ElMessage.error('复制失败，请手动复制')
}

/** 进入编辑模式 */
const startPreviewEdit = () => {
  if (!previewRawContent.value) {
    ElMessage.warning('当前没有可编辑内容')
    return
  }
  previewEditContent.value = previewRawContent.value
  previewEditing.value = true
}

/** 取消编辑 */
const cancelPreviewEdit = () => {
  previewEditing.value = false
  previewEditContent.value = ''
}

/** 保存编辑内容 */
const savePreviewEdit = async () => {
  if (!previewFilePath.value) {
    ElMessage.warning('文件路径无效')
    return
  }
  previewSaving.value = true
  try {
    const res: any = await ProjectApi.saveFileContent(previewFilePath.value, previewEditContent.value)
    if (res.success) {
      // 更新预览状态
      previewRawContent.value = previewEditContent.value
      previewFileSize.value = res.size || previewEditContent.value.length

      // 重新渲染代码高亮或 Markdown
      const lang = previewLanguage.value
      const isMd = lang === 'markdown' || previewFilePath.value.toLowerCase().endsWith('.md')

      if (isMd) {
        previewIsMarkdown.value = true
        previewMarkdownHtml.value = md.render(previewEditContent.value)
        previewCode.value = ''
      } else if (lang !== 'plaintext' && hljs.getLanguage(lang)) {
        previewCode.value = hljs.highlight(previewEditContent.value, { language: lang }).value
        previewIsMarkdown.value = false
      } else {
        previewCode.value = hljs.highlightAuto(previewEditContent.value).value
        previewIsMarkdown.value = false
      }

      previewEditing.value = false
      previewEditContent.value = ''
      ElMessage.success('保存成功')

      // 刷新文件树（更新修改时间）
      if (detailActiveTab.value === 'data') {
        dataTreeVersion.value += 1
      } else if (detailActiveTab.value === 'work') {
        workTreeVersion.value += 1
      }
    } else {
      ElMessage.error(res.message || '保存失败')
    }
  } catch (e: any) {
    const msg = e?.response?.data?.message || e?.message || '保存失败'
    ElMessage.error(msg)
  } finally {
    previewSaving.value = false
  }
}

/** 对话框关闭时清理编辑状态 */
const handlePreviewDialogClose = () => {
  previewEditing.value = false
  previewEditContent.value = ''
}

function closeWorkDirContextMenu() {
  workDirContextMenu.value.visible = false
  if (workDirContextMenuClickCleanup) {
    workDirContextMenuClickCleanup()
    workDirContextMenuClickCleanup = null
  }
}

const onWorkDirNodeContextMenu = (e: MouseEvent, data: any, node: Node) => {
  e.preventDefault()
  closeWorkDirContextMenu()
  const rootPath = (project.value.workDir || '').trim()
  let path = ''
  let isDir = false
  if (node.level === 0) {
    path = rootPath
    isDir = true
  } else if (data?.path) {
    path = data.path
    isDir = !!data.isDirectory
  }
  if (!path) return
  const isRoot = !!rootPath && pathsEqual(path, rootPath)
  if (isRoot) return
  const maxX = window.innerWidth - 160
  const maxY = window.innerHeight - 88
  workDirContextMenu.value = {
    visible: true,
    x: Math.min(e.clientX, maxX),
    y: Math.min(e.clientY, maxY),
    path,
    isRoot: false,
    isDirectory: isDir
  }
  nextTick(() => {
    const onDocClick = () => closeWorkDirContextMenu()
    setTimeout(() => {
      document.addEventListener('click', onDocClick, true)
      workDirContextMenuClickCleanup = () => document.removeEventListener('click', onDocClick, true)
    }, 0)
  })
}

const onWorkContextMenuDelete = async () => {
  const target = workDirContextMenu.value.path
  closeWorkDirContextMenu()
  if (!target) return
  try {
    await ElMessageBox.confirm('确认删除该文件/目录？目录会递归删除且不可恢复。', '删除确认', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning'
    })
    const res: any = await ProjectApi.deleteFileOrDirectory(target)
    if (res?.success) {
      ElMessage.success('删除成功')
      workTreeVersion.value += 1
    } else {
      ElMessage.error(res?.message || '删除失败')
    }
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error('删除失败')
    }
  }
}

const handleEnvCheck = async () => {
  const id = Number(route.params.id)
  if (!id) return
  if (!project.value?.workDir) {
    ElMessage.warning('项目未设置工作目录')
    return
  }
  envCheckLoading.value = true
  try {
    const res: any = await ProjectApi.checkProjectWorkDirEnv(id)
    const data = res?.data || {}
    const lines: string[] = []
    lines.push(`开发语言: ${project.value?.devLanguage || '未设置'}`)
    lines.push(`退出码: ${data?.exitCode ?? '-'}`)
    if (data?.stdout) lines.push(`stdout:\n${data.stdout}`)
    if (data?.stderr) lines.push(`stderr:\n${data.stderr}`)
    await ElMessageBox.alert(lines.join('\n'), res?.success ? '环境检测完成' : '环境检测失败', {
      confirmButtonText: '知道了'
    })
    if (!res?.success) {
      ElMessage.error(res?.message || '环境检测失败')
    }
  } catch (error: any) {
    ElMessage.error(error?.message || '环境检测失败')
  } finally {
    envCheckLoading.value = false
  }
}

const handleInstallDeps = async () => {
  const id = Number(route.params.id)
  if (!id) return
  if (runStreamRunning.value) {
    ElMessage.warning('已有任务正在运行，请稍后')
    return
  }
  if (!project.value?.workDir) {
    ElMessage.warning('项目未设置工作目录')
    return
  }
  installDepsLoading.value = true
  try {
    runStreamVisible.value = true
    runStreamRunning.value = true
    runStreamTitle.value = '安装依赖日志'
    runStreamLog.value = ''

    const startRes: any = await ProjectApi.startInstallProjectWorkDirDepsStream(id)
    if (!startRes?.success || !startRes?.taskId) {
      runStreamRunning.value = false
      runStreamLog.value = '启动安装失败\n' + (startRes?.message || '')
      ElMessage.error(startRes?.message || '启动安装失败')
      return
    }
    const taskId = startRes.taskId as string
    for (;;) {
      const pollRes: any = await ProjectApi.getInstallProjectWorkDirDepsStream(id, taskId)
      if (!pollRes?.success) {
        runStreamLog.value = appendRunLog(runStreamLog.value, `轮询失败: ${pollRes?.message || '未知错误'}`)
        runStreamRunning.value = false
        ElMessage.error(pollRes?.message || '安装依赖失败')
        return
      }
      const delta = String(pollRes?.logDelta || '')
      if (delta) {
        runStreamLog.value = appendRunLog(runStreamLog.value, delta.trimEnd())
      }
      if (pollRes?.finished) {
        runStreamRunning.value = false
        if (pollRes?.taskSuccess) {
          runStreamLog.value = appendRunLog(runStreamLog.value, `\n安装完成: ${pollRes?.message || 'SUCCESS'}`)
          ElMessage.success('依赖安装完成')
        } else {
          runStreamLog.value = appendRunLog(runStreamLog.value, `\n安装失败: ${pollRes?.message || 'FAILED'}`)
          ElMessage.error(pollRes?.message || '依赖安装失败')
        }
        return
      }
      await sleep(700)
    }
  } catch (error: any) {
    runStreamRunning.value = false
    runStreamLog.value = appendRunLog(runStreamLog.value, `安装异常: ${error?.message || '未知错误'}`)
    ElMessage.error(error?.message || '依赖安装失败')
  } finally {
    installDepsLoading.value = false
  }
}

const handleRebuildPythonVenv = async () => {
  const id = Number(route.params.id)
  if (!id) return
  if (runStreamRunning.value) {
    ElMessage.warning('已有任务正在运行，请稍后')
    return
  }
  if (!project.value?.workDir) {
    ElMessage.warning('项目未设置工作目录')
    return
  }
  if ((project.value?.devLanguage || '').toLowerCase() !== 'python') {
    ElMessage.warning('仅 python 项目支持重建 Python3.10 venv')
    return
  }
  try {
    await ElMessageBox.confirm('将删除当前项目 venv 并用 Python3.10 重建，是否继续？', '确认重建', {
      confirmButtonText: '继续',
      cancelButtonText: '取消',
      type: 'warning'
    })
  } catch {
    return
  }
  rebuildVenvLoading.value = true
  try {
    runStreamVisible.value = true
    runStreamRunning.value = true
    runStreamTitle.value = '重建 Python3.10 venv 日志'
    runStreamLog.value = ''

    const startRes: any = await ProjectApi.startRebuildPython310VenvStream(id)
    if (!startRes?.success || !startRes?.taskId) {
      runStreamRunning.value = false
      runStreamLog.value = '启动重建失败\n' + (startRes?.message || '')
      ElMessage.error(startRes?.message || '启动重建失败')
      return
    }
    const taskId = startRes.taskId as string
    for (;;) {
      const pollRes: any = await ProjectApi.getRebuildPython310VenvStream(id, taskId)
      if (!pollRes?.success) {
        runStreamLog.value = appendRunLog(runStreamLog.value, `轮询失败: ${pollRes?.message || '未知错误'}`)
        runStreamRunning.value = false
        ElMessage.error(pollRes?.message || '重建失败')
        return
      }
      const delta = String(pollRes?.logDelta || '')
      if (delta) {
        runStreamLog.value = appendRunLog(runStreamLog.value, delta.trimEnd())
      }
      if (pollRes?.finished) {
        runStreamRunning.value = false
        if (pollRes?.taskSuccess) {
          runStreamLog.value = appendRunLog(runStreamLog.value, `\n重建完成: ${pollRes?.message || 'SUCCESS'}`)
          ElMessage.success('Python3.10 venv 重建完成')
        } else {
          runStreamLog.value = appendRunLog(runStreamLog.value, `\n重建失败: ${pollRes?.message || 'FAILED'}`)
          ElMessage.error(pollRes?.message || 'Python3.10 venv 重建失败')
        }
        return
      }
      await sleep(700)
    }
  } catch (error: any) {
    runStreamRunning.value = false
    runStreamLog.value = appendRunLog(runStreamLog.value, `重建异常: ${error?.message || '未知错误'}`)
    ElMessage.error(error?.message || '重建失败')
  } finally {
    rebuildVenvLoading.value = false
  }
}

/**
 * 初始化部署脚本：调用 Claude Code 生成 deploy-local.sh 和 nginx 配置
 */
const handleInitDeployScript = async () => {
  const id = Number(route.params.id)
  if (!id) return
  if (runStreamRunning.value) {
    ElMessage.warning('已有任务正在运行，请稍后')
    return
  }
  if (!project.value?.workDir) {
    ElMessage.warning('项目未设置工作目录')
    return
  }

  initDeployScriptLoading.value = true
  try {
    // 从提示词模板 API 获取模板
    let prompt: string
    try {
      const templateRes: any = await PromptTemplateApi.getTemplateByKey('init_deploy_script')
      if (templateRes?.templateContent) {
        // 替换模板变量
        prompt = templateRes.templateContent
          .replace(/{projectName}/g, project.value.name || '未命名项目')
          .replace(/{workDir}/g, project.value.workDir)
          .replace(/{devLanguage}/g, project.value.devLanguage || '未知')
          .replace(/{gitUrl}/g, project.value.gitUrl || '未设置')
          .replace(/{nginxAppName}/g, project.value.name?.replace(/[^a-zA-Z0-9_-]/g, '-') || 'app')
          .replace(/{envVarsJson}/g, project.value.envVarsJson || '[]')
      } else {
        throw new Error('模板内容为空')
      }
    } catch (err) {
      // 如果模板获取失败，使用内置默认模板
      console.warn('获取提示词模板失败，使用默认模板:', err)
      // 构建环境变量描述
      let envVarsDesc = '无'
      if (project.value.envVarsJson) {
        try {
          const parsed = JSON.parse(project.value.envVarsJson)
          if (Array.isArray(parsed) && parsed.length > 0) {
            envVarsDesc = parsed.map((ev: any) => `- ${ev.key}=${ev.value}`).join('\n')
          }
        } catch { /* ignore */ }
      }
      prompt = `## 任务：初始化项目部署脚本和 nginx 配置

### 项目信息
- 项目名称：${project.value.name || '未命名项目'}
- 工作目录：${project.value.workDir}
- 开发语言：${project.value.devLanguage || '未知'}
- Git 地址：${project.value.gitUrl || '未设置'}
- 环境变量：
${envVarsDesc}

### 重要规则

**⚠️ 所有 Shell 命令执行必须使用 sudo**
- 创建文件、目录时使用 sudo
- 修改系统配置文件（如 nginx 配置）时使用 sudo
- 执行部署脚本时使用 sudo
- 进程管理（启动/停止/重启）时使用 sudo
- 文件操作（复制/移动/删除）涉及系统目录时使用 sudo

**⚠️ 前后端分离原则**
- 前端构建物（dist）始终输出到独立的发布目录（如 /opt/claude_sprint/frontend-dist），由 nginx 直接服务
- **禁止**将前端构建物复制到后端的 static/resources 目录
- 后端只提供 API 服务，不托管前端静态文件

### 执行步骤

**步骤 1：项目类型检测**
- 检查工作目录下是否存在 \`pom.xml\`（Java/Maven 后端）
- 检查是否存在 \`frontend/package.json\`（Vue/前端）
- 确定项目类型：纯后端 / 纯前端 / 前后端分离

**步骤 2：部署脚本生成/优化**
- 检查工作目录下是否存在 \`deploy-local.sh\`
- 如不存在，根据项目类型创建脚本：
  - 参数支持：DEPLOY_PROJECT_DIR, DEPLOY_FRONTEND_DIR, DEPLOY_SERVER_PORT 等
  - 模式支持：backend|bd|frontend|ft|all（拉代码并构建），简称优先
  - 服务管理支持：start|stop|restart|status（不拉代码不构建）
  - 自动检测未被占用的端口（从 8081 开始递增）
  - **关键**：所有系统级命令（lsof, kill, pgrep, nginx, cp 到系统目录等）前加 sudo
  - **环境变量注入**：脚本需支持 \`JVM_OPTS_ENV\` 环境变量（格式: KEY1=VALUE1 KEY2=VALUE2），并在启动 Java 进程时转换为 \`-Dkey=value\` 传递给 JVM
  - **nginx 初始化控制**：支持 \`DEPLOY_INIT_NGINX\` 环境变量（默认 false），只有设为 true 时才同步 nginx 配置
- 如存在，检查并优化：
  - 端口自动检测逻辑
  - 进程停止/启动/重启/状态查看逻辑（使用 sudo）
  - 健康检查
  - nginx 备份只保留一份（先删旧备份再创建新备份，使用 sudo）
  - **环境变量注入逻辑**：确保脚本支持 \`JVM_OPTS_ENV\` 参数并注入 JVM 参数
  - **nginx 初始化控制**：确保 \`DEPLOY_INIT_NGINX\` 参数生效，默认 false

**步骤 3：nginx 配置生成**
- 使用 sudo 在 \`/etc/nginx/conf.d/\` 创建 \`${project.value.name?.replace(/[^a-zA-Z0-9_-]/g, '-') || 'app'}.conf\`
- 配置内容：
  - 前端静态资源服务（location /），指向独立的前端发布目录
  - 后端 API 代理（location /api/）
  - WebSocket 代理（location /ws/）
  - SSE 流式响应支持
- 使用 sudo 测试并重载 nginx 配置

**步骤 4：输出总结**
- 列出生成/修改的文件
- 提示后续操作（如 sudo nginx -s reload）

参考现有模板：
- 部署脚本参考：${project.value.workDir}/deploy-local.sh 或 /srv/zfei/projects/claude_sprint/deploy-local.sh
- nginx 配置参考：/srv/zfei/projects/claude_sprint/nginx-optimized.conf

请依次执行上述步骤，并在每个步骤完成后报告进度。`
    }

    const startRes: any = await ProjectApi.startClaudeAdhocCommandStream(id, { prompt })
    if (!startRes?.success || !startRes?.taskId) {
      ElMessage.error(startRes?.message || '启动 Claude 命令失败')
      return
    }

    runStreamVisible.value = true
    runStreamRunning.value = true
    runStreamTitle.value = '初始化部署脚本执行日志'
    runStreamLog.value = ''

    const taskId = startRes.taskId as string
    for (;;) {
      const pollRes: any = await ProjectApi.getClaudeAdhocCommandStream(id, taskId)
      if (!pollRes?.success) {
        runStreamLog.value = appendRunLog(runStreamLog.value, `轮询失败: ${pollRes?.message || '未知错误'}`)
        runStreamRunning.value = false
        ElMessage.error(pollRes?.message || '执行失败')
        return
      }
      const delta = String(pollRes?.logDelta || '')
      if (delta) {
        runStreamLog.value = appendRunLog(runStreamLog.value, delta.trimEnd())
      }
      if (pollRes?.finished) {
        runStreamRunning.value = false
        if (pollRes?.taskSuccess) {
          runStreamLog.value = appendRunLog(runStreamLog.value, `\n完成: ${pollRes?.message || 'SUCCESS'}`)
          ElMessage.success('部署脚本初始化完成')
          // 刷新文件树
          workTreeVersion.value++
        } else {
          runStreamLog.value = appendRunLog(runStreamLog.value, `\n失败: ${pollRes?.message || 'FAILED'}`)
          ElMessage.error(pollRes?.message || '执行失败')
        }
        return
      }
      await sleep(700)
    }
  } catch (error: any) {
    runStreamRunning.value = false
    runStreamLog.value = appendRunLog(runStreamLog.value, `异常: ${error?.message || '未知错误'}`)
    ElMessage.error(error?.message || '执行失败')
  } finally {
    initDeployScriptLoading.value = false
  }
}

const handleGitPull = async () => {
  const id = Number(route.params.id)
  if (!id) return
  if (runStreamRunning.value) {
    ElMessage.warning('已有任务正在运行，请稍后')
    return
  }
  if (!project.value?.workDir) {
    ElMessage.warning('项目未设置工作目录')
    return
  }
  gitPullLoading.value = true
  try {
    runStreamVisible.value = true
    runStreamRunning.value = true
    runStreamTitle.value = 'Git Pull 日志'
    runStreamLog.value = ''

    const startRes: any = await ProjectApi.startGitPullWorkDirStream(id)
    if (!startRes?.success || !startRes?.taskId) {
      runStreamRunning.value = false
      runStreamLog.value = '启动 git pull 失败\n' + (startRes?.message || '')
      ElMessage.error(startRes?.message || '启动 git pull 失败')
      return
    }
    const taskId = startRes.taskId as string
    for (;;) {
      const pollRes: any = await ProjectApi.getGitPullWorkDirStream(id, taskId)
      if (!pollRes?.success) {
        runStreamLog.value = appendRunLog(runStreamLog.value, `轮询失败: ${pollRes?.message || '未知错误'}`)
        runStreamRunning.value = false
        ElMessage.error(pollRes?.message || 'git pull 失败')
        return
      }
      const delta = String(pollRes?.logDelta || '')
      if (delta) {
        runStreamLog.value = appendRunLog(runStreamLog.value, delta.trimEnd())
      }
      if (pollRes?.finished) {
        runStreamRunning.value = false
        if (pollRes?.taskSuccess) {
          runStreamLog.value = appendRunLog(runStreamLog.value, `\ngit pull 完成: ${pollRes?.message || 'SUCCESS'}`)
          ElMessage.success('git pull 完成')
          workTreeVersion.value += 1
        } else {
          runStreamLog.value = appendRunLog(runStreamLog.value, `\ngit pull 失败: ${pollRes?.message || 'FAILED'}`)
          ElMessage.error(pollRes?.message || 'git pull 失败')
        }
        return
      }
      await sleep(700)
    }
  } catch (error: any) {
    runStreamRunning.value = false
    runStreamLog.value = appendRunLog(runStreamLog.value, `git pull 异常: ${error?.message || '未知错误'}`)
    ElMessage.error(error?.message || 'git pull 失败')
  } finally {
    gitPullLoading.value = false
  }
}

const openGitPushDialog = () => {
  if (!project.value?.workDir) {
    ElMessage.warning('项目未设置工作目录')
    return
  }
  gitPushCommitMsg.value = ''
  gitPushDialogVisible.value = true
}

const openGitHistoryDialog = async () => {
  const id = Number(route.params.id)
  if (!id) return
  if (!project.value?.workDir) {
    ElMessage.warning('项目未设置工作目录')
    return
  }
  gitHistoryLoading.value = true
  gitHistoryDialogVisible.value = true
  gitHistoryCommits.value = []
  gitHistoryExpandedHashes.value = []
  try {
    const res: any = await ProjectApi.getWorkDirGitLog(id, 5)
    if (!res?.success) {
      ElMessage.error(res?.message || '获取提交记录失败')
      return
    }
    if (res.isGitRepo === false) {
      ElMessage.warning('当前目录不是 Git 仓库')
      return
    }
    gitHistoryCommits.value = (Array.isArray(res.commits) ? res.commits : []).map((c: any) => ({
      ...c,
      files: [],
      filesLoading: false,
      filesLoaded: false,
      filesError: ''
    }))
  } catch (e: any) {
    ElMessage.error(e?.message || '获取提交记录失败')
  } finally {
    gitHistoryLoading.value = false
  }
}

const handleGitHistoryRowClick = (row: any) => {
  const hash = row.hash
  const idx = gitHistoryExpandedHashes.value.indexOf(hash)
  if (idx >= 0) {
    gitHistoryExpandedHashes.value.splice(idx, 1)
  } else {
    gitHistoryExpandedHashes.value.push(hash)
    if (!row.filesLoaded) {
      loadGitHistoryFiles(row)
    }
  }
}

const handleGitHistoryExpandChange = (expandedRows: any[]) => {
  gitHistoryExpandedHashes.value = expandedRows.map((r: any) => r.hash)
}

const loadGitHistoryFiles = async (row: any) => {
  const id = Number(route.params.id)
  if (!id || !row.hash) return
  row.filesLoading = true
  row.filesError = ''
  try {
    const res: any = await ProjectApi.getWorkDirGitLogFiles(id, row.hash)
    if (!res?.success) {
      row.filesError = res?.message || '获取文件列表失败'
      return
    }
    row.files = Array.isArray(res.files) ? res.files : []
    row.filesLoaded = true
  } catch (e: any) {
    row.filesError = e?.message || '获取文件列表失败'
  } finally {
    row.filesLoading = false
  }
}

const handleGitPush = async () => {
  const id = Number(route.params.id)
  if (!id) return
  if (runStreamRunning.value) {
    ElMessage.warning('已有任务正在运行，请稍后')
    return
  }
  if (!project.value?.workDir) {
    ElMessage.warning('项目未设置工作目录')
    return
  }
  if (!gitPushCommitMsg.value.trim()) {
    ElMessage.warning('请输入提交信息')
    return
  }
  gitPushDialogVisible.value = false
  gitPushLoading.value = true
  try {
    runStreamVisible.value = true
    runStreamRunning.value = true
    runStreamTitle.value = 'Git Push 日志'
    runStreamLog.value = ''

    const startRes: any = await ProjectApi.startGitPushWorkDirStream(id, gitPushCommitMsg.value.trim())
    if (!startRes?.success || !startRes?.taskId) {
      runStreamRunning.value = false
      runStreamLog.value = '启动 git push 失败\n' + (startRes?.message || '')
      ElMessage.error(startRes?.message || '启动 git push 失败')
      return
    }
    const taskId = startRes.taskId as string
    for (;;) {
      const pollRes: any = await ProjectApi.getGitPushWorkDirStream(id, taskId)
      if (!pollRes?.success) {
        runStreamLog.value = appendRunLog(runStreamLog.value, `轮询失败: ${pollRes?.message || '未知错误'}`)
        runStreamRunning.value = false
        ElMessage.error(pollRes?.message || 'git push 失败')
        return
      }
      const delta = String(pollRes?.logDelta || '')
      if (delta) {
        runStreamLog.value = appendRunLog(runStreamLog.value, delta.trimEnd())
      }
      if (pollRes?.finished) {
        runStreamRunning.value = false
        if (pollRes?.taskSuccess) {
          runStreamLog.value = appendRunLog(runStreamLog.value, `\ngit push 完成: ${pollRes?.message || 'SUCCESS'}`)
          ElMessage.success('git push 完成')
          // 退出 Git 模式并刷新工程目录
          workTreeGitMode.value = false
          gitChangedPaths.value = []
          workTreeVersion.value += 1
        } else {
          runStreamLog.value = appendRunLog(runStreamLog.value, `\ngit push 失败: ${pollRes?.message || 'FAILED'}`)
          ElMessage.error(pollRes?.message || 'git push 失败')
        }
        return
      }
      await sleep(700)
    }
  } catch (error: any) {
    runStreamRunning.value = false
    runStreamLog.value = appendRunLog(runStreamLog.value, `git push 异常: ${error?.message || '未知错误'}`)
    ElMessage.error(error?.message || 'git push 失败')
  } finally {
    gitPushLoading.value = false
  }
}

/** AI 提交推送（双击 Git Push 按钮触发） */
const handleAiGitPush = async () => {
  const id = Number(route.params.id)
  if (!id) return
  if (runStreamRunning.value) {
    ElMessage.warning('已有任务正在运行，请稍后')
    return
  }
  if (!project.value?.workDir) {
    ElMessage.warning('项目未设置工作目录')
    return
  }
  gitPushLoading.value = true
  try {
    runStreamVisible.value = true
    runStreamRunning.value = true
    runStreamTitle.value = 'AI 提交推送日志'
    runStreamLog.value = ''

    const startRes: any = await ProjectApi.startAiGitPushWorkDirStream(id)
    if (!startRes?.success || !startRes?.taskId) {
      runStreamRunning.value = false
      runStreamLog.value = '启动 AI 提交推送失败\n' + (startRes?.message || '')
      ElMessage.error(startRes?.message || '启动 AI 提交推送失败')
      return
    }
    const taskId = startRes.taskId as string
    for (;;) {
      const pollRes: any = await ProjectApi.getAiGitPushWorkDirStream(id, taskId)
      if (!pollRes?.success) {
        runStreamLog.value = appendRunLog(runStreamLog.value, `轮询失败: ${pollRes?.message || '未知错误'}`)
        runStreamRunning.value = false
        ElMessage.error(pollRes?.message || 'AI 提交推送失败')
        return
      }
      const delta = String(pollRes?.logDelta || '')
      if (delta) {
        runStreamLog.value = appendRunLog(runStreamLog.value, delta.trimEnd())
      }
      if (pollRes?.finished) {
        runStreamRunning.value = false
        if (pollRes?.taskSuccess) {
          runStreamLog.value = appendRunLog(runStreamLog.value, `\nAI 提交推送完成: ${pollRes?.message || 'SUCCESS'}`)
          ElMessage.success('AI 提交推送完成')
          // 退出 Git 模式并刷新工程目录
          workTreeGitMode.value = false
          gitChangedPaths.value = []
          workTreeVersion.value += 1
        } else {
          runStreamLog.value = appendRunLog(runStreamLog.value, `\nAI 提交推送失败: ${pollRes?.message || 'FAILED'}`)
          ElMessage.error(pollRes?.message || 'AI 提交推送失败')
        }
        return
      }
      await sleep(700)
    }
  } catch (error: any) {
    runStreamRunning.value = false
    runStreamLog.value = appendRunLog(runStreamLog.value, `AI 提交推送异常: ${error?.message || '未知错误'}`)
    ElMessage.error(error?.message || 'AI 提交推送失败')
  } finally {
    gitPushLoading.value = false
  }
}

// 服务操作相关方法
// 部署相关方法（stub - PortAllocationApi deleted, ServerApi deleted）
const openDeployDialog = async () => {
  const id = Number(route.params.id)
  if (!id) return
  // 直接打开部署对话框，跳过端口检查
  await loadServerList()
  selectedServerId.value = getLocalServerId() ?? (serverList.value[0]?.id ?? null)
  deployMode.value = 'all'
  deployInitNginx.value = false
  deployDialogVisible.value = true
}

// 获取操作类型的中文描述
const getDeployModeLabel = (mode: string): string => {
  const labels: Record<string, string> = {
    all: '全部部署',
    backend: '后端部署',
    frontend: '前端部署',
    start: '启动应用',
    stop: '停止应用',
    restart: '重启应用',
    status: '查看状态'
  }
  return labels[mode] || mode
}

const copyDeployLogText = () => {
  if (!deployLogText.value) return
  const textarea = document.createElement('textarea')
  textarea.value = deployLogText.value
  textarea.style.position = 'fixed'
  textarea.style.opacity = '0'
  document.body.appendChild(textarea)
  textarea.select()
  try {
    document.execCommand('copy')
    deployLogCopied.value = true
    setTimeout(() => { deployLogCopied.value = false }, 2000)
  } catch {
    ElMessage.error('复制失败')
  } finally {
    document.body.removeChild(textarea)
  }
}

const handleDeploy = async () => {
  const id = Number(route.params.id)
  if (!id) return
  if (!project.value?.workDir) {
    ElMessage.warning('项目未设置工作目录')
    return
  }
  if (!selectedServerId.value) {
    ElMessage.warning('请选择目标服务器')
    return
  }

  // 根据服务器选择调用不同的部署接口
  if (isLocalServer(selectedServerId.value)) {
    // 本机部署
    handleLocalDeploy(id)
  } else if (isK8sServer(selectedServerId.value)) {
    // K8s 部署：强制 mode 为 all
    handleRemoteDeploy(id, 'all')
  } else {
    // 远程 SSH 部署
    handleRemoteDeploy(id)
  }
}

// 本机部署
const handleLocalDeploy = async (id: number) => {
  deployDialogVisible.value = false
  deployLoading.value = true
  deployLogDialogVisible.value = true
  deployRunning.value = true
  deployLogText.value = `执行操作: ${getDeployModeLabel(deployMode.value)}...\n`
  deployLogReadLen = 0  // 重置已读取长度
  deployPollErrorCount = 0  // 重置错误计数
  deployResultUrls.value = null  // 重置端口信息

  try {
    const startRes: any = await ProjectApi.startDeployWorkDirStream(id, deployMode.value, deployInitNginx.value)
    if (!startRes?.success || !startRes?.taskId) {
      deployRunning.value = false
      deployLogText.value += '启动部署失败\n' + (startRes?.message || '')
      ElMessage.error(startRes?.message || '启动部署失败')
      deployLoading.value = false
      return
    }
    deployTaskId = startRes.taskId as string
    if (deployPollTimer) {
      clearTimeout(deployPollTimer)
      deployPollTimer = null
    }

    // 使用递归 setTimeout 实现动态间隔轮询
    const pollDeployStatus = async () => {
      try {
        const pollRes: any = await ProjectApi.getDeployWorkDirStream(id, deployTaskId)

        // 后端返回 success: false（如任务不存在），也需要重试容错
        if (!pollRes?.success) {
          deployPollErrorCount++
          if (deployPollErrorCount > 15) {
            deployRunning.value = false
            deployLoading.value = false
            deployLogText.value += `轮询失败: ${pollRes?.message || '未知错误'}（已重试 15 次）\n`
            ElMessage.error(pollRes?.message || '部署失败')
          } else {
            deployLogText.value += `[重试 ${deployPollErrorCount}/15] ${pollRes?.message || '服务暂时不可用'}，5秒后重试...\n`
            deployPollTimer = window.setTimeout(pollDeployStatus, 5000)
          }
          return
        }

        deployPollErrorCount = 0  // 成功时重置错误计数

        // 后端返回完整日志，前端只追加新增部分
        const fullLog = String(pollRes?.logDelta || '')
        if (fullLog.length > deployLogReadLen) {
          const delta = fullLog.substring(deployLogReadLen)
          deployLogReadLen = fullLog.length
          deployLogText.value += delta
        }
        if (pollRes?.finished) {
          deployRunning.value = false
          deployLoading.value = false
          const isServiceOp = ['start', 'stop', 'restart', 'status'].includes(deployMode.value)
          if (pollRes?.taskSuccess) {
            deployLogText.value += `\n${isServiceOp ? '操作' : '部署'}完成: ${pollRes?.message || 'SUCCESS'}\n`
            ElMessage.success(`${isServiceOp ? '操作' : '部署'}完成`)
            // 处理部署完成后的端口信息
            if (pollRes?.frontendUrl || pollRes?.backendUrl) {
              deployResultUrls.value = {
                frontendUrl: pollRes.frontendUrl,
                backendUrl: pollRes.backendUrl
              }
            } else {
              deployResultUrls.value = null
            }
          } else {
            deployLogText.value += `\n${isServiceOp ? '操作' : '部署'}失败: ${pollRes?.message || 'FAILED'}\n`
            ElMessage.error(pollRes?.message || `${isServiceOp ? '操作' : '部署'}失败`)
            deployResultUrls.value = null
          }
          // 清理服务端状态文件
          ProjectApi.deleteDeployStream(id, deployTaskId).catch(() => {})
        } else {
          // 未完成，800ms 后继续轮询
          deployPollTimer = window.setTimeout(pollDeployStatus, 800)
        }
      } catch {
        deployPollErrorCount++
        // 部署期间后端可能重启，增加容错重试（最多 15 次，每次间隔 5 秒，总计约 75 秒）
        if (deployPollErrorCount > 15) {
          deployRunning.value = false
          deployLoading.value = false
          deployLogText.value += `轮询异常: 服务长时间无响应（已重试 15 次）\n`
        } else {
          // 等待 5 秒后重试
          deployLogText.value += `[重试 ${deployPollErrorCount}/15] 服务暂时不可用，5秒后重试...\n`
          deployPollTimer = window.setTimeout(pollDeployStatus, 5000)
        }
      }
    }

    // 启动轮询
    pollDeployStatus()
  } catch {
    deployRunning.value = false
    deployLoading.value = false
    deployLogText.value += '启动部署异常\n'
    ElMessage.error('启动部署异常')
  }
}

// 远程部署
const handleRemoteDeploy = async (id: number, overrideMode?: string) => {
  deployDialogVisible.value = false
  deployLogDialogVisible.value = true
  deployRunning.value = true
  remoteDeployStarting.value = true
  deployLogText.value = `正在连接服务器...\n`

  try {
    const mode = (overrideMode || deployMode.value) as 'all' | 'backend' | 'frontend'
    const url = DeployApi.getDeployStreamUrl(id, selectedServerId.value, mode)
    remoteDeployAbort = new AbortController()

    // 获取 token 用于认证
    const token = localStorage.getItem('token')
    const headers: Record<string, string> = { 'Accept': 'text/event-stream' }
    if (token) {
      headers['Authorization'] = `Bearer ${token}`
    }

    const response = await fetch(url, {
      signal: remoteDeployAbort.signal,
      headers
    })

    remoteDeployStarting.value = false

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`)
    }

    const reader = response.body?.getReader()
    if (!reader) {
      throw new Error('无法读取响应流')
    }

    const decoder = new TextDecoder()
    let buffer = ''

    while (true) {
      const { done, value } = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (const line of lines) {
        if (line.startsWith('data:')) {
          const data = line.substring(5).trim()
          deployLogText.value += data + '\n'
        } else if (line.startsWith('event: done')) {
          deployLogText.value += '\n部署完成\n'
        } else if (line.startsWith('event: error')) {
          deployLogText.value += '\n部署失败\n'
        } else if (line.startsWith('event: output')) {
          // 忽略 event: output 行，数据在 data: 行
        } else if (line.trim() && !line.startsWith(':') && !line.startsWith('event:')) {
          deployLogText.value += line + '\n'
        }
      }
    }

    deployLogText.value += '\n部署完成\n'
    ElMessage.success('部署完成')
  } catch (error: any) {
    if (error.name === 'AbortError') {
      deployLogText.value += '\n部署已取消\n'
    } else {
      deployLogText.value += `\n部署失败: ${error?.message || '未知错误'}\n`
      ElMessage.error(error?.message || '部署失败')
    }
  } finally {
    deployRunning.value = false
    remoteDeployStarting.value = false
    remoteDeployAbort = null
  }
}

/**
 * 加载服务器列表（stub - ServerApi deleted）
 */
const loadServerList = async () => {
  serverList.value = []
  selectedServerId.value = null
}

const handleUpdateFeatureList = async () => {
  const id = Number(route.params.id)
  if (!id) return
  if (!project.value?.workDir) {
    ElMessage.warning('项目未设置工作目录')
    return
  }
  updateFeatureListLoading.value = true
  try {
    const res: any = await ProjectApi.updateFeatureListJson(id)
    if (res?.success) {
      const count = Number(res?.count || 0)
      const path = String(res?.path || '')
      ElMessage.success(`任务清单已更新（${count} 条）${path ? `：${path}` : ''}`)
      if (workTreeGitMode.value) {
        await toggleWorkTreeGitMode()
      }
      workTreeVersion.value += 1
    } else {
      ElMessage.error(res?.message || '更新任务清单失败')
    }
  } catch (error: any) {
    ElMessage.error(error?.message || '更新任务清单失败')
  } finally {
    updateFeatureListLoading.value = false
  }
}

const handleDataNodeClick = async (data: any) => {
  if (data.isDirectory) {
    dataTreeCwd.value = (data.path || '').trim()
    selectedDataDirPath.value = dataTreeCwd.value
    dataTreeVersion.value += 1
    return
  }
  if (!isPreviewable(data.name)) {
    ElMessage.info('该文件类型不支持预览')
    return
  }
  await handleWorkNodeClick(data)
}

function closeDataDirContextMenu() {
  dataDirContextMenu.value.visible = false
  if (dataDirContextMenuClickCleanup) {
    dataDirContextMenuClickCleanup()
    dataDirContextMenuClickCleanup = null
  }
}

/** Element Plus Tree: (event, data, node) */
const onDataDirNodeContextMenu = (e: MouseEvent, data: any, node: Node) => {
  e.preventDefault()
  closeDataDirContextMenu()
  const rootPath = (dataTreeCwd.value || project.value.dataDir || '').trim()
  let path = ''
  let isDir = false
  if (node.level === 0) {
    path = rootPath
    isDir = true
  } else if (data?.path) {
    path = data.path
    isDir = !!data.isDirectory
  }
  if (!path || !isDir) {
    return
  }
  const projectDataRoot = (project.value.dataDir || '').trim()
  const isRoot = !!projectDataRoot && pathsEqual(path, projectDataRoot)
  selectedDataDirPath.value = path
  if (isRoot) {
    return
  }
  const maxX = window.innerWidth - 160
  const maxY = window.innerHeight - 88
  dataDirContextMenu.value = {
    visible: true,
    x: Math.min(e.clientX, maxX),
    y: Math.min(e.clientY, maxY),
    path,
    isRoot: false
  }
  nextTick(() => {
    const onDocClick = () => closeDataDirContextMenu()
    setTimeout(() => {
      document.addEventListener('click', onDocClick, true)
      dataDirContextMenuClickCleanup = () => document.removeEventListener('click', onDocClick, true)
    }, 0)
  })
}

const onContextMenuRename = async () => {
  const path = dataDirContextMenu.value.path
  closeDataDirContextMenu()
  await handleRenameDataDir(path)
}

const onContextMenuDelete = async () => {
  const path = dataDirContextMenu.value.path
  closeDataDirContextMenu()
  await handleDeleteDataDir(path)
}

const refreshDataTree = () => {
  dataTreeVersion.value += 1
}

const handleDataDirUpload = async (options: UploadRequestOptions) => {
  // 验证文件对象是否有效
  if (!options.file) {
    ElMessage.error('文件对象无效，请重新选择')
    options.onError?.(new Error('invalid file') as any)
    return
  }

  // 确保文件对象是 File 或 Blob
  const fileToUpload = options.file instanceof Blob ? options.file : new Blob([options.file])
  const fileName = options.file.name || 'unknown'

  const projectDataDir = (project.value?.dataDir || '').trim()
  if (!projectDataDir) {
    ElMessage.warning('该项目未设置数据目录，请先在项目设置中配置数据目录路径')
    options.onError?.(new Error('no dataDir') as any)
    return
  }

  const parentPath = (dataTreeCwd.value || selectedDataDirPath.value || projectDataDir).trim()
  if (!parentPath) {
    ElMessage.warning('无法确定上传目录，请先进入数据目录')
    options.onError?.(new Error('no path') as any)
    return
  }

  // 初始化进度
  dataDirUploadPending.value += 1
  dataDirUploadProgress.value[fileName] = 0

  try {
    const res: any = await ProjectApi.uploadDataDirFile(parentPath, fileToUpload, (percent) => {
      dataDirUploadProgress.value[fileName] = percent
    })
    if (res?.success) {
      ElMessage.success(`${fileName} 上传成功`)
      options.onSuccess?.(res)
      refreshDataTree()
    } else {
      ElMessage.error(res?.message || '上传失败')
      options.onError?.(new Error(res?.message || 'fail') as any)
    }
  } catch (err) {
    console.error('[handleDataDirUpload] error:', err)
    ElMessage.error('上传失败')
    options.onError?.(new Error('fail') as any)
  } finally {
    dataDirUploadPending.value -= 1
    // 延迟清除进度，让用户看到完成状态
    setTimeout(() => {
      const newProgress = { ...dataDirUploadProgress.value }
      delete newProgress[fileName]
      dataDirUploadProgress.value = newProgress
    }, 500)
  }
}

const handleCreateDataDir = async () => {
  const parentPath = (dataTreeCwd.value || selectedDataDirPath.value || project.value.dataDir || '').trim()
  if (!parentPath) {
    ElMessage.warning('请先选择数据目录节点')
    return
  }
  try {
    const { value } = await ElMessageBox.prompt('请输入新子目录名称', '新建子目录', {
      confirmButtonText: '创建',
      cancelButtonText: '取消',
      inputPattern: /^[^\\/]+$/,
      inputErrorMessage: '目录名不能包含 / 或 \\'
    })
    const res: any = await ProjectApi.createDataSubDirectory(parentPath, value.trim())
    if (res?.success) {
      ElMessage.success('创建成功')
      refreshDataTree()
    } else {
      ElMessage.error(res?.message || '创建失败')
    }
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error('创建失败')
    }
  }
}

const handleRenameDataDir = async (pathOverride?: string) => {
  const targetPath = (pathOverride ?? selectedDataDirPath.value)?.trim()
  if (!targetPath) {
    ElMessage.warning('请右键选择要重命名的子目录')
    return
  }
  if (pathsEqual(targetPath, project.value.dataDir || '')) {
    ElMessage.warning('不允许重命名数据目录根路径')
    return
  }
  const originalDirName =
    targetPath
      .trim()
      .replace(/\\/g, '/')
      .replace(/\/+$/, '')
      .split('/')
      .filter(Boolean)
      .pop() || ''
  try {
    const { value } = await ElMessageBox.prompt('请输入新目录名称', '重命名子目录', {
      confirmButtonText: '重命名',
      cancelButtonText: '取消',
      inputValue: originalDirName,
      inputPattern: /^[^\\/]+$/,
      inputErrorMessage: '目录名不能包含 / 或 \\'
    })
    const res: any = await ProjectApi.renameDataSubDirectory(targetPath, value.trim())
    if (res?.success) {
      ElMessage.success('重命名成功')
      if (pathsEqual(selectedDataDirPath.value, targetPath)) {
        selectedDataDirPath.value = dataTreeCwd.value
      }
      refreshDataTree()
    } else {
      ElMessage.error(res?.message || '重命名失败')
    }
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error('重命名失败')
    }
  }
}

const handleDeleteDataDir = async (pathOverride?: string) => {
  const targetPath = (pathOverride ?? selectedDataDirPath.value)?.trim()
  if (!targetPath) {
    ElMessage.warning('请右键选择要删除的子目录')
    return
  }
  if (pathsEqual(targetPath, project.value.dataDir || '')) {
    ElMessage.warning('不允许删除数据目录根路径')
    return
  }
  try {
    await ElMessageBox.confirm('确认删除该子目录？目录必须为空。', '删除子目录', {
      confirmButtonText: '删除',
      cancelButtonText: '取消',
      type: 'warning'
    })
    const res: any = await ProjectApi.deleteDataSubDirectory(targetPath)
    if (res?.success) {
      ElMessage.success('删除成功')
      if (pathsEqual(selectedDataDirPath.value, targetPath)) {
        selectedDataDirPath.value = dataTreeCwd.value
      }
      refreshDataTree()
    } else {
      ElMessage.error(res?.message || '删除失败')
    }
  } catch (e: any) {
    if (e !== 'cancel') {
      ElMessage.error('删除失败')
    }
  }
}

// 方法
const goBack = () => {
  router.push('/projects')
}

const getStatusType = (status: number) => {
  const statusTypes = ['info', 'success', 'primary', 'warning', 'danger']
  return statusTypes[status] || 'info'
}

const getStatusText = (status: number) => {
  const statusTexts = ['未初始化', '待启动', '进行中', '已暂停', '已完成']
  return statusTexts[status] || '未知'
}

const getTaskStatusType = (status: number) => {
  const statusTypes = ['info', 'success', 'warning', 'success', 'warning', 'danger']
  return statusTypes[status] || 'info'
}

const getTaskStatusText = (status: number) => {
  const statusTexts = ['待办', '执行中', '审查中', '已完成', '已暂停', '失败']
  return statusTexts[status] || '未知'
}

const formatDate = (dateStr: string | null | undefined) => {
  if (dateStr == null || String(dateStr).trim() === '') return '—'
  const date = new Date(dateStr)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleDateString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}

const openCliOsUserDialog = () => {
  cliOsUserInput.value = String((project.value as any)?.cliOsUser || '').trim()
  cliOsUserDialogVisible.value = true
}

const submitCliOsUser = async () => {
  const id = Number(route.params.id)
  if (!id) return
  cliOsUserSaving.value = true
  try {
    const res: any = await ProjectApi.updateProjectCliOsUser(id, { cliOsUser: cliOsUserInput.value.trim() })
    if (res?.success) {
      ElMessage.success(res.message || '已保存')
      cliOsUserDialogVisible.value = false
      await loadProject()
    } else {
      ElMessage.error(res?.message || '保存失败')
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '保存失败')
  } finally {
    cliOsUserSaving.value = false
  }
}

const submitClearCliOsUser = async () => {
  cliOsUserInput.value = ''
  const id = Number(route.params.id)
  if (!id) return
  cliOsUserSaving.value = true
  try {
    const res: any = await ProjectApi.updateProjectCliOsUser(id, { cliOsUser: '' })
    if (res?.success) {
      ElMessage.success(res.message || '已清除关联')
      cliOsUserDialogVisible.value = false
      await loadProject()
    } else {
      ElMessage.error(res?.message || '清除失败')
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '清除失败')
  } finally {
    cliOsUserSaving.value = false
  }
}

const editProject = () => {
  // 填充表单数据
  editForm.value = {
    name: project.value.name,
    description: project.value.description,
    gitUrl: project.value.gitUrl || '',
    devLanguage: project.value.devLanguage || '',
    workDir: project.value.workDir,
    dataDir: project.value.dataDir || '',
    cliOsUser: (project.value as any).cliOsUser || '',
    claudeProfileId: (project.value as any).claudeProfileId || '',
    status: project.value.status
  }
  if (workspaceRoot.value && editForm.value.workDir?.startsWith(`${workspaceRoot.value}/`)) {
    editWorkDirRel.value = editForm.value.workDir.slice(workspaceRoot.value.length + 1)
  } else {
    editWorkDirRel.value = editForm.value.workDir || ''
  }
  if (dataRoot.value && editForm.value.dataDir?.startsWith(`${dataRoot.value}/`)) {
    editDataDirRel.value = editForm.value.dataDir.slice(dataRoot.value.length + 1)
  } else {
    editDataDirRel.value = editForm.value.dataDir || ''
  }
  editVisible.value = true
}

const submitEdit = async () => {
  if (!editFormRef.value) return

  try {
    await editFormRef.value.validate()
    if (workspaceRoot.value) {
      if (!editWorkDirRel.value.trim()) {
        ElMessage.warning('请填写工作目录子路径')
        return
      }
      editForm.value.workDir = editWorkDirRel.value.trim()
    }
    if (dataRoot.value) {
      editForm.value.dataDir = editDataDirRel.value.trim()
    } else {
      editForm.value.dataDir = ''
    }
    const id = Number(route.params.id)
    const res: any = await ProjectApi.updateProject(id, editForm.value)

    if (res.success) {
      ElMessage.success('项目更新成功')
      editVisible.value = false
      // 重新加载项目信息
      await loadProject()
    } else {
      ElMessage.error(res.message || '项目更新失败')
    }
  } catch (error: any) {
    ElMessage.error(error.message || '项目更新失败')
  }
}

const deleteProject = async () => {
  const id = Number(route.params.id)
  if (!id) return
  try {
    await ElMessageBox.confirm('确定要删除该项目吗？删除后不可恢复。', '确认删除', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消'
    })
    const ok = await ProjectApi.deleteProject(id)
    if (ok) {
      ElMessage.success('项目已删除')
      router.push('/projects')
    } else {
      ElMessage.error('删除失败')
    }
  } catch (e) {
    if (e !== 'cancel') {
      console.error('删除项目失败:', e)
      ElMessage.error('删除失败')
    }
  }
}

// ========== 环境变量相关方法 ==========
const addEnvVar = () => {
  envVars.value.push({ key: '', value: '', keyError: false })
}

const removeEnvVar = (index: number) => {
  envVars.value.splice(index, 1)
}

const saveEnvVars = async () => {
  const id = Number(route.params.id)
  if (!id) return

  // 验证 key 格式
  let hasError = false
  const keyPattern = /^[a-zA-Z_][a-zA-Z0-9_]*$/
  for (const envVar of envVars.value) {
    if (envVar.key && !keyPattern.test(envVar.key)) {
      envVar.keyError = true
      hasError = true
    }
  }
  if (hasError) {
    ElMessage.error('存在无效的环境变量名，请检查')
    return
  }

  // 过滤掉空的环境变量
  const validEnvVars = envVars.value.filter(ev => ev.key.trim())

  envVarsSaving.value = true
  try {
    const res: any = await ProjectApi.updateProjectEnvVars(id, validEnvVars)
    if (res.success) {
      ElMessage.success('环境变量保存成功')
      // 更新本地数据
      envVars.value = validEnvVars.map(ev => ({ ...ev, keyError: false }))
    } else {
      ElMessage.error(res.message || '保存失败')
    }
  } catch (e: any) {
    ElMessage.error('保存失败: ' + (e.message || '未知错误'))
  } finally {
    envVarsSaving.value = false
  }
}

const loadEnvVarsFromProject = () => {
  try {
    if (project.value.envVarsJson) {
      const parsed = JSON.parse(project.value.envVarsJson)
      if (Array.isArray(parsed)) {
        envVars.value = parsed.map((ev: any) => ({
          key: ev.key || '',
          value: ev.value || '',
          keyError: false
        }))
      }
    }
  } catch {
    envVars.value = []
  }
}

// 环境变量选择相关方法
const toggleEnvVarSelect = (index: number, selected: boolean) => {
  if (selected) {
    envVarsSelected.value.add(index)
  } else {
    envVarsSelected.value.delete(index)
  }
}

const selectAllEnvVars = () => {
  envVars.value.forEach((_, index) => {
    envVarsSelected.value.add(index)
  })
}

const clearEnvVarsSelection = () => {
  envVarsSelected.value.clear()
}

const copyEnvVarsToPromptVars = () => {
  if (envVarsSelected.value.size === 0) {
    ElMessage.warning('请先选择要复制的环境变量')
    return
  }

  const copiedKeys: string[] = []
  envVarsSelected.value.forEach(index => {
    const envVar = envVars.value[index]
    if (envVar && envVar.key.trim()) {
      // 检查是否已存在相同 key
      const existingIndex = promptVars.value.findIndex(pv => pv.key === envVar.key)
      if (existingIndex >= 0) {
        // 已存在则更新值，并设置为只读
        promptVars.value[existingIndex].value = envVar.value
        promptVars.value[existingIndex].remark = envVar.remark || promptVars.value[existingIndex].remark
        promptVars.value[existingIndex].readonly = true
      } else {
        // 不存在则添加，标记为只读
        promptVars.value.push({
          key: envVar.key,
          value: envVar.value,
          remark: envVar.remark || '',
          keyError: false,
          readonly: true
        })
      }
      copiedKeys.push(envVar.key)
    }
  })

  // 清空选择
  envVarsSelected.value.clear()

  if (copiedKeys.length > 0) {
    ElMessage.success(`已复制 ${copiedKeys.length} 条环境变量到提示词变量`)
  }
}

// ========== 提示词变量相关方法 ==========
const addPromptVar = () => {
  promptVars.value.push({ key: '', value: '', remark: '', keyError: false })
}

const removePromptVar = (index: number) => {
  promptVars.value.splice(index, 1)
}

const savePromptVars = async () => {
  const id = Number(route.params.id)
  if (!id) return

  // 验证 key 格式
  let hasError = false
  const keyPattern = /^[a-zA-Z_][a-zA-Z0-9_]*$/
  for (const promptVar of promptVars.value) {
    if (promptVar.key && !keyPattern.test(promptVar.key)) {
      promptVar.keyError = true
      hasError = true
    }
  }
  if (hasError) {
    ElMessage.error('存在无效的变量名，请检查')
    return
  }

  // 过滤掉空的提示词变量
  const validPromptVars = promptVars.value.filter(pv => pv.key.trim())

  promptVarsSaving.value = true
  try {
    const res: any = await ProjectApi.updateProjectPromptVars(id, validPromptVars)
    if (res.success) {
      ElMessage.success('提示词变量保存成功')
      // 更新本地数据，保留 readonly 状态
      const envVarKeys = new Set(envVars.value.map(ev => ev.key.trim()).filter(k => k))
      promptVars.value = validPromptVars.map(pv => ({
        ...pv,
        keyError: false,
        readonly: envVarKeys.has(pv.key?.trim())
      }))
      // 更新 project 对象
      project.value.promptVarsJson = JSON.stringify(validPromptVars)
      // 重置未保存状态
      markPromptVarsSaved()
    } else {
      ElMessage.error(res.message || '保存失败')
    }
  } catch (e: any) {
    ElMessage.error('保存失败: ' + (e.message || '未知错误'))
  } finally {
    promptVarsSaving.value = false
  }
}

const loadPromptVarsFromProject = () => {
  try {
    if (project.value.promptVarsJson) {
      const parsed = JSON.parse(project.value.promptVarsJson)
      if (Array.isArray(parsed)) {
        // 获取环境变量的 key 集合，用于判断只读状态
        const envVarKeys = new Set(envVars.value.map(ev => ev.key.trim()).filter(k => k))
        promptVars.value = parsed.map((pv: any) => ({
          key: pv.key || '',
          value: pv.value || '',
          remark: pv.remark || '',
          keyError: false,
          // 如果 key 存在于环境变量中，则标记为只读
          readonly: envVarKeys.has(pv.key?.trim())
        }))
      }
    }
  } catch {
    promptVars.value = []
  }
  // 标记初始化完成
  markPromptVarsInitialized()
}

const appendRunLog = (origin: string, delta: string) => {
  const text = String(delta || '')
  if (!origin) return text
  if (!text) return origin
  return `${origin}\n${text}`
}

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

const loadProject = async () => {
  const id = Number(route.params.id)
  if (!id) return
  try {
    project.value = await ProjectApi.getProjectDetail(id)
    // 加载环境变量
    loadEnvVarsFromProject()
    // 加载提示词变量
    loadPromptVarsFromProject()
    // 检测是否为小程序工程
    loadMiniProgramDetect()
    dataTreeCwd.value = (project.value.dataDir || '').trim()
    selectedDataDirPath.value = dataTreeCwd.value
    dataTreeVersion.value += 1
  } catch {
    ElMessage.error('加载项目信息失败')
  }
}

const loadMiniProgramDetect = async () => {
  const id = Number(route.params.id)
  if (!id) return
  try {
    const res: any = await ProjectApi.detectMiniProgramProject(id)
    if (res?.success && res.data) {
      miniProgramDetect.value = res.data
    } else {
      miniProgramDetect.value = null
    }
  } catch {
    miniProgramDetect.value = null
  }
}

const loadProjectClaudeConfig = async () => {
  const id = Number(route.params.id)
  if (!id) return
  projectClaudeConfigLoading.value = true
  try {
    const res: any = await ProjectApi.getProjectClaudeConfig(id)
    if (res?.success) {
      projectClaudeConfig.value = {
        claudeMd: res.claudeMd || '',
        claudeSettings: res.claudeSettings || '',
        skillRelativePath: res.skillRelativePath || '',
        skillContent: res.skillContent || ''
      }
    } else {
      ElMessage.error(res?.message || '加载项目级 Claude 配置失败')
    }
  } catch (error: any) {
    ElMessage.error(error?.message || '加载项目级 Claude 配置失败')
  } finally {
    projectClaudeConfigLoading.value = false
  }
}

// 加载可用的 Skill 列表
const loadAvailableSkills = async () => {
  const id = Number(route.params.id)
  if (!id) return
  skillsLoading.value = true
  try {
    const res: any = await ProjectApi.getAvailableSkills(id)
    if (res?.success) {
      availableSkills.value = res.skills || []
      globalSkillsPath.value = res.globalSkillsPath || ''
    } else {
      ElMessage.error(res?.message || '加载 Skill 列表失败')
    }
  } catch (error: any) {
    ElMessage.error(error?.message || '加载 Skill 列表失败')
  } finally {
    skillsLoading.value = false
  }
}

// 选择 Skill 时更新内容
const onSkillSelect = (skillName: string) => {
  const skill = availableSkills.value.find(s => s.name === skillName)
  if (skill) {
    projectClaudeConfig.value.skillRelativePath = skill.relativePath
    projectClaudeConfig.value.skillContent = skill.content
  }
}

// 点击表格行选择 Skill
const onSkillTableRowClick = (row: any) => {
  selectSkillForEdit(row)
}

// 选择 Skill 进行编辑
const selectSkillForEdit = (skill: any) => {
  projectClaudeConfig.value.skillRelativePath = skill.relativePath
  projectClaudeConfig.value.skillContent = skill.content
}

// 清除 Skill 选择
const clearSkillSelection = () => {
  projectClaudeConfig.value.skillRelativePath = ''
  projectClaudeConfig.value.skillContent = ''
}

const saveProjectClaudeConfig = async () => {
  const id = Number(route.params.id)
  if (!id) return
  projectClaudeConfigSaving.value = true
  try {
    const res: any = await ProjectApi.updateProjectClaudeConfig(id, {
      claudeMd: projectClaudeConfig.value.claudeMd,
      claudeSettings: projectClaudeConfig.value.claudeSettings,
      skillRelativePath: projectClaudeConfig.value.skillRelativePath,
      skillContent: projectClaudeConfig.value.skillContent
    })
    if (res?.success) {
      ElMessage.success('项目级 Claude 配置已保存')
      await loadProjectClaudeConfig()
    } else {
      ElMessage.error(res?.message || '保存失败')
    }
  } catch (error: any) {
    ElMessage.error(error?.message || '保存失败')
  } finally {
    projectClaudeConfigSaving.value = false
  }
}

// Agent functions removed (tab removed)

const loadRoots = async () => {
  try {
    const ws: any = await SettingsApi.getWorkspaceRoot()
    workspaceRoot.value = ws.value || ''
  } catch {
    workspaceRoot.value = ''
  }
  try {
    const dr: any = await SettingsApi.getDataRoot()
    dataRoot.value = dr.value || ''
  } catch {
    dataRoot.value = ''
  }
}

watch(
  () => route.params.id,
  () => {
    // 切换项目时重置 tab 状态
    detailActiveTab.value = 'work'
    // 清除 URL query 中的 tab 参数
    const newQuery = { ...route.query }
    delete newQuery.tab
    router.replace({ query: newQuery })
    workTreeGitMode.value = false
    gitChangedPaths.value = []
    workTreeVersion.value += 1
  }
)

watch(
  detailActiveTab,
  (tab) => {
    // 更新 URL query 参数（不触发导航）
    const newQuery = { ...route.query, tab }
    router.replace({ query: newQuery })
    // 同步到 localStorage 作为备份
    try {
      const id = Number(route.params.id)
      const key = Number.isFinite(id) && id > 0 ? `project-detail-active-tab:${id}` : 'project-detail-active-tab:default'
      localStorage.setItem(key, String(tab || 'work'))
    } catch {
      // 忽略本地存储异常（如隐私模式）
    }
  }
)

onMounted(async () => {
  restoreTabFromUrl()
  loadRoots()
  loadShowMoreActions()
  loadClaudeProfiles()
  // 先加载项目数据，确保 project.id 可用后再加载依赖 projectId 的资源
  await loadProject()
  loadProjectClaudeConfig()
  loadAvailableSkills()
  loadRuntimeStatus()
  // 自动刷新功能已关闭
  // autoRefreshTimer = window.setInterval(() => {
  //   loadProject()
  //   loadProjectTasks()
  // }, 5000)
})

let autoRefreshTimer: number | null = null

// 未保存状态跟踪
const hasUnsavedPromptVars = ref(false)
const promptVarsInitialized = ref(false)

// 监听 promptVars 变化，标记未保存状态（初始化后生效）
watch(
  () => promptVars.value,
  () => {
    if (promptVarsInitialized.value) {
      hasUnsavedPromptVars.value = true
    }
  },
  { deep: true }
)

// 标记初始化完成
const markPromptVarsInitialized = () => {
  promptVarsInitialized.value = true
}

// 保存成功后重置未保存状态
const markPromptVarsSaved = () => {
  hasUnsavedPromptVars.value = false
}

// 路由离开提示
onBeforeRouteLeave((to, from, next) => {
  next()
})

onUnmounted(() => {
  if (previewCopiedTimer) {
    window.clearTimeout(previewCopiedTimer)
    previewCopiedTimer = null
  }
  if (autoRefreshTimer) {
    window.clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
  if (deployPollTimer) {
    window.clearTimeout(deployPollTimer)
    deployPollTimer = null
  }
  closeWorkDirContextMenu()
  closeDataDirContextMenu()
})
</script>

<style scoped>
.detail-tabs-clean {
  width: 100%;
  height: 100%;
}

.detail-tabs-clean :deep(.el-tabs__content) {
  padding: 0;
}

.detail-tabs-clean :deep(.el-tab-pane) {
  padding: 0;
}

/* 项目详情 Tab：顶栏下划线样式，无竖分割线、无灰底盒子，避免首项被裁切 */
.detail-tabs-clean :deep(.el-tabs__header) {
  margin: 0 0 1rem;
}

.detail-tabs-clean :deep(.el-tabs__nav-wrap) {
  margin: 0;
  border: none;
  border-radius: 0;
  background: transparent;
  padding: 0 2px;
}

.detail-tabs-clean :deep(.el-tabs__nav-scroll) {
  padding: 0 4px;
}

.detail-tabs-clean :deep(.el-tabs__nav-wrap::after) {
  display: block;
}

.detail-tabs-clean :deep(.el-tabs__nav) {
  border: none;
}

.detail-tabs-clean :deep(.el-tabs__item) {
  height: 40px;
  padding: 0 18px;
  line-height: 40px;
  font-size: 14px;
  color: var(--el-text-color-regular, #606266);
  border-right: none !important;
  box-sizing: border-box;
}

.detail-tabs-clean :deep(.el-tabs__item:hover) {
  color: var(--el-color-primary);
}

.detail-tabs-clean :deep(.el-tabs__item.is-active) {
  font-weight: 600;
  color: var(--el-color-primary);
  background: transparent;
}

.detail-tabs-clean :deep(.el-tabs__item.is-active::before) {
  display: none;
}

.detail-tabs-clean :deep(.el-tabs__active-bar) {
  display: block;
  height: 2px;
  border-radius: 1px;
}

.detail-tabs-clean .file-tree-card :deep(.el-card__body) {
  max-height: min(70vh, 720px);
  overflow: auto;
}

.shadcn-form-dialog :deep(.el-dialog__body) {
  padding-top: 8px;
}

.top-row {
  display: flex;
  align-items: stretch;
  padding: 0;
  margin: 0;
}

.top-row > .el-col {
  display: flex;
}

.tasks-list {
  margin-bottom: 20px;
}

.tasks-list .el-card__header {
  display: flex;
  align-items: center;
  font-weight: bold;
}

.tasks-list .el-card__header .el-icon {
  margin-right: 8px;
  @apply text-indigo-600;
}

.work-dir-path {
  font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', monospace;
  font-size: 13px;
  @apply text-slate-600;
}

.project-db-info .el-card__header {
  display: flex;
  align-items: center;
  font-weight: bold;
}

.project-db-info .el-card__header .el-icon {
  margin-right: 8px;
  @apply text-indigo-600;
}

.file-tree-card .el-card__header {
  padding: 12px 20px;
}

.file-tree-header {
  display: flex;
  align-items: center;
  gap: 12px;
}

.file-tree-header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-left: auto;
}

.file-tree-title {
  display: flex;
  align-items: center;
  font-weight: bold;
  flex-shrink: 0;
}

.file-tree-title .el-icon {
  margin-right: 8px;
  @apply text-indigo-600;
}

.file-tree-title--clickable {
  cursor: pointer;
  user-select: none;
  border-radius: 6px;
  padding: 2px 8px;
  margin: -2px -8px;
  transition:
    background 0.15s ease,
    color 0.15s ease;
}

.file-tree-title--clickable:hover {
  background: var(--el-fill-color-light, #f3f4f6);
}

.file-tree-title--clickable.is-git-mode {
  color: var(--el-color-primary);
}

.file-tree-title--clickable:focus-visible {
  outline: 2px solid var(--el-color-primary-light-5);
  outline-offset: 1px;
}

.file-tree-path {
  font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', monospace;
  font-size: 12px;
  @apply text-slate-400;
}

.file-tree-path--clickable {
  cursor: pointer;
  transition: color 0.2s;
}

.file-tree-path--clickable:hover {
  @apply text-blue-500;
}

.file-tree-actions {
  display: flex;
  gap: 8px;
}

/* 上传进度显示 */
.upload-progress-container {
  padding: 12px;
  background: var(--el-fill-color-light);
  border-radius: 6px;
  margin-bottom: 8px;
}

.upload-progress-item {
  margin-bottom: 8px;
}

.upload-progress-item:last-child {
  margin-bottom: 0;
}

.upload-progress-name {
  display: block;
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-bottom: 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 300px;
}

/* 更多操作弹框按钮网格布局 */
.more-actions-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
}

.more-actions-grid .el-button {
  width: 100%;
  justify-content: flex-start;
  padding: 12px 16px;
  height: auto;
}

.more-actions-grid .el-button .el-icon {
  margin-right: 8px;
}

.more-actions-user-badge {
  margin-top: 16px;
  padding-top: 12px;
  border-top: 1px solid var(--el-border-color-lighter);
  text-align: center;
}

.tree-node {
  display: inline-flex;
  align-items: center;
  font-size: 14px;
  line-height: 1.5;
}

/* 数据目录：文件行占满节点内容区，右侧放「下载」「删除」 */
.file-tree-card :deep(.el-tree-node > .el-tree-node__content) {
  min-height: 32px;
  align-items: center;
}

.file-tree-card :deep(.el-tree-node > .el-tree-node__content > .el-tree-node__label) {
  flex: 1;
  min-width: 0;
  overflow: hidden;
}

.tree-node--data-file {
  flex: 1;
  min-width: 0;
  max-width: 100%;
  box-sizing: border-box;
}

.tree-node--data-file .tree-label {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tree-bulk-cb {
  flex-shrink: 0;
  margin-right: 4px;
}

.tree-download-btn,
.tree-delete-btn {
  flex-shrink: 0;
  margin-left: 4px;
}

.tree-icon {
  margin-right: 6px;
  font-size: 16px;
}

.tree-icon-folder {
  @apply text-amber-500;
}

.tree-icon-file {
  @apply text-slate-400;
}

.tree-label {
  @apply text-slate-800;
}

.tree-created {
  margin-left: 12px;
  flex-shrink: 0;
  min-width: 132px;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  @apply text-slate-400;
}

.tree-size {
  margin-left: 12px;
  font-size: 12px;
  @apply text-slate-300;
}

.tree-label-previewable {
  cursor: pointer;
}

.tree-label-previewable:hover {
  text-decoration: underline;
  @apply text-indigo-600;
}

.preview-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: none;
  border-radius: 4px 4px 0 0;
  padding: 8px 12px;
  font-size: 12px;
  @apply border border-b-0 border-slate-200 bg-slate-100;
}

.preview-path {
  max-width: 70%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', monospace;
  @apply text-slate-600;
}

.preview-meta {
  flex-shrink: 0;
  @apply text-slate-400;
}

.preview-toolbar-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.preview-content {
  max-height: 70vh;
  overflow: auto;
  border-radius: 0 0 4px 4px;
  @apply border border-slate-200;
}

.preview-editor-wrapper {
  height: 70vh;
  display: flex;
  flex-direction: column;
}

.preview-editor {
  flex: 1;
  width: 100%;
  min-height: 100%;
  padding: 16px;
  border: none;
  outline: none;
  resize: none;
  font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', 'Liberation Mono', monospace;
  font-size: 13px;
  line-height: 1.6;
  tab-size: 2;
  background: #fafafa;
  color: #333;
  box-sizing: border-box;
}

.preview-editor:focus {
  background: #fff;
}

.preview-content pre {
  margin: 0;
  overflow-x: auto;
  padding: 16px;
  tab-size: 4;
  font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', monospace;
  font-size: 13px;
  line-height: 1.6;
  @apply bg-white;
}

.preview-content pre code {
  font-family: inherit;
}

.run-stream-meta {
  margin-bottom: 8px;
  color: #64748b;
  font-size: 13px;
}

.run-stream-log {
  max-height: 58vh;
  min-height: 220px;
  overflow: auto;
  margin: 0;
  padding: 12px;
  border: 1px solid #e2e8f0;
  border-radius: 6px;
  background: #0f172a;
  color: #e2e8f0;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', monospace;
  font-size: 12px;
  line-height: 1.6;
}

.decompose-dialog-body {
  max-height: 75vh;
  overflow-y: auto;
  padding-right: 4px;
}

.cli-terminal-toggle {
  display: flex;
  align-items: center;
  gap: 4px;
  cursor: pointer;
  font-size: 13px;
  margin-bottom: 6px;
  user-select: none;
  @apply text-indigo-600 hover:text-indigo-500;
}

.cli-terminal {
  width: 100%;
  height: 240px;
  box-sizing: border-box;
  overflow-y: auto;
  overflow-x: hidden;
  word-wrap: break-word;
  white-space: pre-wrap;
  transition: height 0.3s ease;
  @apply rounded-md bg-slate-950 px-4 py-3 font-mono text-[13px] leading-relaxed text-slate-200;
}

.cli-terminal-collapsed {
  height: 0;
  padding: 0 16px;
  overflow: hidden;
}

.cli-terminal-placeholder {
  @apply italic text-slate-500;
}

.cli-terminal-line {
  min-height: 1.6em;
}

.cli-terminal-cursor {
  display: inline-block;
  animation: blink 1s step-end infinite;
  @apply text-sky-400;
}

@keyframes blink {
  50% {
    opacity: 0;
  }
}

.decomposed-tasks {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.task-preview-card {
  width: 100%;
}

.task-preview-card :deep(.el-card__header) {
  padding: 8px 12px;
  @apply bg-slate-100;
}

.task-preview-card :deep(.el-card__body) {
  padding: 12px;
}

.task-preview-header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.task-name-input {
  flex: 1;
}

/* AI 拆解任务输入框拖拽区域样式 */
.decompose-input-wrapper {
  position: relative;
  border-radius: 4px;
  transition: all 0.2s ease;
}

.decompose-input-wrapper.is-dragging {
  border: 2px dashed var(--el-color-primary);
  background: rgba(var(--el-color-primary-rgb), 0.05);
}

.decompose-input-wrapper.is-dragging :deep(.el-textarea__inner) {
  background: transparent;
}

.session-recovery-list {
  max-height: 320px;
  overflow-y: auto;
}

.session-recovery-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 4px;
  border-bottom: 1px solid #ebeef5;
  cursor: pointer;
  transition: background 0.2s;
}

.session-recovery-item:last-child {
  border-bottom: none;
}

.session-recovery-item:hover {
  background: #f5f7fa;
}

.session-recovery-item-main {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  min-width: 0;
}

.session-recovery-item-summary {
  font-size: 13px;
  color: #303133;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 200px;
}

.session-recovery-item-time {
  font-size: 12px;
  color: #909399;
}

.session-recovery-item-size {
  font-size: 12px;
  color: #909399;
  white-space: nowrap;
}

.session-recovery-empty {
  padding: 24px;
  text-align: center;
  color: #909399;
  font-size: 14px;
}

/* 服务状态卡片 */
.service-status-card {
  height: 100%;
}

.service-status-card :deep(.el-card__header) {
  padding: 12px 16px;
  border-bottom: 1px solid #e2e8f0;
}

.service-status-card :deep(.el-card__body) {
  padding: 16px;
}

.service-status-item {
  padding: 12px 0;
  border-bottom: 1px solid #f1f5f9;
}

.service-status-item:last-of-type {
  border-bottom: none;
}

.service-status-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.service-status-label {
  font-weight: 500;
  color: #1e293b;
}

.service-status-info {
  font-size: 12px;
  color: #64748b;
}

.service-status-time {
  font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', monospace;
}

.service-status-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #007aff;
  text-decoration: none;
  font-size: 12px;
  font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', monospace;
}

.service-status-link:hover {
  color: #5856d6;
  text-decoration: underline;
}

.service-status-links {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 6px;
}

.service-status-links .service-status-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #007aff;
  text-decoration: none;
  font-size: 12px;
  font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', monospace;
}

.service-status-links .service-status-link:hover {
  color: #5856d6;
  text-decoration: underline;
}

/* macOS 风格玻璃态 */
.glass-card {
  background: rgba(255, 255, 255, 0.72);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border: 1px solid rgba(255, 255, 255, 0.3);
}

/* 操作按钮网格 - 仪表盘风格 */
/* 操作按钮 - 三层布局 */
.action-grid {
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px solid rgba(0, 0, 0, 0.06);
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.action-row {
  display: flex;
  gap: 10px;
}

.action-row--primary {
  justify-content: center;
}

.action-row--footer {
  justify-content: flex-end;
}

.action-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 14px 16px 12px;
  min-width: 80px;
  flex: 1;
  max-width: 160px;
  background: rgba(255, 255, 255, 0.6);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
  border: 1px solid rgba(0, 0, 0, 0.08);
  border-radius: 10px;
  cursor: pointer;
  transition: all 0.2s ease;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.06);
  color: #1d1d1f;
}

.action-card:hover {
  background: rgba(255, 255, 255, 0.9);
  border-color: rgba(0, 122, 255, 0.25);
  box-shadow: 0 3px 10px rgba(0, 122, 255, 0.1);
  transform: translateY(-1px);
  color: #007aff;
}

.action-card:hover .action-card-icon {
  color: #007aff;
}

.action-card:active {
  transform: translateY(0);
  box-shadow: 0 1px 2px rgba(0, 0, 0, 0.06);
}

.action-card:disabled {
  opacity: 0.5;
  cursor: not-allowed;
  transform: none;
}

.action-card--primary {
  background: linear-gradient(135deg, rgba(0, 122, 255, 0.08), rgba(88, 86, 214, 0.08));
  border-color: rgba(0, 122, 255, 0.15);
}

.action-card--primary:hover {
  background: linear-gradient(135deg, rgba(0, 122, 255, 0.15), rgba(88, 86, 214, 0.15));
  border-color: rgba(0, 122, 255, 0.35);
  box-shadow: 0 4px 14px rgba(0, 122, 255, 0.18);
  color: #0066d6;
}

.action-card-icon {
  color: #86868b;
  transition: color 0.2s ease;
}

.action-card-label {
  font-size: 12px;
  font-weight: 500;
  white-space: nowrap;
  color: inherit;
}

/* 修复 SSH 终端抽屉高度问题 */
:deep(.el-drawer__body) {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 0;
  overflow: hidden;
}

/* 部署对话框按钮组不换行 */
.deploy-radio-group {
  display: flex;
  flex-wrap: nowrap;
}

.deploy-radio-group .el-radio-button {
  flex: 1;
}

.deploy-radio-group .el-radio-button__inner {
  width: 100%;
}

.deploy-btn-group {
  display: flex;
  flex-wrap: nowrap;
}

.deploy-btn-group .el-button {
  flex: 1;
  white-space: nowrap;
}
</style>

<!-- Global styles for v-html rendered Markdown (scoped styles cannot penetrate v-html) -->
<style>
/* ========================
   Markdown Preview — Typora-like typography
   ======================== */
.file-preview-dialog .markdown-preview {
  padding: 32px 40px;
  background: #fff;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Roboto', 'Oxygen', 'Ubuntu', 'Cantarell', 'Helvetica Neue', Arial, 'PingFang SC', 'Noto Sans', 'Microsoft YaHei', sans-serif;
  font-size: 16px;
  line-height: 1.8;
  color: #2c3e50;
  word-wrap: break-word;
  overflow-wrap: break-word;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}

/* ---- Headings ---- */
.file-preview-dialog .markdown-preview h1,
.file-preview-dialog .markdown-preview h2,
.file-preview-dialog .markdown-preview h3,
.file-preview-dialog .markdown-preview h4,
.file-preview-dialog .markdown-preview h5,
.file-preview-dialog .markdown-preview h6 {
  margin-top: 32px;
  margin-bottom: 16px;
  font-weight: 600;
  line-height: 1.35;
  color: #1a1a1a;
}

.file-preview-dialog .markdown-preview h1 {
  font-size: 32px;
  font-weight: 700;
  padding-bottom: 14px;
  margin-top: 0;
  border-bottom: 2px solid #eaecef;
}

.file-preview-dialog .markdown-preview h2 {
  font-size: 26px;
  padding-bottom: 12px;
  border-bottom: 1px solid #eaecef;
}

.file-preview-dialog .markdown-preview h3 {
  font-size: 21px;
}

.file-preview-dialog .markdown-preview h4 {
  font-size: 18px;
}

.file-preview-dialog .markdown-preview h5 {
  font-size: 16px;
  color: #444;
}

.file-preview-dialog .markdown-preview h6 {
  font-size: 15px;
  color: #666;
}

/* ---- Paragraphs ---- */
.file-preview-dialog .markdown-preview p {
  margin: 14px 0;
  letter-spacing: 0.01em;
}

/* ---- Links ---- */
.file-preview-dialog .markdown-preview a {
  color: #0366d6;
  text-decoration: none;
  border-bottom: 1px solid transparent;
  transition: border-color 0.15s;
}
.file-preview-dialog .markdown-preview a:hover {
  border-bottom-color: #0366d6;
}

/* ---- Inline code ---- */
.file-preview-dialog .markdown-preview code {
  background: rgba(175, 184, 193, 0.12);
  padding: 2px 7px;
  border-radius: 4px;
  font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', 'Fira Code', 'Cascadia Code', monospace;
  font-size: 0.88em;
  color: #c7254e;
}

/* ---- Code blocks ---- */
.file-preview-dialog .markdown-preview pre {
  position: relative;
  background: #f6f8fa;
  padding: 18px 20px;
  border-radius: 8px;
  overflow-x: auto;
  margin: 20px 0;
  border: 1px solid #e8e8e8;
  line-height: 1.65;
}
.file-preview-dialog .markdown-preview pre code {
  background: transparent;
  padding: 0;
  border-radius: 0;
  color: inherit;
  font-size: 14px;
  font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', 'Fira Code', monospace;
}

/* ---- Blockquote ---- */
.file-preview-dialog .markdown-preview blockquote {
  border-left: 4px solid #0366d6;
  padding: 10px 18px;
  margin: 20px 0;
  color: #57606a;
  background: rgba(3, 102, 214, 0.04);
  border-radius: 0 6px 6px 0;
}
.file-preview-dialog .markdown-preview blockquote p:last-child {
  margin-bottom: 0;
}

/* ---- Tables ---- */
.file-preview-dialog .markdown-preview table {
  width: 100%;
  border-collapse: collapse;
  margin: 20px 0;
  font-size: 15px;
}
.file-preview-dialog .markdown-preview th,
.file-preview-dialog .markdown-preview td {
  padding: 10px 14px;
  border: 1px solid #d0d7de;
  text-align: left;
}
.file-preview-dialog .markdown-preview th {
  background: #f6f8fa;
  font-weight: 600;
  color: #24292e;
}
.file-preview-dialog .markdown-preview tr:nth-child(even) td {
  background: #fafbfc;
}

/* ---- Lists ---- */
.file-preview-dialog .markdown-preview ul,
.file-preview-dialog .markdown-preview ol {
  margin: 14px 0;
  padding-left: 28px;
}
.file-preview-dialog .markdown-preview li {
  margin: 5px 0;
  line-height: 1.75;
}
.file-preview-dialog .markdown-preview li > ul,
.file-preview-dialog .markdown-preview li > ol {
  margin: 4px 0;
}

/* ---- GFM Task Lists ---- */
.file-preview-dialog .markdown-preview .task-list-item {
  list-style: none;
  margin-left: -24px;
}
.file-preview-dialog .markdown-preview .task-list-item input[type="checkbox"] {
  margin-right: 8px;
  transform: scale(1.15);
  accent-color: #0366d6;
}

/* ---- Horizontal rule ---- */
.file-preview-dialog .markdown-preview hr {
  border: none;
  height: 2px;
  background: #eaecef;
  margin: 32px 0;
}

/* ---- Images ---- */
.file-preview-dialog .markdown-preview img {
  max-width: 100%;
  height: auto;
  border-radius: 6px;
  margin: 16px 0;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.08);
}

/* ---- Strikethrough ---- */
.file-preview-dialog .markdown-preview del {
  color: #999;
}

</style>
