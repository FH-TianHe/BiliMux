package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/FH-TianHe/BiliMux/config"
	"github.com/FH-TianHe/BiliMux/handlers"
	"github.com/FH-TianHe/BiliMux/manager"
	"github.com/FH-TianHe/BiliMux/utils"
)

var (
	proxyAddr  = flag.String("proxy", "localhost:8080", "代理服务器监听地址")
	statsPort  = flag.Int("stats", 8081, "统计服务端口")
	maxConns   = flag.Int("max-conns", 1000, "最大并发连接数")
	configFile = flag.String("config", "config/config.json", "配置文件路径")
)

func main() {
	flag.Parse()
	rand.Seed(time.Now().UnixNano())

	// 加载配置文件
	if err := config.LoadConfig(*configFile); err != nil {
		log.Fatalf("加载配置文件失败: %v", err)
	}

	log.Printf("启动B站直播间WebSocket代理服务器: %s (最大连接数: %d)", *proxyAddr, *maxConns)

	cm := manager.NewConnectionManager(*maxConns)
	defer cm.CloseAll()

	// 启动清理过期会话的goroutine
	go utils.CleanExpiredSessions()

	// 启动统计服务
	go func() {
		statsMux := http.NewServeMux()
		statsMux.HandleFunc("/stats", handlers.StatsHandler(cm))
		log.Printf("启动统计服务: http://localhost:%d/stats", *statsPort)
		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *statsPort), statsMux))
	}()

	// 添加扫码登录路由
	http.HandleFunc("/login/qrcode", handlers.QRCodeHandler)
	http.HandleFunc("/login/check", handlers.CheckLoginHandler)

	// 主代理服务
	http.HandleFunc("/", handlers.ProxyHandler(cm))

	// 启动HTTP服务器
	server := &http.Server{
		Addr:    *proxyAddr,
		Handler: nil,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP服务器启动失败: %v", err)
		}
	}()

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("接收到中断信号，关闭服务器...")

	// 优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("服务器关闭失败: %v", err)
	}

	log.Println("服务器已关闭")
}
