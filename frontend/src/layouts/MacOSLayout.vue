<script setup lang="ts">
import { computed, ref, h } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  NDropdown,
  NIcon
} from 'naive-ui'
import {
  Folder,
  LogOut,
} from 'lucide-vue-next'

defineOptions({
  name: 'MacOSLayout'
})

const route = useRoute()
const router = useRouter()

// 导航菜单项
const menuItems = [
  { path: '/projects', icon: Folder, label: '项目管理', key: 'projects' }
]

// 当前激活的菜单项
const activeKey = computed(() => {
  const path = route.path
  if (path.startsWith('/projects')) return 'projects'
  return ''
})

// 用户信息
const username = ref(localStorage.getItem('displayName') || localStorage.getItem('username') || '用户')

// 用户下拉菜单
const userDropdownOptions = [
  { label: '退出登录', key: 'logout', icon: () => h(NIcon, null, { default: () => h(LogOut) }) }
]

const handleUserDropdown = (key: string) => {
  if (key === 'logout') {
    localStorage.removeItem('token')
    localStorage.removeItem('userId')
    localStorage.removeItem('username')
    localStorage.removeItem('displayName')
    router.push('/login')
  }
}

// 导航
const navigateTo = (path: string) => {
  router.push(path)
}
</script>

<template>
  <div class="macos-layout">
    <!-- 侧边栏 -->
    <aside class="sidebar glass">
      <!-- Logo 区域 -->
      <div class="sidebar__logo">
        <div class="logo-icon">
          <svg viewBox="0 0 24 24" fill="none" class="w-8 h-8">
            <path
              d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2z"
              fill="url(#gradient)"
            />
            <path
              d="M8 12l2 2 4-4"
              stroke="white"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            />
            <defs>
              <linearGradient id="gradient" x1="0%" y1="0%" x2="100%" y2="100%">
                <stop offset="0%" stop-color="#007aff" />
                <stop offset="100%" stop-color="#5856d6" />
              </linearGradient>
            </defs>
          </svg>
        </div>
        <span class="logo-text">Loafer</span>
      </div>

      <!-- 导航菜单 -->
      <nav class="sidebar__nav">
        <div
          v-for="item in menuItems"
          :key="item.key"
          class="sidebar-item"
          :class="{ 'sidebar-item--active': activeKey === item.key }"
          @click="navigateTo(item.path)"
        >
          <n-icon :component="item.icon" :size="20" />
          <span class="sidebar-item__label">{{ item.label }}</span>
        </div>
      </nav>

      <!-- 底部区域 -->
      <div class="sidebar__footer">
        <!-- 用户信息 -->
        <n-dropdown
          :options="userDropdownOptions"
          @select="handleUserDropdown"
          placement="right-start"
        >
          <div class="sidebar-user">
            <div class="user-avatar">
              {{ username.charAt(0).toUpperCase() }}
            </div>
            <span class="user-name">{{ username }}</span>
          </div>
        </n-dropdown>
      </div>
    </aside>

    <!-- 主内容区 -->
    <main class="main-content">
      <router-view />
    </main>
  </div>
</template>

<style scoped>
.macos-layout {
  display: flex;
  min-height: 100vh;
  background: #f5f5f7;
}

/* 侧边栏 */
.sidebar {
  position: fixed;
  left: 0;
  top: 0;
  bottom: 0;
  width: 240px;
  display: flex;
  flex-direction: column;
  border-right: 1px solid rgba(0, 0, 0, 0.08);
  z-index: 100;
  background: #ffffff;
}

/* Logo */
.sidebar__logo {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 20px 16px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
}

.logo-icon {
  flex-shrink: 0;
}

.logo-text {
  font-size: 16px;
  font-weight: 600;
  color: #1d1d1f;
  white-space: nowrap;
}

/* 导航 */
.sidebar__nav {
  flex: 1;
  padding: 12px 8px;
  overflow-y: auto;
  overflow-x: hidden;
}

.sidebar-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 8px;
  color: #86868b;
  cursor: pointer;
  transition: all 0.15s ease;
  margin-bottom: 4px;
}

.sidebar-item:hover {
  background: rgba(0, 0, 0, 0.04);
  color: #1d1d1f;
}

.sidebar-item--active {
  background: rgba(0, 122, 255, 0.1);
  color: #007aff;
  font-weight: 500;
}

.sidebar-item--active:hover {
  background: rgba(0, 122, 255, 0.15);
}

.sidebar-item__label {
  white-space: nowrap;
}

/* 底部 */
.sidebar__footer {
  padding: 12px 8px;
  border-top: 1px solid rgba(0, 0, 0, 0.05);
}

.sidebar-user {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.15s ease;
}

.sidebar-user:hover {
  background: rgba(0, 0, 0, 0.04);
}

.user-avatar {
  flex-shrink: 0;
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: linear-gradient(135deg, #007aff, #5856d6);
  color: white;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  font-weight: 600;
}

.user-name {
  font-size: 14px;
  color: #1d1d1f;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 主内容区 */
.main-content {
  flex: 1;
  margin-left: 240px;
  min-height: 100vh;
  padding: 16px;
}
</style>