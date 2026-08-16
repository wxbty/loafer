<template>
  <div class="tdd-step-cards">
    <el-empty v-if="!stepStatuses.length" :description="emptyHint" />

    <div
      v-for="step in stepStatuses"
      :key="step.stepIndex"
      class="tdd-step-card"
      :class="{
        'is-done': step.phase === 'DONE',
        'is-failed': step.phase === 'FAILED',
        'is-active': isActivePhase(step.phase)
      }"
    >
      <!-- 卡片头部 -->
      <div class="tdd-card-header">
        <div class="tdd-card-title">
          <span class="tdd-step-num">步骤 {{ (step.stepIndex ?? 0) + 1 }}</span>
          <span v-if="getStepLabel(step.stepIndex)" class="tdd-step-name">
            {{ getStepLabel(step.stepIndex) }}
          </span>
        </div>
        <div class="tdd-card-meta">
          <span v-if="(step.attempts ?? 0) > 1" class="tdd-attempts-badge">
            重试 {{ step.attempts }} 次
          </span>
          <el-tag :type="phaseTagType(step.phase)" size="small" effect="plain">
            {{ phaseLabel(step.phase) }}
          </el-tag>
        </div>
      </div>

      <!-- 三段进度条 -->
      <div class="tdd-progress-bar">
        <div
          v-for="(stage, i) in mainStages"
          :key="stage.key"
          class="tdd-progress-segment"
          :class="stageState(step, stage.key)"
        >
          <div class="tdd-segment-track">
            <div class="tdd-segment-dot">
              <span class="tdd-segment-icon">{{ stageIcon(step, stage.key) }}</span>
            </div>
            <div v-if="i < mainStages.length - 1" class="tdd-segment-line" />
          </div>
          <div class="tdd-segment-label">{{ stage.label }}</div>
        </div>
      </div>

      <!-- 当前状态消息 -->
      <div v-if="step.currentMessage" class="tdd-status-msg" :class="{ 'is-done': step.phase === 'DONE', 'is-failed': step.phase === 'FAILED' }">
        {{ step.currentMessage }}
      </div>

      <!-- 验收标准分组 -->
      <div v-if="step.assertions?.length" class="tdd-criteria-section">
        <div class="tdd-section-header">
          <span class="tdd-section-title">验收标准</span>
          <span class="tdd-pass-count" :class="allGreen(step) ? 'all-green' : 'partial'">
            {{ greenCount(step) }}/{{ step.assertions.length }} 通过
          </span>
        </div>

        <div
          v-for="criterion in criteriaGroups(step)"
          :key="criterion.id"
          class="tdd-criterion-group"
        >
          <!-- 标准头 -->
          <div
            class="tdd-criterion-header"
            @click="toggleCriterion(step.stepIndex, criterion.id)"
          >
            <div class="tdd-criterion-left">
              <span class="tdd-criterion-dot" :class="criterionStatus(criterion.assertions)">
                {{ criterionIcon(criterion.assertions) }}
              </span>
              <span class="tdd-criterion-id">{{ criterion.id }}</span>
              <span class="tdd-criterion-behavior">{{ criterion.behavior }}</span>
            </div>
            <div class="tdd-criterion-right">
              <span class="tdd-criterion-count">
                {{ criterion.assertions.filter(a => a.lastStatus === 'GREEN').length }}/{{ criterion.assertions.length }}
              </span>
              <el-icon class="tdd-chevron" :class="{ expanded: isCriterionOpen(step.stepIndex, criterion.id) }">
                <ArrowRight />
              </el-icon>
            </div>
          </div>

          <!-- 断言列表（可折叠） -->
          <div v-if="isCriterionOpen(step.stepIndex, criterion.id)" class="tdd-assertions-list">
            <div
              v-for="a in criterion.assertions"
              :key="a.id"
              class="tdd-assertion-row"
              :class="(a.lastStatus || 'PENDING').toLowerCase()"
            >
              <div class="tdd-assertion-main">
                <span class="tdd-assertion-status-dot" :class="(a.lastStatus || 'PENDING').toLowerCase()"></span>
                <span class="tdd-assertion-type-badge" :class="a.type">{{ typeLabel(a.type) }}</span>
                <span class="tdd-assertion-id">{{ a.id }}</span>
                <span class="tdd-assertion-desc">{{ a.description }}</span>
                <button
                  v-if="a.lastDetail"
                  class="tdd-detail-toggle"
                  @click="toggleDetail(step.stepIndex, a.id)"
                >
                  {{ isDetailOpen(step.stepIndex, a.id) ? '收起' : '详情' }}
                </button>
              </div>
              <pre
                v-if="isDetailOpen(step.stepIndex, a.id) && a.lastDetail"
                class="tdd-assertion-detail"
              >{{ a.lastDetail }}</pre>
            </div>
          </div>
        </div>
      </div>

      <!-- 受影响文件（折叠） -->
      <div v-if="step.testSpec?.affectedFiles?.length" class="tdd-files-section">
        <div class="tdd-files-header" @click="toggleFiles(step.stepIndex)">
          <el-icon><Document /></el-icon>
          <span>受影响文件（{{ step.testSpec.affectedFiles.length }}）</span>
          <el-icon class="tdd-chevron" :class="{ expanded: isFilesOpen(step.stepIndex) }">
            <ArrowRight />
          </el-icon>
        </div>
        <div v-if="isFilesOpen(step.stepIndex)" class="tdd-files-list">
          <code v-for="(f, i) in step.testSpec.affectedFiles" :key="i" class="tdd-file-path">
            {{ f }}
          </code>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { ArrowRight, Document } from '@element-plus/icons-vue'
