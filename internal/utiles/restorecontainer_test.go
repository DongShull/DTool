package utiles

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DongShull/DTool/internal/svc"
	dockerclient "github.com/docker/docker/client"
)

func TestRestoreShouldStart(t *testing.T) {
	trueValue := true
	falseValue := false
	tests := []struct {
		name     string
		entry    ContainerBackupEntry
		override *bool
		want     bool
	}{
		{name: "old backup defaults to start", entry: ContainerBackupEntry{}, want: true},
		{name: "known stopped stays stopped", entry: ContainerBackupEntry{StateKnown: true, WasRunning: false}, want: false},
		{name: "known running starts", entry: ContainerBackupEntry{StateKnown: true, WasRunning: true}, want: true},
		{name: "override start", entry: ContainerBackupEntry{StateKnown: true, WasRunning: false}, override: &trueValue, want: true},
		{name: "override stop", entry: ContainerBackupEntry{StateKnown: true, WasRunning: true}, override: &falseValue, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := restoreShouldStart(tt.entry, tt.override); got != tt.want {
				t.Fatalf("restoreShouldStart() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRestoreOldContainerName(t *testing.T) {
	at := time.Date(2026, 10, 30, 12, 34, 56, 123456789, time.UTC)
	got := restoreOldContainerName("demo", at)
	if !strings.HasPrefix(got, "demo-dc-old-") {
		t.Fatalf("restoreOldContainerName() = %q, want demo-dc-old- prefix", got)
	}
	if got == restoreOldContainerName("demo", at.Add(time.Nanosecond)) {
		t.Fatalf("restoreOldContainerName() should be unique per timestamp")
	}
}

func TestRollbackRestoredContainerNoop(t *testing.T) {
	if msg := rollbackRestoredContainer(nil, "", "", "", "", false, false); msg != "" {
		t.Fatalf("rollbackRestoredContainer() = %q, want empty", msg)
	}
}

// 回归用例：Restore 曾全程持有全局变更锁（含路径解析、读文件、镜像拉取），
// 测试先占住变更锁再调用，旧实现会死锁；新实现只在每条目的变更窗口取锁，
// 文件名非法这类前置校验应立即报错返回。
func TestRestoreContainerSetupDoesNotTakeMutationLock(t *testing.T) {
	t.Setenv("PROGRESS_STORE", filepath.Join(t.TempDir(), "progress.json"))
	unlock := LockContainerMutation()
	defer unlock()

	cli, err := dockerclient.NewClientWithOpts()
	if err != nil {
		t.Skipf("无法构造 Docker 客户端: %v", err)
	}
	svcCtx := &svc.ServiceContext{
		DockerClient:  cli,
		ProgressStore: make(svc.ProgressStoreType),
	}
	err = RestoreContainer(context.Background(), svcCtx, "../illegal-name", "test-task", RestoreOptions{})
	if err == nil {
		t.Fatal("RestoreContainer() expected illegal filename error")
	}
}
