package utiles

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/docker/go-connections/nat"
	"github.com/docker/docker/api/types/network"
)

// BackupEntryDiff 单个容器在两次备份之间的差异结论。
type BackupEntryDiff struct {
	// Name 参与对比的容器名（以新备份里的名字为准）。
	Name string `json:"name"`
	// Status: added=新备份新增；removed=新备份已不存在；
	// changed=两边都有但配置不同；unchanged=两边一致。
	Status string `json:"status"`
	// Fields 列出配置有差异的字段名（仅 changed 时有值）。
	Fields []string `json:"fields,omitempty"`
	// OldImage / NewImage 镜像变化（仅 changed 且镜像不同时有值）。
	OldImage string `json:"oldImage,omitempty"`
	NewImage string `json:"newImage,omitempty"`
}

// BackupDiffResult 两次备份的整体对比结果。
type BackupDiffResult struct {
	OldFile string            `json:"oldFile"`
	NewFile string            `json:"newFile"`
	Added   []string          `json:"added"`
	Removed []string          `json:"removed"`
	Changed []BackupEntryDiff `json:"changed"`
	// UnchangedCount 两边完全一致的容器数，单独计数避免结果过长。
	UnchangedCount int `json:"unchangedCount"`
}

// CompareBackups 对比两个 JSON 备份文件的容器配置差异。
// old 在前（基线），new 在后（当前）。文件名走 ResolveBackupPath 校验，
// 防路径穿越；解析失败或不是数组时返回错误。
func CompareBackups(oldFile, newFile string) (*BackupDiffResult, error) {
	oldEntries, err := readBackupEntries(oldFile)
	if err != nil {
		return nil, fmt.Errorf("旧备份读取失败: %w", err)
	}
	newEntries, err := readBackupEntries(newFile)
	if err != nil {
		return nil, fmt.Errorf("新备份读取失败: %w", err)
	}

	oldByName := make(map[string]ContainerBackupEntry, len(oldEntries))
	for _, e := range oldEntries {
		name := backupEntryName(e)
		if name != "" {
			oldByName[name] = e
		}
	}
	newByName := make(map[string]ContainerBackupEntry, len(newEntries))
	for _, e := range newEntries {
		name := backupEntryName(e)
		if name != "" {
			newByName[name] = e
		}
	}

	result := &BackupDiffResult{
		OldFile: filepath.Base(oldFile),
		NewFile: filepath.Base(newFile),
		Added:   []string{},
		Removed: []string{},
		Changed: []BackupEntryDiff{},
	}

	for name, newEntry := range newByName {
		oldEntry, existed := oldByName[name]
		if !existed {
			result.Added = append(result.Added, name)
			continue
		}
		if diff := diffEntry(name, oldEntry, newEntry); diff != nil {
			result.Changed = append(result.Changed, *diff)
		} else {
			result.UnchangedCount++
		}
	}
	for name := range oldByName {
		if _, exists := newByName[name]; !exists {
			result.Removed = append(result.Removed, name)
		}
	}
	sort.Strings(result.Added)
	sort.Strings(result.Removed)
	sort.Slice(result.Changed, func(i, j int) bool {
		return result.Changed[i].Name < result.Changed[j].Name
	})
	return result, nil
}

// readBackupEntries 读取并解析备份文件；文件名经 ResolveBackupPath 校验。
func readBackupEntries(filename string) ([]ContainerBackupEntry, error) {
	fullPath, err := ResolveBackupPath(filename, ".json")
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, err
	}
	var entries []ContainerBackupEntry
	if err = json.Unmarshal(content, &entries); err != nil {
		return nil, fmt.Errorf("%s 不是合法的 JSON 配置备份（compose 备份不支持对比）", filepath.Base(fullPath))
	}
	return entries, nil
}

// backupEntryName 取备份条目的容器名，兜底镜像名。
func backupEntryName(e ContainerBackupEntry) string {
	if e.Name != "" {
		return e.Name
	}
	return ""
}

