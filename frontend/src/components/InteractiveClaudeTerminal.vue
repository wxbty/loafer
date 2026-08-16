<template>
  <el-dialog
    v-model="dialogVisible"
    :title="`Claude Code CLI - ${projectName}`"
    width="90vw"
    :fullscreen="isFullscreen"
    :before-close="handleClose"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    class="claude-terminal-dialog"
  >
    <template #header>
      <div class="dialog-header">
        <div class="header-left">
          <el-icon><Monitor /></el-icon>
          <span>Claude Code CLI</span>
          <el-tag v-if="connected" size="small" type="success" class="ml-2">已连接</el-tag>
          <el-tag v-else size="small" type="info" class="ml-2">未连接</el-tag>
        </div>
        <div class="header-right">
          <el-button size="small" @click="triggerFileUpload">
            <el-icon><Picture /></el-icon>
            上传图片
          </el-button>
          <el-button size="small" @click="toggleFullscreen">
            <el-icon><FullScreen v-if="!isFullscreen" /><Close v-else /></el-icon>
            {{ isFullscreen ? '退出全屏' : '全屏' }}
          </el-button>
          <el-button size="small" @click="clearTerminal">
            <el-icon><Delete /></el-icon>
            清屏
          </el-button>
          <el-button size="small" type="danger" @click="handleClose">
            <el-icon><Close /></el-icon>
            关闭
          </el-button>
        </div>
      </div>
    </template>

    <!-- 会话选择器 -->
    <div v-if="showSessionSelector" class="session-selector">
      <div class="session-selector-content">
        <el-icon><Monitor /></el-icon>
        <h3>选择要连接的会话</h3>
        <p class="hint">该项目已有 {{ existingSessions.length }} 个活跃会话</p>

        <div class="session-list">
          <div
            v-for="session in existingSessions"
            :key="session.sessionId"
            class="session-item"
            :class="{ active: selectedSessionId === session.sessionId }"
            @click="selectedSessionId = session.sessionId"
          >
            <div class="session-info">
              <div class="session-id">
                <el-tag size="small" type="success">{{ session.sessionId }}</el-tag>
                <el-tag v-if="session.claudeSessionUuid" size="small" type="warning" class="ml-2">可恢复</el-tag>
              </div>
              <div class="session-meta">
                <span>创建于: {{ formatTime(session.createdAt) }}</span>
                <span>活跃于: {{ formatTime(session.lastActiveAt) }}</span>
              </div>
            </div>
            <el-icon v-if="selectedSessionId === session.sessionId"><Check /></el-icon>
          </div>
        </div>

        <div class="session-actions">
          <el-button @click="showSessionSelector = false">取消</el-button>
          <el-button type="primary" :disabled="!selectedSessionId" @click="connectToExistingSession(selectedSessionId)">
            连接
          </el-button>
        </div>

        <div class="session-create-new">
          <el-divider>或</el-divider>
          <el-button type="success" @click="createNewSession">
            <el-icon><Plus /></el-icon>
            创建新会话
          </el-button>
        </div>
      </div>
    </div>

    <!-- 已上传图片预览区 -->
    <div v-if="uploadedImages.length > 0" class="image-preview-bar">
      <div
        v-for="img in uploadedImages"
        :key="img.path"
        class="image-preview-item"
      >
        <img :src="img.url" class="image-thumb" @click="openImagePreview(img)" />
        <div class="image-delete-btn" @click.stop="removeImage(img.path)">
          <el-icon><Close /></el-icon>
        </div>
      </div>
    </div>

    <div
      class="terminal-wrapper"
      :class="{ 'drag-over': isDragging }"
      @click="focusTerminal"
      @dragover.prevent="handleDragOver"
      @dragleave.prevent="handleDragLeave"
      @drop.prevent="handleDrop"
    >
      <div v-if="connecting" class="terminal-overlay">
        <el-icon class="is-loading"><Loading /></el-icon>
        <span>正在连接...</span>
      </div>
      <div v-if="error" class="terminal-overlay error">
        <el-icon><WarningFilled /></el-icon>
        <span>{{ error }}</span>
        <el-button type="primary" size="small" @click="connect">重试</el-button>
      </div>
      <!-- 正在自动重连提示 -->
      <div v-if="reconnecting" class="terminal-overlay reconnecting">
        <el-icon class="is-loading"><Loading /></el-icon>
        <span>正在尝试重新连接...</span>
        <span class="reconnect-hint">({{ reconnectAttempts }}/{{ MAX_RECONNECT_ATTEMPTS }})</span>
      </div>
      <!-- 拖拽上传提示 -->
      <div v-if="isDragging" class="terminal-overlay drag-overlay">
        <el-icon><Picture /></el-icon>
        <span>释放以上传图片</span>
      </div>
      <div ref="terminalContainer" class="terminal-container"></div>
      <input
        ref="fileInputRef"
        type="file"
        accept="image/*"
        style="display: none"
        @change="handleFileSelect"
      />
    </div>

    <template #footer>
      <div class="dialog-footer">
        <span v-if="workDir" class="work-dir">工作目录: {{ workDir }}</span>
        <div class="hints">
          <el-tag size="small" type="info">Ctrl+C 中断</el-tag>
          <el-tag size="small" type="info">截图后 Ctrl+V 粘贴</el-tag>
        </div>
      </div>
    </template>

    <!-- 图片放大预览遮罩层 -->
    <div
      v-show="previewVisible"
      class="image-preview-overlay"
      @click="previewVisible = false"
    >
      <img :src="previewImageUrl" class="image-preview-full" />
      <div class="image-preview-close" @click="previewVisible = false">
        <el-icon><Close /></el-icon>
      </div>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch, onBeforeUnmount, onMounted, nextTick } from 'vue'