import type { TddAssertion, TddPhase, TddStepStatus } from '@/api/monitoring'

const props = defineProps<{
  stepStatuses: TddStepStatus[]
  stepLabels?: any[]
  emptyHint?: string
}>()

const emptyHint = ref(props.emptyHint || 'TDD 步骤状态尚未生成（任务启动后会在此实时展示）')

function getStepLabel(index: number | undefined): string {
  if (index === undefined || !props.stepLabels) return ''
  const item = props.stepLabels[index]
  if (!item) return ''
  if (typeof item === 'string') return item
  return item.name || item.description || item.action || ''
}

const mainStages = [
  { key: 'prepare', label: '准备测试' },
  { key: 'implement', label: '实现代码' },
  { key: 'verify', label: '验收通过' }
] as const
type MainStageKey = (typeof mainStages)[number]['key']

function stageOfPhase(phase: TddPhase | undefined): MainStageKey | null {
  switch (phase) {
    case 'PREPARING_TESTS':
    case 'RED_VERIFIED': return 'prepare'
    case 'IMPLEMENTING': return 'implement'
    case 'GREEN_VERIFIED':
    case 'REGRESSION':
    case 'DONE': return 'verify'
    default: return null
  }
}

function stageState(step: TddStepStatus, key: MainStageKey): string {
  if (step.phase === 'DONE') return 'done'
  if (step.phase === 'FAILED') {
    const idx = mainStages.findIndex(s => s.key === key)
    const failedAt = mainStages.findIndex(s => s.key === inferFailedStage(step))
    if (idx < failedAt) return 'done'
    if (idx === failedAt) return 'failed'
    return 'idle'
  }
  const current = stageOfPhase(step.phase)
  if (!current) return 'idle'
  const idx = mainStages.findIndex(s => s.key === key)
  const cur = mainStages.findIndex(s => s.key === current)
  if (idx < cur) return 'done'
  if (idx === cur) return 'active'
  return 'idle'
}

function inferFailedStage(step: TddStepStatus): MainStageKey {
  if (!step.assertions?.length) return 'prepare'
  if (!step.assertions.some(a => a.redVerified)) return 'prepare'
  if (!step.assertions.some(a => a.greenVerified)) return 'implement'
  return 'verify'
}

function stageIcon(step: TddStepStatus, key: MainStageKey): string {
  const s = stageState(step, key)
  if (s === 'done') return '✓'
  if (s === 'failed') return '✗'
  if (s === 'active') return '●'
  return ''
}

function isActivePhase(phase: TddPhase | undefined): boolean {
  return !!phase && phase !== 'DONE' && phase !== 'FAILED'
}