// diffEntry 对比单个容器的新旧配置，返回差异；一致返回 nil。
// 对比维度：镜像、启动命令、环境变量、挂载、端口、重启策略、网络。
func diffEntry(name string, oldEntry, newEntry ContainerBackupEntry) *BackupEntryDiff {
	var fields []string
	oldCfg, newCfg := oldEntry.Config, newEntry.Config
	oldHost, newHost := oldEntry.HostConfig, newEntry.HostConfig

	if oldCfg == nil || newCfg == nil {
		fields = append(fields, "config")
	} else {
		if oldCfg.Image != newCfg.Image {
			fields = append(fields, "image")
		}
		if !equalStrings(oldCfg.Cmd, newCfg.Cmd) {
			fields = append(fields, "cmd")
		}
		if !equalStrings(oldCfg.Entrypoint, newCfg.Entrypoint) {
			fields = append(fields, "entrypoint")
		}
		if !equalEnv(oldCfg.Env, newCfg.Env) {
			fields = append(fields, "env")
		}
		if !equalStringMap(oldCfg.Labels, newCfg.Labels) {
			fields = append(fields, "labels")
		}
	}
	if oldHost == nil || newHost == nil {
		fields = append(fields, "hostConfig")
	} else {
		if !equalBinds(oldHost.Binds, newHost.Binds) {
			fields = append(fields, "mounts")
		}
		if !equalPortMap(oldHost.PortBindings, newHost.PortBindings) {
			fields = append(fields, "ports")
		}
		if oldHost.RestartPolicy.Name != newHost.RestartPolicy.Name {
			fields = append(fields, "restartPolicy")
		}
		if !equalStringMap(networkEndpointNames(oldEntry.NetworkingConfig), networkEndpointNames(newEntry.NetworkingConfig)) {
			fields = append(fields, "networks")
		}
	}
	if len(fields) == 0 {
		return nil
	}
	diff := &BackupEntryDiff{Name: name, Status: "changed", Fields: fields}
	if oldCfg != nil && newCfg != nil && oldCfg.Image != newCfg.Image {
		diff.OldImage = oldCfg.Image
		diff.NewImage = newCfg.Image
	}
	return diff
}

// equalBinds 比较挂载列表（内容一致即等，忽略顺序）。
func equalBinds(a, b []string) bool { return equalStrings(a, b) }

// equalStrings 比较字符串切片内容（忽略顺序）。
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	am := make(map[string]int, len(a))
	for _, s := range a {
		am[s]++
	}
	for _, s := range b {
		am[s]--
		if am[s] < 0 {
			return false
		}
	}
	return true
}

// equalStringMap 比较字符串到字符串的映射（labels 常用）。
func equalStringMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// equalEnv 比较容器环境变量切片（"KEY=VAL" 格式，等价转成映射比较）。
func equalEnv(a, b []string) bool {
	return equalStringMap(envToMap(a), envToMap(b))
}

// envToMap 把 "KEY=VAL" 切片转映射；无 "=" 的项按空值处理。
func envToMap(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, kv := range env {
		if k, v, found := strings.Cut(kv, "="); found {
			out[k] = v
		} else {
			out[kv] = ""
		}
	}
	return out
}

// networkEndpointNames 提取网络配置里的网络名集合（值为空串占位）。
func networkEndpointNames(cfg *network.NetworkingConfig) map[string]string {
	out := map[string]string{}
	if cfg == nil {
		return out
	}
	for name := range cfg.EndpointsConfig {
		out[name] = ""
	}
	return out
}

// equalPortMap 比较端口绑定映射：只比较“容器端口 → 宿主端口”的最终映射。
func equalPortMap(a, b nat.PortMap) bool {
	if len(a) != len(b) {
		return false
	}
	for port, bindings := range a {
		other, ok := b[port]
		if !ok || len(bindings) != len(other) {
			return false
		}
		hostPortsA := make([]string, 0, len(bindings))
		hostPortsB := make([]string, 0, len(other))
		for _, bd := range bindings {
			hostPortsA = append(hostPortsA, bd.HostPort)
		}
		for _, bd := range other {
			hostPortsB = append(hostPortsB, bd.HostPort)
		}
		if !equalStrings(hostPortsA, hostPortsB) {
			return false
		}
	}
	return true
}

// FormatBackupDiff 把对比结果渲染成人类可读的多行文本（MCP 文本结果用）。
func FormatBackupDiff(r *BackupDiffResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "对比 %s → %s\n", r.OldFile, r.NewFile)
	fmt.Fprintf(&b, "新增 %d 个，移除 %d 个，修改 %d 个，未变化 %d 个\n", len(r.Added), len(r.Removed), len(r.Changed), r.UnchangedCount)
	if len(r.Added) > 0 {
		fmt.Fprintf(&b, "\n新增：\n")
		for _, name := range r.Added {
			fmt.Fprintf(&b, "  + %s\n", name)
		}
	}
	if len(r.Removed) > 0 {
		fmt.Fprintf(&b, "\n移除：\n")
		for _, name := range r.Removed {
			fmt.Fprintf(&b, "  - %s\n", name)
		}
	}
	if len(r.Changed) > 0 {
		fmt.Fprintf(&b, "\n修改：\n")
		for _, c := range r.Changed {
			fmt.Fprintf(&b, "  ~ %s（%s）\n", c.Name, strings.Join(c.Fields, ", "))
			if c.OldImage != "" || c.NewImage != "" {
				fmt.Fprintf(&b, "      镜像: %s → %s\n", c.OldImage, c.NewImage)
			}
		}
	}
	return b.String()
}
