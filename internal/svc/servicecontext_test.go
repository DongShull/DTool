package svc

import (
	"encoding/json"

	"github.com/DongShull/DTool/internal/config"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestProgressConcurrentUpdatesPersistValidJSON(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "progress.json")
	t.Setenv("PROGRESS_STORE", storePath)

	ctx := &ServiceContext{ProgressStore: make(ProgressStoreType)}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx.UpdateProgress(fmt.Sprintf("task-%d", i), TaskProgress{
				TaskID:     fmt.Sprintf("task-%d", i),
				Percentage: 100,
				Message:    "done",
				IsDone:     true,
			})
		}(i)
	}
	wg.Wait()

	data, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("read progress store: %v", err)
	}
	var stored ProgressStoreType
	if err = json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("progress store is not valid JSON: %v\n%s", err, data)
	}
	if len(stored) != 20 {
		t.Fatalf("stored tasks = %d, want 20", len(stored))
	}
	if progress, ok := ctx.GetProgress("task-7"); !ok || !progress.IsDone {
		t.Fatalf("task-7 not persisted in memory: %+v, %v", progress, ok)
	}
}

func TestFilterProgressByStatusAndName(t *testing.T) {
	ctx := &ServiceContext{ProgressStore: make(ProgressStoreType)}
	ctx.ProgressStore["t1"] = TaskProgress{TaskID: "t1", Name: "更新容器", IsDone: true, StartedAt: 100}
	ctx.ProgressStore["t2"] = TaskProgress{TaskID: "t2", Name: "恢复容器", IsDone: true, Cancelled: true, StartedAt: 200}
	ctx.ProgressStore["t3"] = TaskProgress{TaskID: "t3", Name: "更新容器", IsDone: false, StartedAt: 300}

	if got := len(ctx.FilterProgress("", "")); got != 3 {
		t.Fatalf("all = %d, want 3", got)
	}
	if got := ctx.FilterProgress("running", ""); len(got) != 1 || got[0].TaskID != "t3" {
		t.Fatalf("running = %+v, want [t3]", got)
	}
	if got := ctx.FilterProgress("cancelled", ""); len(got) != 1 || got[0].TaskID != "t2" {
		t.Fatalf("cancelled = %+v, want [t2]", got)
	}
	if got := ctx.FilterProgress("done", ""); len(got) != 1 || got[0].TaskID != "t1" {
		t.Fatalf("done = %+v, want [t1]", got)
	}
	if got := ctx.FilterProgress("", "更新"); len(got) != 2 {
		t.Fatalf("name=更新: got %d, want 2", len(got))
	}
	if got := ctx.FilterProgress("", "更新"); got[0].TaskID != "t3" {
		t.Fatalf("name filter should keep newest first, got %s first", got[0].TaskID)
	}
	// 非法状态码视为全部，而不是静默空列表
	if got := len(ctx.FilterProgress("bogus", "")); got != 3 {
		t.Fatalf("bogus status = %d, want 3 (treated as all)", got)
	}
}

func TestUpdateProgressMarksCancelledFromMessage(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "progress.json")
	t.Setenv("PROGRESS_STORE", storePath)
	ctx := &ServiceContext{ProgressStore: make(ProgressStoreType)}

	// 任务执行代码报“任务已取消”时，UpdateProgress 自动补 Cancelled 标记
	ctx.UpdateProgress("x", TaskProgress{TaskID: "x", Percentage: 50, IsDone: false})
	ctx.UpdateProgress("x", TaskProgress{TaskID: "x", Percentage: 100, Message: "任务已取消", DetailMsg: "任务已取消", IsDone: true})
	got, ok := ctx.GetProgress("x")
	if !ok {
		t.Fatal("task x missing")
	}
	if !got.Cancelled {
		t.Fatalf("cancelled flag not set: %+v", got)
	}
	if got.Status() != "cancelled" {
		t.Fatalf("status = %s, want cancelled", got.Status())
	}
	if got.CancelRequestedAt == 0 {
		t.Error("CancelRequestedAt should be filled when missing")
	}

	// 正常完成的任务不能被误标
	ctx.UpdateProgress("y", TaskProgress{TaskID: "y", Percentage: 100, Message: "更新完成", IsDone: true})
	gotY, _ := ctx.GetProgress("y")
	if gotY.Cancelled || gotY.Status() != "done" {
		t.Fatalf("normal done task misclassified: %+v", gotY)
	}

	// 已有 CancelRequestedAt（取消发起时间）不被覆盖
	ctx.UpdateProgress("z", TaskProgress{TaskID: "z", IsDone: false})
	ctx.UpdateProgress("z", TaskProgress{TaskID: "z", Message: "正在取消...", DetailMsg: "已发出取消请求", IsDone: true, Cancelled: true, CancelRequestedAt: 12345})
	gotZ, _ := ctx.GetProgress("z")
	if gotZ.CancelRequestedAt != 12345 {
		t.Fatalf("CancelRequestedAt = %d, want 12345 (preserved)", gotZ.CancelRequestedAt)
	}
}

func TestLoadProgressRestoresCancelledFlag(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "progress.json")
	t.Setenv("PROGRESS_STORE", storePath)
	legacy := ProgressStoreType{
		"c1": {TaskID: "c1", Message: "任务已取消", DetailMsg: "任务已取消", IsDone: true, FinishedAt: 100},
	}
	data, _ := json.Marshal(legacy)
	if err := os.WriteFile(storePath, data, 0644); err != nil {
		t.Fatal(err)
	}
	ctx := NewServiceContext(config.Config{})
	got, ok := ctx.GetProgress("c1")
	if !ok {
		t.Fatal("c1 not loaded")
	}
	// 历史磁盘数据没有 Cancelled 字段，读取时按取消文案补齐
	if !got.Cancelled || got.Status() != "cancelled" {
		t.Fatalf("legacy cancelled task not restored: %+v", got)
	}
}
