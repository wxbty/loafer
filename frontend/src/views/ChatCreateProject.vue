<template>
  <div class="chat-create-page min-h-screen w-full flex flex-col items-center bg-slate-50">
    <!-- Header -->
    <div class="w-full max-w-3xl px-6 pt-12 pb-6 text-center">
      <div class="flex items-center justify-center gap-3 mb-2">
        <h1 class="text-3xl font-bold tracking-tight text-slate-900">AI 对话式创建项目</h1>
        <!-- 新建会话按钮 -->
        <el-tooltip content="开始新的创建会话" placement="top">
          <el-button
            circle
            size="small"
            type="primary"
            plain
            @click="handleNewSession"
          >
            <el-icon><Plus /></el-icon>
          </el-button>
        </el-tooltip>
      </div>
      <p class="mt-2 text-sm text-slate-500">描述你的需求，AI 帮你提炼关键信息，一键生成项目运行环境</p>
      <!-- 会话恢复提示 -->
      <div v-if="sessionRestored" class="mt-2 flex items-center justify-center gap-2">
        <el-tag type="warning" size="small" effect="plain" round>
          <el-icon class="mr-1"><RefreshRight /></el-icon>
          已从上次进度恢复（刷新页面不丢失）
        </el-tag>
      </div>
    </div>

    <!-- Step indicator -->
    <div class="w-full max-w-3xl px-6 mb-6">
      <el-steps :active="activeStep" finish-status="success" align-center>
        <el-step title="描述需求" description="告诉 AI 你想做什么" />
        <el-step title="确认需求" description="确认 AI 提炼的信息" />
        <el-step title="需求澄清" description="AI 生成确认问题" />
        <el-step title="确认环境" description="查看项目运行上下文" />
        <el-step title="全链路执行" description="计划→编码→部署→测试" />
      </el-steps>
    </div>

    <!-- Step 1: Chat input -->
    <div v-if="activeStep === 0" class="w-full max-w-3xl px-6">
      <el-card shadow="always" class="!rounded-2xl">
        <div class="flex flex-col gap-4">
          <div>
            <label class="mb-1 block text-sm font-medium text-slate-700">描述你的项目需求</label>
            <el-input
              v-model="userMessage"
              type="textarea"
              :rows="5"
              resize="vertical"
              placeholder="例如：我想做一个团队协作工具，支持任务看板、文档共享、实时聊天、日程管理，需要用户权限控制……"
              @keydown.ctrl.enter="handleChat"
            />
          </div>

          <!-- Streaming output -->
          <div v-if="streamOutput || streaming" class="rounded-xl bg-gray-900 p-4">
            <div class="mb-2 flex items-center justify-between">
              <span class="text-xs font-medium text-gray-400">
                <el-icon v-if="streaming" class="is-loading mr-1 align-text-bottom"><Loading /></el-icon>
                {{ streaming ? 'AI 正在分析…' : '分析完成' }}
              </span>
              <el-button v-if="streaming" link type="danger" size="small" @click="abortStream">停止</el-button>
            </div>
            <div ref="streamBoxRef" class="stream-box max-h-60 overflow-auto font-mono text-sm leading-relaxed text-gray-100">
              <pre class="whitespace-pre-wrap break-words">{{ streamOutput || '等待输出…' }}</pre>
            </div>
          </div>

          <div class="flex items-center justify-between">
            <span class="text-xs text-slate-400">按 Ctrl+Enter 快速提交</span>
            <div class="flex gap-2">
              <el-button @click="goBack">返回</el-button>
              <el-button
                type="primary"
                size="large"
                :loading="streaming"
                :disabled="!userMessage.trim()"
                @click="handleChat"
              >
                <el-icon class="mr-1"><Promotion /></el-icon>
                提交需求
              </el-button>
            </div>
          </div>
        </div>
      </el-card>
    </div>

    <!-- Step 2: Confirm requirements -->
    <div v-if="activeStep === 1" class="w-full max-w-3xl px-6">
      <el-card shadow="always" class="!rounded-2xl">
        <template #header>
          <div class="flex items-center gap-2">
            <el-icon class="text-indigo-500"><Document /></el-icon>
            <span class="text-base font-semibold text-slate-800">需求确认</span>
            <span class="text-xs text-slate-400">请检查 AI 提炼的信息，可手动修改</span>
          </div>
        </template>

        <div class="flex flex-col gap-4">
          <!-- 项目名称选择 -->
          <div>
            <label class="mb-2 block text-sm font-medium text-slate-700">项目名称（3-8字）</label>
            <div class="flex flex-wrap gap-2 mb-2">
              <el-tag
                v-for="name in summary?.projectNameOptions || []"
                :key="name"
                :type="confirmedName === name ? 'primary' : 'info'"
                :effect="confirmedName === name ? 'dark' : 'plain'"
                class="!cursor-pointer !rounded-full !text-sm"
                @click="confirmedName = name"
              >
                {{ name }}
              </el-tag>
            </div>
            <el-input v-model="confirmedName" placeholder="选择或输入项目名称（3-8字）" clearable />
          </div>

          <!-- 英文工程名选择 -->
          <div>
            <label class="mb-2 block text-sm font-medium text-slate-700">英文工程名 / Git 仓库名</label>
            <div class="flex flex-wrap gap-2 mb-2">
              <el-tag
                v-for="rn in summary?.repoNameOptions || []"
                :key="rn"
                :type="confirmedRepoName === rn ? 'primary' : 'info'"
                :effect="confirmedRepoName === rn ? 'dark' : 'plain'"
                class="!cursor-pointer !rounded-full !text-sm"
                @click="confirmedRepoName = rn"
              >
                {{ rn }}
              </el-tag>
            </div>
            <el-input v-model="confirmedRepoName" placeholder="选择或输入英文工程名（小写字母+下划线）" clearable />
          </div>

          <div>
            <label class="mb-1 block text-sm font-medium text-slate-700">需求描述</label>
            <el-input
              v-model="confirmedDescription"
              type="textarea"
              :rows="4"
              placeholder="需求描述"
            />
          </div>

          <!-- Key features -->
          <div v-if="summary?.keyFeatures?.length">
            <label class="mb-1 block text-sm font-medium text-slate-700">关键功能</label>
            <div class="flex flex-wrap gap-2">
              <el-tag
                v-for="(feat, i) in summary.keyFeatures"
                :key="i"
                type="info"
                effect="plain"
                class="!rounded-full"
              >
                {{ feat }}
              </el-tag>
            </div>
          </div>

          <!-- Tech requirements -->
          <div v-if="summary?.techRequirements?.length">
            <label class="mb-1 block text-sm font-medium text-slate-700">技术要求</label>
            <div class="flex flex-wrap gap-2">
              <el-tag
                v-for="(tech, i) in summary.techRequirements"
                :key="i"
                type="success"
                effect="plain"
                class="!rounded-full"
              >
                {{ tech }}
              </el-tag>
            </div>
          </div>

          <!-- Target user -->
          <div v-if="summary?.userType">
            <label class="mb-1 block text-sm font-medium text-slate-700">目标用户</label>
            <el-tag type="warning" effect="plain" class="!rounded-full">{{ summary.userType }}</el-tag>
          </div>
        </div>

        <div class="mt-6 flex justify-between">
          <el-button @click="activeStep = 0">重新描述</el-button>
          <el-button type="primary" size="large" :disabled="!confirmedName.trim()" @click="handleConfirmRequirement">
            确认需求
            <el-icon class="ml-1"><ArrowRight /></el-icon>
          </el-button>
        </div>
      </el-card>
    </div>

    <!-- Step 3: Confirm context -->
    <div v-if="activeStep === 3" class="w-full max-w-3xl px-6">
      <el-card shadow="always" class="!rounded-2xl" v-loading="loadingContext">
        <template #header>
          <div class="flex items-center gap-2">
            <el-icon class="text-green-500"><Setting /></el-icon>
            <span class="text-base font-semibold text-slate-800">项目运行上下文</span>
            <span class="text-xs text-slate-400">以下资源将在创建时自动生成</span>
          </div>
        </template>

        <div v-if="contextPreview" class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <!-- Work directory -->
          <div class="context-item">
            <div class="context-item__label">
              <el-icon><FolderOpened /></el-icon>
              <span>工作目录</span>
            </div>
            <code class="context-item__value">{{ contextPreview.workDir }}</code>
          </div>

          <!-- Git repo -->
          <div class="context-item">
            <div class="context-item__label">
              <el-icon><Connection /></el-icon>
              <span>Gitee 仓库</span>
            </div>
            <code class="context-item__value">{{ contextPreview.gitRepoName }}</code>
          </div>

          <!-- Frontend port -->
          <div class="context-item">
            <div class="context-item__label">
              <el-icon><Monitor /></el-icon>
              <span>前端端口</span>
            </div>
            <code class="context-item__value">{{ contextPreview.frontendPort }}</code>
          </div>

          <!-- Backend port -->
          <div class="context-item">
            <div class="context-item__label">
              <el-icon><Cpu /></el-icon>
              <span>后端端口</span>
            </div>
            <code class="context-item__value">{{ contextPreview.backendPort }}</code>
          </div>

          <!-- Database -->
          <div class="context-item">
            <div class="context-item__label">
              <el-icon><Coin /></el-icon>
              <span>数据库</span>
            </div>
            <code class="context-item__value">{{ contextPreview.database }}</code>
          </div>

          <!-- Nginx -->
          <div class="context-item">
            <div class="context-item__label">
              <el-icon><Grid /></el-icon>
              <span>Nginx 转发</span>
            </div>
            <code class="context-item__value">{{ contextPreview.nginxDomain }}</code>
          </div>

          <!-- Server -->
          <div class="context-item">
            <div class="context-item__label">
              <el-icon><Position /></el-icon>
              <span>部署服务器</span>
            </div>
            <code class="context-item__value">{{ contextPreview.serverHost }}</code>
          </div>

          <!-- Dev language -->
          <div class="context-item">
            <div class="context-item__label">
              <el-icon><Files /></el-icon>
              <span>开发语言</span>
            </div>
            <code class="context-item__value">{{ contextPreview.devLanguage }}</code>
          </div>
        </div>

        <el-alert
          v-if="contextPreview"
          class="mt-4"
          type="info"
          :closable="false"
          show-icon
        >
          <template #title>
            确认后将创建项目资源（Gitee 仓库、端口、Nginx、数据库）并开始全链路执行。
          </template>
        </el-alert>

        <div class="mt-6 flex justify-between">
          <el-button @click="activeStep = 2">返回修改</el-button>
          <el-button
            type="success"
            size="large"
            :disabled="!contextPreview"
            @click="handleCreateProject"
          >
            <el-icon class="mr-1"><Check /></el-icon>
            确认并开始执行
          </el-button>
        </div>
      </el-card>
    </div>

    <!-- Step 2: Requirement clarification (grill-me style) -->
    <div v-if="activeStep === 2" class="w-full max-w-3xl px-6 pb-16">
      <el-card shadow="always" class="!rounded-2xl" v-loading="clarifyLoading" :element-loading-text="clarifyLoadingText">
        <template #header>
          <div class="flex items-center gap-2">
            <el-icon class="text-amber-500" :class="{ 'is-loading': clarifyLoading }">
              <Loading v-if="clarifyLoading" /><ChatLineRound v-else />
            </el-icon>
            <span class="text-base font-semibold text-slate-800">需求澄清</span>
            <el-tag v-if="clarifyAiGenerated" type="success" size="small" effect="plain">AI 分析</el-tag>
            <el-tag v-else type="info" size="small" effect="plain">规则生成</el-tag>
            <!-- 停止按钮：AI 生成中始终可见 -->
            <el-button
              v-if="clarifyLoading"
              type="danger"
              size="small"
              class="ml-auto"
              @click="abortClarify"
            >
              <el-icon class="mr-1"><Close /></el-icon>
              停止生成
            </el-button>
          </div>
        </template>

        <!-- Requirement context -->
        <div v-if="clarifyRequirement" class="mb-4 rounded-lg bg-slate-50 border border-slate-200 p-3">
          <div class="mb-1 text-xs font-medium text-slate-500">原始需求</div>
          <p class="text-sm text-slate-700 leading-relaxed">{{ clarifyRequirement }}</p>
        </div>

        <!-- Questions list -->
        <div v-if="clarifyQuestions.length > 0" class="space-y-4">
          <div
            v-for="(q, idx) in clarifyQuestions"
            :key="q.id"
            class="rounded-lg border border-slate-200 p-4 transition-all hover:border-indigo-200"
          >
            <!-- Question header -->
            <div class="flex items-start gap-2 mb-3">
              <span class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-indigo-100 text-xs font-bold text-indigo-600">
                {{ idx + 1 }}
              </span>
              <div class="flex-1">
                <div class="flex items-center gap-2 flex-wrap">
                  <span class="text-sm font-medium text-slate-800">{{ q.question }}</span>
                  <el-tag v-if="q.required" type="danger" size="small" effect="plain">必选</el-tag>
                  <el-tag size="small" effect="plain" :type="clarifyCategoryTagType(q.category)">{{ clarifyCategoryLabel(q.category) }}</el-tag>
                </div>
              </div>
            </div>
            <!-- Answer: radio options -->
            <div class="ml-8 space-y-2">
              <label
                v-for="opt in q.options"
                :key="opt"
                class="flex cursor-pointer items-start gap-2 rounded-lg border px-3 py-2 text-sm transition-all"
                :class="clarifyAnswers[q.id] === opt
                  ? 'border-indigo-400 bg-indigo-50 text-indigo-700'
                  : 'border-slate-200 hover:border-indigo-200 hover:bg-slate-50'"
              >
                <input
                  type="radio"
                  :name="`clarify-${q.id}`"
                  :value="opt"
                  v-model="clarifyAnswers[q.id]"
                  class="mt-0.5 accent-indigo-500"
                />
                <span>{{ opt }}</span>
              </label>
              <!-- Custom option: 始终显示，作为最后一个手动填写选项 -->
              <label
                class="flex cursor-pointer items-start gap-2 rounded-lg border px-3 py-2 text-sm transition-all"
                :class="clarifyAnswers[q.id] === CUSTOM_OPTION
                  ? 'border-indigo-400 bg-indigo-50 text-indigo-700'
                  : 'border-slate-200 hover:border-indigo-200 hover:bg-slate-50'"
              >
                <input
                  type="radio"
                  :name="`clarify-${q.id}`"
                  :value="CUSTOM_OPTION"
                  v-model="clarifyAnswers[q.id]"
                  class="mt-0.5 accent-indigo-500"
                />
                <div class="flex-1">
                  <span>{{ CUSTOM_LABEL }}</span>
                  <el-input
                    v-if="clarifyAnswers[q.id] === CUSTOM_OPTION"
                    v-model="clarifyCustomInputs[q.id]"
                    size="small"
                    placeholder="请输入你的具体需求..."
                    class="mt-2"
                    :rows="2"
                    type="textarea"
                    resize="vertical"
                  />
                </div>
              </label>
            </div>
          </div>
        </div>

        <!-- Empty state -->
        <div v-else-if="!clarifyLoading" class="py-8 text-center">
          <p class="text-sm text-slate-400 mb-3">暂无澄清问题</p>
          <el-button type="primary" size="small" @click="loadClarifyQuestions">
            <el-icon class="mr-1"><RefreshRight /></el-icon>
            重新生成问题
          </el-button>
        </div>

        <!-- Actions -->
        <div class="mt-6 flex justify-between">
          <el-button @click="activeStep = 1">返回修改</el-button>
          <div class="flex gap-2">
            <el-button @click="startPipelineDirectly" :loading="creating" :disabled="clarifyLoading">跳过澄清</el-button>
            <el-button
              type="primary"
              size="large"
              :loading="clarifySubmitting"
              :disabled="clarifyLoading || !clarifyQuestions.length"
              @click="handleSubmitClarify"
            >
              <el-icon class="mr-1"><Check /></el-icon>
              确认并查看环境
            </el-button>
          </div>
        </div>
      </el-card>
    </div>

    <!-- Step 4: Pipeline execution -->
    <div v-if="activeStep === 4" class="w-full max-w-3xl px-6 pb-16">
      <el-card shadow="always" class="!rounded-2xl">
        <template #header>
          <div class="flex items-center gap-2">
            <el-icon class="text-indigo-500" :class="{ 'is-loading': pipelineRunning }">
              <Loading v-if="pipelineRunning" /><CircleCheckFilled v-else />
            </el-icon>
            <span class="text-base font-semibold text-slate-800">
              {{
                pipelineRunning
                  ? '端到端全链路执行中'
                  : pipelineFinished
                    ? (pipelineHasError
                        ? (pipelineStages.some(s => s.status === 'paused')
                            ? '流水线已暂停，等待用户确认后继续'
                            : '流水线执行失败')
                        : '全链路执行完成')
                    : '准备执行'
              }}
            </span>
            <span class="text-xs text-slate-400">
              需求 → 计划 → 编码 → 部署 → 测试 → 成品
            </span>
            <!-- 停止按钮放在头部，始终可见 -->
            <el-button
              v-if="pipelineRunning"
              type="danger"
              size="small"
              class="ml-auto"
              :loading="abortingPipeline"
              @click="abortPipeline"
            >
              <el-icon v-if="!abortingPipeline" class="mr-1"><Close /></el-icon>
              {{ abortingPipeline ? '正在停止...' : '停止执行' }}
            </el-button>
          </div>
        </template>

        <!-- Pipeline stages -->
        <div class="space-y-3">
          <div
            v-for="(stage, idx) in pipelineStages"
            :key="stage.name"
            class="rounded-lg border transition-all"
            :class="{
              'bg-blue-50 border-blue-200': stage.status === 'running',
              'bg-green-50 border-green-200': stage.status === 'completed',
              'bg-red-50 border-red-200': stage.status === 'failed',
              'bg-gray-50 border-gray-200': stage.status === 'pending',
              'bg-yellow-50 border-yellow-200': stage.status === 'skipped',
              'bg-slate-100 border-slate-300': stage.status === 'aborted',
              'bg-orange-50 border-orange-200': stage.status === 'paused',
            }"
          >
            <div class="flex items-start gap-3 p-3 cursor-pointer" @click="stage.status === 'completed' && toggleStageExpand(stage.name)">
              <!-- Stage icon -->
              <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-sm font-bold"
                :class="{
                  'bg-blue-500 text-white': stage.status === 'running',
                  'bg-green-500 text-white': stage.status === 'completed',
                  'bg-red-500 text-white': stage.status === 'failed',
                  'bg-gray-300 text-gray-600': stage.status === 'pending',
                  'bg-yellow-400 text-white': stage.status === 'skipped',
                  'bg-slate-400 text-white': stage.status === 'aborted',
                  'bg-orange-500 text-white': stage.status === 'paused',
                }"
              >
                <el-icon v-if="stage.status === 'running'" class="is-loading"><Loading /></el-icon>
                <el-icon v-else-if="stage.status === 'completed'"><Check /></el-icon>
                <el-icon v-else-if="stage.status === 'failed'"><Close /></el-icon>
                <el-icon v-else-if="stage.status === 'aborted'"><Close /></el-icon>
                <el-icon v-else-if="stage.status === 'paused'"><VideoPause /></el-icon>
                <span v-else>{{ idx + 1 }}</span>
              </div>

              <!-- Stage content -->
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2">
                  <span class="text-sm font-medium text-slate-800">{{ stage.label }}</span>
                  <el-tag v-if="stage.status === 'running'" type="primary" size="small">执行中</el-tag>
                  <el-tag v-else-if="stage.status === 'completed'" type="success" size="small">完成</el-tag>
                  <el-tag v-else-if="stage.status === 'failed'" type="danger" size="small">失败</el-tag>
                  <el-tag v-else-if="stage.status === 'skipped'" type="warning" size="small">跳过</el-tag>
                  <el-tag v-else-if="stage.status === 'aborted'" type="info" size="small" effect="dark">已停止</el-tag>
                  <el-tag v-else-if="stage.status === 'paused'" type="warning" size="small" effect="dark">已暂停</el-tag>
                  <el-tag v-else type="info" size="small">等待</el-tag>
                  <!-- 展开/收起提示 -->
                  <el-icon v-if="stage.status === 'completed' && stage.summary" class="ml-auto text-slate-400 transition-transform" :class="{ 'rotate-180': expandedStages.has(stage.name) }">
                    <ArrowDown />
                  </el-icon>
                </div>
                <p v-if="stage.message" class="mt-1 text-xs text-slate-500">{{ stage.message }}</p>
              </div>
            </div>

            <!-- 展开内容：概要总结 + 中间产物 -->
            <div v-if="stage.status === 'completed' && expandedStages.has(stage.name) && (stage.summary || stage.artifacts?.length)" class="border-t border-slate-200 px-3 pb-3 pt-2">
              <!-- 概要总结 -->
              <div v-if="stage.summary" class="mb-3">
                <div class="mb-1 text-xs font-medium text-slate-600">阶段总结</div>
                <p class="text-xs text-slate-600 leading-relaxed">{{ stage.summary }}</p>
              </div>

              <!-- 中间产物 -->
              <div v-if="stage.artifacts?.length">
                <div class="mb-1 text-xs font-medium text-slate-600">中间产物</div>
                <div class="flex flex-wrap gap-2">
                  <div
                    v-for="artifact in stage.artifacts"
                    :key="artifact.name"
                    class="flex items-center gap-1 rounded-lg bg-white border border-slate-200 px-2 py-1 hover:border-indigo-300 transition-colors"
                  >
                    <el-icon class="text-indigo-400" :size="14">
                      <Document v-if="artifact.type === 'markdown' || artifact.type === 'text'" />
                      <Link v-else />
                    </el-icon>
                    <span class="text-xs text-slate-700">{{ artifact.name }}</span>
                    <el-button v-if="artifact.type !== 'url'" link type="primary" size="small" @click.stop="viewArtifact(artifact)">查看</el-button>
                    <el-button v-if="artifact.type !== 'url' && artifact.filename" link type="primary" size="small" @click.stop="downloadArtifact(artifact)">下载</el-button>
                    <el-button v-if="artifact.type === 'url'" link type="primary" size="small" @click.stop="viewArtifact(artifact)">打开</el-button>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Streaming log -->
        <div v-if="pipelineLog" class="mt-4 rounded-xl bg-gray-900 p-4">
          <div class="mb-2 flex items-center justify-between">
            <span class="text-xs font-medium text-gray-400">
              <el-icon v-if="pipelineRunning" class="is-loading mr-1 align-text-bottom"><Loading /></el-icon>
              {{ pipelineRunning ? '实时日志' : '执行日志' }}
            </span>
            <el-button v-if="pipelineRunning" link type="danger" size="small" :loading="abortingPipeline" @click="abortPipeline">
              <el-icon v-if="!abortingPipeline" class="mr-1"><Close /></el-icon>
              {{ abortingPipeline ? '正在停止...' : '停止执行' }}
            </el-button>
          </div>
          <div ref="pipelineLogRef" class="max-h-60 overflow-auto font-mono text-sm leading-relaxed text-gray-100">
            <pre class="whitespace-pre-wrap break-words">{{ pipelineLog }}</pre>
          </div>
        </div>

        <!-- Access URLs (when completed) -->
        <div v-if="pipelineFinished && pipelineResult?.accessUrls?.length" class="mt-4 space-y-2">
          <div class="text-sm font-medium text-slate-700">访问地址</div>
          <div v-for="url in pipelineResult.accessUrls" :key="url" class="flex items-center gap-2 rounded-lg bg-green-50 px-4 py-2">
            <el-icon class="text-green-500"><Link /></el-icon>
            <a :href="url" target="_blank" class="text-sm text-indigo-500 hover:underline">{{ url }}</a>
          </div>
        </div>

        <!-- Actions -->
        <div v-if="!pipelineRunning" class="mt-6 flex justify-between">
          <el-button @click="goBack">返回列表</el-button>
          <div class="flex gap-2">
            <el-button v-if="pipelineFinished && !pipelineHasError && pipelineAllCompleted" @click="goToProjectDetail">进入项目管理</el-button>
            <el-button v-if="pipelineHasError || (pipelineFinished && !pipelineAllCompleted)" type="warning" @click="retryPipeline">
              {{ pipelineAllCompleted ? '重试流水线' : '继续执行流水线（断点续跑）' }}
            </el-button>
          </div>
        </div>
      </el-card>
    </div>

    <!-- 产物查看对话框 -->
    <el-dialog v-model="artifactDialogVisible" :title="artifactDialogTitle" width="70%" top="5vh" destroy-on-close>
      <div v-loading="artifactLoading" class="max-h-[70vh] overflow-auto">
        <pre class="whitespace-pre-wrap break-words rounded-lg bg-gray-50 p-4 text-sm text-slate-700 font-mono">{{ artifactDialogContent }}</pre>
      </div>
      <template #footer>
        <el-button @click="artifactDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  Loading, Promotion, Document, ArrowRight, Setting,
  FolderOpened, Connection, Monitor, Cpu, Coin, Grid,
  Position, Files, Check, CircleCheckFilled, Close, Link, ArrowDown,
  ChatLineRound, Plus, RefreshRight
} from '@element-plus/icons-vue'
import { ProjectApi } from '@/api/project'
import request from '@/utils/request'
import { consumeSseStream, type SseCallbacks } from '@/utils/sseStream'
import {
  generateSessionId, getSessionIdFromURL, setSessionIdInURL,
  saveSession, loadSession, createEmptySession, cleanExpiredSessions,
  createDebouncedSaver, deleteSession,
  type ChatSessionState
} from '@/utils/useChatSession'

