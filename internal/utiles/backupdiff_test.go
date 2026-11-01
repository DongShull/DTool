package utiles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	dockerBackend "github.com/docker/docker/api/types/backend"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
)

// setupDiffBackupDir 建一个临时备份目录并把 BACKUP_DIR 指向它。
// 一次调用只建一个目录；writeDiffBackup 往里写文件，保证新旧备份同目录可对比。
func setupDiffBackupDir(t *testing.T) {
	t.Helper()
	t.Setenv("BACKUP_DIR", t.TempDir())
}

// writeDiffBackup 把备份条目写入 BACKUP_DIR。
func writeDiffBackup(t *testing.T, name string, entries []ContainerBackupEntry) string {
	t.Helper()
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal backup: %v", err)
	}
	full := filepath.Join(BackupDir(), name)
	if err := os.WriteFile(full, data, 0600); err != nil {
		t.Fatalf("write backup: %v", err)
	}
	return name
}

func diffEntryFor(name, image string, env []string, ports nat.PortMap) ContainerBackupEntry {
	return ContainerBackupEntry{
		ContainerCreateConfig: dockerBackend.ContainerCreateConfig{
			Name: name,
			Config: &container.Config{
				Image: image,
				Env:   env,
			},
			HostConfig: &container.HostConfig{
				PortBindings: ports,
			},
			NetworkingConfig: &network.NetworkingConfig{},
		},
	}
}

func TestCompareBackupsAddedRemovedUnchanged(t *testing.T) {
	oldEntries := []ContainerBackupEntry{
		diffEntryFor("web", "nginx:1.25", []string{"A=1"}, nil),
		diffEntryFor("gone", "redis:7", nil, nil),
	}
	newEntries := []ContainerBackupEntry{
		diffEntryFor("web", "nginx:1.25", []string{"A=1"}, nil),
		diffEntryFor("fresh", "mysql:8", nil, nil),
	}
	setupDiffBackupDir(t)
	oldFile := writeDiffBackup(t, "backup-old.json", oldEntries)
	newFile := writeDiffBackup(t, "backup-new.json", newEntries)

	result, err := CompareBackups(oldFile, newFile)
	if err != nil {
		t.Fatalf("CompareBackups: %v", err)
	}
	if len(result.Added) != 1 || result.Added[0] != "fresh" {
		t.Errorf("added = %v, want [fresh]", result.Added)
	}
	if len(result.Removed) != 1 || result.Removed[0] != "gone" {
		t.Errorf("removed = %v, want [gone]", result.Removed)
	}
	if result.UnchangedCount != 1 {
		t.Errorf("unchanged = %d, want 1", result.UnchangedCount)
	}
	if len(result.Changed) != 0 {
		t.Errorf("changed = %v, want empty", result.Changed)
	}
}

func TestCompareBackupsDetectsImageEnvPortChanges(t *testing.T) {
	oldPorts := nat.PortMap{}
	newPorts := nat.PortMap{}
	oldPort, _ := nat.NewPort("tcp", "80")
	newPort, _ := nat.NewPort("tcp", "8080")
	oldPorts[oldPort] = []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: "8080"}}
	newPorts[newPort] = []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: "9090"}}

	oldEntries := []ContainerBackupEntry{
		diffEntryFor("web", "nginx:1.25", []string{"A=1"}, oldPorts),
	}
	newEntries := []ContainerBackupEntry{
		diffEntryFor("web", "nginx:1.27", []string{"A=1", "B=2"}, newPorts),
	}
	setupDiffBackupDir(t)
	oldFile := writeDiffBackup(t, "backup-old.json", oldEntries)
	newFile := writeDiffBackup(t, "backup-new.json", newEntries)

	result, err := CompareBackups(oldFile, newFile)
	if err != nil {
		t.Fatalf("CompareBackups: %v", err)
	}
	if len(result.Changed) != 1 {
		t.Fatalf("changed = %v, want 1 entry", result.Changed)
	}
	c := result.Changed[0]
	want := map[string]bool{"image": true, "env": true, "ports": true}
	if len(c.Fields) != len(want) {
		t.Fatalf("fields = %v, want %v", c.Fields, want)
	}
	for _, f := range c.Fields {
		if !want[f] {
			t.Errorf("unexpected field %q in %v", f, c.Fields)
		}
	}
	if c.OldImage != "nginx:1.25" || c.NewImage != "nginx:1.27" {
		t.Errorf("images = %s → %s, want nginx:1.25 → nginx:1.27", c.OldImage, c.NewImage)
	}
}

func TestCompareBackupsEnvOrderIrrelevant(t *testing.T) {
	oldEntries := []ContainerBackupEntry{
		diffEntryFor("web", "nginx:1.25", []string{"A=1", "B=2"}, nil),
	}
	newEntries := []ContainerBackupEntry{
		diffEntryFor("web", "nginx:1.25", []string{"B=2", "A=1"}, nil),
	}
	setupDiffBackupDir(t)
	oldFile := writeDiffBackup(t, "backup-old.json", oldEntries)
	newFile := writeDiffBackup(t, "backup-new.json", newEntries)

	result, err := CompareBackups(oldFile, newFile)
	if err != nil {
		t.Fatalf("CompareBackups: %v", err)
	}
	if result.UnchangedCount != 1 || len(result.Changed) != 0 {
		t.Errorf("env order should not matter: changed=%v unchanged=%d", result.Changed, result.UnchangedCount)
	}
}

func TestCompareBackupsRejectsComposeAndTraversal(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BACKUP_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services: {}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := CompareBackups("compose.yaml", "compose.yaml"); err == nil {
		t.Error("compose 备份应被拒绝，不能当 JSON 配置对比")
	}
	if _, err := CompareBackups("../secret.json", "x.json"); err == nil {
		t.Error("路径穿越应被拒绝")
	}
}
