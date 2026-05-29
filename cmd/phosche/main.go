// Package main 开发模式入口包。
//
// 本文件位于 cmd/phosche/main.go，是 phosche 的开发环境主入口。
// 开发模式下，传递给 app.Run() 的第一个参数为 nil（不嵌入前端静态资源），
// 前端页面由独立的 Vite 开发服务器提供，支持热重载等开发体验。
package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/zwh8800/phosche/internal/app"
)

// main 是开发环境程序入口函数。
//
// 1. 解析 -config 命令行参数，指定配置文件路径（默认 config.yaml）。
// 2. 配置 slog 日志系统：使用 JSON 格式输出到标准输出，日志级别为 Info。
// 3. 调用 app.Run(nil, *configPath) 启动服务，传入 nil 表示不嵌入前端
//    静态文件，由 Vite 开发服务器处理前端请求，实现前后端分离开发。
func main() {
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	app.Run(nil, *configPath)
}
