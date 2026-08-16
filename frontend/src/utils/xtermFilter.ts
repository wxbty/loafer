// 鼠标跟踪相关的 DEC private modes —— 一旦被启用，xterm 会把鼠标拖拽事件
// 转给应用而不再用于文本选择，导致网页终端无法选中复制文本。
//
// 1000 X10、1001 高亮、1002 拖拽、1003 全事件、1004 焦点、
// 1005 UTF-8、1006 SGR、1015 URXVT、1016 SGR-像素
const MOUSE_TRACKING_MODES = new Set([
  '1000', '1001', '1002', '1003', '1004', '1005', '1006', '1015', '1016',
])

// DEC private mode SET：ESC [ ? <params> h
// 也匹配 ESC [ > 形式（DECSET 的另一种格式）
const DEC_PRIVATE_SET = /\x1b\[[\?>]([\d;]+)h/gi

function stripFromChunk(s: string): string {
  const original = s
  const result = s.replace(DEC_PRIVATE_SET, (_m, params: string) => {
    const parts = params.split(';')
    const kept = parts.filter((p) => !MOUSE_TRACKING_MODES.has(p))
    return kept.length ? `\x1b[?${kept.join(';')}h` : ''
  })
  return result
}

// 末尾若有未闭合的转义序列，返回其起始下标；否则返回 -1。
// 用于跨 WebSocket 包缓冲，避免 \x1b[?1000;1002;1006h 之类被切到两包里漏过过滤。
function trailingIncompleteIndex(s: string): number {
  const lastEsc = s.lastIndexOf('\x1b')
  if (lastEsc === -1) return -1
  const rest = s.substring(lastEsc + 1)
  if (rest.length === 0) return lastEsc
  if (rest[0] !== '[') return -1 // 非 CSI 的简单 ESC 序列视为闭合
  for (let i = 1; i < rest.length; i++) {
    const c = rest.charCodeAt(i)
    if (c >= 0x40 && c <= 0x7e) return -1 // 已闭合
  }
  return lastEsc
}

// 跨包缓冲：把末尾未闭合的转义序列挪到下一次输入再处理。
export class MouseTrackingFilter {
  private pending = ''

  feed(chunk: string): string {
    const combined = this.pending + chunk
    const idx = trailingIncompleteIndex(combined)
    if (idx === -1) {
      this.pending = ''
      return stripFromChunk(combined)
    }
    this.pending = combined.substring(idx)
    return stripFromChunk(combined.substring(0, idx))
  }

  reset(): void {
    this.pending = ''
  }
}

// 主动禁用所有鼠标跟踪模式。周期性写入以撤销任何漏过过滤的启用序列，
// 保证拖拽选择文字始终可用（代价是 TUI 应用无法使用鼠标交互）。
export const MOUSE_TRACKING_DISABLE_SEQ =
  '\x1b[?1000l\x1b[?1001l\x1b[?1002l\x1b[?1003l\x1b[?1004l\x1b[?1005l\x1b[?1006l\x1b[?1015l\x1b[?1016l'

// ============================================================================
// 鼠标事件序列检测
// 当鼠标跟踪模式被启用时，xterm.js 会通过 onData 发送鼠标事件序列
// 这些序列应该被忽略，不应该发送到后端
// ============================================================================

// SGR 模式 (1006): ESC [ < button ; x ; y M/m
// 注意：button 编码包含修饰键信息：button + (shift*4) + (meta*8) + (ctrl*16)
// 按下事件用 M，释放事件用 m
const MOUSE_EVENT_SGR = /^\x1b\[<(\d+);(\d+);(\d+)([Mm])$/

// URXVT 模式 (1015): ESC [ button ; x ; y M
const MOUSE_EVENT_URXVT = /^\x1b\[(\d+);(\d+);(\d+)M$/

// X10 模式 (1000) 前缀: ESC [ M + 3 bytes
// 这是最基本的模式，后面跟着 3 个字节：button, x, y（都编码为 32+value）
const MOUSE_EVENT_X10_PREFIX = '\x1b[M'

// SGR-像素模式 (1016): ESC [ < button ; x ; y ; pixels M/m
// 与 SGR 类似，但坐标是像素值
const MOUSE_EVENT_SGR_PIXELS = /^\x1b\[<(\d+);(\d+);(\d+);(\d+)([Mm])$/

/**
 * 检测输入数据是否为鼠标事件序列
 * @param data - onData 接收到的数据
 * @returns true 如果是鼠标事件序列，应该被忽略
 */
export function isMouseEventSequence(data: string): boolean {
  // 空数据直接返回
  if (!data || data.length === 0) return false

  // 检查 SGR 模式 (1006) - 最常用的模式
  if (MOUSE_EVENT_SGR.test(data)) {
    return true
  }

  // 检查 SGR-像素模式 (1016)
  if (MOUSE_EVENT_SGR_PIXELS.test(data)) {
    return true
  }

  // 检查 URXVT 模式 (1015)
  if (MOUSE_EVENT_URXVT.test(data)) {
    return true
  }

  // 检查 X10 模式 (1000) - 以 ESC[M 开头，后跟 3 字节
  // 注意：有些情况下可能会有多个事件连续发送
  if (data.startsWith(MOUSE_EVENT_X10_PREFIX)) {
    // X10 模式的事件长度为 5 字节（ESC[M + 3 bytes）
    // 但也可能有多个事件连续发送，所以检查是否是 5 的倍数
    if (data.length === 5 || (data.length > 5 && data.length % 5 === 0 && /\x1b\[M.{3}/g.test(data))) {
      return true
    }
  }

  // 检查是否是纯鼠标事件序列的组合
  // 有时浏览器会连续发送多个鼠标事件
  if (data.length > 5 && data.startsWith(MOUSE_EVENT_X10_PREFIX)) {
    // 尝试解析为多个 X10 事件
    const events = data.split(/\x1b\[M/)
    if (events.every((e, i) => i === 0 || e.length === 3)) {
      return true
    }
  }

  return false
}

// ============================================================================
// Bracketed Paste Mode 序列检测
// 当 bracketed paste mode (DECSET 2004) 被启用时，终端应用会收到：
//   ESC [ 200 ~  <粘贴内容>  ESC [ 201 ~
// 右键粘贴图片时，xterm.js 可能只发送空的 200~/201~ 标记（图片数据不走 onData），
// 这些空标记不应被当作用户输入发送到后端。
// ============================================================================

const BRACKETED_PASTE_START = '\x1b[200~'
const BRACKETED_PASTE_END = '\x1b[201~'

/**
 * 检测数据是否只包含 bracketed paste 标记（200~/201~）而没有实际内容。
 * 例如：ESC[200~ESC[201~ 或单独的 ESC[200~ 或 ESC[201~
 */
export function isBracketedPasteOnly(data: string): boolean {
  if (!data || data.length === 0) return false

  // 构建一个只包含 bracketed paste 标记的正则：^\[200~\[201~$ 等
  // 允许的顺序：start+end, start, end
  const onlyMarkers = /^(\x1b\[200~)?(\x1b\[201~)?$/
  if (onlyMarkers.test(data)) {
    return true
  }

  return false
}

/**
 * 将字符串转换为十六进制表示，方便调试
 */
function dataToHex(data: string): string {
  return Array.from(data).map(c => {
    const code = c.charCodeAt(0)
    if (code === 0x1b) return 'ESC'
    if (code >= 32 && code < 127) return c
    return `0x${code.toString(16).padStart(2, '0')}`
  }).join(' ')
}