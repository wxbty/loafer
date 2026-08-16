package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"loafer-agent/internal/config"
	"loafer-agent/internal/db"
	"loafer-agent/internal/router"

	"github.com/joho/godotenv"
)

func main() {
	// 加载 .env 文件（不存在时静默跳过，依赖系统环境变量）
	_ = godotenv.Load()

	cfg := config.Load()

	database, err := db.Init(&cfg.Database)
	if err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}

	r := router.Setup(cfg, database)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: r,
	}

	go func() {
		log.Printf("服务启动，监听端口 %d", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("服务启动失败: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("正在关闭服务...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("服务关闭异常: %v", err)
	}
	log.Println("服务已停止")
}
