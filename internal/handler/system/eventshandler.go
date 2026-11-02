package system

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/utiles"
	"github.com/zeromicro/go-zero/core/logx"
)

// eventsMaxDuration 单条 SSE 连接的最长存活时间。客户端断开或超时后自行重连，
// 避免长连接在反向代理/进程重启时悄悄挂住。
const eventsMaxDuration = 10 * time.Minute

// eventsHeartbeat 心跳间隔：SSE 长时间没有事件时，代理可能判定空闲并掐断。
const eventsHeartbeat = 20 * time.Second

// EventsHandler 把 Docker 事件流桥接成 SSE（GET /api/events?since=）。
// 前端用它做"容器状态实时刷新"，不必轮询。
func EventsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(http.Flusher); !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		streamCtx, cancel := context.WithTimeout(r.Context(), eventsMaxDuration)
		defer cancel()

		msgCh, errCh, err := utiles.StreamDockerEvents(streamCtx, svcCtx, r.URL.Query().Get("since"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		header := w.Header()
		header.Set("Content-Type", "text/event-stream")
		header.Set("Cache-Control", "no-cache")
		header.Set("Connection", "keep-alive")
		header.Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		flush := func() bool {
			return http.NewResponseController(w).Flush() == nil
		}
		writeEvent := func(event string, payload []byte) bool {
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload); err != nil {
				return false
			}
			return flush()
		}
		if _, err := fmt.Fprintf(w, "retry: 3000\n\n"); err != nil || !flush() {
			return
		}

		heartbeat := time.NewTicker(eventsHeartbeat)
		defer heartbeat.Stop()

		for {
			select {
			case <-streamCtx.Done():
				// 超时或客户端断开：发一条注释让客户端知道该重连了。
				_, _ = fmt.Fprint(w, ": stream deadline reached, reconnecting\n\n")
				flush()
				return
			case <-heartbeat.C:
				if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil || !flush() {
					return
				}
			case msg, ok := <-msgCh:
				if !ok {
					return
				}
				payload, marshalErr := json.Marshal(utiles.EventSummary(msg))
				if marshalErr != nil {
					logx.Errorf("序列化 docker 事件失败: %v", marshalErr)
					continue
				}
				if !writeEvent(string(msg.Type), payload) {
					return
				}
			case streamErr, ok := <-errCh:
				if !ok {
					return
				}
				if streamErr != nil {
					logx.Errorf("docker 事件流错误: %v", streamErr)
					return
				}
			}
		}
	}
}