function phaseLabel(phase: TddPhase | undefined): string {
  const map: Partial<Record<TddPhase, string>> = {
    PREPARING_TESTS: '准备测试中',
    RED_VERIFIED: '测试就绪',
    IMPLEMENTING: '编码中',
    GREEN_VERIFIED: '验证通过',
    REGRESSION: '回归检查',
    DONE: '已完成',
    FAILED: '失败'
  }
  return map[phase as TddPhase] ?? '待启动'
}

function phaseTagType(phase: TddPhase | undefined): 'info' | 'success' | 'warning' | 'danger' {
  if (phase === 'DONE' || phase === 'GREEN_VERIFIED') return 'success'
  if (phase === 'FAILED') return 'danger'
  if (phase === 'IMPLEMENTING' || phase === 'REGRESSION') return 'warning'
  return 'info'
}

function typeLabel(type: string): string {
  const map: Record<string, string> = {
    shell: 'shell',
    file_exists: 'file',
    http_status: 'http',
    git_commit: 'git'
  }
  return map[type] || type
}

// 按 criteriaId 分组断言
function criteriaGroups(step: TddStepStatus) {
  const criteriaMap = new Map<string, { id: string; behavior: string; assertions: TddAssertion[] }>()
  const criteria = step.testSpec?.acceptanceCriteria || []

  for (const c of criteria) {
    criteriaMap.set(c.id, { id: c.id, behavior: c.behavior, assertions: [] })
  }

  for (const a of (step.assertions || [])) {
    const cid = a.criteriaId || a.id.split('-')[0]
    if (!criteriaMap.has(cid)) {
      criteriaMap.set(cid, { id: cid, behavior: '', assertions: [] })
    }
    criteriaMap.get(cid)!.assertions.push(a)
  }

  return Array.from(criteriaMap.values()).filter(c => c.assertions.length > 0)
}

function criterionStatus(assertions: TddAssertion[]): string {
  if (!assertions.length) return 'pending'
  if (assertions.every(a => a.lastStatus === 'GREEN')) return 'green'
  if (assertions.some(a => a.lastStatus === 'RED')) return 'red'
  return 'pending'
}

function criterionIcon(assertions: TddAssertion[]): string {
  const s = criterionStatus(assertions)
  if (s === 'green') return '✓'
  if (s === 'red') return '✗'
  return '○'
}

function greenCount(step: TddStepStatus): number {
  return (step.assertions || []).filter(a => a.lastStatus === 'GREEN').length
}

function allGreen(step: TddStepStatus): boolean {
  const assertions = step.assertions || []
  return assertions.length > 0 && assertions.every(a => a.lastStatus === 'GREEN')
}

// 折叠状态
const openCriteria = ref<Set<string>>(new Set())
const openDetails = ref<Set<string>>(new Set())
const openFiles = ref<Set<number>>(new Set())

function criterionKey(stepIndex: number | undefined, cid: string) {
  return `${stepIndex ?? -1}#${cid}`
}

function isCriterionOpen(stepIndex: number | undefined, cid: string) {
  return openCriteria.value.has(criterionKey(stepIndex, cid))
}

function toggleCriterion(stepIndex: number | undefined, cid: string) {
  const k = criterionKey(stepIndex, cid)
  const s = new Set(openCriteria.value)
  s.has(k) ? s.delete(k) : s.add(k)
  openCriteria.value = s
}

function detailKey(stepIndex: number | undefined, aid: string) {
  return `${stepIndex ?? -1}#${aid}`
}

function isDetailOpen(stepIndex: number | undefined, aid: string) {
  return openDetails.value.has(detailKey(stepIndex, aid))
}

function toggleDetail(stepIndex: number | undefined, aid: string) {
  const k = detailKey(stepIndex, aid)
  const s = new Set(openDetails.value)
  s.has(k) ? s.delete(k) : s.add(k)
  openDetails.value = s
}

function isFilesOpen(stepIndex: number | undefined) {
  return openFiles.value.has(stepIndex ?? -1)
}

function toggleFiles(stepIndex: number | undefined) {
  const k = stepIndex ?? -1
  const s = new Set(openFiles.value)
  s.has(k) ? s.delete(k) : s.add(k)
  openFiles.value = s
}
</script>

