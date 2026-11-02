package utiles

import (
	"fmt"

	"github.com/DongShull/DTool/internal/svc"
	backupCompose "github.com/DongShull/DTool/internal/utiles/backup_compose"
	dockerTypes "github.com/docker/docker/api/types"
	"github.com/zeromicro/go-zero/core/logx"
)

func Backup2Compose(ctx *svc.ServiceContext) (err error) {
	// 与 JSON 备份共用同一把锁：两类备份写同一目录，并发会让保留份数计数错乱。
	backupMu.Lock()
	defer backupMu.Unlock()
	if dir, reason := CheckBackupDirSafe(); dir != "" && reason != "" {
		return fmt.Errorf("%s", reason)
	}
	containerList, err := GetContainerList(ctx)
	if err != nil {
		return err
	}
	var containerJSONs []dockerTypes.ContainerJSON
	for _, v := range containerList {
		containerID := v.ID
		inspectedContainer, err := GetContainerInspect(ctx, containerID)
		if err != nil {
			logx.Error("获取容器信息失败" + err.Error())
			return err
		}
		containerJSONs = append(containerJSONs, inspectedContainer)
	}
	err = backupCompose.DockerConfig2ComposeYaml(containerJSONs, BackupDir())
	if err != nil {
		logx.Error("备份失败" + err.Error())
		return err
	}
	// 与 JSON 备份共用保留策略：超出 BACKUP_KEEP（默认 7）的旧 compose 备份自动清理。
	pruneOldBackups(BackupDir(), "backup-*.yaml", BackupKeepCount())
	// compose 导出包含全部容器的 env（可能含密码），属于敏感动作，必须留痕。
	AuditLog("backup.compose", BackupDir(), "已导出全部容器为 compose 备份")
	return nil
}
