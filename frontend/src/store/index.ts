import { defineStore } from 'pinia'

// 主应用状态
export const useAppStore = defineStore('app', {
  state: () => ({
    isLoading: false,
    sidebarCollapse: false,
    notifications: [] as {
      id: number
      title: string
      message: string
      type: 'success' | 'error' | 'warning' | 'info'
    }[]
  }),
  actions: {
    showLoading() {
      this.isLoading = true
    },
    hideLoading() {
      this.isLoading = false
    },
    toggleSidebar() {
      this.sidebarCollapse = !this.sidebarCollapse
    },
    addNotification(title: string, message: string, type: 'success' | 'error' | 'warning' | 'info' = 'info') {
      const id = Date.now()
      this.notifications.push({
        id,
        title,
        message,
        type
      })
      // 3秒后自动移除通知
      setTimeout(() => {
        this.removeNotification(id)
      }, 3000)
    },
    removeNotification(id: number) {
      const index = this.notifications.findIndex(n => n.id === id)
      if (index !== -1) {
        this.notifications.splice(index, 1)
      }
    }
  },
  getters: {
    notificationCount: (state) => state.notifications.length
  }
})

// 任务状态管理
export const useTaskStore = defineStore('task', {
  state: () => ({
    tasks: [],
    currentTask: null,
    taskStates: []
  }),
  actions: {
    setTasks(tasks: any[]) {
      this.tasks = tasks
    },
    setCurrentTask(task: any) {
      this.currentTask = task
    },
    setTaskStates(states: any[]) {
      this.taskStates = states
    },
    updateTaskStatus(taskId: number, status: number) {
      const task = this.tasks.find(t => t.id === taskId)
      if (task) {
        task.status = status
      }
    }
  },
  getters: {
    completedTasks: (state) => state.tasks.filter(t => t.status === 3),
    inProgressTasks: (state) => state.tasks.filter(t => t.status === 1),
    pausedTasks: (state) => state.tasks.filter(t => t.status === 4)
  }
})
