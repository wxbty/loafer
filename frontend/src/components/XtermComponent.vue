<template>
  <div class="xterm-component">
    <div ref="terminalRef" class="terminal" tabindex="0"></div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { Terminal } from 'xterm'
import { FitAddon } from 'xterm-addon-fit'
import 'xterm/css/xterm.css'
import { MouseTrackingFilter, isMouseEventSequence } from '@/utils/xtermFilter'

interface Props {
  options?: any
  sessionId?: string
}

interface Emits {
  (e: 'on-created'): void
  (e: 'on-input', input: string): void
}

const props = withDefaults(defineProps<Props>(), {
  options: () => ({
    cursorBlink: true,
    fontSize: 14,
    fontFamily: '"Cascadia Code", "Fira Code", "JetBrains Mono", "Source Code Pro", "Noto Sans Mono", Consolas, "Courier New", monospace',
    allowProposedApi: true,
    theme: {
      background: '#1e1e1e',
      foreground: '#d4d4d4',
      cursor: '#d4d4d4'
    }
  })
})

const emit = defineEmits<Emits>()

const terminalRef = ref<HTMLElement | null>(null)
const terminal = ref<Terminal | null>(null)
const fitAddon = ref<FitAddon | null>(null)

const outputFilter = new MouseTrackingFilter()

const writeln = (text: string) => {
  if (terminal.value) {
    terminal.value.writeln(outputFilter.feed(text))
  }
}

const write = (text: string) => {
  if (terminal.value) {
    terminal.value.write(outputFilter.feed(text))
  }
}

const clear = () => {
  if (terminal.value) {
    terminal.value.clear()
  }
}

const focus = () => {
  if (terminal.value) {
    terminal.value.focus()
  }
}

onMounted(() => {
  if (!terminalRef.value) return

  const options = {
    rightClickSelectsWord: true,
    ...(props.options || {}),
  }
  terminal.value = new Terminal(options)
  outputFilter.reset()
  fitAddon.value = new FitAddon()
  terminal.value.loadAddon(fitAddon.value)
  terminal.value.open(terminalRef.value)
  fitAddon.value.fit()

  terminal.value.onData((data) => {
    if (isMouseEventSequence(data)) {
      return
    }
    emit('on-input', data)
  })

  emit('on-created')
})

onBeforeUnmount(() => {
  outputFilter.reset()
  if (terminal.value) {
    terminal.value.dispose()
    terminal.value = null
  }
})

defineExpose({
  writeln,
  write,
  clear,
  focus
})
</script>

<style scoped>
.xterm-component {
  width: 100%;
  height: 100%;
}

.terminal {
  width: 100%;
  height: 100%;
  outline: none;
}

.terminal:focus {
  outline: none;
}
</style>