// Package main 是 phosche 的生产环境入口包。
//
// 本文件通过 //go:embed 指令将前端构建产物（web/dist 目录）嵌入到
// Go 二进制文件中，使得单个二进制文件即可同时提供后端 API 和前端 SPA 服务。
// 构建前端（make build-frontend）后生效；开发模式下（dev_mode: true）
// 前端由 Vite 独立运行，不使用此嵌入资源。
package main

import "embed"

//go:embed web/dist
// webDist 持有编译后的前端静态资源文件系统。
// 仅在执行 make build-frontend 构建前端后有效；
// 开发模式下（dev_mode: true）前端由 Vite 独立运行，不使用此嵌入资源。
var webDist embed.FS
