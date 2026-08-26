package executor

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"scheduler-platform/internal/model"
	"scheduler-platform/internal/notify"
	"scheduler-platform/internal/store"
	"scheduler-platform/pkg/ilog"
)

// Executor 任务执行器：按任务类型真正执行并回写执行结果与日志。
type Executor struct {
	store *store.Store
	http  *http.Client
	// mu 保护 runningCount
	mu sync.Mutex
	// runningCount 各任务当前并发执行数：taskID -> count
	runningCount map[int64]int
}

// 单任务并发信号量语义：超过 Concurrency 上限时拒绝执行（防止并发超限）。
// concurrencyThreshold 为 0 表示不限制（保持向后兼容）。

// New 创建执行器。
func New(s *store.Store) *Executor {
	return &Executor{
		store:        s,
		http:         &http.Client{Timeout: 0}, // 无全局超时，具体超时由每次执行按 task.Timeout 控制
		runningCount: make(map[int64]int),
	}
}

// Config 任务执行配置（解析自 Task.ExecutorConfig）。
type Config struct {
	URL     string            `json:"url"`     // 兼容旧数据：完整 http/grpc 目标地址（新逻辑用节点 address + path 拼接）
	Path    string            `json:"path"`    // http 请求路径，如 /api/xxx；与节点 address 拼成完整 URL
	Method  string            `json:"method"`  // GET/POST/PUT/DELETE
	Body    string            `json:"body"`    // http 请求体 / grpc JSON body
	Headers map[string]string `json:"headers"` // http 自定义头 / grpc metadata
	Command string            `json:"command"` // shell 命令

	// 认证（http/grpc 通用）
	AuthType string `json:"authType"` // none / basic / bearer
	Username string `json:"username"`
	Password string `json:"password"`
	Token    string `json:"token"`

	// HTTP 响应断言：状态码与响应体包含校验
	AssertStatus int    `json:"assertStatus"` // 期望状态码，0 表示不校验
	AssertBody   string `json:"assertBody"`   // 响应体需包含的子串，空表示不校验

	// gRPC 泛化调用（反射 + JSON 泛化，无需 .proto 编译）
	Service    string `json:"service"`    // 如 hello.HelloService
	MethodName string `json:"methodName"` // 如 SayHello

	// 变量提取：从响应中提取值，key 为变量名，value 为点路径（如 data.token）
	Extract map[string]string `json:"extract"`
}

