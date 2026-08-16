import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import path from 'path'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { NaiveUiResolver } from 'unplugin-vue-components/resolvers'

export default defineConfig(({ mode }) => {
  // 加载环境变量
  const env = loadEnv(mode, process.cwd(), '')
  // 默认使用相对路径，让 nginx 代理处理
  const apiBaseUrl = env.VITE_API_BASE_URL || ''
  const wsBaseUrl = env.VITE_WS_BASE_URL || (apiBaseUrl ? apiBaseUrl.replace(/^http/, 'ws') : '')

  return {
    plugins: [
      vue(),
      AutoImport({
        imports: [
          'vue',
          'vue-router',
          {
            'naive-ui': [
              'useDialog',
              'useMessage',
              'useNotification',
              'useLoadingBar'
            ]
          }
        ],
        dts: 'src/auto-imports.dts'
      }),
      Components({
        resolvers: [NaiveUiResolver()],
        dts: 'src/components.d.ts'
      })
    ],
    resolve: {
      alias: {
        '@': path.resolve(__dirname, 'src')
      }
    },
    server: {
      port: 3000,
      proxy: {
        '/api': {
          target: apiBaseUrl,
          changeOrigin: true,
          configure: (proxy) => {
            proxy.on('proxyRes', (proxyRes, req) => {
              const contentType = proxyRes.headers['content-type'] || ''
              if (contentType.includes('text/event-stream')) {
                proxyRes.headers['Cache-Control'] = 'no-cache'
                proxyRes.headers['X-Accel-Buffering'] = 'no'
              }
            })
          }
        },
        '/ws': {
          target: wsBaseUrl,
          changeOrigin: true,
          ws: true
        }
      }
    }
  }
})