defineOptions({ name: 'ChatCreateProject' })

const router = useRouter()
const route = useRoute()

// ===== Step management =====
const activeStep = ref(0)

// ===== Session persistence =====
const sessionId = ref('')
const sessionRestored = ref(false)
const debouncedSave = createDebouncedSaver(600)

/** 收集当前所有状态为 ChatSessionState */
const collectState = (): ChatSessionState => {
  return {
    sessionId: sessionId.value,
    activeStep: activeStep.value,
    userMessage: userMessage.value,
    streamOutput: streamOutput.value,
    summary: summary.value,
    confirmedName: confirmedName.value,
    confirmedRepoName: confirmedRepoName.value,
    confirmedDescription: confirmedDescription.value,
    contextPreview: contextPreview.value,
    createdProject: createdProject.value,
    clarifyQuestions: clarifyQuestions.value,
    clarifyAnswers: clarifyAnswers.value,
    clarifyCustomInputs: clarifyCustomInputs.value,
    clarifyRequirement: clarifyRequirement.value,
    pipelineStages: pipelineStages.value,
    pipelineLog: pipelineLog.value,
    pipelineResult: pipelineResult.value,
    pipelineFinished: pipelineFinished.value,
    pipelineHasError: pipelineHasError.value,
    // SSE 运行中无法恢复，标记为 false
    pipelineRunning: false,
    createdAt: sessionCreatedAt.value,
    updatedAt: new Date().toISOString(),
  }
}

