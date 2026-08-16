import axios from 'axios'
import { createDiscreteApi } from 'naive-ui'

const { message, dialog } = createDiscreteApi(['message', 'dialog'])

const request = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '/api',
  timeout: 10000
})

request.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('token')
    if (token) {
      config.headers['Authorization'] = `Bearer ${token}`
    }
    if (!config.headers['Accept']) {
      config.headers['Accept'] = 'application/json;charset=UTF-8'
    }
    // 将 skipErrorMessage 标记到 headers 中，确保错误处理器能可靠获取
    if (config.skipErrorMessage) {
      config.headers['X-Skip-Error-Message'] = 'true'
    }
    return config
  },
  (error) => Promise.reject(error)
)

request.interceptors.response.use(
  (response) => {
    const { data, status } = response
    if (status === 200) {
      return data
    }
    if (!response.config.skipErrorMessage) {
      message.error(data?.message || '请求失败')
    }
    return Promise.reject(data)
  },
  (error) => {
    const { response } = error
    // 统一检查是否跳过错误消息（优先从 headers 获取，更可靠）
    const skipErrorMessage = error.config?.headers?.['X-Skip-Error-Message'] === 'true' || error.config?.skipErrorMessage

    if (response) {
      const { status, data } = response

      // 如果请求配置了跳过错误消息，则不显示 toast
      if (!skipErrorMessage) {
        switch (status) {
          case 401:
            dialog.info({
              title: '提示',
              content: data?.message || '请先登录',
              positiveText: '确定',
              onPositiveClick: () => {
                localStorage.removeItem('token')
                window.location.href = '/login'
                return true
              }
            })
            break
          case 403:
            message.error('您没有权限访问该资源')
            break
          case 404:
            message.error('请求的资源不存在')
            break
          case 500:
            message.error(data?.message || '服务器内部错误')
            break
          default:
            message.error(data?.message || '请求失败')
        }
      }
    } else if (!skipErrorMessage) {
      if (error.code === 'ERR_NETWORK') {
        message.error('网络连接失败')
      } else if (error.code === 'ECONNABORTED') {
        message.error('请求超时')
      }
    }

    return Promise.reject(error)
  }
)

export default request
