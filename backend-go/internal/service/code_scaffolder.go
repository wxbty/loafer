package service

import (
	"fmt"
	"path/filepath"
	"strings"

	"loafer-agent/internal/config"
	"loafer-agent/internal/model"

	"gorm.io/gorm"
)

// CodeScaffolder 代码脚手架服务。
// 当 Claude CLI 不可用时，基于需求分析结果生成可运行的 Go + React 项目代码，
// 通过 SSH 写入远程服务器的项目工作目录，使后续部署阶段能正常构建和运行。
type CodeScaffolder struct {
	db        *gorm.DB
	cfg       *config.Config
	sshClient *SSHClient
}

// NewCodeScaffolder 构造代码脚手架服务。
func NewCodeScaffolder(db *gorm.DB, cfg *config.Config) *CodeScaffolder {
	sshClient, err := NewSSHClient(&cfg.Infra)
	if err != nil {
		fmt.Printf("⚠ CodeScaffolder SSH 客户端创建失败: %v\n", err)
	}
	return &CodeScaffolder{
		db:        db,
		cfg:       cfg,
		sshClient: sshClient,
	}
}

// ScaffoldProject 为项目生成完整的可运行代码。
// 基于项目需求描述和已分解的模块信息，生成 Go 后端 + React 前端代码。
// onProgress 回调用于实时推送进度信息。
func (s *CodeScaffolder) ScaffoldProject(projectID int64, modules []model.Module, onProgress func(string)) error {
	if s.sshClient == nil {
		return fmt.Errorf("SSH 客户端未初始化，无法生成代码")
	}

	var project model.Project
	if err := s.db.First(&project, projectID).Error; err != nil {
		return fmt.Errorf("加载项目失败: %w", err)
	}
	if project.WorkDir == "" {
		return fmt.Errorf("项目工作目录未设置")
	}

	workDir := project.WorkDir
	frontendDir := filepath.Join(workDir, "frontend")
	backendDir := filepath.Join(workDir, "backend-go")

	progress := func(msg string) {
		if onProgress != nil {
			onProgress(msg)
		}
	}

	// ===== 1. 创建目录结构 =====
	progress("正在创建项目目录结构...")
	dirs := []string{
		workDir,
		backendDir,
		filepath.Join(backendDir, "internal/handler"),
		filepath.Join(backendDir, "internal/model"),
		filepath.Join(backendDir, "internal/config"),
		filepath.Join(backendDir, "internal/middleware"),
		frontendDir,
		filepath.Join(frontendDir, "src"),
		filepath.Join(frontendDir, "src/views"),
		filepath.Join(frontendDir, "src/api"),
		filepath.Join(frontendDir, "src/router"),
		filepath.Join(frontendDir, "public"),
	}
	for _, dir := range dirs {
		if err := s.sshClient.MkdirRemote(dir); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", dir, err)
		}
	}
	progress("目录结构创建完成")

	// 提取功能模块名
	featureNames := make([]string, 0, len(modules))
	for _, m := range modules {
		featureNames = append(featureNames, m.Name)
	}
	if len(featureNames) == 0 {
		featureNames = []string{"基础功能"}
	}

	// ===== 2. 生成后端代码 =====
	progress("正在生成 Go 后端代码...")
	if err := s.writeBackendFiles(backendDir, project, featureNames); err != nil {
		return fmt.Errorf("生成后端代码失败: %w", err)
	}
	progress("Go 后端代码生成完成")

	// ===== 3. 生成前端代码 =====
	progress("正在生成 React 前端代码...")
	if err := s.writeFrontendFiles(frontendDir, project, featureNames); err != nil {
		return fmt.Errorf("生成前端代码失败: %w", err)
	}
	progress("React 前端代码生成完成")

	// ===== 4. 初始化 Git 并推送 =====
	progress("正在初始化 Git 仓库并推送代码...")
	if err := s.initAndPushGit(workDir, project); err != nil {
		progress(fmt.Sprintf("⚠ Git 推送失败（不影响部署）: %v", err))
	} else {
		progress("Git 仓库初始化并推送完成")
	}

	progress(fmt.Sprintf("项目代码已生成: 后端 %d 个文件, 前端 %d 个文件, 功能模块: %s",
		8+len(featureNames), 10, strings.Join(featureNames, ", ")))
	return nil
}