import { Terminal } from 'xterm'
import { FitAddon } from 'xterm-addon-fit'
import 'xterm/css/xterm.css'
import { ElMessage } from 'element-plus'
import { ClaudeApi } from '@/api/claude'
import { ProjectApi } from '@/api/project'
import { MouseTrackingFilter, isMouseEventSequence, isBracketedPasteOnly } from '@/utils/xtermFilter'

interface Props {
  modelValue: boolean
  projectId: string | number
  projectName: string
  workDir?: string
  resumeSessionUuid?: string
}

interface Emits {
  (e: 'update:modelValue', value: boolean): void
  (e: 'connected', sessionId: string): void
  (e: 'disconnected', sessionId: string): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const dialogVisible = ref(props.modelValue)
const isFullscreen = ref(false)
const connecting = ref(false)
const connected = ref(false)
const error = ref('')
const sessionId = ref('')
const reconnecting = ref(false)  // 正在自动重连

// Claude Code 自己的 session UUID（用于 --resume 恢复）
const lastClaudeSessionUuid = ref('')

// 会话选择器相关
const existingSessions = ref<any[]>([])
const showSessionSelector = ref(false)
const selectedSessionId = ref('')

const terminalContainer = ref<HTMLElement | null>(null)
const fileInputRef = ref<HTMLInputElement | null>(null)
let terminal: Terminal | null = null
let fitAddon: FitAddon | null = null
let websocket: WebSocket | null = null
let outputBuffer: string[] = []
let reconnectAttempts = 0
const MAX_RECONNECT_ATTEMPTS = 5

// 拖拽上传相关
const isDragging = ref(false)
let dragCounter = 0

// 已上传图片列表（截图粘贴/上传后显示预览）
interface UploadedImage {
  path: string
  url: string
  name: string
}
const uploadedImages = ref<UploadedImage[]>([])

// 图片放大预览
const previewVisible = ref(false)
const previewImageUrl = ref('')
const openImagePreview = (img: UploadedImage) => {
  previewImageUrl.value = img.url
  previewVisible.value = true
}

const removeImage = (path: string) => {
  uploadedImages.value = uploadedImages.value.filter(img => img.path !== path)
}

// 粘贴缓冲和分批发送
let pasteBuffer = ''
let pasteTimer: ReturnType<typeof setTimeout> | null = null
const PASTE_DEBOUNCE_MS = 50 // 粘贴去抖延迟
const PASTE_BATCH_SIZE = 1000 // 每批发送的字符数
const PASTE_BATCH_DELAY = 100 // 批次间隔

// 静默期标志：在连接初始化期间忽略输入，避免发送终端控制序列
let ignoreInput = true
const IGNORE_INPUT_DELAY = 200 // 静默期时长（毫秒）

// 心跳机制
let heartbeatTimer: ReturnType<typeof setInterval> | null = null
const HEARTBEAT_INTERVAL = 30000 // 30秒发送一次心跳

// 终端大小变更相关
let resizeTimeout: ReturnType<typeof setTimeout> | null = null
let resizeObserver: ResizeObserver | null = null

// 全局 paste 事件处理器（捕获阶段截获截图粘贴）
let globalPasteHandler: ((e: ClipboardEvent) => void) | null = null
// xterm.js textarea 上的 paste 处理器（右键粘贴直接作用在 textarea 上）
let textareaPasteHandler: ((e: ClipboardEvent) => void) | null = null

// 鼠标跟踪过滤（跨包缓冲）
const outputFilter = new MouseTrackingFilter()

watch(() => props.modelValue, (val) => {
  dialogVisible.value = val
  if (val) {
    nextTick(() => init())
  } else {
    // 对话框关闭时，清空终端内容和输出缓冲区
    if (terminal) {
      terminal.clear()
    }
    outputBuffer = []
  }
})

watch(dialogVisible, (val) => {
  emit('update:modelValue', val)
})

const init = async () => {
  // 进入静默期，忽略终端初始化期间的控制序列
  ignoreInput = true
  pasteBuffer = ''
  if (pasteTimer) {
    clearTimeout(pasteTimer)
    pasteTimer = null
  }
  connecting.value = true
  error.value = ''

  // 全新打开终端时清空旧 sessionId，避免误连其他会话
  // 只有断线恢复流程会保留并使用旧 sessionId
  sessionId.value = ''
  reconnectAttempts = 0
  reconnecting.value = false

  try {
    // 1. 创建终端
    if (!terminal) {
      terminal = new Terminal({
        cursorBlink: true,
        cursorStyle: 'block',
        fontSize: 14,
        fontFamily: '"Cascadia Code", "Fira Code", "JetBrains Mono", "Source Code Pro", "Noto Sans Mono", Consolas, "Courier New", monospace',
        scrollback: 10000,
        convertEol: true,
        rightClickSelectsWord: true,
        allowProposedApi: true, // 启用提议的 API
        theme: {
          background: '#1a1a2e',
          foreground: '#eaeaea',
          cursor: '#eaeaea',
          selectionBackground: 'rgba(120, 150, 200, 0.55)'
        }
      })
      outputFilter.reset()

      fitAddon = new FitAddon()
      terminal.loadAddon(fitAddon)

      if (terminalContainer.value) {
        terminal.open(terminalContainer.value)

        // 直接禁用 xterm.js 内部的鼠标跟踪模式
        // @ts-ignore - 访问内部 API
        const core = terminal._core
        if (core && core.coreMouseService) {
          // @ts-ignore
          // @ts-ignore
          core.coreMouseService.activeProtocol = 'NONE'
          // @ts-ignore
        }

        // 重新启用 SelectionService（鼠标模式启用时会被禁用）
        // @ts-ignore
        if (core && core._instantiationService && core._instantiationService._services) {
          // @ts-ignore
          const servicesMap = core._instantiationService._services._entries
          if (servicesMap) {
            for (const [, value] of servicesMap) {
              // @ts-ignore
              if (value && typeof value.handleMouseDown === 'function') {
                // @ts-ignore
                // @ts-ignore
                if (!value._enabled) {
                  // @ts-ignore
                  value.enable()
                }
              }
            }
          }
        }

        // 等待 DOM 更新后再适配终端大小
        await nextTick()
        setTimeout(() => {
          fitAddon?.fit()
          terminal?.focus()
        }, 150)
      } else {
        console.error('[Terminal] terminalContainer.value is null!')
      }

      // 监听输入 - 添加缓冲和分批发送以避免大量粘贴导致断连
      terminal.onData((data) => {
        // 详细日志：显示数据的十六进制表示
        const dataHex = Array.from(data).map(c => {
          const code = c.charCodeAt(0)
          if (code === 0x1b) return 'ESC'
          if (code >= 32 && code < 127) return c
          return `0x${code.toString(16).padStart(2, '0')}`
        }).join(' ')

        // 忽略鼠标事件序列（当鼠标跟踪模式被意外启用时，拖拽选择会触发这些序列）
        if (isMouseEventSequence(data)) {
          return
        }

        // 忽略 bracketed paste 标记序列（右键粘贴图片时 xterm.js 可能只发送 200~/201~ 空标记）
        if (isBracketedPasteOnly(data)) {
          return
        }

        // 在静默期忽略输入（避免发送终端控制序列）
        if (ignoreInput) {
          return
        }
        // 检测到回车且有待处理图片时，先注入图片读取命令
        if (data.includes('\r') && uploadedImages.value.length > 0 && websocket && websocket.readyState === WebSocket.OPEN) {
          const imageCommands = uploadedImages.value.map(img =>
            `请使用 Read 工具读取这张图片：${img.path}`
          ).join('\n') + '\n'
          websocket.send(JSON.stringify({ type: 'INPUT', data: imageCommands }))
          uploadedImages.value = []
        }
        // 将数据添加到粘贴缓冲区
        pasteBuffer += data

        // 清除之前的定时器
        if (pasteTimer) {
          clearTimeout(pasteTimer)
        }

        // 设置新的定时器，延迟后发送缓冲区内容
        pasteTimer = setTimeout(() => {
          sendPasteBuffer()
        }, PASTE_DEBOUNCE_MS)
      })

      // 注册全局 paste 事件（捕获阶段），用于截获截图粘贴
      // xterm.js 内部会在 textarea 上处理 paste，事件可能不冒泡到 terminal.element，
      // 因此在 document 上捕获更可靠
      globalPasteHandler = async (e: ClipboardEvent) => {
        // 只有焦点在终端内才处理
        if (!terminal?.element?.contains(document.activeElement)) {
          return
        }
        const items = e.clipboardData?.items
        const types = e.clipboardData?.types
        if (items) {
          for (const item of items) {
            if (item.type.startsWith('image/')) {
              e.preventDefault()
              e.stopImmediatePropagation()
              const file = item.getAsFile()
              if (file) {
                await handleImageFile(file)
              }
              return
            }
          }
        }
        // 检查 files 属性（某些右键粘贴场景可能只在这里有图片）
        const files = e.clipboardData?.files
        if (files && files.length > 0) {
          for (const file of files) {
            if (file.type.startsWith('image/')) {
              e.preventDefault()
              e.stopImmediatePropagation()
              await handleImageFile(file)
              return
            }
          }
        }
      }
      document.addEventListener('paste', globalPasteHandler, true)

      // 在 xterm.js 的 textarea 上用 capture 阶段绑定 paste 事件
      // 右键粘贴时事件直接发生在 textarea 上，xterm.js 内部也绑了 paste 处理器
      // 用 capture 确保我们的处理器在 xterm.js 之前执行，拿到图片数据后
      // stopImmediatePropagation 阻止 xterm.js 继续处理（避免发送空 bracketed paste 标记）
      textareaPasteHandler = async (e: ClipboardEvent) => {
        const items = e.clipboardData?.items
        const types = e.clipboardData?.types
        if (!items) {
          return
        }
        for (const item of items) {
          if (item.type.startsWith('image/')) {
            e.preventDefault()
            e.stopImmediatePropagation()
            const file = item.getAsFile()
            if (file) {
              await handleImageFile(file)
            }
            return
          }
        }
        // 检查 files 属性（某些右键粘贴场景可能只在这里有图片）
        const files = e.clipboardData?.files
        if (files && files.length > 0) {
          for (const file of files) {
            if (file.type.startsWith('image/')) {
              e.preventDefault()
              e.stopImmediatePropagation()
              await handleImageFile(file)
              return
            }
          }
        }
      }
      const ta = terminal?.textarea
      if (ta && textareaPasteHandler) {
        ta.addEventListener('paste', textareaPasteHandler, true)
      }

      // 监听窗口大小变化
      window.addEventListener('resize', handleResize)

      // 使用 ResizeObserver 监听终端容器大小变化
      if (terminalContainer.value) {
        resizeObserver = new ResizeObserver(() => {
          if (resizeTimeout) {
            clearTimeout(resizeTimeout)
          }
          // 防抖处理
          resizeTimeout = setTimeout(() => {
            handleResize()
          }, 100)
        })
        resizeObserver.observe(terminalContainer.value)
      }
    }

    // 2. 如果有指定的恢复会话 UUID，直接恢复；否则检查项目已有会话
    if (props.resumeSessionUuid) {
      await directResume(props.resumeSessionUuid)
    } else {
      await checkExistingSessions()
    }

  } catch (err: any) {
    connecting.value = false
    error.value = err.message || '连接失败'
    console.error('Terminal init error:', err)
  }
}

const connectWebSocket = () => {
  return new Promise<void>((resolve, reject) => {
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${wsProtocol}//${window.location.host}/ws/terminal/${sessionId.value}`

    websocket = new WebSocket(wsUrl)

    websocket.onopen = () => {
      // 清空可能存在的旧缓冲数据（避免发送终端初始化时的控制序列）
      pasteBuffer = ''
      if (pasteTimer) {
        clearTimeout(pasteTimer)
        pasteTimer = null
      }
      // 先 fit 再发送尺寸，确保后端 PTY 拿到的是真实列数
      fitAddon?.fit()
      // 发送初始终端大小
      sendTerminalSize()
      // 启动心跳
      startHeartbeat()
      // 确保终端已经准备好
      setTimeout(() => {
        fitAddon?.fit()
        sendTerminalSize()
        terminal?.focus()

        // 再次强制禁用 xterm.js 内部的鼠标跟踪模式
        if (terminal) {
          // @ts-ignore
          const core = terminal._core
          if (core && core.coreMouseService) {
            // @ts-ignore
            core.coreMouseService.activeProtocol = 'NONE'
            // @ts-ignore
          }
          // 重新启用 SelectionService
          // @ts-ignore
          if (core && core._instantiationService && core._instantiationService._services) {
            // @ts-ignore
            const servicesMap = core._instantiationService._services._entries
            if (servicesMap) {
              for (const [, value] of servicesMap) {
                // @ts-ignore
                if (value && typeof value.handleMouseDown === 'function' && !value._enabled) {
                  // @ts-ignore
                  value.enable()
                }
              }
            }
          }
        }

        // 清空 fit/focus 可能触发的控制序列
        pasteBuffer = ''
        if (pasteTimer) {
          clearTimeout(pasteTimer)
          pasteTimer = null
        }
        // 静默期结束，开始接受用户输入
        ignoreInput = false
      }, 50)
      resolve()
    }

    websocket.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data)
        if (msg.type === 'OUTPUT' && msg.data !== undefined) {
          if (terminal) {
            // 跨包缓冲过滤鼠标跟踪 SET 序列，避免被切分后漏过
            terminal.write(outputFilter.feed(msg.data))

            // 每次输出后检查并强制禁用鼠标跟踪模式
            // @ts-ignore
            const core = terminal._core
            if (core && core.coreMouseService && core.coreMouseService.activeProtocol !== 'NONE') {
              // @ts-ignore
              // @ts-ignore
              core.coreMouseService.activeProtocol = 'NONE'
              // 重新启用 SelectionService
              // @ts-ignore
              if (core._instantiationService && core._instantiationService._services) {
                // @ts-ignore
                const servicesMap = core._instantiationService._services._entries
                if (servicesMap) {
                  for (const [, value] of servicesMap) {
                    // @ts-ignore
                    if (value && typeof value.handleMouseDown === 'function' && !value._enabled) {
                      // @ts-ignore
                      value.enable()
                    }
                  }
                }
              }
            }
          }
          outputBuffer.push(msg.data)
          // 限制缓冲区大小
          if (outputBuffer.length > 10000) {
            outputBuffer = outputBuffer.slice(-5000)
          }
        } else if (msg.type === 'ERROR') {
          console.error('WebSocket error message:', msg.data)
          // 检查是否包含 Claude session UUID（进程已退出时可恢复）
          const uuidMatch = msg.data?.match(/claudeSessionUuid:([a-f0-9-]{36})/)
          if (uuidMatch) {
            lastClaudeSessionUuid.value = uuidMatch[1]
          }
          if (terminal) {
            terminal.write(`\x1b[31m错误: ${msg.data}\x1b[0m\r\n`)
          }
        } else if (msg.type === 'DISCONNECTED') {
          if (terminal) {
            terminal.write('\r\n\x1b[33m会话已断开\x1b[0m\r\n')
          }
          connected.value = false
          stopHeartbeat()
          // 触发断线处理，显示重连选项
          handleDisconnect()
        }
      } catch (e) {
        // 非 JSON 消息，直接输出（同样要过滤）
        if (terminal) {
          terminal.write(outputFilter.feed(event.data))
        }
      }
    }