/** 立即保存当前状态到 localStorage（保存前先冲刷流式日志缓冲，避免丢帧） */
const persistSession = () => {
  if (!sessionId.value) return
  flushLogBuffers()
  saveSession(collectState())
}

/** 防抖保存（用于高频更新场景如 SSE 日志） */
const persistSessionDebounced = () => {
  if (!sessionId.value) return
  flushLogBuffers()
  debouncedSave(collectState())
}

/** 从 localStorage 恢复状态 */
const restoreFromSession = (state: ChatSessionState) => {
  activeStep.value = state.activeStep ?? 0
  userMessage.value = state.userMessage ?? ''
  streamOutput.value = truncateLog(state.streamOutput)
  summary.value = state.summary ?? null
  confirmedName.value = state.confirmedName ?? ''
  confirmedRepoName.value = state.confirmedRepoName ?? ''
  confirmedDescription.value = state.confirmedDescription ?? ''
  contextPreview.value = state.contextPreview ?? null
  createdProject.value = state.createdProject ?? null
  clarifyQuestions.value = state.clarifyQuestions ?? []
  clarifyAnswers.value = state.clarifyAnswers ?? {}
  clarifyCustomInputs.value = state.clarifyCustomInputs ?? {}
  clarifyRequirement.value = state.clarifyRequirement ?? ''
  pipelineStages.value = state.pipelineStages?.length
    ? state.pipelineStages
    : [
        { name: 'plan', label: '生成计划', status: 'pending', message: '', summary: '', artifacts: [] },
        { name: 'decompose', label: '分解任务', status: 'pending', message: '', summary: '', artifacts: [] },
        { name: 'code', label: '编码实现', status: 'pending', message: '', summary: '', artifacts: [] },
        { name: 'deploy', label: '部署上线', status: 'pending', message: '', summary: '', artifacts: [] },
        { name: 'test', label: '测试验证', status: 'pending', message: '', summary: '', artifacts: [] },
      ]
  pipelineLog.value = truncateLog(state.pipelineLog)
  pipelineResult.value = state.pipelineResult ?? null
  pipelineFinished.value = state.pipelineFinished ?? false
  pipelineHasError.value = state.pipelineHasError ?? false
  // 如果保存时 pipeline 正在运行，刷新后标记为中断
  if (state.pipelineRunning) {
    pipelineRunning.value = false
    pipelineFinished.value = true
    pipelineHasError.value = true
    if (!pipelineLog.value.includes('[页面刷新中断]')) {
      pipelineLog.value += '\n[页面刷新中断] 流水线因页面刷新中断，请点击重试\n'
    }
  } else {
    pipelineRunning.value = false
  }
  sessionCreatedAt.value = state.createdAt ?? new Date().toISOString()
}