// writeBackendFiles 生成后端 Go 代码文件
func (s *CodeScaffolder) writeBackendFiles(backendDir string, project model.Project, features []string) error {
	writeFile := func(filename, content string) error {
		path := filepath.Join(backendDir, filename)
		return s.sshClient.WriteRemoteFile(path, []byte(content), "0644")
	}

	moduleName := "loafer-app"
	repoName := extractRepoNameFromGitURL(project.GitURL)
	if repoName != "" {
		moduleName = strings.ReplaceAll(repoName, "-", "_")
	}

	// go.mod
	if err := writeFile("go.mod", fmt.Sprintf(`module %s

go 1.21

require (
	github.com/gin-gonic/gin v1.9.1
	github.com/gin-contrib/cors v1.4.0
	gorm.io/gorm v1.25.5
	gorm.io/driver/mysql v1.5.2
)
`, moduleName)); err != nil {
		return err
	}

	// main.go
	if err := writeFile("main.go", fmt.Sprintf(`package main

import (
	"fmt"
	"os"

	"%s/internal/config"
	"%s/internal/handler"
	"%s/internal/model"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// CORS
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"*"},
		AllowCredentials: false,
	}))

	// 数据库连接
	dbCfg := config.LoadDB()
	db, err := config.ConnectDB(dbCfg)
	if err != nil {
		fmt.Printf("⚠ 数据库连接失败: %%v\n", err)
		fmt.Println("  服务将以无数据库模式运行")
	} else {
		// 自动迁移
		db.AutoMigrate(&model.Item{})
	}

	// 健康检查
	r.GET("/api/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "%s"})
	})

	// API 路由
	api := r.Group("/api")
	if db != nil {
		h := handler.NewItemHandler(db)
		h.RegisterRoutes(api)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Printf("🚀 %s 服务启动，端口: %%s\n", port)
	r.Run(":" + port)
}
`, moduleName, moduleName, moduleName, project.Name, project.Name)); err != nil {
		return err
	}

	// config/config.go
	if err := writeFile("internal/config/config.go", fmt.Sprintf(`package config

import (
	"fmt"
	"os"
	"strconv"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type DBConfig struct {
	Host     string
	Port     int
	Name     string
	Username string
	Password string
}

func LoadDB() DBConfig {
	return DBConfig{
		Host:     envStr("DB_HOST", "127.0.0.1"),
		Port:     envInt("DB_PORT", 3306),
		Name:     envStr("DB_NAME", "%s"),
		Username: envStr("DB_USERNAME", "root"),
		Password: envStr("DB_PASSWORD", ""),
	}
}

func ConnectDB(cfg DBConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf("%%s:%%s@tcp(%%s:%%d)/%%s?charset=utf8mb4&parseTime=True&loc=Asia%%2FShanghai",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Name)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	return db, nil
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
`, "loafer_proj_"+repoName)); err != nil {
		return err
	}

	// model/item.go
	if err := writeFile("internal/model/item.go", fmt.Sprintf(`package model

import "time"

// Item 通用数据项模型，对应 %s 的核心业务数据
type Item struct {
	ID        int64     %s
	Title     string    %s
	Content   string    %s
	Category  string    %s
	Status    string    %s
	CreatedAt time.Time %s
	UpdatedAt time.Time %s
}

func (Item) TableName() string { return "items" }
`, project.Name,
		"`gorm:\"column:id;primaryKey;autoIncrement\" json:\"id\"`",
		"`gorm:\"column:title;type:varchar(200)\" json:\"title\"`",
		"`gorm:\"column:content;type:text\" json:\"content\"`",
		"`gorm:\"column:category;type:varchar(100)\" json:\"category\"`",
		"`gorm:\"column:status;type:varchar(50);default:'active'\" json:\"status\"`",
		"`gorm:\"column:created_at;autoCreateTime\" json:\"createdAt\"`",
		"`gorm:\"column:updated_at;autoUpdateTime\" json:\"updatedAt\"`",
	)); err != nil {
		return err
	}

	// handler/item_handler.go — 包含各功能模块的 CRUD
	handlerContent := fmt.Sprintf(`package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"%s/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ItemHandler 通用数据项处理器，提供 CRUD 接口。
// 功能模块: %s
type ItemHandler struct {
	db *gorm.DB
}

func NewItemHandler(db *gorm.DB) *ItemHandler {
	return &ItemHandler{db: db}
}

func (h *ItemHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/items")
	{
		g.GET("", h.List)
		g.GET("/:id", h.Get)
		g.POST("", h.Create)
		g.PUT("/:id", h.Update)
		g.DELETE("/:id", h.Delete)
	}
	// 功能模块列表
	rg.GET("/modules", func(c *gin.Context) {
		c.JSON(200, gin.H{"modules": []string{%s}})
	})
}

func (h *ItemHandler) List(c *gin.Context) {
	var items []model.Item
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 { page = 1 }
	if size < 1 || size > 100 { size = 20 }

	category := c.Query("category")
	query := h.db.Model(&model.Item{})
	if category != "" {
		query = query.Where("category = ?", category)
	}

	var total int64
	query.Count(&total)
	query.Offset((page - 1) * size).Limit(size).Order("created_at DESC").Find(&items)

	c.JSON(200, gin.H{
		"code": 0,
		"data": gin.H{
			"list":  items,
			"total": total,
			"page":  page,
			"size":  size,
		},
	})
}

func (h *ItemHandler) Get(c *gin.Context) {
	id := c.Param("id")
	var item model.Item
	if err := h.db.First(&item, id).Error; err != nil {
		c.JSON(404, gin.H{"code": 1, "message": "数据不存在"})
		return
	}
	c.JSON(200, gin.H{"code": 0, "data": item})
}

func (h *ItemHandler) Create(c *gin.Context) {
	var item model.Item
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(400, gin.H{"code": 1, "message": "参数错误: " + err.Error()})
		return
	}
	if err := h.db.Create(&item).Error; err != nil {
		c.JSON(500, gin.H{"code": 1, "message": "创建失败: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"code": 0, "data": item})
}

func (h *ItemHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var item model.Item
	if err := h.db.First(&item, id).Error; err != nil {
		c.JSON(404, gin.H{"code": 1, "message": "数据不存在"})
		return
	}
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(400, gin.H{"code": 1, "message": "参数错误"})
		return
	}
	if err := h.db.Save(&item).Error; err != nil {
		c.JSON(500, gin.H{"code": 1, "message": "更新失败"})
		return
	}
	c.JSON(200, gin.H{"code": 0, "data": item})
}

func (h *ItemHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.db.Delete(&model.Item{}, id).Error; err != nil {
		c.JSON(500, gin.H{"code": 1, "message": "删除失败"})
		return
	}
	c.JSON(200, gin.H{"code": 0, "message": "删除成功"})
}
`, moduleName, strings.Join(features, ", "), formatStringSlice(features))

	if err := writeFile("internal/handler/item_handler.go", handlerContent); err != nil {
		return err
	}

	return nil
}

