// Package main 生产构建入口包。
//
// 本文件是 phosche 的生产环境主入口，与 embed.go 同属于根包 main。
// 生产模式下，embed.go 中通过 //go:embed 嵌入的前端静态资源（webDist）
// 被传递给 app.Run()，由内置 HTTP 服务器直接提供 SPA 静态文件服务，
// 无需依赖独立的前端开发服务器。
package main

import (
	"flag"

	"github.com/zwh8800/phosche/internal/app"
)

// main 是生产环境程序入口函数。
//
// 1. 解析 -config 命令行参数，指定配置文件路径（默认 config.yaml）。
// 2. 调用 app.Run(webDist, *configPath) 启动服务，其中 webDist 是
//    embed.go 中嵌入的前端静态文件系统，实现生产模式下前后端一体部署。
func main() {
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	flag.Parse()

	app.Run(webDist, *configPath)
}
