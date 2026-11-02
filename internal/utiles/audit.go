package utiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

type auditEntry struct {
	Time   string `json:"time"`
	Action string `json:"action"`
	Target string `json:"target"`
	Detail string `json:"detail"`
}

var auditMu sync.Mutex

func auditLogPath() string {
	if p := os.Getenv("AUDIT_LOG"); p != "" {
		return p
	}
	return "/data/logs/audit.jsonl"
}

// AuditLog 追加一条审计记录（JSON Lines），失败静默不影响业务。
func AuditLog(action, target, detail string) {
	entry := auditEntry{
		Time:   time.Now().Format("2006-01-02 15:04:05"),
		Action: action,
		Target: target,
		Detail: detail,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		logx.Errorf("审计序列化失败（action=%s）: %v", action, err)
		return
	}
	auditMu.Lock()
	defer auditMu.Unlock()
	path := auditLogPath()
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		logx.Errorf("审计目录不可用（action=%s）: %v", action, err)
		return
	}
	// 简单轮转：超过 10MB 就保留一份 .1，避免长期运行无限增长。
	if info, statErr := os.Stat(path); statErr == nil && info.Size() > 10<<20 {
		_ = os.Remove(path + ".1")
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		logx.Errorf("审计文件打开失败（action=%s）: %v", action, err)
		return
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)
	if _, writeErr := f.Write(append(data, '\n')); writeErr != nil {
		logx.Errorf("审计写入失败（action=%s）: %v", action, writeErr)
	}
}
