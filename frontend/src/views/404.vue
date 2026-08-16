<template>
  <div class="not-found">
    <n-result status="404" title="404 - 页面未找到" description="您访问的页面不存在或已被移动">
      <template #icon>
        <n-icon :component="ImageOff" :size="80" />
      </template>
      <template #footer>
        <n-space justify="center">
          <n-button type="primary" @click="goBack">
            <template #icon>
              <n-icon :component="ArrowLeft" />
            </template>
            返回上一页
          </n-button>
          <n-button @click="goHome">
            <template #icon>
              <n-icon :component="Home" />
            </template>
            回到首页
          </n-button>
        </n-space>
      </template>
    </n-result>
  </div>
</template>

<script setup lang="ts">
import { NButton, NIcon, NResult, NSpace } from 'naive-ui'
import { ArrowLeft, Home, ImageOff } from 'lucide-vue-next'
import { useRouter } from 'vue-router'

const router = useRouter()

const goBack = () => {
  if (window.history.length > 1) {
    router.back()
  } else {
    router.push('/')
  }
}

const goHome = () => {
  router.push('/')
}
</script>

<style scoped>
.not-found {
  display: flex;
  min-height: 100%;
  align-items: center;
  justify-content: center;
  padding: 2rem 1rem;
}
</style>