    websocket.onerror = (err) => {
      console.error('[Terminal] WebSocket onerror:', err)
      stopHeartbeat()
      reject(new Error('WebSocket 连接失败'))
    }

    websocket.onclose = (event) => {
      connected.value = false
      stopHeartbeat()
      if (sessionId.value) {
        emit('disconnected', sessionId.value)
      }

      // 用户主动关闭（code 1000）不显示恢复选择器
      if (event.code === 1000) {
        return
      }

      // 非正常关闭时，调用统一的断线处理逻辑
      handleDisconnect()
    }

    // 连接超时
    setTimeout(() => {
      if (websocket?.readyState !== WebSocket.OPEN) {
        websocket?.close()
        reject(new Error('连接超时'))
      }
    }, 15000)
  })
}

/**
 * 发送终端大小到后端
 */
const sendTerminalSize = () => {
  if (!websocket || websocket.readyState !== WebSocket.OPEN || !terminal) {
    return
  }

  const cols = terminal.cols
  const rows = terminal.rows

  websocket.send(JSON.stringify({
    type: 'RESIZE',
    cols,
    rows
  }))
}

/**
 * 启动心跳机制
 */
const startHeartbeat = () => {
  stopHeartbeat() // 先清除现有定时器
  heartbeatTimer = setInterval(() => {
    if (websocket && websocket.readyState === WebSocket.OPEN) {
      websocket.send(JSON.stringify({ type: 'HEARTBEAT', data: 'ping' }))
    }
  }, HEARTBEAT_INTERVAL)
}

