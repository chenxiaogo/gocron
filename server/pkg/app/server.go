package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"scheduler-platform/internal/executor"
	"scheduler-platform/internal/model"
	"scheduler-platform/internal/scheduler"
	"scheduler-platform/internal/store"
	"scheduler-platform/pkg/app/config"
	"scheduler-platform/pkg/app/route"
	"scheduler-platform/pkg/cache"
	"scheduler-platform/pkg/database"
	"scheduler-platform/pkg/ecode"
	"scheduler-platform/pkg/ilog"
	"scheduler-platform/pkg/shutdown"
)

// ApiServer 封装 HTTP 服务器。
type ApiServer struct {
	server   *http.Server
	config   *config.Config
	shutdown shutdown.Hook
}

// NewServer 组装并启动应用。
func NewServer(ctx context.Context) error {
	ecode.Register(ecode.EcodeMessageMap)

	if err := config.EnsureConfigFile(); err != nil {
		return fmt.Errorf("ensure config file: %w", err)
	}

	conf, err := config.TryLoadFromDisk("")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := setupLogger(conf); err != nil {
		return fmt.Errorf("logger setup: %w", err)
	}
	defer ilog.Sync()

	// 初始化数据库（SQLite 默认 / MySQL 可选）并建表
	db, err := database.New(&conf.Database)
	if err != nil {
		return fmt.Errorf("init database: %w", err)
	}
	if err := db.AutoMigrate(
		&model.Task{}, &model.Execution{}, &model.TaskLog{}, &model.Node{}, &model.User{},
		&model.TaskNode{}, &model.Role{}, &model.Permission{}, &model.RolePermission{}, &model.UserRole{},
		&model.Menu{}, &model.MenuBtn{}, &model.RoleMenu{}, &model.RoleMenuBtn{},
		&model.Api{}, &model.CasbinRule{}, &model.NotifySetting{},
	); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	ilog.Info("database migrated: " + conf.Database.Driver)

	// 初始化缓存（可选，Redis 未配置时跳过）
	var gc *cache.GlobalCache
	if conf.Cache.Enabled {
		cacheCfg := buildCacheConfig(conf)
		var err error
		gc, err = cache.NewGlobalCache(cacheCfg)
		if err != nil {
			ilog.Warnf("cache init failed, running without cache: %v", err)
		} else {
			ilog.Infof("cache enabled: local=%v remote=%v", conf.Cache.LocalEnabled, conf.Cache.RedisAddr)
		}
	}

	// 初始化数据仓库（落库 + 空库种子）
	dataStore := store.New(db, gc)

	// 任务执行器（按 HTTP/Shell/Cron/GRPC 类型真正执行并回写结果）
	ex := executor.New(dataStore)

	// 启动调度器（robfig/cron）
	sched := scheduler.New(dataStore, ex)
	sched.Start()

	// 创建路由
	r, err := route.NewRoute(ctx, conf, dataStore, sched, ex)
	if err != nil {
		return fmt.Errorf("create route: %w", err)
	}

	// 静态文件（前端构建产物）
	attachStatic(r, conf)

	server := &http.Server{
		Addr:         conf.ListenAddr(),
		Handler:      r.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	hook := shutdown.NewHook().
		WithSignals(os.Interrupt, os.Kill).
		WithTimeout(30 * time.Second)
	hook.AddCloseFunc(func(c context.Context) {
		ilog.Info("shutting down http server...")
		sched.Stop()
		_ = server.Shutdown(c)
	})

	ilog.Infof("scheduler platform listening on %s", conf.ListenAddr())
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server listen: %w", err)
	}
	return nil
}

func setupLogger(conf *config.Config) error {
	options := []ilog.OptionFunc{
		ilog.WithAppName(conf.App.Name),
		ilog.WithLogLevel(conf.Logger.Level),
		ilog.WithWriteToConsole(true),
	}
	if conf.Logger.FilePath != "" {
		options = append(options, ilog.WithLogPath(conf.Logger.FilePath), ilog.WithWriteToFile(true))
	}
	return ilog.NewLogger(options...)
}

// buildCacheConfig 将应用配置转换为 cache 包配置。
func buildCacheConfig(conf *config.Config) *cache.Config {
	cfg := cache.DefaultConfig()
	cfg.DefaultTTL = conf.Cache.DefaultTTL
	cfg.NotFoundTTL = conf.Cache.NotFoundTTL
	cfg.LogLevel = conf.Logger.Level

	if conf.Cache.LocalEnabled {
		cfg.Local.Enabled = true
		cfg.Local.Type = "freecache"
		cfg.Local.Size = conf.Cache.LocalSize
		cfg.Local.TTL = conf.Cache.LocalTTL
	}

	if conf.Cache.RedisAddr != "" {
		cfg.Remote.Enabled = true
		cfg.Remote.Type = "redis"
		cfg.Remote.Addresses = []string{conf.Cache.RedisAddr}
		cfg.Remote.Password = conf.Cache.RedisPassword
		cfg.Remote.DB = conf.Cache.RedisDB
		cfg.Remote.PoolSize = conf.Cache.RedisPoolSize
	}

	return cfg
}
