<template>
  <div class="login-page">
    <div class="login-container">
      <!-- 左侧装饰 -->
      <div class="login-decoration">
        <div class="decoration-content">
          <svg viewBox="0 0 80 80" fill="none" class="logo-large">
            <path
              d="M40 8C22.327 8 8 22.327 8 40s14.327 32 32 32 32-14.327 32-32S57.673 8 40 8z"
              fill="url(#gradientLarge)"
            />
            <path
              d="M28 40l8 8 16-16"
              stroke="white"
              stroke-width="4"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
            <defs>
              <linearGradient id="gradientLarge" x1="0%" y1="0%" x2="100%" y2="100%">
                <stop offset="0%" stop-color="#007aff" />
                <stop offset="100%" stop-color="#5856d6" />
              </linearGradient>
            </defs>
          </svg>
          <h1 class="decoration-title">Loafer</h1>
          <p class="decoration-subtitle">全自动项目开发平台</p>
        </div>
      </div>

      <!-- 右侧登录表单 -->
      <div class="login-form-wrapper">
        <div class="login-form-container">
          <div class="login-header">
            <h2 class="login-title">登录</h2>
            <p class="login-subtitle">请输入您的账户信息</p>
          </div>

          <n-form
            ref="formRef"
            :model="formValue"
            :rules="rules"
            label-placement="top"
            require-mark-placement="right-hanging"
          >
            <n-form-item label="用户名" path="username">
              <n-input
                v-model:value="formValue.username"
                placeholder="请输入用户名"
                size="large"
                class="macos-input-wrapper"
              >
                <template #prefix>
                  <n-icon :component="UserIcon" />
                </template>
              </n-input>
            </n-form-item>

            <n-form-item label="密码" path="password">
              <n-input
                v-model:value="formValue.password"
                type="password"
                placeholder="请输入密码"
                size="large"
                show-password-on="click"
                class="macos-input-wrapper"
              >
                <template #prefix>
                  <n-icon :component="LockIcon" />
                </template>
              </n-input>
            </n-form-item>

            <n-form-item>
              <n-button
                type="primary"
                block
                size="large"
                :loading="loading"
                @click="handleLogin"
                class="login-button"
              >
                登录
              </n-button>
            </n-form-item>
          </n-form>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { NButton, NForm, NFormItem, NIcon, NInput, useMessage } from 'naive-ui'
import type { FormInst, FormRules } from 'naive-ui'
import { User as UserIcon, Lock as LockIcon } from 'lucide-vue-next'
import { AuthApi } from '@/api/auth'

defineOptions({
  name: 'Login'
})

const router = useRouter()
const message = useMessage()

const formRef = ref<FormInst | null>(null)
const loading = ref(false)
const formValue = ref({
  username: '',
  password: ''
})

const rules: FormRules = {
  username: {
    required: true,
    message: '请输入用户名',
    trigger: 'blur'
  },
  password: {
    required: true,
    message: '请输入密码',
    trigger: 'blur'
  }
}

const handleLogin = async () => {
  try {
    await formRef.value?.validate()
    loading.value = true

    const response = await AuthApi.login(formValue.value)

    if (response.success && response.data) {
      localStorage.setItem('token', response.data.token)
      localStorage.setItem('userId', String(response.data.userId))
      localStorage.setItem('username', response.data.username)
      localStorage.setItem('displayName', response.data.displayName || '')
      localStorage.setItem('userType', response.data.userType || 'normal')
      message.success('登录成功')
      router.push('/')
    } else {
      message.error(response.message || '登录失败')
    }
  } catch {
    // 表单校验失败或请求异常
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #f5f5f7 0%, #e8e8ed 100%);
  padding: 20px;
}

.login-container {
  display: flex;
  width: 100%;
  max-width: 900px;
  background: white;
  border-radius: 20px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.08), 0 2px 8px rgba(0, 0, 0, 0.04);
  overflow: hidden;
}

/* 左侧装饰区域 */
.login-decoration {
  flex: 1;
  background: linear-gradient(135deg, #007aff 0%, #5856d6 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px;
  min-height: 400px;
}

.decoration-content {
  text-align: center;
  color: white;
}

.logo-large {
  width: 80px;
  height: 80px;
  margin-bottom: 24px;
}

.decoration-title {
  font-size: 28px;
  font-weight: 600;
  margin-bottom: 8px;
}

.decoration-subtitle {
  font-size: 16px;
  opacity: 0.9;
}

/* 右侧表单区域 */
.login-form-wrapper {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px;
  min-height: 400px;
}

.login-form-container {
  width: 100%;
  max-width: 320px;
}

.login-header {
  text-align: center;
  margin-bottom: 32px;
}

.login-title {
  font-size: 24px;
  font-weight: 600;
  color: #1d1d1f;
  margin-bottom: 8px;
}

.login-subtitle {
  font-size: 14px;
  color: #86868b;
}

.login-button {
  margin-top: 8px;
  border-radius: 10px;
  font-weight: 500;
}

/* 响应式 */
@media (max-width: 768px) {
  .login-container {
    flex-direction: column;
    max-width: 400px;
  }

  .login-decoration {
    min-height: 200px;
    padding: 24px;
  }

  .logo-large {
    width: 48px;
    height: 48px;
    margin-bottom: 16px;
  }

  .decoration-title {
    font-size: 20px;
  }

  .decoration-subtitle {
    font-size: 14px;
  }

  .login-form-wrapper {
    padding: 24px;
  }
}

/* Naive UI 输入框样式覆盖 */
.macos-input-wrapper :deep(.n-input__input-el) {
  font-size: 15px;
}

.macos-input-wrapper :deep(.n-input) {
  border-radius: 10px;
}
</style>