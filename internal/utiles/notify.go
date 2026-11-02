package utiles

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// NotifyTaskDone 在任务收尾时向 NOTIFY_WEBHOOK_URL POST 一条 JSON 通知。
// 未配置 webhook 时静默跳过；发送失败只记日志，不影响业务。
func NotifyTaskDone(kind, name, taskID string, ok bool, message string) {
	rawURL := os.Getenv("NOTIFY_WEBHOOK_URL")
	if rawURL == "" {
		return
	}
	// 只允许 http/https，避免 file:// 之类被当作 webhook 目标。
	if parsed, err := url.Parse(rawURL); err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		logx.Errorf("webhook 地址非法（需要 http/https）: %s", rawURL)
		return
	}
	payload := map[string]interface{}{
		"kind":    kind,
		"name":    name,
		"taskID":  taskID,
		"ok":      ok,
		"message": message,
		"time":    time.Now().Format("2006-01-02 15:04:05"),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		logx.Errorf("webhook 负载序列化失败: %v", err)
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}

	// 两次尝试（间隔 500ms）：webhook 侧限流/瞬时 5xx 很常见，
	// 而"升级失败告警"恰恰是最不能丢的一条。旧实现既不看重试、
	// 也不看状态码 —— 429/500 被当成发送成功。
	var lastErr string
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		resp, postErr := client.Post(rawURL, "application/json", bytes.NewReader(body))
		if postErr != nil {
			lastErr = postErr.Error()
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return
		}
		lastErr = fmt.Sprintf("HTTP %s", resp.Status)
	}
	logx.Errorf("webhook 通知发送失败（kind=%s name=%s ok=%v）: %s", kind, name, ok, lastErr)
}
