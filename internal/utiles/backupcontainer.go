package utiles

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DongShull/DTool/internal/svc"
	dockerBackend "github.com/docker/docker/api/types/backend"
	"github.com/docker/docker/api/types/network"
	"github.com/zeromicro/go-zero/core/logx"
)

// backupMu 串行化整次备份：防止并发备份的 rename 覆盖与保留份数计数错乱。
var backupMu sync.Mutex

// BackupContainer 把全部容器配置备份为一个 JSON 文件。
// 单个容器 inspect 失败不中止整体备份，失败清单通过返回值带回；
// 全部容器都失败时才返回错误。
// include 非空时只备份名称命中的容器（精确匹配；支持从容器列表的
// 带斜杠名称与 inspect 后的去斜杠名称两种写法）。
func BackupContainer(ctx *svc.ServiceContext, include ...string) (failures []string, err error) {
	if ctx.DockerClient == nil {
		return nil, fmt.Errorf("Docker 客户端未初始化，请检查 DOCKER_HOST 配置")
	}
	// P0：备份互斥。文件名时间戳原为秒级，同秒两次备份（HTTP 与 MCP 并发、
	// 或前端双击）会让后一次的原子 rename 覆盖前一次；即便文件名已加纳秒，
	// 也串行化整次备份，保证 BACKUP_KEEP 的份数计数不被并发打乱。
	backupMu.Lock()
	defer backupMu.Unlock()
	if dir, reason := CheckBackupDirSafe(); dir != "" && reason != "" {
		return nil, fmt.Errorf("%s", reason)
	}
	containerList, err := GetContainerList(ctx)
	if err != nil {
		return nil, err
	}
	// 过滤集合统一成去斜杠名字，同时保留 ID 便于按 ID 过滤。
	var includeSet map[string]bool
	if len(include) > 0 {
		includeSet = make(map[string]bool, len(include))
		for _, name := range include {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			includeSet[name] = true
			includeSet[strings.TrimPrefix(name, "/")] = true
		}
		if len(includeSet) == 0 {
			return nil, fmt.Errorf("备份过滤条件为空：include 里没有有效容器名")
		}
	}
	backupList := make([]ContainerBackupEntry, 0, len(containerList))
	failures = make([]string, 0)
	for i := range containerList {
		containerID := containerList[i].ID
		// 备份过滤：include 非空时只备份命中的容器（名字或 ID 精确匹配）。
		if includeSet != nil {
			hit := includeSet[containerID]
			if !hit {
				for _, name := range containerList[i].Names {
					if includeSet[strings.TrimPrefix(name, "/")] {
						hit = true
						break
					}
				}
			}
			if !hit {
				continue
			}
		}
		inspectCtx, cancel := dockerTimeout(context.Background(), dockerInspectTimeout)
		inspectedContainer, err := ctx.DockerClient.ContainerInspect(inspectCtx, containerID)
		cancel()
		if err != nil {
			// 一个坏容器不应该挡住其它容器的日常备份。
			logx.Errorf("容器 %s 获取信息失败，跳过备份: %v", containerID, err)
			failures = append(failures, containerID+": "+err.Error())
			continue
		}
		if inspectedContainer.Config == nil {
			logx.Errorf("容器 %s 配置为空，跳过备份", containerID)
			failures = append(failures, containerID+": 配置为空，无法备份")
			continue
		}
		containerName := strings.TrimPrefix(inspectedContainer.Name, "/")
		if containerName == "" && len(containerList[i].Names) > 0 {
			containerName = strings.TrimPrefix(containerList[i].Names[0], "/")
		}
		if containerName == "" {
			containerName = containerID
			if len(containerName) > 12 {
				containerName = containerName[:12]
			}
			logx.Errorf("容器 %s 名称为空，使用 ID 作为备份名称", containerID)
		}
		inspectedContainer.Config.Hostname = ""
		inspectedContainer.Image = inspectedContainer.Config.Image
		var endpoints map[string]*network.EndpointSettings
		if inspectedContainer.NetworkSettings != nil {
			endpoints = sanitizeEndpointSettings(inspectedContainer.NetworkSettings.Networks)
		}
		networkingConfig := &network.NetworkingConfig{
			EndpointsConfig: endpoints,
		}
		entry := ContainerBackupEntry{
			ContainerCreateConfig: dockerBackend.ContainerCreateConfig{
				Config:           inspectedContainer.Config,
				HostConfig:       inspectedContainer.HostConfig,
				NetworkingConfig: networkingConfig,
				Name:             containerName,
			},
		}
		if inspectedContainer.State != nil {
			entry.StateKnown = true
			entry.WasRunning = inspectedContainer.State.Running
			entry.WasPaused = inspectedContainer.State.Paused
		}
		backupList = append(backupList, entry)
	}
	if len(backupList) == 0 && len(failures) > 0 {
		return failures, fmt.Errorf("全部 %d 个容器备份失败，未生成备份文件", len(failures))
	}
	if len(backupList) == 0 && includeSet != nil {
		return nil, fmt.Errorf("备份过滤没有命中任何容器：请确认容器名（include=%v）", include)
	}
	jsonData, err := json.MarshalIndent(backupList, "", "  ")
	if err != nil {
		logx.Error("数据序列化失败:", err)
		return failures, err
	}
	backupDir := BackupDir() // env BACKUP_DIR > Settings > 默认 /data/backups
	if backupDir == "" {
		backupDir = "/data/backups" // 如果环境变量未设置，使用默认值
	}
	_, err = os.Stat(backupDir)
	if os.IsNotExist(err) {
		err = os.MkdirAll(backupDir, 0755)
		if err != nil {
			logx.Errorf("创建备份目录失败: %v", err)
			return failures, err
		}
	}
	// 文件名带时间戳（到纳秒）：同一天/同一秒多次备份互不覆盖，可回退到任一次；
	// 超出保留份数（BACKUP_KEEP，默认 7）的旧备份自动清理。
	// 纳秒部分兼容 ResolveBackupPath 的 ^[A-Za-z0-9._-]+$ 文件名白名单与 .json 扩展名判断。
	fileName := "backup-" + time.Now().Format("2006-01-02-150405.000000000") + ".json"
	fullPath := filepath.Join(backupDir, fileName)
	tmp, err := os.CreateTemp(backupDir, ".backup-*.json.tmp")
	if err != nil {
		logx.Errorf("创建备份临时文件失败: %v", err)
		return failures, err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if err = tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		logx.Errorf("设置备份临时文件权限失败: %v", err)
		return failures, err
	}
	if _, err = tmp.Write(jsonData); err != nil {
		_ = tmp.Close()
		logx.Errorf("写入备份临时文件失败: %v", err)
		return failures, err
	}
	if err = tmp.Close(); err != nil {
		logx.Errorf("关闭备份临时文件失败: %v", err)
		return failures, err
	}
	if err = os.Rename(tmpName, fullPath); err != nil {
		logx.Errorf("替换备份文件失败: %v", err)
		return failures, err
	}
	pruneOldBackups(backupDir, "backup-*.json", BackupKeepCount())
	detail := fileName
	if len(include) > 0 {
		detail += "（仅含 " + strings.Join(include, ",") + "）"
	}
	if len(failures) > 0 {
		detail += fmt.Sprintf("；%d 个容器失败", len(failures))
	}
	AuditLog("backup.create", backupDir, detail)
	return failures, nil
}