/**
 * 停止心跳机制
 */
const stopHeartbeat = () => {
  if (heartbeatTimer) {
    clearInterval(heartbeatTimer)
    heartbeatTimer = null
  }
}

/**
 * 统一处理断线逻辑 - 优先尝试 resume 恢复，其次创建新会话
 */
const handleDisconnect = async () => {
  // 不在对话框显示时，不做处理
  if (!dialogVisible.value) {
    return
  }

  // 尝试自动重连
  if (reconnectAttempts < MAX_RECONNECT_ATTEMPTS && sessionId.value) {
    reconnectAttempts++
    reconnecting.value = true

    // 延迟后尝试重连（递增延迟）
    setTimeout(async () => {
      if (!dialogVisible.value) {
        reconnecting.value = false
        return
      }

      try {
        // 检查原进程是否还活着
        const statusRes: any = await ClaudeApi.getSession(sessionId.value)
        if (statusRes?.data?.isProcessAlive) {
          // 进程还活着，尝试重连 WebSocket
          await connectWebSocket()
          connected.value = true
          reconnectAttempts = 0
          reconnecting.value = false
          terminal.writeln('\x1b[32m✓ 已重新连接到会话\x1b[0m')
          return
        }

        // 进程已死，尝试用 claudeSessionUuid resume 恢复
        const uuid = lastClaudeSessionUuid.value || statusRes?.data?.claudeSessionUuid
        if (uuid) {
          try {
            reconnecting.value = false
            await directResume(uuid)
            reconnectAttempts = 0
            return
          } catch (resumeErr: any) {
            console.warn('Resume failed, will create new session:', resumeErr)
          }
        }
      } catch (e) {
      }
      // 进程已死且无法 resume，创建新会话
      reconnecting.value = false
      await createNewSession()
    }, 2000 * reconnectAttempts)
  } else if (sessionId.value) {
    // 重连次数用完，尝试最后一次 resume
    if (lastClaudeSessionUuid.value) {
      try {
        await directResume(lastClaudeSessionUuid.value)
        reconnectAttempts = 0
        return
      } catch (e) {
        console.warn('Final resume attempt failed:', e)
      }
    }
    await createNewSession()
  }
}

