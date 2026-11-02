package utiles

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// UpdateHistoryEntry 一次程序自更新的完整记录。
// 不论成功失败都记一条：失败记录是排查“为什么升不上去”的第一手证据，
// 也是失败通知的数据源。
type UpdateHistoryEntry struct {
	// Time 记录时间（RFC3339）。
	Time string `json:"time"`
	// FromVersion / ToVersion 本地版本与目标版本。
	FromVersion string `json:"fromVersion"`
	ToVersion   string `json:"toVersion"`
	// OK 更新是否走完下载-校验-解压全链路（不含重启后的最终确认）。
	OK bool `json:"ok"`
	// Manual 是否手动触发（HTTP API / MCP），false 为定时自动更新。
	Manual bool `json:"manual"`
	// Message 结果说明；失败时包含错误摘要。
	Message string `json:"message"`
}

var (
	updateHistoryMu sync.Mutex
)

// ErrUpdateProgramDisabled 手动强制更新被环境变量关闭时返回。
var ErrUpdateProgramDisabled = errors.New("程序自更新已被禁用（UPDATE_PROGRAM_ENABLED=false）")

// updateHistoryPath 更新历史文件路径，可用 UPDATE_HISTORY 覆盖。
func updateHistoryPath() string {
	if p := os.Getenv("UPDATE_HISTORY"); p != "" {
		return p
	}
	return "/data/config/update-history.json"
}

const updateHistoryMax = 50

// lastAppendedKey 记录最近一次 AppendUpdateHistory 追加的条目键。
// 历史满了之后会按时间倒序裁剪，数组末尾就不再是"刚追加的那条"，
// 原先用 history[:len-1] 砍尾会误删真实的旧记录、把不该留的记录留下。
var lastAppendedKey updateHistoryEntryKey

type updateHistoryEntryKey struct {
	Time        string
	FromVersion string
	ToVersion   string
}

// AppendUpdateHistory 追加一条更新记录；超出上限时裁掉最旧的。
// 写失败只记日志，不影响更新主流程。
func AppendUpdateHistory(entry UpdateHistoryEntry) {
	if entry.Time == "" {
		entry.Time = time.Now().Format(time.RFC3339)
	}
	updateHistoryMu.Lock()
	defer updateHistoryMu.Unlock()

	path := updateHistoryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		logx.Errorf("创建更新历史目录失败: %v", err)
		return
	}
	history := readUpdateHistoryFile(path)
	history = append(history, entry)
	lastAppendedKey = updateHistoryEntryKey{Time: entry.Time, FromVersion: entry.FromVersion, ToVersion: entry.ToVersion}
	if len(history) > updateHistoryMax {
		// 按时间倒序裁剪：保留最近 updateHistoryMax 条。
		// SliceStable：同秒记录保持插入顺序，避免"最新在前"随机抖动。
		sort.SliceStable(history, func(i, j int) bool {
			return history[i].Time > history[j].Time
		})
		history = history[:updateHistoryMax]
	}
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		logx.Errorf("更新历史序列化失败: %v", err)
		return
	}
	tmp := fmt.Sprintf("%s.tmp-%d", path, os.Getpid())
	if err = os.WriteFile(tmp, data, 0644); err != nil {
		logx.Errorf("写更新历史临时文件失败: %v", err)
		_ = os.Remove(tmp)
		return
	}
	if err = os.Rename(tmp, path); err != nil {
		logx.Errorf("更新历史落地失败: %v", err)
	}
}

// UpdateHistory 返回更新历史，最新在前；limit<=0 表示全部。
func UpdateHistory(limit int) []UpdateHistoryEntry {
	updateHistoryMu.Lock()
	defer updateHistoryMu.Unlock()
	history := readUpdateHistoryFile(updateHistoryPath())
	sort.Slice(history, func(i, j int) bool {
		return history[i].Time > history[j].Time
	})
	if limit > 0 && len(history) > limit {
		history = history[:limit]
	}
	return history
}

