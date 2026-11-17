package utiles

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/DongShull/DTool/internal/svc"
)

func newTestSvcCtx() *svc.ServiceContext {
	return &svc.ServiceContext{
		ProgressStore: make(svc.ProgressStoreType),
	}
}

type hookRecord struct {
	mu     sync.Mutex
	notify []string
	audit  []string
}

func (h *hookRecord) addNotify(s string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.notify = append(h.notify, s)
}

func (h *hookRecord) addAudit(s string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.audit = append(h.audit, s)
}

func testMeta() ManagedTaskMeta {
	return ManagedTaskMeta{
		Name:           "测试容器",
		NotifyKind:     "update",
		AuditAction:    "container.update",
		Placeholder:    svc.TaskProgress{Name: "测试容器", Message: "任务已排队"},
		PanicMessage:   "更新失败",
		PanicDetailFmt: "更新任务异常中断: %v",
		DoneFallback:   "更新任务已结束",
	}
}

// 正常完成：fn 无错，占位进度存在、最终进度 IsDone、Notify/Audit 被调用、taskID 可复用。
func TestRunManagedTaskSuccess(t *testing.T) {
	ctx := newTestSvcCtx()
	rec := &hookRecord{}
	hooks := &ManagedTaskHooks{
		Notify: func(kind, name, taskID string, ok bool, message string) {
			rec.addNotify(fmt.Sprintf("%s|%s|%s|%v|%s", kind, name, taskID, ok, message))
		},
		Audit: func(action, target, detail string) {
			rec.addAudit(fmt.Sprintf("%s|%s|%s", action, target, detail))
		},
	}

	taskID, doneCh := RunManagedTask(ctx, nil, testMeta(), hooks, func(taskCtx context.Context, taskID string) error {
		// fn 内可见占位进度（排队期间可查）。
		if _, ok := ctx.GetProgress(taskID); !ok {
			t.Error("placeholder progress missing while fn runs")
		}
		ctx.UpdateProgress(taskID, svc.TaskProgress{TaskID: taskID, Name: "测试容器", Message: "更新成功", DetailMsg: "done", IsDone: true})
		return nil
	})

	if err := <-doneCh; err != nil {
		t.Fatalf("doneCh err = %v, want nil", err)
	}
	if taskID == "" {
		t.Fatal("taskID empty")
	}
	final, ok := ctx.GetProgress(taskID)
	if !ok || !final.IsDone || final.Message != "更新成功" {
		t.Fatalf("final progress = %+v, ok=%v", final, ok)
	}
	if _, ok := ctx.GetProgress("not-registered"); ok {
		t.Fatal("unrelated task visible")
	}
	// FinishTask 已释放句柄：CancelActiveTask 应找不到任务（间接验证收尾）。
	if ctx.CancelTask(taskID) {
		t.Fatal("task still cancellable after FinishTask")
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.notify) != 1 || !strings.HasPrefix(rec.notify[0], "update|测试容器|"+taskID+"|true|更新成功") {
		t.Fatalf("notify = %v", rec.notify)
	}
	if len(rec.audit) != 1 || rec.audit[0] != "container.update|测试容器|done" {
		t.Fatalf("audit = %v", rec.audit)
	}
}

// fn panic：进程不崩，任务记失败（IsDone + PanicMessage + Detail），Notify ok=false，Audit 收到异常详情，doneCh 返回错误。
func TestRunManagedTaskPanic(t *testing.T) {
	ctx := newTestSvcCtx()
	rec := &hookRecord{}
	hooks := &ManagedTaskHooks{
		Notify: func(kind, name, taskID string, ok bool, message string) {
			rec.addNotify(fmt.Sprintf("%v|%s", ok, message))
		},
		Audit: func(action, target, detail string) {
			rec.addAudit(detail)
		},
	}

	taskID, doneCh := RunManagedTask(ctx, nil, testMeta(), hooks, func(taskCtx context.Context, taskID string) error {
		panic("boom")
	})

	err := <-doneCh
	if err == nil || !strings.Contains(err.Error(), "更新任务异常中断: boom") {
		t.Fatalf("doneCh err = %v, want panic detail", err)
	}
	final, ok := ctx.GetProgress(taskID)
	if !ok || !final.IsDone {
		t.Fatalf("final = %+v, ok=%v; want done+failed", final, ok)
	}
	if final.Message != "更新失败" || !strings.Contains(final.DetailMsg, "boom") {
		t.Fatalf("final = %+v; want PanicMessage + detail", final)
	}
	if ctx.CancelTask(taskID) {
		t.Fatal("task still cancellable after panic")
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.notify) != 1 || rec.notify[0] != "false|更新失败" {
		t.Fatalf("notify = %v", rec.notify)
	}
	if len(rec.audit) != 1 || !strings.Contains(rec.audit[0], "boom") {
		t.Fatalf("audit = %v", rec.audit)
	}
}

// AuditLog 在 fn 出错时也必须被调用，detail 兜底为进度 message。
func TestRunManagedTaskAuditCalledOnError(t *testing.T) {
	ctx := newTestSvcCtx()
	rec := &hookRecord{}
	hooks := &ManagedTaskHooks{
		Notify: func(kind, name, taskID string, ok bool, message string) {},
		Audit: func(action, target, detail string) {
			rec.addAudit(fmt.Sprintf("%s|%s|%s", action, target, detail))
		},
	}

	wantErr := errors.New("docker 说不行")
	_, doneCh := RunManagedTask(ctx, nil, testMeta(), hooks, func(taskCtx context.Context, taskID string) error {
		return wantErr
	})

	if err := <-doneCh; !errors.Is(err, wantErr) {
		t.Fatalf("doneCh err = %v, want %v", err, wantErr)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.audit) != 1 {
		t.Fatalf("audit calls = %d, want 1", len(rec.audit))
	}
	// fn 未写进度：detail 兜底为 DoneFallback。
	if rec.audit[0] != "container.update|测试容器|任务已排队" {
		t.Fatalf("audit = %s, want fallback detail", rec.audit[0])
	}
}
