package shutdown

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Hook 管理应用生命周期，监听系统信号，执行优雅退出。
type Hook struct {
	signals    []os.Signal
	timeout    time.Duration
	closeFuncs []func(ctx context.Context)
	mu         sync.Mutex
	done       chan struct{}
}

// NewHook 创建一个新的生命周期 Hook。
func NewHook() *Hook {
	return &Hook{
		signals: []os.Signal{syscall.SIGINT, syscall.SIGTERM},
		timeout: 30 * time.Second,
		done:    make(chan struct{}),
	}
}

// WithSignals 设置监听的系统信号。
func (h *Hook) WithSignals(signals ...os.Signal) *Hook {
	if len(signals) > 0 {
		h.signals = signals
	}
	return h
}

// WithTimeout 设置优雅退出超时时间。
func (h *Hook) WithTimeout(timeout time.Duration) *Hook {
	h.timeout = timeout
	return h
}

// AddCloseFunc 注册关闭时执行的清理函数。
func (h *Hook) AddCloseFunc(fn func(ctx context.Context)) *Hook {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closeFuncs = append(h.closeFuncs, fn)
	return h
}

// Run 启动应用，阻塞直到收到信号或 run 函数返回错误。
func (h *Hook) Run(run func(ctx context.Context) error) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 信号监听
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, h.signals...)
	defer signal.Stop(sigCh)

	// 启动应用
	runErr := make(chan error, 1)
	go func() {
		runErr <- run(ctx)
	}()

	select {
	case err := <-runErr:
		if err != nil {
			return err
		}
		h.shutdown(ctx)
		return nil
	case <-sigCh:
		fmt.Println("收到退出信号，开始优雅退出...")
		h.shutdown(ctx)
		<-runErr
		return nil
	}
}

// shutdown 执行所有清理函数，带超时保护。
func (h *Hook) shutdown(ctx context.Context) {
	h.mu.Lock()
	funcs := make([]func(ctx context.Context), len(h.closeFuncs))
	copy(funcs, h.closeFuncs)
	h.mu.Unlock()

	shutdownCtx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	var wg sync.WaitGroup
	for _, fn := range funcs {
		wg.Add(1)
		go func(f func(context.Context)) {
			defer wg.Done()
			f(shutdownCtx)
		}(fn)
	}
	wg.Wait()
}