// authHeader 根据认证配置生成请求头。
func (c *Config) authHeader() map[string]string {
	h := map[string]string{}
	switch strings.ToLower(c.AuthType) {
	case "basic":
		raw := c.Username + ":" + c.Password
		h["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
	case "bearer":
		if c.Token != "" {
			h["Authorization"] = "Bearer " + c.Token
		}
	}
	return h
}

// Run 异步执行任务：创建执行记录、按类型执行、回写结果与日志、按配置重试。
// ctx 携带链路 ID（来自 HTTP 请求或 tracex.NewContext），贯穿整个执行链路。
func (e *Executor) Run(ctx context.Context, task *model.Task) {
	if !e.acquire(task) {
		ilog.Trace(ctx).Warnf("task %d (%s) 并发已达上限(%d)，拒绝手动执行", task.ID, task.Name, task.Concurrency)
		return
	}
	go func() {
		defer e.release(task.ID)
		e.runSync(ctx, task, "manual")
	}()
}

// RunSync 同步执行任务，等待执行完成并返回结果。
func (e *Executor) RunSync(ctx context.Context, task *model.Task) (int64, string, string, int64) {
	if !e.acquire(task) {
		ilog.Trace(ctx).Warnf("task %d (%s) 并发已达上限(%d)", task.ID, task.Name, task.Concurrency)
		return 0, "failed", "并发已达上限", 0
	}
	defer e.release(task.ID)
	cfg := parseConfig(task.ExecutorConfig)
	execID, status, msg, duration, _ := e.executeOnce(ctx, task, cfg, "manual", 1)
	return execID, status, msg, duration
}

// Trigger 由调度器触发的异步执行。
func (e *Executor) Trigger(ctx context.Context, task *model.Task) {
	if !e.acquire(task) {
		ilog.Trace(ctx).Warnf("task %d (%s) 并发已达上限(%d)，跳过本次调度", task.ID, task.Name, task.Concurrency)
		return
	}
	go func() {
		defer e.release(task.ID)
		e.runSync(ctx, task, "cron")
	}()
}

// acquire 尝试获取并发配额。concurrency<=0 表示不限制。
func (e *Executor) acquire(task *model.Task) bool {
	if task.Concurrency <= 0 {
		return true
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.runningCount[task.ID] >= task.Concurrency {
		return false
	}
	e.runningCount[task.ID]++
	return true
}

// release 释放并发配额。
func (e *Executor) release(taskID int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.runningCount[taskID] > 1 {
		e.runningCount[taskID]--
	} else {
		delete(e.runningCount, taskID)
	}
}

// runSync 同步执行单个任务（含重试），供异步包装与内部复用。
func (e *Executor) runSync(ctx context.Context, task *model.Task, trigger string) {
	cfg := parseConfig(task.ExecutorConfig)

	// 首次执行
	_, status, msg, duration, _ := e.executeOnce(ctx, task, cfg, trigger, 1)

	// 失败重试
	attempt := 1
	for status != "success" && attempt < maxRetries(task.RetryCount) {
		attempt++
		time.Sleep(time.Duration(task.RetryInterval) * time.Second)
		_, status, msg, duration, _ = e.executeOnce(ctx, task, cfg, trigger, attempt)
	}

	ilog.Trace(ctx).Infof("task %d (%s) finished: status=%s duration=%dms attempts=%d", task.ID, task.Name, status, duration, attempt)

	// 执行结果通知（异步发送，不影响主流程）
	e.sendResultNotify(ctx, task, status, msg, duration, attempt)
}

// executeOnce 执行一次任务，回写执行记录与日志，返回执行记录ID、状态、消息、耗时、日志。
func (e *Executor) executeOnce(ctx context.Context, task *model.Task, cfg *Config, trigger string, attempt int) (int64, string, string, int64, []string) {
	now := time.Now()
	// 解析任务绑定的节点（目标机器）；无绑定则回退数据库中的默认节点
	nodes := e.store.GetTaskNodes(ctx, task.ID)
	if len(nodes) == 0 {
		if def, ok := e.store.DefaultNode(ctx); ok {
			nodes = []*model.Node{def}
		}
	}
	var workerID int64
	var workerName string
	if len(nodes) > 0 {
		workerID = nodes[0].ID
		workerName = nodes[0].Name
	}
	ex := e.store.CreateExecution(ctx, &model.Execution{
		TaskID: task.ID, TaskName: task.Name, TaskType: task.Type,
		TriggerType: trigger, WorkerID: workerID, WorkerName: workerName,
		Status: "running", Attempt: attempt, StartedAt: now,
	})
	if ex == nil {
		ilog.Trace(ctx).Errorf("create execution record failed for task %d", task.ID)
		return 0, "failed", "创建执行记录失败", 0, nil
	}

	// 执行超时控制：基于携带链路 ID 的 ctx 派生，使 trace 贯穿 HTTP 调用等子流程
	var cancel context.CancelFunc
	if task.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(task.Timeout)*time.Second)
	}

	var (
		status   = "success"
		msg      string
		duration int64
		logs     []string
	)
	switch task.Type {
	case model.TaskTypeHTTP:
		status, msg, duration, logs = e.runHTTP(ctx, cfg, nodes)
	case model.TaskTypeGRPC:
		status, msg, duration, logs = e.runGRPC(ctx, cfg, nodes)
	case model.TaskTypeShell:
		status, msg, duration, logs = e.runShell(ctx, cfg, nodes)
	default:
		// cron：纯周期触发任务，本身无动作，标记成功
		logs = []string{fmt.Sprintf("[%s] cron 周期任务触发执行", task.Type)}
	}

	finished := time.Now()
	if status == "running" {
		status = "success"
	}

	// 回写执行结果
	ex.Status = status
	ex.FinishedAt = finished
	ex.DurationMs = duration
	if msg != "" {
		ex.ErrorCode = status
		ex.ErrorMessage = msg
	}
	ex.RetryCount = attempt - 1
	e.store.UpdateExecution(ctx, ex)

	// 写入执行日志
	e.appendLogs(ctx, ex.ID, task.ID, logs)

	// 超时取消放在最后，确保 DB 操作已完成
	if cancel != nil {
		cancel()
	}

	return ex.ID, status, msg, duration, logs
}

// runHTTP 执行 HTTP 调用：对任务绑定的每个节点（目标机器）都发起请求，
// 完整 URL = http://{node.address}{cfg.path}。任一节点失败则本次执行失败。
func (e *Executor) runHTTP(ctx context.Context, cfg *Config, nodes []*model.Node) (string, string, int64, []string) {
	if len(nodes) == 0 {
		return "failed", "HTTP 任务未绑定目标节点", 0,
			[]string{"[error] HTTP 任务未绑定目标节点，无法发起请求"}
	}
	var logs []string
	var total int64
	allOK := true
	lastMsg := ""
	for _, n := range nodes {
		url := buildHTTPURL(n.Address, cfg.Path)
		logs = append(logs, fmt.Sprintf("=== 节点 %s (%s) ===", n.Name, n.Address))
		st, msg, dur, ls := e.runHTTPOnce(ctx, cfg, url)
		logs = append(logs, ls...)
		total += dur
		if st != "success" {
			allOK = false
			lastMsg = msg
		}
	}
	if allOK {
		return "success", "", total, logs
	}
	return "failed", lastMsg, total, logs
}

