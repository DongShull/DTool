package utiles

import "testing"

func TestIsBackupFileName(t *testing.T) {
	ok := []string{
		"backup-2026-11-02-101010.json",
		"backup-2026-11-02-101010.123456789.json",
		"backup-2026-11-02-101010.yaml",
	}
	bad := []string{
		"progress.json",          // 进度文件：曾经可以被 DELETE /backups 删掉
		"update-history.json",    // 更新历史
		"settings.json",          // 设置
		"backup-x.txt",           // 非备份扩展名
		"../backup-x.json",       // 路径穿越
		"backup-",                // 空时间戳
		"BACKUP-2026-11-02.json", // 大小写不符
		"config.yaml",
	}
	for _, name := range ok {
		if !IsBackupFileName(name) {
			t.Errorf("IsBackupFileName(%q) = false, want true", name)
		}
	}
	for _, name := range bad {
		if IsBackupFileName(name) {
			t.Errorf("IsBackupFileName(%q) = true, want false", name)
		}
	}
}

func TestIsSafeBackupDir(t *testing.T) {
	safe := []string{"/data/backups", "/vol1/1000/backups", "/mnt/x/y"}
	unsafe := []string{
		"", "/", "/data/config", "/data/config/image",
		"/data/logs", "/app", "/etc", "/proc", "/sys", "/dev", "/boot",
	}
	for _, dir := range safe {
		if ok, reason := IsSafeBackupDir(dir); !ok {
			t.Errorf("IsSafeBackupDir(%q) = false (%s), want true", dir, reason)
		}
	}
	for _, dir := range unsafe {
		if ok, _ := IsSafeBackupDir(dir); ok {
			t.Errorf("IsSafeBackupDir(%q) = true, want false", dir)
		}
	}
	// 尾斜杠也要按同一规则判定
	if ok, _ := IsSafeBackupDir("/data/config/"); ok {
		t.Errorf("IsSafeBackupDir(/data/config/) = true, want false")
	}
}
