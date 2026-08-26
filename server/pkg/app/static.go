package app

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"scheduler-platform/pkg/app/config"
	"scheduler-platform/pkg/app/route"
	"scheduler-platform/pkg/ilog"
)

// attachStatic 若存在前端构建产物 web/dist，则挂载为静态资源并提供 SPA fallback。
// 后端代码位于 server/ 目录，前端 web/ 在项目根目录，因此前端构建产物相对路径为 ../web/dist。
func attachStatic(r *route.Route, conf *config.Config) {
	dist := "../web/dist"
	if _, err := os.Stat(dist); err != nil {
		ilog.Warnf("前端构建产物不存在 %s，仅提供 API 服务", dist)
		return
	}

	fs := http.Dir(dist)
	fileServer := http.FileServer(fs)

	r.AttachHandler(func(w http.ResponseWriter, req *http.Request) {
		path := strings.TrimPrefix(req.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		full := filepath.Join(dist, path)
		// 若请求的文件不存在，fallback 到 index.html（SPA）
		if !pathExists(full) {
			req.URL.Path = "/"
			fileServer.ServeHTTP(w, req)
			return
		}
		fileServer.ServeHTTP(w, req)
	})
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
