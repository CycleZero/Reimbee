// Package main 嵌入前端静态文件，实现单二进制部署
package main

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed web/dist/*
var embeddedFrontend embed.FS

// frontendFS 解包后的前端文件系统（去除 web/dist 前缀）
var frontendFS fs.FS

func init() {
	var err error
	frontendFS, err = fs.Sub(embeddedFrontend, "web/dist")
	if err != nil {
		panic("无法解包前端文件: " + err.Error())
	}
}

// spaFileSystem SPA 兜底：文件不存在时返回 index.html
type spaFileSystem struct{ fs.FS }

func (s spaFileSystem) Open(name string) (fs.File, error) {
	f, err := s.FS.Open(name)
	if err != nil {
		return s.FS.Open("index.html")
	}
	return f, nil
}

// ServeFrontend 注册前端静态文件路由和 SPA 兜底
func ServeFrontend(e *gin.Engine) {
	if os.Getenv("DEV_FRONTEND") == "true" {
		return
	}

	fileServer := http.FileServer(http.FS(spaFileSystem{frontendFS}))

	e.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api") || strings.HasPrefix(path, "/admin") {
			c.JSON(http.StatusNotFound, gin.H{"error": "接口不存在"})
			return
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}
