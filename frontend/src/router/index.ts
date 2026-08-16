import { createRouter, createWebHistory } from 'vue-router'
import type { RouteRecordRaw } from 'vue-router'
import MacOSLayout from '@/layouts/MacOSLayout.vue'

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('@/views/Login.vue'),
    meta: { requiresAuth: false }
  },
  {
    path: '/',
    component: MacOSLayout,
    meta: { requiresAuth: true },
    children: [
      {
        path: '',
        redirect: '/projects'
      },
      {
        path: 'projects',
        name: 'Projects',
        component: () => import('@/views/Projects.vue')
      },
      {
        path: 'projects/create',
        name: 'CreateProject',
        component: () => import('@/views/CreateProject.vue')
      },
      {
        path: 'projects/chat-create',
        name: 'ChatCreateProject',
        component: () => import('@/views/ChatCreateProject.vue')
      },
      {
        path: 'projects/:id',
        name: 'ProjectDetail',
        component: () => import('@/views/ProjectDetail.vue')
      },
      {
        path: 'settings',
        name: 'Settings',
        component: () => import('@/views/Settings.vue')
      }
    ]
  },
  {
    path: '/404',
    name: 'NotFound',
    component: () => import('@/views/404.vue'),
    meta: { requiresAuth: false }
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/404'
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

router.beforeEach((to, _from, next) => {
  const token = localStorage.getItem('token')
  if (to.meta.requiresAuth && !token) {
    next('/login')
  } else if (to.path === '/login' && token) {
    next('/')
  } else {
    next()
  }
})

export default router