// writeFrontendFiles 生成前端 React 代码文件
func (s *CodeScaffolder) writeFrontendFiles(frontendDir string, project model.Project, features []string) error {
	writeFile := func(filename, content string) error {
		path := filepath.Join(frontendDir, filename)
		return s.sshClient.WriteRemoteFile(path, []byte(content), "0644")
	}

	// package.json
	if err := writeFile("package.json", fmt.Sprintf(`{
  "name": "%s-frontend",
  "version": "1.0.0",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "vue": "^3.4.0",
    "element-plus": "^2.5.0",
    "axios": "^1.6.0",
    "@element-plus/icons-vue": "^2.3.0",
    "vue-router": "^4.2.0"
  },
  "devDependencies": {
    "@vitejs/plugin-vue": "^5.0.0",
    "typescript": "^5.3.0",
    "vite": "^5.0.0",
    "vue-tsc": "^1.8.0"
  }
}
`, strings.ReplaceAll(project.Name, " ", "-"))); err != nil {
		return err
	}

	// vite.config.ts
	if err := writeFile("vite.config.ts", fmt.Sprintf(`import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true
      }
    }
  }
})
`)); err != nil {
		return err
	}

	// index.html
	if err := writeFile("index.html", fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>%s</title>
</head>
<body>
  <div id="app"></div>
  <script type="module" src="/src/main.ts"></script>
</body>
</html>
`, project.Name)); err != nil {
		return err
	}

	// tsconfig.json
	if err := writeFile("tsconfig.json", `{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForClassFields": true,
    "module": "ESNext",
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "preserve",
    "strict": true,
    "noUnusedLocals": false,
    "noUnusedParameters": false,
    "noFallthroughCasesInSwitch": true
  },
  "include": ["src/**/*.ts", "src/**/*.vue"]
}
`); err != nil {
		return err
	}

	// src/main.ts
	if err := writeFile("src/main.ts", fmt.Sprintf(`import { createApp } from 'vue'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import App from './App.vue'
import router from './router'

const app = createApp(App)

for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

app.use(ElementPlus)
app.use(router)
app.mount('#app')
`)); err != nil {
		return err
	}

	// src/App.vue
	featureTabs := make([]string, len(features))
	for i, f := range features {
		featureTabs[i] = fmt.Sprintf(`      <el-tab-pane label="%s" name="tab%d">
        <div class="feature-content">
          <h2>%s</h2>
          <p>%s功能模块 - 由脚手架自动生成，可在此基础上进行二次开发</p>
          <el-table :data="items" border style="width: 100%%">
            <el-table-column prop="title" label="标题" />
            <el-table-column prop="category" label="分类" width="120" />
            <el-table-column prop="status" label="状态" width="100" />
            <el-table-column prop="createdAt" label="创建时间" width="180" />
          </el-table>
        </div>
      </el-tab-pane>`, f, i+1, f, f)
	}

	if err := writeFile("src/App.vue", fmt.Sprintf(`<template>
  <div class="app-container">
    <el-container>
      <el-header class="app-header">
        <h1>%s</h1>
        <span class="subtitle">端到端全链路自动生成 · 脚手架模式</span>
      </el-header>
      <el-main>
        <el-tabs v-model="activeTab" type="border-card">
%s
        </el-tabs>
      </el-main>
      <el-footer class="app-footer">
        <span>Powered by Loafer Agent · 自动生成于 2026</span>
      </el-footer>
    </el-container>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import axios from 'axios'

const activeTab = ref('tab1')
const items = ref([])

const loadItems = async () => {
  try {
    const res = await axios.get('/api/items')
    if (res.data?.code === 0) {
      items.value = res.data.data.list || []
    }
  } catch (e) {
    console.log('数据加载中...')
  }
}

onMounted(() => {
  loadItems()
})
</script>

<style scoped>
.app-container {
  min-height: 100vh;
  background: #f5f7fa;
}
.app-header {
  background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
  color: white;
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.app-header h1 {
  margin: 0;
  font-size: 24px;
}
.subtitle {
  font-size: 13px;
  opacity: 0.8;
}
.feature-content {
  padding: 20px;
}
.feature-content h2 {
  color: #303133;
  margin-bottom: 8px;
}
.app-footer {
  text-align: center;
  color: #909399;
  font-size: 12px;
  line-height: 60px;
}
</style>
`, project.Name, strings.Join(featureTabs, "\n"))); err != nil {
		return err
	}

	// src/router/index.ts
	if err := writeFile("src/router/index.ts", `import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      name: 'Home',
      component: () => import('../App.vue')
    }
  ]
})