const sessionCreatedAt = ref(new Date().toISOString())

/** 初始化会话：从 URL 读取或生成新 session ID */
const initSession = () => {
  cleanExpiredSessions()

  let sid = getSessionIdFromURL()
  if (sid) {
    // URL 中有 sid，尝试恢复
    const saved = loadSession(sid)
    if (saved) {
      sessionId.value = sid
      restoreFromSession(saved)
      sessionRestored.value = true
      ElMessage.success('已恢复上次的创建进度')
      return
    }
    // sid 存在但无数据，继续使用该 sid
    sessionId.value = sid
  } else {
    // 生成新 session ID 并写入 URL
    sid = generateSessionId()
    sessionId.value = sid
    setSessionIdInURL(sid)
  }
  // 创建空会话并保存
  const empty = createEmptySession(sid)
  saveSession(empty)
}

/** 开始新会话（清除当前状态，生成新 ID） */
const startNewSession = () => {
  // 删除旧会话
  if (sessionId.value) {
    deleteSession(sessionId.value)
  }
  // 重置所有状态
  activeStep.value = 0
  userMessage.value = ''
  streamOutput.value = ''
  summary.value = null
  confirmedName.value = ''
  confirmedRepoName.value = ''
  confirmedDescription.value = ''
  contextPreview.value = null
  createdProject.value = null
  clarifyQuestions.value = []
  clarifyAnswers.value = {}
  clarifyCustomInputs.value = {}
  clarifyRequirement.value = ''
  pipelineStages.value = [
    { name: 'plan', label: '生成计划', status: 'pending', message: '', summary: '', artifacts: [] },
    { name: 'decompose', label: '分解任务', status: 'pending', message: '', summary: '', artifacts: [] },
    { name: 'code', label: '编码实现', status: 'pending', message: '', summary: '', artifacts: [] },
    { name: 'deploy', label: '部署上线', status: 'pending', message: '', summary: '', artifacts: [] },
    { name: 'test', label: '测试验证', status: 'pending', message: '', summary: '', artifacts: [] },
  ]
  pipelineRunning.value = false
  pipelineFinished.value = false
  pipelineHasError.value = false
  pipelineLog.value = ''
  pipelineResult.value = null
  sessionRestored.value = false

  // 生成新 session ID
  const sid = generateSessionId()
  sessionId.value = sid
  setSessionIdInURL(sid)
  const empty = createEmptySession(sid)
  saveSession(empty)
}