const connect = async () => {
  await init()
}

const checkExistingSessions = async () => {
  // 新开终端始终创建新会话，避免误连项目中其他用户的活跃会话
  // 会话恢复只在 WebSocket 断线/页面刷新等场景通过 handleDisconnect 触发
  await createNewSession()
}

/**
 * 通过 prop 传入的 UUID 直接恢复会话（从项目详情页"会话恢复"按钮触发）
 */
const directResume = async (uuid: string) => {
  connecting.value = true
  try {
    const res: any = await ClaudeApi.resumeSession(String(props.projectId), uuid)
    if (!res?.data?.sessionId) {
      throw new Error(res?.message || '恢复会话失败')
    }

    sessionId.value = res.data.sessionId
    lastClaudeSessionUuid.value = uuid

    await connectWebSocket()

    connecting.value = false
    connected.value = true
    reconnectAttempts = 0
    emit('connected', sessionId.value)

    terminal.writeln('\x1b[32m✓ 会话已恢复 (Claude UUID: ' + uuid.substring(0, 8) + '...)\x1b[0m')
    // 注意：fit/focus 已在 connectWebSocket 的 onopen 中处理
  } catch (err: any) {
    connecting.value = false
    error.value = '恢复失败: ' + (err.message || '未知错误')
    console.error('Direct resume error:', err)
  }
}

