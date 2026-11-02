package utiles

import (
	"os"

	"github.com/DongShull/DTool/internal/svc"
)

func BackupList(ctx *svc.ServiceContext) ([]string, error) {
	var backupList []string
	dir := BackupDir() // env BACKUP_DIR > Settings > 默认 /data/backups
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		// 只列本工具生成的 backup-*.json / backup-*.yaml：备份目录可被设置到
		// 任意路径，按扩展名枚举会把目录里的其它 json（配置、进度、历史）也列出来。
		if !entry.IsDir() && IsBackupFileName(entry.Name()) {
			backupList = append(backupList, entry.Name())
		}
	}

	return backupList, nil
}
