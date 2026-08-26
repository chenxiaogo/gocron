package handler

import (
	"context"

	"scheduler-platform/internal/executor"
	"scheduler-platform/internal/scheduler"
	"scheduler-platform/internal/service"
	"scheduler-platform/internal/store"
	"scheduler-platform/pkg/app/config"
	"scheduler-platform/pkg/auth"
)

// Logic 聚合所有业务 Handler。
type Logic struct {
	Dashboard *DashboardHandler
	Task      *TaskHandler
	Execution *ExecutionHandler
	Node      *NodeHandler
	User      *UserHandler
	Role      *RoleHandler
	Menu      *MenuHandler
	Api       *ApiHandler
	Notify    *NotifyHandler
	Auth      *AuthHandler
	// JWT 供路由层挂载鉴权中间件
	JWT *auth.JWT
}

// NewLogic 组装全部 Handler。
func NewLogic(ctx context.Context, conf *config.Config, store *store.Store, sched *scheduler.Scheduler, ex *executor.Executor) *Logic {
	svc := service.NewService(store)
	jwt := auth.New(conf.Security.JWTSecret, conf.Security.TokenExpireHours)
	return &Logic{
		Dashboard: NewDashboardHandler(svc),
		Task:      NewTaskHandler(store, sched, ex),
		Execution: NewExecutionHandler(store),
		Node:      NewNodeHandler(store),
		User:      NewUserHandler(store),
		Role:      NewRoleHandler(store),
		Menu:      NewMenuHandler(store),
		Api:       NewApiHandler(store),
		Notify:    NewNotifyHandler(store),
		Auth:      NewAuthHandler(store, conf.Security.JWTSecret, conf.Security.TokenExpireHours),
		JWT:       jwt,
	}
}
