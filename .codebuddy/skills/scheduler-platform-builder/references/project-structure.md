# go-proxy 项目结构参考

本文件详细记录 `go-proxy` 项目的结构、分层规范、关键实现模式，供构建调度平台后端时参考。

> **重要**：在调度平台项目中，以下所有 Go 后端代码（`cmd/`、`internal/`、`pkg/`、`go.mod`、`go.sum`）都必须统一放在 `server/` 目录下，与前端 `web/` 目录平级，形成**前后端分离**布局。具体见 `SKILL.md` 的"项目结构"章节。

## 0. 当前落地实例对照（/home/chenguohong/go/src/gocron）

go-proxy 是参考蓝本，但调度平台当前**已落地**的结构做了一定收敛（以实际代码为准，见 `SKILL.md` "项目结构"章节）：

| go-proxy 概念 | 调度平台落地 |
|---|---|
| 模块名 | `scheduler-platform`（**import 基于模块名而非目录路径**，如 `scheduler-platform/internal/store`） |
| `internal/handler` | 保留，`enter.go` 聚合入口 + auth/dashboard/execution/node/task |
| `internal/repository` | **收敛为 `internal/store`**（单 Store 结构封装全部 GORM 数据访问 + 仅初始化 admin 账号） |
| `internal/service` | 保留（Dashboard 聚合、排序、计数） |
| `internal/middleware` | 新增 `internal/middleware/auth.go`（JWT 鉴权） |
| `internal/scheduler` | 保留（robfig/cron 标准 5 字段 parser） |
| `internal/model` | 保留（Task/Execution/TaskLog/Node/User） |
| `pkg/database` | 新增（SQLite 默认 / MySQL 可选 dialector 切换） |
| `pkg/auth` | 新增（JWT HS256 签发/解析） |
| `pkg/app/static.go` | 新增（托管 `../web/dist`，SPA fallback） |
| 数据源 | **默认 SQLite**（`glebarez/sqlite` 纯 Go，`CGO_ENABLED=0`），MySQL 可选 |

后续在本技能下开发，一律采用上述落地结构（store 而非 repository、SQLite 默认、模块名 scheduler-platform）。

## 1. 目录总览

```
go-proxy/
├── cmd/
│   ├── main.go            # 应用入口
│   ├── conf.d/            # 配置文件目录
│   ├── cert.d/            # TLS 证书目录
│   └── logs/              # 日志目录
├── internal/
│   └── handler/           # Handler 层
│       ├── enter.go       # 聚合入口（NewLogic 统一组装各 Handler）
│       └── handler.go     # 具体业务 Handler
├── pkg/
│   ├── app/               # 应用装配
│   │   ├── server.go      # ApiServer：启动/优雅停止/热加载
│   │   ├── config/        # 配置加载（viper + yaml + 热更新）
│   │   └── route/         # 路由注册
│   ├── ecode/             # 错误码体系（多语言）
│   ├── ilog/              # 日志封装（zap）
│   └── shutdown/          # 优雅退出 Hook
├── proto/                 # gRPC proto 定义
├── deploy/                # 部署相关（k8s yaml、脚本）
├── docs/                  # Swagger 文档
├── go.mod
├── Makefile
└── entrypoint.sh
```

## 2. cmd/main.go 入口模式

使用 `shutdown.Hook` 管理应用生命周期，注册信号监听与清理函数：

```go
func main() {
    hook := shutdown.NewHook().
        WithSignals(syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP).
        WithTimeout(60 * time.Second)

    hook.AddCloseFunc(func(ctx context.Context) {
        log.Println("Cleaning up additional resources...")
    })

    if err := hook.Run(app.NewServer); err != nil {
        log.Fatalf("Application failed: %v", err)
    }
    log.Println("Application exited successfully")
}
```

关键点：
- `hook.Run(app.NewServer)` 接收一个 `func(ctx) error`，统一管理启动与退出。
- 优雅退出具备超时保护（60s）。
- 支持多信号监听（SIGINT/SIGTERM/SIGHUP）。

## 3. pkg/app 应用装配

### server.go

`ApiServer` 封装 HTTP 服务器与依赖，具备**配置热加载**能力：

```go
type ApiServer struct {
    server   *http.Server
    listener net.Listener
    config   *config.Config
    shutdown shutdown.Hook
    running  atomic.Bool
}

func NewServer(ctx context.Context) error {
    ecode.Register(ecode.EcodeMessageMap)   // 注册错误码
    conf, err := config.TryLoadFromDisk("") // 加载配置
    server, err := createServer(ctx, conf)  // 创建服务器
    configCh := config.WatchConfigChange()  // 监听配置变化
    // ...
}
```

核心机制：
- **热加载**：监听配置文件变化，创建新 server → 停旧 server，实现无缝热更新。
- **SO_REUSEPORT**：`createReusableListener` 设置 `SO_REUSEADDR + SO_REUSEPORT`，避免 TIME_WAIT 端口占用。
- **TLS**：`setupTLS` 支持加载证书、最低 TLS1.2。
- **运行时**：`setupRuntime` 将 GOMAXPROCS 设为 `NumCPU()-1`，留 1 核给系统。

### config/

使用 `viper` + yaml，支持磁盘加载 + 文件监听热更新：

```go
type Config struct {
    Host        string          `yaml:"host"`
    Env         string          `yaml:"env"`
    Gateway     Gateway         `yaml:"gateway"`
    Security    Security        `yaml:"security"`
    Logger      Logger          `yaml:"logger"`
}
```

