package utiles

import (
	"regexp"
	"strings"
)

// backupFileNamePattern 限定"本工具自己生成的备份文件"命名：
// backup-<时间戳>.json / backup-<时间戳>.yaml。
//
// 为什么需要这条约束：备份目录可由设置改成任意可写绝对路径，
// 而删除备份接口以前只校验"在该目录内 + .json/.yaml 扩展名"。
// 一旦把 backupDir 指到 /data/config，就能删掉 progress.json、
// update-history.json、settings.json —— 属于可远程触发的数据销毁。
// 限定成本工具的命名格式后，这个面自然关闭。
var backupFileNamePattern = regexp.MustCompile(`^backup-[0-9A-Za-z._-]+\.(json|yaml)$`)

// IsBackupFileName 判断文件名是否是本工具生成的备份文件。
func IsBackupFileName(name string) bool {
	return backupFileNamePattern.MatchString(strings.TrimSpace(name))
}

// IsSafeBackupDir 判断备份目录是否可以安全使用/写入。
// 拒绝根目录与配置、日志、程序目录：这些目录被当成备份目录后，
// 删除备份的作用域会覆盖到配置与审计文件。
func IsSafeBackupDir(dir string) (bool, string) {
	clean := strings.TrimSpace(dir)
	if clean == "" {
		return false, "备份目录不能为空"
	}
	trimmed := strings.TrimRight(clean, "/")
	if trimmed == "" {
		return false, "不能使用根目录作为备份目录"
	}
	// 注意不要把 /data 整个禁掉：默认备份目录就是 /data/backups。
	denied := []string{"/data/config", "/data/logs", "/app", "/etc", "/proc", "/sys", "/dev", "/boot"}
	for _, d := range denied {
		if trimmed == d || strings.HasPrefix(trimmed, d+"/") {
			return false, "备份目录不能位于 " + d + " 之内（会与配置/日志/程序文件混放）"
		}
	}
	return true, ""
}

// CheckBackupDirSafe 检查当前生效的备份目录是否安全（供备份/删除前调用）。
// 不安全时返回原因，调用方应拒绝执行，避免备份文件或删除作用域混进配置目录。
func CheckBackupDirSafe() (string, string) {
	dir := BackupDir()
	if ok, reason := IsSafeBackupDir(dir); !ok {
		return dir, reason
	}
	return dir, ""
}