/** 新建会话按钮处理（带确认） */
const handleNewSession = async () => {
  // 如果当前步骤大于0且有数据，提示确认
  if (activeStep.value > 0 || userMessage.value.trim()) {
    try {
      await ElMessageBox.confirm(
        '当前会话的进度将被清除，确定要开始新的创建会话吗？',
        '新建会话',
        { confirmButtonText: '确定新建', cancelButtonText: '取消', type: 'warning' }
      )
    } catch {
      return
    }
  }
  startNewSession()
}

// 页面关闭前保存
const handleBeforeUnload = () => {
  persistSession()
}

/**
 * 从 URL query 参数加载已有项目（点击项目列表中的项目名称跳转时使用）。
 * 根据项目实际进度跳转到对应步骤：
 * - 从未执行流水线 → 步骤2（需求澄清）
 * - 已进入开发阶段 → 步骤4（全链路执行），并恢复各阶段状态
 */
const loadExistingProject = async () => {
  const pid = route.query.projectId
  if (!pid) return
  const projectId = Number(pid)
  if (!projectId || Number.isNaN(projectId)) return

  try {
    const res: any = await ProjectApi.getProjectDetail(projectId)
    const project = res?.data ?? res
    if (!project || !project.id) {
      ElMessage.error('项目加载失败')
      return
    }

    // 填充项目信息
    createdProject.value = project
    confirmedName.value = project.name || ''
    confirmedRepoName.value = project.repoName || project.name || ''
    confirmedDescription.value = project.description || ''

    // 查询流水线实际进度
    let stages: any[] = []
    let accessUrls: string[] = []
    try {
      const statusRes: any = await ProjectApi.getPipelineStatus(projectId)
      const statusData = statusRes?.data ?? statusRes
      stages = Array.isArray(statusData?.stages) ? statusData.stages : []
      accessUrls = Array.isArray(statusData?.accessUrls) ? statusData.accessUrls : []
    } catch {
      // 状态查询失败时按未开始处理，走需求澄清
    }

    const pipelineStarted = stages.some(s => s.status && s.status !== 'pending')

    if (!pipelineStarted) {
      // 从未执行流水线：跳转到需求澄清步骤
      activeStep.value = 2
      await loadClarifyQuestions()
      ElMessage.success(`已加载项目「${project.name}」，请确认需求细节`)
      return
    }

    // 已进入开发阶段：跳转到全链路执行步骤，恢复阶段状态
    pipelineStages.value = stages.map((s: any) => ({
      name: String(s.name || ''),
      label: String(s.label || ''),
      status: s.status || 'pending',
      message: s.message || '',
      summary: s.summary || '',
      artifacts: Array.isArray(s.artifacts) ? s.artifacts : [],
    }))
    activeStep.value = 4
    pipelineRunning.value = false
    pipelineFinished.value = true
    pipelineHasError.value = pipelineStages.value.some(
      s => s.status === 'failed' || s.status === 'paused' || s.status === 'aborted'
    )
    if (accessUrls.length > 0) {
      pipelineResult.value = { accessUrls }
    }
    pipelineLog.value = '[状态恢复] 已根据项目实际进度恢复流水线状态。点击「重试流水线」可断点续跑。\n'
    persistSession()
    ElMessage.success(`已加载项目「${project.name}」，当前处于开发阶段`)
  } catch (e: any) {
    ElMessage.error(e?.message || '项目加载失败')
  }
}

onMounted(async () => {
  initSession()
  window.addEventListener('beforeunload', handleBeforeUnload)
  // 如果 URL 中有 projectId，始终从服务器加载最新状态（本地恢复的状态可能已过时）
  const hasProjectId = !!route.query.projectId
  if (hasProjectId) {
    await loadExistingProject()
  }
})

onUnmounted(() => {
  persistSession()
  window.removeEventListener('beforeunload', handleBeforeUnload)
})

// 监听 activeStep 变化自动保存
watch(activeStep, () => {
  persistSession()
})

// ===== Step 1: Chat =====
const userMessage = ref('')
const streamOutput = ref('')
const streaming = ref(false)
const streamBoxRef = ref<HTMLElement | null>(null)
let currentController: AbortController | null = null

interface RequirementSummary {
  projectName: string
  projectNameOptions: string[]
  repoName: string
  repoNameOptions: string[]
  summary: string
  keyFeatures: string[]
  techRequirements: string[]
  userType: string
}

const summary = ref<RequirementSummary | null>(null)

const scrollToBottom = () => {
  nextTick(() => {
    const el = streamBoxRef.value
    if (el) el.scrollTop = el.scrollHeight
  })
}

const abortStream = () => {
  currentController?.abort()
  currentController = null
  streaming.value = false
}

