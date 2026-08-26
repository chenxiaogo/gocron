package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"scheduler-platform/pkg/ilog"
)

// Config 通知渠道配置。用于全局默认配置，任务可覆盖。
type Config struct {
	// Enabled 是否启用通知
	Enabled bool `json:"enabled"`
	// DingtalkWebhook 钉钉机器人 Webhook 地址（形如 https://oapi.dingtalk.com/robot/send?access_token=xxx）
	DingtalkWebhook string `json:"dingtalkWebhook"`
	// DingtalkSecret 钉钉机器人加签密钥（可选；配置后自动加签）
	DingtalkSecret string `json:"dingtalkSecret"`
	// WebhookURL 通用 Webhook 地址（POST JSON）
	WebhookURL string `json:"webhookUrl"`
	// WebhookSecret 通用 Webhook 的签名密钥（可选）
	WebhookSecret string `json:"webhookSecret"`
	// NotifyOnSuccess 是否在任务成功时也发送通知（默认仅失败/超时）
	NotifyOnSuccess bool `json:"notifyOnSuccess"`
}

// dingtalkPayload 钉钉机器人消息体（markdown 类型）。
type dingtalkPayload struct {
	MsgType  string `json:"msgtype"`
	Markdown struct {
		Title string `json:"title"`
		Text  string `json:"text"`
	} `json:"markdown"`
}

// webhookPayload 通用 Webhook 消息体（JSON）。
type webhookPayload struct {
	Title   string                 `json:"title"`
	Content string                 `json:"content"`
	Type    string                 `json:"type"` // success / failed / timeout
	Time    string                 `json:"time"`
	Extra   map[string]interface{} `json:"extra,omitempty"`
}

// HTTP 客户端，带超时，避免阻塞任务执行。
var client = &http.Client{Timeout: 5 * time.Second}

// SendDingtalk 发送钉钉机器人通知。
// webhook 形如 https://oapi.dingtalk.com/robot/send?access_token=xxx
// secret 为加签密钥（可选），配置后自动拼接 sign。
func SendDingtalk(ctx context.Context, webhook, secret, title, text string) error {
	if webhook == "" {
		return fmt.Errorf("钉钉 webhook 为空")
	}
	if secret != "" {
		webhook = withSign(webhook, secret)
	}
	p := dingtalkPayload{MsgType: "markdown"}
	p.Markdown.Title = title
	p.Markdown.Text = text
	return postJSON(ctx, webhook, p, nil)
}

// SendWebhook 发送通用 Webhook 通知（POST JSON）。
func SendWebhook(ctx context.Context, webhook, secret, title, content, notifyType string) error {
	if webhook == "" {
		return fmt.Errorf("webhook 地址为空")
	}
	payload := webhookPayload{
		Title:   title,
		Content: content,
		Type:    notifyType,
		Time:    time.Now().Format("2006-01-02 15:04:05"),
	}
	headers := map[string]string{}
	if secret != "" {
		headers["X-Notify-Sign"] = hmacSign(secret, time.Now().Format("20060102150405"))
		headers["X-Notify-Timestamp"] = fmt.Sprintf("%d", time.Now().Unix())
	}
		return postJSON(ctx, webhook, payload, headers)
}

// postJSON 向目标 URL POST JSON 数据。
func postJSON(ctx context.Context, u string, payload interface{}, headers map[string]string) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook 返回状态码 %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// withSign 钉钉加签：timestamp + "\n" + secret，HMAC-SHA256 后 base64 + urlencode。
func withSign(webhook, secret string) string {
	timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
	raw := timestamp + "\n" + secret
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(raw))
	sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	sep := "?"
	if hasQuery(webhook) {
		sep = "&"
	}
	return webhook + sep + "timestamp=" + timestamp + "&sign=" + url.QueryEscape(sign)
}

func hasQuery(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '?' {
			return true
		}
	}
	return false
}

// hmacSign 通用签名（HMAC-SHA256 base64）。
func hmacSign(secret, data string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// TrySend 根据通知配置发送一条通知。notifyType 支持 dingtalk / webhook / dingtalk,webhook。
// 静默失败：通知异常只记录日志，不影响任务执行主流程。ctx 携带链路 ID，使通知日志带 trace。
func TrySend(ctx context.Context, cfg *Config, title, content, notifyType string) {
	if cfg == nil || !cfg.Enabled {
		return
	}
	for _, t := range splitTypes(notifyType) {
		switch t {
		case "dingtalk":
			if cfg.DingtalkWebhook == "" {
				continue
			}
			if err := SendDingtalk(ctx, cfg.DingtalkWebhook, cfg.DingtalkSecret, title, content); err != nil {
				ilog.Trace(ctx).Warnf("send dingtalk notify failed: %v", err)
			} else {
				ilog.Trace(ctx).Infof("dingtalk notify sent: %s", title)
			}
		case "webhook":
			if cfg.WebhookURL == "" {
				continue
			}
			nt := "info"
			if len(notifyType) == 0 {
				nt = "info"
			}
			if err := SendWebhook(ctx, cfg.WebhookURL, cfg.WebhookSecret, title, content, nt); err != nil {
				ilog.Trace(ctx).Warnf("send webhook notify failed: %v", err)
			} else {
				ilog.Trace(ctx).Infof("webhook notify sent: %s", title)
			}
		}
	}
}

func splitTypes(s string) []string {
	if s == "" {
		return []string{}
	}
	var out []string
	cur := ""
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
		} else {
			cur += string(s[i])
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