const createNewSession = async () => {
  try {
    // 创建后端会话
    const res: any = await ClaudeApi.createSession(String(props.projectId))
    if (!res?.data?.sessionId) {
      throw new Error(res?.message || '创建会话失败')
    }
    sessionId.value = res.data.sessionId

    // 建立 WebSocket 连接
    await connectWebSocket()

    connecting.value = false
    connected.value = true
    reconnectAttempts = 0
    emit('connected', sessionId.value)

    terminal.writeln('\x1b[32m✓ Claude Code CLI 已连接\x1b[0m')
    // 注意：fit/focus 已在 connectWebSocket 的 onopen 中处理

    // 延迟提取 Claude session UUID（后端异步提取，稍等几秒后查询）
    setTimeout(async () => {
      try {
        const sessionRes: any = await ClaudeApi.getSession(sessionId.value)
        if (sessionRes?.data?.claudeSessionUuid) {
          lastClaudeSessionUuid.value = sessionRes.data.claudeSessionUuid
        }
      } catch (e) {
      }
    }, 5000)
  } catch (err: any) {
    connecting.value = false
    error.value = err.message || '连接失败'
    console.error('Create session error:', err)
  }
}

const connectToExistingSession = async (selectedId: string) => {
  try {
    sessionId.value = selectedId
    selectedSessionId.value = selectedId
    showSessionSelector.value = false

    // 建立 WebSocket 连接
    await connectWebSocket()

    connecting.value = false
    connected.value = true
    reconnectAttempts = 0
    emit('connected', sessionId.value)

    terminal.writeln('\x1b[32m✓ Claude Code CLI 已连接（会话: ' + selectedId + ')\x1b[0m')
    // 注意：fit/focus 已在 connectWebSocket 的 onopen 中处理

    // 提取 Claude session UUID
    setTimeout(async () => {
      try {
        const sessionRes: any = await ClaudeApi.getSession(sessionId.value)
        if (sessionRes?.data?.claudeSessionUuid) {
          lastClaudeSessionUuid.value = sessionRes.data.claudeSessionUuid
        }
      } catch (e) {
      }
    }, 3000)
  } catch (err: any) {
    connecting.value = false
    error.value = err.message || '连接失败'
    console.error('Connect to existing session error:', err)
    // 连接失败，显示选择器让用户重试
    showSessionSelector.value = true
  }
}

const handleClose = async () => {
  // 清理粘贴缓冲
  if (pasteTimer) {
    clearTimeout(pasteTimer)
    pasteTimer = null
  }
  pasteBuffer = ''

  // 清理全局 paste 监听器
  if (globalPasteHandler) {
    document.removeEventListener('paste', globalPasteHandler, true)
    globalPasteHandler = null
  }

  // 清理 textarea paste 监听器（capture 阶段）
  if (textareaPasteHandler && terminal?.textarea) {
    terminal.textarea.removeEventListener('paste', textareaPasteHandler, true)
    textareaPasteHandler = null
  }

  // 清理 resize 定时器
  if (resizeTimeout) {
    clearTimeout(resizeTimeout)
    resizeTimeout = null
  }

  // 清理 ResizeObserver
  if (resizeObserver) {
    resizeObserver.disconnect()
    resizeObserver = null
  }

  outputFilter.reset()

  // 停止心跳
  stopHeartbeat()

  // 关闭 WebSocket
  if (websocket) {
    websocket.close(1000, 'User closed')
    websocket = null
  }

  // 不再主动关闭后端会话，保留会话以便下次打开时恢复（类似小程序 AI 对话模式）
  // 后端会话由 SessionPool idle-timeout 自动清理
  if (sessionId.value) {
    emit('disconnected', sessionId.value)
    sessionId.value = ''
  }

  // 释放终端实例，确保下次打开时重新创建并重新注册事件处理器
  if (terminal) {
    terminal.dispose()
    terminal = null
  }
  if (fitAddon) {
    fitAddon = null
  }
  outputBuffer = []

  // 移除窗口 resize 监听器（下次 init 会重新绑定）
  window.removeEventListener('resize', handleResize)

  // 重置拖拽状态
  isDragging.value = false
  dragCounter = 0

  // 清空已上传图片
  uploadedImages.value = []

  connected.value = false
  dialogVisible.value = false
}

const toggleFullscreen = () => {
  isFullscreen.value = !isFullscreen.value
  nextTick(() => {
    setTimeout(() => {
      fitAddon?.fit()
      // 全屏切换后发送新的终端大小
      sendTerminalSize()
    }, 100)
  })
}

const clearTerminal = () => {
  terminal?.clear()
  outputBuffer = []
}

const focusTerminal = () => {
  terminal?.focus()
}

const handleResize = () => {
  fitAddon?.fit()
  // 终端大小变化后发送新的大小到后端
  sendTerminalSize()
}

const formatTime = (timeStr: string) => {
  if (!timeStr) return '-'
  const date = new Date(timeStr)
  return date.toLocaleString('zh-CN', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit'
  })
}

const formatSize = (bytes: number) => {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}

/**
 * 分批发送粘贴缓冲区内容，避免大量数据导致 PTY 进程阻塞
 */