关键点：
- `config` 单例封装，含 `loadOnce`/`watchOnce` 同步原语。
- **MD5 内容比对**：`hasConfigContentChanged` 用 MD5 判断配置是否真正变化，避免重复触发。
- **防抖**：`changeTimer` 合并多次 fsnotify 事件。
- `DeepCopy()` 保证并发安全返回副本。
- `Validate()` 提供完整配置校验（host/paths/gateway/TLS/CORS/logger）。

### route/

路由注册采用 **Registrar 接口**，业务 Handler 实现该接口即可自动注册：

```go
type Registrar interface {
    RegisterRoute(gin.IRoutes)
}

// 在 setupRoutes 中统一注册
for _, reg := range []Registrar{
    r.logic.Goproxy,
} {
    reg.RegisterRoute(r.engine.Group(""))
}
```

中间件链（Route 构造函数中挂载）：
- 日志中间件（ProductionLogger / DebugLogger 按环境区分）
- `gin.Recovery()` 崩溃恢复
- CORS 中间件
- 时间检查中间件
- 404 处理 `NoRoute`

## 4. internal/handler Handler 层

### enter.go —— 聚合入口

```go
type Logic struct {
    Goproxy *goproxy
}

func NewLogic(ctx context.Context, conf *config.Config, paths []config.PathConfig) (*Logic, error) {
    handler, err := NewGoproxy(ctx, conf, paths)
    return &Logic{Goproxy: handler}, nil
}
```

### handler.go —— 具体 Handler

Handler 实现 `Registrar` 接口，方法使用 Swagger 注解：

```go
func (s *goproxy) RegisterRoute(router gin.IRoutes) {
    router.GET("/", s.Index)
}

// @Summary Get index page
// @Description ...
// @Tags goproxy
// @Produce html
// @Success 200 {string} string
// @Router / [get]
func (s *goproxy) Index(c *gin.Context) {
    // ...
    c.HTML(http.StatusOK, "index", data)
}
```

## 5. pkg/ecode 错误码体系

支持**中英文多语言**的错误码系统：

```go
var (
    _messages atomic.Value
    currentLangIndex int32 = 0
    supportedLangs = map[string]int32{"zh-CN": 0, "en-US": 1}
)

type Code int

func (e Code) Message() string    { return e.getMessage(0) } // 中文
func (e Code) MessageEn() string  { return e.getMessage(1) } // 英文
func (e Code) MessageForLang() string { return e.getMessage(int(atomic.LoadInt32(&currentLangIndex))) }

func Register(cm map[int][]string) { _messages.Store(cm) }
func New(e int) Code { return add(e) }   // 注册时校验重复
```

核心机制：
- 通过 `_messages` 存储 `map[code][]string{中文, 英文}`。
- `SetLanguage(lang)` 全局切换语言。
- `Cause(err)` 从 error 链中提取错误码。
- `Equal`/`EqualError` 判断错误码相等。
- **每个业务错误码用 `New()` 注册，重复会 panic**。

## 6. pkg/ilog 日志封装

基于 zap 的结构化日志，支持：
- `WithAppName` 应用名
- `WithLogLevel` 日志级别
- `WithWriteToConsole` / `WithWriteToFile` 控制台/文件输出
- `WithLogPath` + lumberjack 日志轮转

## 7. pkg/shutdown 优雅退出

链式 API 构建退出 Hook：

```go
hook := shutdown.NewHook().
    WithSignals(syscall.SIGINT, syscall.SIGTERM).
    WithTimeout(30 * time.Second)

hook.AddCloseFunc(func(ctx context.Context) { /* 清理 */ })
hook.Run(func(ctx context.Context) error { /* 启动逻辑 */ })
```

## 8. 应用到调度平台的建议

构建调度平台时，复用以下 go-proxy 模式（**所有路径均在 `server/` 目录下**）：

1. **入口**：`server/cmd/main.go` 用 shutdown.Hook 管理生命周期。
2. **应用装配**：`server/pkg/app/server.go` 负责 Gin 引擎、中间件、监听、优雅停止、AutoMigrate。
3. **配置**：`server/pkg/app/config` 用 viper 支持热更新 + 校验 + `EnsureConfigFile` 自动生成默认配置。
4. **路由**：`server/pkg/app/route` 用 Registrar 接口，Handler 实现 `RegisterRoute`，受保护分组挂 JWT 中间件。
5. **错误码**：`server/pkg/ecode` 多语言错误码体系。
6. **日志**：`server/pkg/ilog` zap 封装。
7. **分层**：Handler 不写业务逻辑，委托给 Service → Store → GORM。
8. **中间件**：鉴权（JWT）、CORS、日志、Recovery。

对调度平台，落地结构（以 `/home/chenguohong/go/src/gocron` 为准）：
- `server/internal/middleware/` JWT 鉴权中间件。
- `server/internal/service/`、`server/internal/store/`（数据仓库，替代 repository）、`server/internal/model/` 分层。
- `server/internal/scheduler/`（robfig/cron 标准 5 字段 parser）。
- `server/pkg/database/`（SQLite 默认 / MySQL 可选）、`server/pkg/auth/`（JWT）。
- `server/pkg/app/static.go`（托管 `../web/dist`）。
- `server/go.mod` 模块名为 `scheduler-platform`（不是 `<项目名>/server`），import 直接用 `scheduler-platform/xxx`。