/** Parse SSE done payload into RequirementSummary */
const parseSummaryPayload = (payload: string): RequirementSummary => {
  const empty: RequirementSummary = {
    projectName: '', projectNameOptions: [], repoName: '', repoNameOptions: [],
    summary: '', keyFeatures: [], techRequirements: [], userType: ''
  }
  if (!payload) return empty
  try {
    const obj = JSON.parse(payload)
    const inner = obj?.data ?? obj
    return {
      projectName: String(inner?.projectName ?? ''),
      projectNameOptions: Array.isArray(inner?.projectNameOptions) ? inner.projectNameOptions.map(String) : [],
      repoName: String(inner?.repoName ?? ''),
      repoNameOptions: Array.isArray(inner?.repoNameOptions) ? inner.repoNameOptions.map(String) : [],
      summary: String(inner?.summary ?? ''),
      keyFeatures: Array.isArray(inner?.keyFeatures) ? inner.keyFeatures.map(String) : [],
      techRequirements: Array.isArray(inner?.techRequirements) ? inner.techRequirements.map(String) : [],
      userType: String(inner?.userType ?? '')
    }
  } catch {
    return { ...empty, summary: payload }
  }
}

const handleChat = () => {
  if (!userMessage.value.trim()) {
    ElMessage.warning('请先描述你的需求')
    return
  }

  // reset
  streamOutput.value = ''
  summary.value = null
  streaming.value = true

  const callbacks: SseCallbacks = {
    onOutput: (payload: string) => {
      streamAppender.append(payload)
    },
    onDone: (payload: string) => {
      streaming.value = false
      currentController = null
      const parsed = parseSummaryPayload(payload)
      summary.value = parsed

      // Pre-fill confirmed fields
      confirmedName.value = parsed.projectName || Array.from(userMessage.value).slice(0, 8).join('')
      confirmedRepoName.value = parsed.repoName || ''
      confirmedDescription.value = parsed.summary || userMessage.value

      // Advance to step 2
      activeStep.value = 1
      ElMessage.success('需求分析完成，请确认')
      persistSession()
    },
    onError: (msg: string) => {
      streaming.value = false
      currentController = null
      ElMessage.error(msg || 'AI 分析失败，请重试')
    }
  }

  currentController = ProjectApi.chatExtractRequirement(userMessage.value, callbacks)
}

// ===== Step 2: Confirm requirement =====
const confirmedName = ref('')
const confirmedRepoName = ref('')
const confirmedDescription = ref('')

const handleConfirmRequirement = async () => {
  if (!confirmedName.value.trim()) {
    ElMessage.warning('项目名称不能为空')
    return
  }
  if (!confirmedRepoName.value.trim()) {
    ElMessage.warning('英文工程名不能为空')
    return
  }

  // Move to step 2 (需求澄清) and load clarify questions
  activeStep.value = 2
  await loadClarifyQuestions()
  persistSession()
}

// ===== Step 3: Context preview =====
interface ContextPreview {
  workDir: string
  gitRepoName: string
  gitUrl: string
  frontendPort: number
  backendPort: number
  database: string
  nginxDomain: string
  serverHost: string
  devLanguage: string
}

const contextPreview = ref<ContextPreview | null>(null)
const loadingContext = ref(false)

const loadContextPreview = async () => {
  loadingContext.value = true
  contextPreview.value = null
  try {
    const res: any = await ProjectApi.previewProjectContext({
      name: confirmedName.value,
      repoName: confirmedRepoName.value,
      description: confirmedDescription.value
    })
    if (res && res.success !== false) {
      contextPreview.value = res?.data ?? res
    } else {
      ElMessage.error(res?.message || '获取上下文预览失败')
      activeStep.value = 2
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '获取上下文预览失败')
    activeStep.value = 2
  } finally {
    loadingContext.value = false
  }
}

// ===== Step 4: Create project + Pipeline =====
const creating = ref(false)
const createdProject = ref<any>(null)

// Pipeline state
interface StageArtifact {
  type: string
  name: string
  filename?: string
  apiPath: string
  previewText?: string
}

interface PipelineStage {
  name: string
  label: string
  status: 'pending' | 'running' | 'completed' | 'failed' | 'skipped' | 'aborted' | 'paused'
  message: string
  summary?: string
  artifacts?: StageArtifact[]
}

const pipelineStages = ref<PipelineStage[]>([
  { name: 'plan', label: '生成计划', status: 'pending', message: '', summary: '', artifacts: [] },
  { name: 'decompose', label: '分解任务', status: 'pending', message: '', summary: '', artifacts: [] },
  { name: 'code', label: '编码实现', status: 'pending', message: '', summary: '', artifacts: [] },
  { name: 'deploy', label: '部署上线', status: 'pending', message: '', summary: '', artifacts: [] },
  { name: 'test', label: '测试验证', status: 'pending', message: '', summary: '', artifacts: [] },
])
const pipelineRunning = ref(false)
const pipelineFinished = ref(false)
const pipelineHasError = ref(false)
/** 流水线是否全部阶段完成（completed/skipped 视为完成） */
const pipelineAllCompleted = computed(() =>
  pipelineStages.value.length > 0 &&
  pipelineStages.value.every(s => s.status === 'completed' || s.status === 'skipped')
)
const pipelineLog = ref('')
const pipelineLogRef = ref<HTMLElement | null>(null)
const pipelineResult = ref<any>(null)
let pipelineController: AbortController | null = null

// 阶段展开状态 & 产物查看
const expandedStages = ref<Set<string>>(new Set())
const artifactDialogVisible = ref(false)
const artifactDialogContent = ref('')
const artifactDialogTitle = ref('')
const artifactLoading = ref(false)

const toggleStageExpand = (stageName: string) => {
  if (expandedStages.value.has(stageName)) {
    expandedStages.value.delete(stageName)
  } else {
    expandedStages.value.add(stageName)
  }
}

const viewArtifact = async (artifact: StageArtifact) => {
  // URL 类型直接打开
  if (artifact.type === 'url' && artifact.apiPath.startsWith('http')) {
    window.open(artifact.apiPath, '_blank')
    return
  }

  artifactDialogTitle.value = artifact.name
  artifactDialogVisible.value = true
  artifactLoading.value = true
  artifactDialogContent.value = artifact.previewText || '加载中...'

  try {
    // 使用 axios request 实例（自动携带 Authorization 头）
    const res: any = await request.get(artifact.apiPath)
    const data = res?.data ?? res
    if (data?.content) {
      artifactDialogContent.value = data.content
    } else if (data?.data?.content) {
      artifactDialogContent.value = data.data.content
    } else if (typeof data === 'string') {
      artifactDialogContent.value = data
    } else if (data) {
      artifactDialogContent.value = JSON.stringify(data, null, 2)
    }
  } catch (e: any) {
    artifactDialogContent.value = `加载失败: ${e?.message || e}`
  } finally {
    artifactLoading.value = false
  }
}

const downloadArtifact = async (artifact: StageArtifact) => {
  try {
    // 使用 axios request 实例（自动携带 Authorization 头），以 blob 方式下载
    const res: any = await request.get(artifact.apiPath, {
      params: { download: '1' },
      responseType: 'blob',
      skipErrorMessage: true,
    } as any)

    // axios responseType=blob 时，拦截器返回的是 Blob 对象
    const blob = res instanceof Blob ? res : new Blob([res])
    const blobUrl = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = blobUrl
    a.download = artifact.filename || 'artifact'
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(blobUrl)
  } catch (e: any) {
    ElMessage.error('下载失败: ' + (e?.message || e))
  }
}

const scrollPipelineLog = () => {
  nextTick(() => {
    const el = pipelineLogRef.value
    if (el) el.scrollTop = el.scrollHeight
  })
}