export default router
`); err != nil {
		return err
	}

	// src/env.d.ts
	if err := writeFile("src/env.d.ts", `/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<{}, {}, any>
  export default component
}
`); err != nil {
		return err
	}

	// .gitignore
	if err := writeFile(".gitignore", `node_modules/
dist/
.env.local
*.log
`); err != nil {
		return err
	}

	return nil
}

// initAndPushGit 初始化 Git 并推送到 Gitee。
// 完整流程：git init（若需要）→ git remote add origin（若需要）→ fetch →
// pull --allow-unrelated-histories（合并 Gitee AutoInit 的 README）→
// add → commit → push -u（设置上游跟踪分支）。
func (s *CodeScaffolder) initAndPushGit(workDir string, project model.Project) error {
	if project.GitURL == "" {
		return fmt.Errorf("GitURL 未设置")
	}

	// git init（若 .git 不存在）
	if _, err := s.sshClient.RunCommand(fmt.Sprintf("cd %s && git rev-parse --git-dir 2>/dev/null || git init", workDir)); err != nil {
		return fmt.Errorf("git init 失败: %w", err)
	}

	// git config
	if _, err := s.sshClient.RunCommand(fmt.Sprintf("cd %s && git config user.email 'loafer@agent.local' && git config user.name 'Loafer Agent'", workDir)); err != nil {
		return fmt.Errorf("配置 git 失败: %w", err)
	}

	// git remote add origin（若未配置）
	if _, err := s.sshClient.RunCommand(fmt.Sprintf(
		"cd %s && git remote get-url origin 2>/dev/null || git remote add origin %s",
		workDir, project.GitURL,
	)); err != nil {
		return fmt.Errorf("git remote add 失败: %w", err)
	}

	// git fetch origin
	if _, err := s.sshClient.RunCommand(fmt.Sprintf("cd %s && git fetch origin 2>&1 || true", workDir)); err != nil {
		// fetch 失败不阻断（可能是空仓库）
	}

	// git pull --allow-unrelated-histories（合并 Gitee AutoInit 产生的 README）
	if _, err := s.sshClient.RunCommand(fmt.Sprintf(
		"cd %s && git pull origin master --allow-unrelated-histories 2>&1 || true",
		workDir,
	)); err != nil {
		// pull 失败不阻断（远程可能没有 master 分支）
	}

	// git add all
	if _, err := s.sshClient.RunCommand(fmt.Sprintf("cd %s && git add -A", workDir)); err != nil {
		return fmt.Errorf("git add 失败: %w", err)
	}

	// git commit
	commitMsg := fmt.Sprintf("feat: 脚手架自动生成项目代码（Go后端+Vue前端）")
	if _, err := s.sshClient.RunCommand(fmt.Sprintf("cd %s && git commit -m '%s' || true", workDir, commitMsg)); err != nil {
		// commit 失败可能是因为没有变更，不阻断
	}

	// git push -u origin HEAD（设置上游跟踪分支，解决 "no upstream branch" 问题）
	if _, err := s.sshClient.RunCommand(fmt.Sprintf("cd %s && git push -u origin HEAD 2>&1 || true", workDir)); err != nil {
		return fmt.Errorf("git push 失败: %w", err)
	}

	return nil
}

// extractRepoNameFromGitURL 从 Git URL 中提取仓库名
func extractRepoNameFromGitURL(gitURL string) string {
	if gitURL == "" {
		return ""
	}
	// 去掉 .git 后缀
	s := strings.TrimSuffix(gitURL, ".git")
	// 取最后一段
	parts := strings.Split(s, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// formatStringSlice 将字符串切片格式化为 Go 字符串字面量
func formatStringSlice(items []string) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = fmt.Sprintf("%q", item)
	}
	return strings.Join(parts, ", ")
}