// readUpdateHistoryFile 读取历史文件；调用方需持锁。文件缺失或损坏返回空。
func readUpdateHistoryFile(path string) []UpdateHistoryEntry {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var history []UpdateHistoryEntry
	if err = json.Unmarshal(data, &history); err != nil {
		// 保留损坏文件再重建：旧实现直接返回 nil，下一次写入就把
		// 最多 50 条历史一次性覆盖掉，事后无法追溯也无法修复。
		backupPath := fmt.Sprintf("%s.corrupt-%d", path, time.Now().Unix())
		if renameErr := os.Rename(path, backupPath); renameErr == nil {
			logx.Errorf("更新历史解析失败，已备份为 %s 并重建: %v", backupPath, err)
		} else {
			logx.Errorf("更新历史解析失败（备份失败 %v）: %v", renameErr, err)
		}
		return nil
	}
	return history
}

// updateResultMessage 把自更新结果转成一句人话，进历史记录。
func updateResultMessage(err error) string {
	if err == nil {
		return "更新包已换入，服务即将重启"
	}
	if errors.Is(err, ErrUpdateProgramRunning) {
		return "跳过：已有一轮更新在执行"
	}
	return err.Error()
}

// RemoveLastUpdateHistory 删除刚追加的最后一条记录。
// 用于“本次调用不算一次更新尝试”的场合（如 FNOS 配置拒绝），
// 避免历史里堆满非更新原因的失败记录。
func RemoveLastUpdateHistory() {
	updateHistoryMu.Lock()
	defer updateHistoryMu.Unlock()
	path := updateHistoryPath()
	history := readUpdateHistoryFile(path)
	if len(history) == 0 {
		return
	}
	// 只删除"刚追加的那条"；找不到就什么都不做（宁可不删，也不误删真实记录）。
	removed := false
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Time == lastAppendedKey.Time &&
			history[i].FromVersion == lastAppendedKey.FromVersion &&
			history[i].ToVersion == lastAppendedKey.ToVersion {
			history = append(history[:i], history[i+1:]...)
			removed = true
			break
		}
	}
	if !removed {
		logx.Errorf("RemoveLastUpdateHistory 未找到刚追加的记录，跳过删除")
		return
	}
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return
	}
	tmp := fmt.Sprintf("%s.tmp-%d", path, os.Getpid())
	if err = os.WriteFile(tmp, data, 0644); err != nil {
		logx.Errorf("写更新历史临时文件失败: %v", err)
		_ = os.Remove(tmp)
		return
	}
	if err = os.Rename(tmp, path); err != nil {
		logx.Errorf("更新历史落地失败: %v", err)
	}
}

// updateVersionPattern 版本号合法字符：字母数字与 . - _ +（semver 常见字符）。
var updateVersionPattern = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)

// UpdateProgramDisabled 手动强制更新开关：环境变量 UPDATE_PROGRAM_ENABLED=false
// 时禁用（例如交给系统包管理器管理的部署，禁止程序自我替换二进制）。
// 未设置或为 true 时启用。只约束手动触发；定时自动更新有自己的配置门。
func UpdateProgramDisabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("UPDATE_PROGRAM_ENABLED")), "false")
}

// SanitizeUpdateTarget 校验目标版本号：只允许版本号常见字符，
// 防止把路径分隔符或 URL 片段拼进下载链接。
func SanitizeUpdateTarget(version string) (string, error) {
	v := strings.TrimSpace(version)
	if v == "" {
		return "", fmt.Errorf("目标版本号为空")
	}
	if !updateVersionPattern.MatchString(v) {
		return "", fmt.Errorf("目标版本号包含非法字符: %q", v)
	}
	// 纯点串（"." / ".." / "..."）能通过字符白名单，但拼进下载 URL 会做路径上跳，
	// 配了 githubProxy 时还能打到代理的其它上游路径。
	if strings.Trim(v, ".") == "" {
		return "", fmt.Errorf("目标版本号非法: %q", v)
	}
	return v, nil
}