/**
 * 有上限的流式追加器。
 *
 * 解决"任务运行中浏览器白屏/页面无响应"：CLI 的每行输出都是一个 SSE 帧，
 * 直接 `pipelineLog.value += payload` 会每次触发全量重渲染，且每 600ms 的
 * localStorage 保存会序列化整段日志——两者代价都随日志长度增长，日志涨到
 * 数 MB 后主线程被持续占满，浏览器弹出"页面无响应"。
 *
 * 策略：
 *   1. rAF 合并：同一动画帧内的多次追加只做一次 DOM 更新（限帧，防后端洪峰）；
 *   2. 上限截断：超过 max 只保留尾部，从根本上限定渲染与序列化的规模。
 */
const makeCappedAppender = (
  target: { value: string },
  max: number,
  onFlush: () => void,
) => {
  let buffer: string[] = []
  let raf: number | null = null
  const flush = () => {
    raf = null
    if (buffer.length === 0) return
    const chunk = buffer.join('')
    buffer.length = 0
    const marker = '\n…[内容过长，已截断，仅保留最近内容]…\n'
    let next = target.value + chunk
    if (next.length > max) {
      next = marker + next.slice(-(max - marker.length))
    }
    target.value = next
    onFlush()
  }
  return {
    append: (text: string) => {
      if (!text) return
      buffer.push(text)
      if (raf === null) raf = requestAnimationFrame(flush)
    },
    flush,
  }
}

/** 流水线日志上限：200KB，足够回看近段输出，同时保证渲染/localStorage 写开销可控 */
const MAX_LOG_CHARS = 200_000

/**
 * 对已存在的日志串应用上限截断（保留尾部）。
 * 恢复历史会话时使用：修复前旧的会话状态里可能存有多 MB 的 pipelineLog
 * （thinking_tokens 噪声时期写入），原样加载整段渲染会拖垮主线程导致白屏/
 * 页面无响应，期间页面上的 axios 请求（10s 超时）也被阻塞处理而报「请求超时」。
 */
const truncateLog = (value: string | undefined | null): string => {
  const s = value ?? ''
  if (s.length <= MAX_LOG_CHARS) return s
  const marker = '\n…[内容过长，已截断，仅保留最近内容]…\n'
  return marker + s.slice(-(MAX_LOG_CHARS - marker.length))
}

const logAppender = makeCappedAppender(pipelineLog, MAX_LOG_CHARS, scrollPipelineLog)
const streamAppender = makeCappedAppender(streamOutput, MAX_LOG_CHARS, scrollToBottom)

/** 冲刷所有流式追加器的待写缓冲（持久化/卸载前调用，保证日志完整） */
const flushLogBuffers = () => {
  logAppender.flush()
  streamAppender.flush()
}

const handleCreateProject = async () => {
  if (!contextPreview.value) return
  // 在此步骤创建项目（入DB），然后开始全链路执行
  if (!createdProject.value?.id) {
    const ok = await ensureProjectCreated()
    if (!ok) return
  }
  activeStep.value = 4
  ElMessage.success('项目已创建，开始全链路执行')
  persistSession()
  startPipeline()
}

// ===== Step 3: Requirement clarification (grill-me) =====
interface ClarifyQuestion {
  id: string
  question: string
  category: string
  options: string[]
  allowCustom: boolean
  required: boolean
}

const clarifyLoading = ref(false)
const clarifyLoadingText = ref('AI 正在分析你的需求，生成针对性的确认问题…')
const clarifyAiGenerated = ref(false)
const clarifySubmitting = ref(false)
const clarifyQuestions = ref<ClarifyQuestion[]>([])
/** 澄清请求的 AbortController，用于中止 AI 生成 */
let clarifyController: AbortController | null = null
/** 每个问题的选中值：选项文本 或 '__custom__' */
const clarifyAnswers = ref<Record<string, string>>({})
/** 自定义输入文本（当用户选择"其他"时） */
const clarifyCustomInputs = ref<Record<string, string>>({})
const clarifyRequirement = ref('')

/** 自定义选项标识 */
const CUSTOM_OPTION = '__custom__'
/** 自定义选项显示文本 */
const CUSTOM_LABEL = '其他（自定义输入）'

const clarifyCategoryLabel = (category: string): string => {
  const labels: Record<string, string> = {
    scope: '范围',
    tech: '技术',
    data: '数据',
    ui: '界面',
    security: '安全',
    performance: '性能',
  }
  return labels[category] || category
}

const clarifyCategoryTagType = (category: string): 'primary' | 'success' | 'warning' | 'info' | 'danger' => {
  const types: Record<string, 'primary' | 'success' | 'warning' | 'info' | 'danger'> = {
    scope: 'primary',
    tech: 'success',
    data: 'warning',
    ui: 'info',
    security: 'danger',
    performance: 'info',
  }
  return types[category] || 'info'
}

/** 获取某个问题最终答案文本（用于提交） */
const getClarifyAnswerText = (q: ClarifyQuestion): string => {
  const selected = clarifyAnswers.value[q.id]
  if (!selected) return ''
  if (selected === CUSTOM_OPTION) {
    return clarifyCustomInputs.value[q.id]?.trim() || ''
  }
  return selected
}

const loadClarifyQuestions = async () => {
  // 中止上一次未完成的请求
  clarifyController?.abort()
  clarifyController = new AbortController()

  clarifyLoading.value = true
  clarifyLoadingText.value = 'AI 正在分析你的需求，生成针对性的确认问题…'
  clarifyQuestions.value = []
  clarifyAnswers.value = {}
  clarifyCustomInputs.value = {}
  clarifyRequirement.value = ''
  clarifyAiGenerated.value = false

  try {
    const projectId = createdProject.value?.id
    let data: any

    if (projectId) {
      // 已有项目（如从URL加载的已有项目），使用项目ID接口
      const res: any = await ProjectApi.generateClarifyQuestions(projectId, clarifyController.signal)
      data = res?.data ?? res
    } else {
      // 新项目尚未入库，使用独立接口直接传入需求文本（不写DB）
      const res: any = await ProjectApi.generateClarifyQuestionsStandalone(confirmedDescription.value, clarifyController.signal)
      data = res?.data ?? res
    }

    if (data?.questions && Array.isArray(data.questions)) {
      clarifyQuestions.value = data.questions.map((q: any) => ({
        id: String(q.id || ''),
        question: String(q.question || ''),
        category: String(q.category || 'scope'),
        options: Array.isArray(q.options) ? q.options.map(String) : [],
        allowCustom: !!q.allowCustom,
        required: !!q.required,
      }))
      clarifyRequirement.value = String(data.requirement || '')
      clarifyAiGenerated.value = !!data.aiGenerated
    }
  } catch (e: any) {
    // 用户主动中止时不报错
    if (e?.name === 'CanceledError' || e?.code === 'ERR_CANCELED') {
      ElMessage.info('已停止 AI 生成')
    } else {
      ElMessage.error(e?.message || '加载澄清问题失败')
    }
  } finally {
    clarifyLoading.value = false
    clarifyController = null
  }
}

/** 中止需求澄清的 AI 生成请求 */
const abortClarify = () => {
  clarifyController?.abort()
  clarifyLoading.value = false
  clarifyController = null
}

/** 将澄清答案合并到需求描述中（本地合并，不入DB） */
const mergeClarifyAnswers = (baseDescription: string, answers: Array<{ question: string; answer: string }>): string => {
  if (answers.length === 0) return baseDescription
  let result = baseDescription
  result += '\n\n## 需求澄清记录\n'
  for (const ans of answers) {
    if (ans.answer?.trim()) {
      result += `\n**Q: ${ans.question}**\nA: ${ans.answer}\n`
    }
  }
  return result
}

