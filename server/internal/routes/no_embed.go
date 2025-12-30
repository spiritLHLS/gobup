//go:build !embed
// +build !embed

package routes

import (
	"github.com/gin-gonic/gin"
)

// embedEnabled 标记是否启用了前端嵌入
const embedEnabled = false

// setupStaticRoutes 设置静态文件路由（非嵌入模式）
// 开发模式下，前端将独立运行在 Vite 开发服务器上
func setupStaticRoutes(router *gin.Engine) error {
	// 非嵌入模式下，仅提供静态文件服务（如果有的话）
	// 用于开发环境，前端独立部署
	router.Static("/static", "./web/dist")
	router.StaticFile("/", "./web/dist/index.html")

	return nil
}

// isAPIPath 检查路径是否为API路径
func isAPIPath(path string) bool {
	return false // 在非嵌入模式下不需要此函数
}