// runHTTPOnce 向单个 URL 发起 HTTP 调用（支持认证、响应断言与变量提取）。
func (e *Executor) runHTTPOnce(ctx context.Context, cfg *Config, url string) (string, string, int64, []string) {
	method := strings.ToUpper(cfg.Method)
	if method == "" {
		method = "GET"
	}

	var body io.Reader
	if cfg.Body != "" {
		body = bytes.NewBufferString(cfg.Body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return "failed", "构造请求失败: " + err.Error(), 0,
			[]string{"[error] 构造请求失败: " + err.Error()}
	}
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	// 认证头（优先于用户自定义同名头）
	for k, v := range cfg.authHeader() {
		req.Header.Set(k, v)
	}
	if cfg.Body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	start := time.Now()
	resp, err := e.http.Do(req)
	duration := time.Since(start).Milliseconds()
	if err != nil {
		return "failed", err.Error(), duration,
			[]string{"[error] " + err.Error()}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	logs := []string{fmt.Sprintf("[http] %s %s -> %d", method, url, resp.StatusCode)}
	if len(respBody) > 0 {
		logs = append(logs, fmt.Sprintf("[body] %s", truncate(string(respBody), 1000)))
	}

	// 变量提取（记录到日志）
	if len(cfg.Extract) > 0 {
		logs = append(logs, e.extractLogs("http", respBody, cfg.Extract)...)
	}

	// 状态码断言
	if cfg.AssertStatus > 0 && resp.StatusCode != cfg.AssertStatus {
		return "failed", fmt.Sprintf("HTTP %d 不满足期望状态码 %d", resp.StatusCode, cfg.AssertStatus), duration,
			append(logs, fmt.Sprintf("[assert] 期望状态码 %d，实际 %d", cfg.AssertStatus, resp.StatusCode))
	}
	if resp.StatusCode >= 400 {
		return "failed", fmt.Sprintf("HTTP %d", resp.StatusCode), duration, logs
	}
	// 响应体包含断言
	if cfg.AssertBody != "" && !strings.Contains(string(respBody), cfg.AssertBody) {
		return "failed", fmt.Sprintf("响应体未包含断言内容: %q", cfg.AssertBody), duration,
			append(logs, fmt.Sprintf("[assert] 响应体未包含: %q", cfg.AssertBody))
	}
	return "success", "", duration, logs
}

// buildHTTPURL 由节点地址与请求路径拼接完整 URL。
// 节点 address 可能为 "192.168.1.10:8080" 或已带协议；path 如 "/api/xxx"。
func buildHTTPURL(address, path string) string {
	addr := strings.TrimSpace(address)
	if addr == "" {
		return ""
	}
	// 未带协议前缀时补 http://
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}
	p := strings.TrimSpace(path)
	if p == "" {
		return addr
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return addr + p
}

// extractLogs 从响应体中按点路径提取变量并返回日志行。
func (e *Executor) extractLogs(kind string, body []byte, extract map[string]string) []string {
	var logs []string
	var data interface{}
	if json.Unmarshal(body, &data) != nil {
		return logs
	}
	for name, path := range extract {
		v := lookupPath(data, strings.TrimPrefix(path, "$."))
		if v == nil {
			logs = append(logs, fmt.Sprintf("[%s] 变量 %s 提取失败（路径 %s 不存在）", kind, name, path))
			continue
		}
		logs = append(logs, fmt.Sprintf("[%s] 变量 %s = %v", kind, name, v))
	}
	return logs
}

// lookupPath 沿点路径在 JSON 数据中取值。
func lookupPath(data interface{}, path string) interface{} {
	if path == "" {
		return data
	}
	cur := data
	for _, seg := range strings.Split(path, ".") {
		switch v := cur.(type) {
		case map[string]interface{}:
			next, ok := v[seg]
			if !ok {
				return nil
			}
			cur = next
		case []interface{}:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil
			}
			cur = v[idx]
		default:
			return nil
		}
	}
	return cur
}

// runShell 执行 shell 命令。对任务绑定的每个节点执行（当前在本机执行，
// 节点作为执行归属标记；下发到节点 agent 的机制后续规划）。
func (e *Executor) runShell(ctx context.Context, cfg *Config, nodes []*model.Node) (string, string, int64, []string) {
	if strings.TrimSpace(cfg.Command) == "" {
		return "failed", "未配置 command（ExecutorConfig）", 0,
			[]string{"[error] shell 任务未配置执行命令 command"}
	}
	// 节点由 executeOnce 保证：无绑定时已回退数据库默认节点
	var logs []string
	var total int64
	allOK := true
	lastMsg := ""
	for _, n := range nodes {
		logs = append(logs, fmt.Sprintf("=== 节点 %s (%s) ===", n.Name, n.Address))
		cmd := exec.CommandContext(ctx, "sh", "-c", cfg.Command)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		start := time.Now()
		err := cmd.Run()
		duration := time.Since(start).Milliseconds()
		total += duration

		if stdout.Len() > 0 {
			logs = append(logs, "[stdout] "+strings.TrimSpace(stdout.String()))
		}
		if stderr.Len() > 0 {
			logs = append(logs, "[stderr] "+strings.TrimSpace(stderr.String()))
		}
		if err != nil {
			allOK = false
			lastMsg = err.Error()
			logs = append(logs, "[error] "+err.Error())
		}
	}
	if allOK {
		return "success", "", total, logs
	}
	return "failed", lastMsg, total, logs
}

// sendResultNotify 根据任务通知配置发送执行结果通知。
// 通知在异步 goroutine 中执行，失败仅记录日志，不影响任务主流程。
func (e *Executor) sendResultNotify(ctx context.Context, task *model.Task, status, msg string, duration int64, attempt int) {
	if !task.NotifyEnabled {
		return
	}
	// 成功且未配置成功通知时，不发（仅失败/超时通知）
	if status == "success" && !task.NotifyOnSuccess {
		return
	}
	// 只有有实际结果的状态才通知（success/failed/timeout）
	if status != "success" && status != "failed" && status != "timeout" {
		return
	}

	notifyType := task.NotifyType
	if notifyType == "" {
		notifyType = "dingtalk"
	}

	// 组装通知内容
	content := fmt.Sprintf(
		"#### 任务执行%s\n- **任务**: %s (ID: %d)\n- **类型**: %s\n- **状态**: %s\n- **耗时**: %d ms\n- **尝试次数**: %d\n- **时间**: %s",
		statusDesc(status), task.Name, task.ID, task.Type, status, duration, attempt,
		time.Now().Format("2006-01-02 15:04:05"),
	)
	if msg != "" {
		content += fmt.Sprintf("\n- **详情**: %s", msg)
	}

	// 任务未指定渠道地址时，回退到全局默认配置
	cfg := &notify.Config{
		Enabled:    true,
		DingtalkWebhook: task.NotifyDingtalk,
		DingtalkSecret:  task.NotifySecret,
		WebhookURL:      task.NotifyWebhook,
		WebhookSecret:   task.NotifyWebhookSec,
	}
	if cfg.DingtalkWebhook == "" || cfg.WebhookURL == "" {
		if global := e.store.GetNotifySetting(ctx); global != nil {
			if cfg.DingtalkWebhook == "" {
				cfg.DingtalkWebhook = global.DingtalkWebhook
				cfg.DingtalkSecret = global.DingtalkSecret
			}
			if cfg.WebhookURL == "" {
				cfg.WebhookURL = global.WebhookURL
				cfg.WebhookSecret = global.WebhookSecret
			}
		}
	}
	title := fmt.Sprintf("[调度平台] 任务「%s」%s", task.Name, statusDesc(status))

	// 异步发送（捕获 ctx 携带的链路 ID，通知日志同样带 trace）
	go notify.TrySend(ctx, cfg, title, content, notifyType)
}

func statusDesc(status string) string {
	switch status {
	case "success":
		return "成功"
	case "failed":
		return "失败"
	case "timeout":
		return "超时"
	default:
		return status
	}
}

// appendLogs 写入执行日志。
func (e *Executor) appendLogs(ctx context.Context, execID, taskID int64, logs []string) {
	for i, line := range logs {
		if line == "" {
			continue
		}
		e.store.AppendLog(ctx, &model.TaskLog{
			ExecutionID: execID, TaskID: taskID,
			Stream: "stdout", Level: "info",
			Content: line, Sequence: i,
		})
	}
}

// parseConfig 解析执行配置。
func parseConfig(s string) *Config {
	cfg := &Config{}
	if s == "" {
		return cfg
	}
	if err := json.Unmarshal([]byte(s), cfg); err != nil {
		ilog.Warnf("parse executor config failed: %v", err)
	}
	return cfg
}

func maxRetries(retryCount int) int {
	if retryCount < 0 {
		return 0
	}
	return retryCount + 1 // +1 为首次执行
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