const sendPasteBuffer = () => {
  if (!pasteBuffer.length || !websocket || websocket.readyState !== WebSocket.OPEN) {
    return
  }

  // 如果内容较短，直接发送
  if (pasteBuffer.length <= PASTE_BATCH_SIZE) {
    websocket.send(JSON.stringify({ type: 'INPUT', data: pasteBuffer }))
    pasteBuffer = ''
    return
  }

  // 内容较长，分批发送
  const sendBatch = (index: number) => {
    if (index >= pasteBuffer.length) {
      pasteBuffer = ''
      return
    }

    const batch = pasteBuffer.substring(index, index + PASTE_BATCH_SIZE)
    websocket.send(JSON.stringify({ type: 'INPUT', data: batch }))

    // 延迟发送下一批次，给 PTY 进程处理时间
    setTimeout(() => {
      sendBatch(index + PASTE_BATCH_SIZE)
    }, PASTE_BATCH_DELAY)
  }

  sendBatch(0)
}

/**
 * 处理图片文件上传（粘贴、拖拽、文件选择共用）
 * 上传后仅显示在预览栏，等用户按回车发送输入时一并发出读取命令
 */
const handleImageFile = async (file: File) => {
  if (!websocket || websocket.readyState !== WebSocket.OPEN) {
    terminal?.writeln('\x1b[33m未连接到终端，无法上传图片\x1b[0m')
    return
  }

  try {
    const res: any = await ProjectApi.uploadTempImage(file)
    if (!res?.path) {
      throw new Error(res?.message || '上传失败')
    }

    const imagePath = res.path
    const imageUrl = `${window.location.origin}/api/files/image-preview?path=${encodeURIComponent(imagePath)}`

    uploadedImages.value.push({
      path: imagePath,
      url: imageUrl,
      name: file.name || imagePath.split('/').pop() || 'screenshot.png'
    })
  } catch (err: any) {
    terminal?.writeln(`\x1b[31m上传失败: ${err.message}\x1b[0m`)
  }
}

const handleBeforeUnload = () => {
  // 页面刷新/关闭前仅断开 WebSocket，不关闭后端会话以便恢复
  if (websocket) {
    websocket.close(1000, 'Page unloading')
  }
}

// ---------------- 拖拽上传 ----------------

const handleDragOver = (e: DragEvent) => {
  dragCounter++
  // 检查是否包含文件
  if (e.dataTransfer?.types.includes('Files')) {
    isDragging.value = true
  }
}

const handleDragLeave = () => {
  dragCounter--
  if (dragCounter <= 0) {
    isDragging.value = false
    dragCounter = 0
  }
}

const handleDrop = async (e: DragEvent) => {
  dragCounter = 0
  isDragging.value = false

  const files = e.dataTransfer?.files
  if (!files || files.length === 0) return

  // 只处理第一个图片文件
  const file = files[0]
  if (!file.type.startsWith('image/')) {
    terminal?.writeln('\x1b[33m仅支持图片文件\x1b[0m')
    return
  }

  await handleImageFile(file)
}

// ---------------- 点击上传 ----------------

const triggerFileUpload = () => {
  fileInputRef.value?.click()
}

const handleFileSelect = async (e: Event) => {
  const target = e.target as HTMLInputElement
  const files = target.files
  if (!files || files.length === 0) return

  const file = files[0]
  if (!file.type.startsWith('image/')) {
    terminal?.writeln('\x1b[33m仅支持图片文件\x1b[0m')
    target.value = ''
    return
  }

  await handleImageFile(file)
  target.value = ''
}

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize)
  window.removeEventListener('beforeunload', handleBeforeUnload)

  // 组件卸载时不关闭后端会话，保留以便恢复
  if (sessionId.value) {
    // 仅断开 WebSocket，不调用 closeSession
    emit('disconnected', sessionId.value)
  }

  // 清理全局 paste 监听器
  if (globalPasteHandler) {
    document.removeEventListener('paste', globalPasteHandler, true)
    globalPasteHandler = null
  }

  // 清理 textarea paste 监听器（capture 阶段）
  if (textareaPasteHandler && terminal?.textarea) {
    terminal.textarea.removeEventListener('paste', textareaPasteHandler, true)
    textareaPasteHandler = null
  }

  // 清理 resize 定时器
  if (resizeTimeout) {
    clearTimeout(resizeTimeout)
    resizeTimeout = null
  }

  // 清理 ResizeObserver
  if (resizeObserver) {
    resizeObserver.disconnect()
    resizeObserver = null
  }

  if (websocket) {
    websocket.close(1000)
  }
  if (terminal) {
    terminal.dispose()
    terminal = null
  }
})

onMounted(() => {
  window.addEventListener('beforeunload', handleBeforeUnload)
})
</script>

<style scoped>
.claude-terminal-dialog {
  --el-dialog-bg-color: #1a1a2e;
}

.claude-terminal-dialog :deep(.el-dialog__header) {
  padding: 12px 16px;
  background: #16213e;
  border-bottom: 1px solid #0f3460;
}

.claude-terminal-dialog :deep(.el-dialog__body) {
  padding: 0;
  background: #1a1a2e;
}

.claude-terminal-dialog :deep(.el-dialog__footer) {
  padding: 8px 16px;
  background: #16213e;
  border-top: 1px solid #0f3460;
}