<style scoped>
/* 整体容器 */
.tdd-step-cards {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

/* 单步卡片 */
.tdd-step-card {
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  border-left: 4px solid #dcdfe6;
  background: #fff;
  overflow: hidden;
}
.tdd-step-card.is-done { border-left-color: #67c23a; }
.tdd-step-card.is-failed { border-left-color: #f56c6c; }
.tdd-step-card.is-active { border-left-color: #409eff; }

/* 卡片头 */
.tdd-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px 8px;
  gap: 12px;
}
.tdd-card-title {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.tdd-step-num {
  font-weight: 700;
  color: #303133;
  white-space: nowrap;
  font-size: 14px;
}
.tdd-step-name {
  color: #606266;
  font-size: 13px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tdd-card-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}
.tdd-attempts-badge {
  font-size: 11px;
  color: #e6a23c;
  background: #fdf6ec;
  border: 1px solid #f5dab1;
  border-radius: 10px;
  padding: 1px 8px;
}

/* 三段进度条 */
.tdd-progress-bar {
  display: flex;
  align-items: flex-start;
  padding: 8px 16px 12px;
  background: #f8fafc;
  border-top: 1px solid #f0f2f5;
  border-bottom: 1px solid #f0f2f5;
}
.tdd-progress-segment {
  display: flex;
  flex-direction: column;
  align-items: center;
  flex: 1;
  gap: 4px;
}
.tdd-segment-track {
  display: flex;
  align-items: center;
  width: 100%;
}
.tdd-segment-dot {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  background: #ebeef5;
  border: 2px solid #dcdfe6;
  transition: all 0.2s;
}
.tdd-segment-icon {
  font-size: 13px;
  color: #c0c4cc;
  line-height: 1;
}
.tdd-segment-line {
  flex: 1;
  height: 2px;
  background: #dcdfe6;
  transition: background 0.2s;
}

/* 进度段状态 */
.tdd-progress-segment.done .tdd-segment-dot {
  background: #e1f3d8;
  border-color: #67c23a;
}
.tdd-progress-segment.done .tdd-segment-icon { color: #67c23a; }
.tdd-progress-segment.done .tdd-segment-line { background: #67c23a; }

.tdd-progress-segment.active .tdd-segment-dot {
  background: #d9ecff;
  border-color: #409eff;
  box-shadow: 0 0 0 3px rgba(64, 158, 255, 0.2);
}
.tdd-progress-segment.active .tdd-segment-icon { color: #409eff; }

.tdd-progress-segment.failed .tdd-segment-dot {
  background: #fde2e2;
  border-color: #f56c6c;
}
.tdd-progress-segment.failed .tdd-segment-icon { color: #f56c6c; }

.tdd-segment-label {
  font-size: 11px;
  color: #909399;
  text-align: center;
  white-space: nowrap;
}
.tdd-progress-segment.done .tdd-segment-label { color: #67c23a; }
.tdd-progress-segment.active .tdd-segment-label { color: #409eff; font-weight: 600; }
.tdd-progress-segment.failed .tdd-segment-label { color: #f56c6c; }

/* 状态消息 */
.tdd-status-msg {
  margin: 0 16px 12px;
  padding: 6px 12px;
  border-radius: 4px;
  font-size: 13px;
  background: #ecf5ff;
  color: #409eff;
  border-left: 3px solid #409eff;
  margin-top: 10px;
}
.tdd-status-msg.is-done {
  background: #f0f9eb;
  color: #67c23a;
  border-left-color: #67c23a;
}
.tdd-status-msg.is-failed {
  background: #fef0f0;
  color: #f56c6c;
  border-left-color: #f56c6c;
}

/* 验收标准区 */
.tdd-criteria-section {
  padding: 0 16px 12px;
}
.tdd-section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 0 6px;
}
.tdd-section-title {
  font-size: 13px;
  font-weight: 600;
  color: #303133;
}
.tdd-pass-count {
  font-size: 12px;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: 10px;
}
.tdd-pass-count.all-green { background: #e1f3d8; color: #529b2e; }
.tdd-pass-count.partial { background: #fdf6ec; color: #b88230; }

/* 验收标准分组 */
.tdd-criterion-group {
  border: 1px solid #ebeef5;
  border-radius: 6px;
  margin-bottom: 6px;
  overflow: hidden;
}
.tdd-criterion-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  cursor: pointer;
  background: #fafafa;
  transition: background 0.15s;
  gap: 8px;
  user-select: none;
}
.tdd-criterion-header:hover { background: #f0f2f5; }

.tdd-criterion-left {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: 1;
}
.tdd-criterion-dot {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 11px;
  font-weight: 700;
  flex-shrink: 0;
}
.tdd-criterion-dot.green { background: #e1f3d8; color: #529b2e; }
.tdd-criterion-dot.red { background: #fde2e2; color: #c45656; }
.tdd-criterion-dot.pending { background: #f4f4f5; color: #909399; }

.tdd-criterion-id {
  font-family: ui-monospace, monospace;
  font-size: 12px;
  font-weight: 600;
  color: #606266;
  white-space: nowrap;
}
.tdd-criterion-behavior {
  font-size: 13px;
  color: #303133;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tdd-criterion-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}
.tdd-criterion-count {
  font-size: 12px;
  color: #909399;
}
.tdd-chevron {
  font-size: 12px;
  color: #c0c4cc;
  transition: transform 0.2s;
}
.tdd-chevron.expanded { transform: rotate(90deg); }

/* 断言列表 */
.tdd-assertions-list {
  border-top: 1px solid #ebeef5;
}
.tdd-assertion-row {
  padding: 7px 12px 7px 32px;
  border-bottom: 1px solid #f5f7fa;
  font-size: 13px;
}
.tdd-assertion-row:last-child { border-bottom: none; }
.tdd-assertion-row.green { background: #fafff8; }
.tdd-assertion-row.red { background: #fff8f8; }
.tdd-assertion-row.pending { background: #fafafa; }

.tdd-assertion-main {
  display: flex;
  align-items: center;
  gap: 7px;
  flex-wrap: wrap;
}
.tdd-assertion-status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}
.tdd-assertion-status-dot.green { background: #67c23a; }
.tdd-assertion-status-dot.red { background: #f56c6c; }
.tdd-assertion-status-dot.pending { background: #c0c4cc; }

.tdd-assertion-type-badge {
  font-size: 10px;
  font-family: ui-monospace, monospace;
  padding: 1px 5px;
  border-radius: 3px;
  background: #f0f2f5;
  color: #606266;
  white-space: nowrap;
  flex-shrink: 0;
}
.tdd-assertion-type-badge.shell { background: #f0f7ff; color: #2b6cb0; }
.tdd-assertion-type-badge.file { background: #f0fff4; color: #276749; }
.tdd-assertion-type-badge.http { background: #fffbeb; color: #92400e; }
.tdd-assertion-type-badge.git { background: #faf5ff; color: #553c9a; }

.tdd-assertion-id {
  font-family: ui-monospace, monospace;
  font-size: 11px;
  color: #909399;
  white-space: nowrap;
  flex-shrink: 0;
}
.tdd-assertion-desc {
  color: #303133;
  flex: 1;
  min-width: 120px;
}
.tdd-detail-toggle {
  flex-shrink: 0;
  font-size: 11px;
  color: #909399;
  background: none;
  border: 1px solid #dcdfe6;
  border-radius: 3px;
  padding: 1px 7px;
  cursor: pointer;
  transition: all 0.15s;
}
.tdd-detail-toggle:hover {
  color: #409eff;
  border-color: #409eff;
}
.tdd-assertion-detail {
  margin: 6px 0 2px;
  padding: 8px 10px;
  background: #1e1e2e;
  color: #cdd6f4;
  font-size: 12px;
  line-height: 1.5;
  border-radius: 4px;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 200px;
  overflow-y: auto;
}

/* 受影响文件 */
.tdd-files-section {
  border-top: 1px solid #f0f2f5;
  margin: 0 16px;
  padding: 8px 0 12px;
}
.tdd-files-header {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: #909399;
  cursor: pointer;
  padding: 2px 0;
  user-select: none;
}
.tdd-files-header:hover { color: #606266; }
.tdd-files-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-top: 6px;
}
.tdd-file-path {
  font-family: ui-monospace, monospace;
  font-size: 11px;
  background: #f8fafc;
  border: 1px solid #ebeef5;
  border-radius: 4px;
  padding: 3px 8px;
  color: #606266;
  word-break: break-all;
}
</style>