/** 如果项目尚未入库，创建项目并返回 true；已入库则返回 true */
const ensureProjectCreated = async (): Promise<boolean> => {
  if (createdProject.value?.id) return true

  creating.value = true
  try {
    const res: any = await ProjectApi.chatCreateProject({
      name: confirmedName.value,
      repoName: confirmedRepoName.value,
      description: confirmedDescription.value
    })
    if (!res || res.success === false) {
      ElMessage.error(res?.message || '创建项目失败')
      return false
    }
    createdProject.value = res?.data ?? res
    return true
  } catch (e: any) {
    ElMessage.error(e?.message || '创建项目失败')
    return false
  } finally {
    creating.value = false
  }
}

const handleSubmitClarify = async () => {
  // 校验必答问题
  const missingRequired = clarifyQuestions.value.filter(
    q => q.required && !getClarifyAnswerText(q)
  )
  if (missingRequired.length > 0) {
    ElMessage.warning(`请回答必答问题: ${missingRequired[0].question}`)
    return
  }

  // 组装答案
  const answers = clarifyQuestions.value
    .filter(q => getClarifyAnswerText(q))
    .map(q => ({
      questionId: q.id,
      question: q.question,
      answer: getClarifyAnswerText(q),
    }))

  clarifySubmitting.value = true
  try {
    // 将澄清答案合并到需求描述中（本地操作，不入DB）
    const mergedDescription = mergeClarifyAnswers(confirmedDescription.value, answers)
    confirmedDescription.value = mergedDescription

    // 如果已有项目（从URL加载），提交澄清答案到DB
    if (createdProject.value?.id && answers.length > 0) {
      await ProjectApi.submitClarifyAnswers(createdProject.value.id, answers)
    }

    // 进入确认环境步骤
    activeStep.value = 3
    await loadContextPreview()
    ElMessage.success('需求已确认，请查看项目运行环境')
    persistSession()
  } catch (e: any) {
    ElMessage.error(e?.message || '提交澄清答案失败')
  } finally {
    clarifySubmitting.value = false
  }
}

const startPipelineDirectly = async () => {
  // 跳过澄清，直接进入确认环境步骤
  activeStep.value = 3
  await loadContextPreview()
  ElMessage.info('已跳过需求澄清，请查看项目运行环境')
  persistSession()
}

const startPipeline = () => {
  const projectId = createdProject.value?.id
  if (!projectId) {
    ElMessage.error('项目ID缺失，无法启动流水线')
    return
  }

  // 注意：不再把阶段状态清空为 pending。断点续跑/重试时保留已有状态，
  // 由 SSE 的 stage_update 帧替换为最新值；否则若 SSE 中途失败，
  // 会把真实状态覆盖成"全部等待"并持久化，表现为"点击继续执行后状态全没了"。
  pipelineRunning.value = true
  pipelineFinished.value = false
  pipelineHasError.value = false
  pipelineLog.value = ''
  pipelineResult.value = null

  const baseURL = import.meta.env.VITE_API_BASE_URL || '/api'
  const url = `${baseURL}/pipeline/project/${projectId}/run`

  pipelineController = consumeSseStream(url, {}, {
    onOutput: (payload: string) => {
      // 尝试解析 JSON 事件（stage_update / pipeline_start）
      try {
        const parsed = JSON.parse(payload)
        if (parsed.type === 'stage_update' || parsed.type === 'pipeline_start') {
          if (parsed.stages && Array.isArray(parsed.stages)) {
            // pipeline_start 是初始 pending 帧；若已有真实阶段状态（如断点续跑恢复的），
            // 跳过它，避免把已有状态闪回成"全部等待"
            const hasRealStatus = pipelineStages.value.some((s: any) => s.status !== 'pending')
            if (parsed.type === 'stage_update' || !hasRealStatus) {
              pipelineStages.value = parsed.stages.map((s: any) => ({
                name: s.name,
                label: s.label,
                status: s.status,
                message: s.message || '',
                summary: s.summary || '',
                artifacts: s.artifacts || [],
              }))
            }
          }
          // 阶段更新时防抖保存
          persistSessionDebounced()
          return
        }
      } catch {
        // 非 JSON，作为普通日志输出
      }
      logAppender.append(payload)
      // 日志更新时防抖保存
      persistSessionDebounced()
    },
    onDone: (payload: string) => {
      pipelineRunning.value = false
      pipelineFinished.value = true
      try {
        pipelineResult.value = JSON.parse(payload)
      } catch {
        pipelineResult.value = { raw: payload }
      }
      // 检查是否有失败或暂停的阶段
      const hasPaused = pipelineStages.value.some(s => s.status === 'paused')
      pipelineHasError.value = pipelineStages.value.some(s => s.status === 'failed') || hasPaused
      if (!pipelineHasError.value) {
        ElMessage.success('全链路执行完成！')
      } else if (hasPaused) {
        ElMessage.warning('流水线已暂停，请在处理完成后点击重试继续')
      }
      // 完成后立即保存
      persistSession()
    },
    onError: (msg: string) => {
      pipelineRunning.value = false
      pipelineFinished.value = true
      pipelineHasError.value = true
      logAppender.append(`\n[错误] ${msg}\n`)
      ElMessage.error('流水线执行出错: ' + msg)
      // 错误时立即保存
      persistSession()
    },
  })
}

const abortingPipeline = ref(false)

const abortPipeline = async () => {
  // 先尝试确认
  try {
    await ElMessageBox.confirm(
      '确定要停止全链路执行吗？所有正在运行的 Claude Code CLI 进程将被终止。',
      '停止执行',
      { confirmButtonText: '确定停止', cancelButtonText: '取消', type: 'warning' }
    )
  } catch {
    return
  }

  abortingPipeline.value = true
  // 1. 调用后端接口杀死所有 CLI 进程
  const projectId = createdProject.value?.id
  if (projectId) {
    try {
      const res: any = await ProjectApi.abortPipeline(projectId)
      const killed = res?.data?.killed ?? res?.killed ?? 0
      logAppender.append(`\n[停止] 已终止 ${killed} 个 Claude Code CLI 进程\n`)
    } catch {
      logAppender.append('\n[停止] 后端进程停止请求失败（可能已自行退出）\n')
    }
  }

  // 2. 断开前端 SSE 连接
  pipelineController?.abort()
  pipelineController = null
  pipelineRunning.value = false
  pipelineFinished.value = true
  pipelineHasError.value = true
  // 3. 更新所有正在执行的阶段为「已停止」
  pipelineStages.value = pipelineStages.value.map(s =>
    s.status === 'running'
      ? { ...s, status: 'aborted', message: '用户手动停止' }
      : s
  )
  logAppender.append('[已停止] 用户手动终止了全链路执行\n')
  persistSession()
  abortingPipeline.value = false
}

const retryPipeline = () => {
  startPipeline()
}

// ===== Navigation =====
const goBack = () => {
  persistSession()
  router.push('/projects')
}

const goToProjectDetail = () => {
  const id = createdProject.value?.id
  if (id) {
    router.push(`/projects/${id}`)
  } else {
    router.push('/projects')
  }
}
</script>

<style scoped>
.stream-box pre {
  margin: 0;
}

.context-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px 16px;
  border-radius: 8px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
}

.context-item__label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: #64748b;
  font-weight: 500;
}

.context-item__value {
  font-size: 14px;
  color: #1e293b;
  font-family: 'Menlo', 'Monaco', 'Courier New', monospace;
  word-break: break-all;
}
</style>