.dialog-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  color: #eaeaea;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 500;
}

.header-right {
  display: flex;
  gap: 8px;
}

.terminal-wrapper {
  position: relative;
  width: 100%;
  height: calc(90vh - 140px);
  min-height: 400px;
  background: #1a1a2e;
  overflow: hidden;
}

.terminal-container {
  width: 100%;
  height: 100%;
  padding: 0;
  box-sizing: border-box;
}

.terminal-overlay {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  gap: 12px;
  background: rgba(26, 26, 46, 0.9);
  color: #eaeaea;
  z-index: 10;
}

.terminal-overlay.error {
  color: #f56c6c;
}

.terminal-overlay.reconnecting {
  color: #409eff;
}

.reconnect-hint {
  font-size: 12px;
  opacity: 0.7;
}

.terminal-overlay .el-icon {
  font-size: 32px;
}

.dialog-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  color: #9ca3af;
  font-size: 12px;
}

.work-dir {
  opacity: 0.8;
}

.hints {
  display: flex;
  gap: 8px;
}

:deep(.xterm) {
  padding: 8px;
}

:deep(.xterm-viewport) {
  overflow-y: auto !important;
}

/* 会话选择器样式 */
.session-selector {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  display: flex;
  justify-content: center;
  align-items: center;
  background: rgba(26, 26, 46, 0.95);
  z-index: 20;
}

.session-selector-content {
  background: #16213e;
  border-radius: 8px;
  padding: 24px;
  width: 90%;
  max-width: 600px;
  border: 1px solid #0f3460;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.4);
}

.session-selector-content h3 {
  color: #eaeaea;
  margin: 12px 0 8px 0;
  font-size: 18px;
  font-weight: 600;
}

.session-selector-content .hint {
  color: #9ca3af;
  font-size: 14px;
  margin-bottom: 16px;
}

.session-list {
  max-height: 300px;
  overflow-y: auto;
  margin-bottom: 16px;
  border: 1px solid #0f3460;
  border-radius: 6px;
  background: #1a1a2e;
}

.session-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  border-bottom: 1px solid #0f3460;
  cursor: pointer;
  transition: all 0.2s ease;
}

.session-item:last-child {
  border-bottom: none;
}

.session-item:hover {
  background: #0f3460;
}

.session-item.active {
  background: #0a2540;
  border-left: 3px solid #4fc3f7;
}

.session-info {
  display: flex;
  flex-direction: column;
  gap: 8px;
  flex: 1;
}

.session-id {
  display: flex;
  align-items: center;
  gap: 8px;
}

.session-meta {
  display: flex;
  gap: 16px;
  color: #9ca3af;
  font-size: 12px;
}

.session-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 16px;
}

.session-create-new {
  margin-top: 16px;
  text-align: center;
}

.session-create-new :deep(.el-divider__text) {
  color: #9ca3af;
  font-size: 12px;
  padding: 0 12px;
}

/* 恢复选择器额外样式 */
.quick-resume {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  margin: 16px 0;
}

.resume-hint {
  color: #9ca3af;
  font-size: 12px;
}

.no-sessions {
  padding: 24px;
  text-align: center;
  color: #9ca3af;
  font-size: 14px;
}

/* 拖拽上传视觉反馈 */
.terminal-wrapper.drag-over {
  border: 2px dashed #4fc3f7;
  background: rgba(79, 195, 247, 0.08);
}

.terminal-wrapper.drag-over .terminal-container {
  pointer-events: none;
}

.drag-overlay {
  color: #4fc3f7;
  background: rgba(26, 26, 46, 0.85);
}

.drag-overlay .el-icon {
  font-size: 48px;
}

/* 已上传图片预览栏 */
.image-preview-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding: 8px 12px;
  background: #16213e;
  border-bottom: 1px solid #0f3460;
  max-height: 100px;
  overflow-y: auto;
}

.image-preview-item {
  position: relative;
  width: 64px;
  height: 64px;
  border-radius: 4px;
  overflow: hidden;
  border: 1px solid #0f3460;
  cursor: pointer;
  flex-shrink: 0;
}

.image-preview-item:hover {
  border-color: #4fc3f7;
}

.image-thumb {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.image-delete-btn {
  position: absolute;
  top: 2px;
  right: 2px;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: rgba(245, 108, 108, 0.9);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  font-size: 10px;
  opacity: 0;
  transition: opacity 0.2s;
  z-index: 2;
}

.image-preview-item:hover .image-delete-btn {
  opacity: 1;
}

.image-delete-btn:hover {
  background: #f56c6c;
}

/* 图片放大预览遮罩层 */
.image-preview-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.85);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 3000;
  cursor: zoom-out;
}

.image-preview-full {
  max-width: 90vw;
  max-height: 90vh;
  object-fit: contain;
  border-radius: 4px;
  box-shadow: 0 4px 24px rgba(0, 0, 0, 0.5);
}

.image-preview-close {
  position: absolute;
  top: 16px;
  right: 16px;
  width: 36px;
  height: 36px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.15);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  font-size: 18px;
  transition: background 0.2s;
  z-index: 3001;
}

.image-preview-close:hover {
  background: rgba(245, 108, 108, 0.8);
}
</style>