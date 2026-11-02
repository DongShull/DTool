package mcp

import (
	"context"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerProgressTools(s *server.MCPServer, ctx *svc.ServiceContext) {
	s.AddTool(mcp.NewTool("dt_list_progress",
		mcp.WithDescription("列出任务进度（含重启后从磁盘恢复的历史任务）；可按状态与任务名过滤"),
		mcp.WithString("status", mcp.Description("可选：running / cancelled / done；不传返回全部")),
		mcp.WithString("name", mcp.Description("可选：按任务名子串过滤（不区分大小写）")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		return jsonResult(ctx.FilterProgress(optionalString(args, "status"), optionalString(args, "name"))), nil
	})

	s.AddTool(mcp.NewTool("dt_export_progress",
		mcp.WithDescription("导出任务进度为 JSON 文本（含汇总：总数/按状态计数 + 任务清单）；过滤条件与列表一致，另支持 from/to 时间范围（Unix 秒，按任务开始时间）"),
		mcp.WithString("status", mcp.Description("可选：running / cancelled / done；不传返回全部")),
		mcp.WithString("name", mcp.Description("可选：按任务名子串过滤（不区分大小写）")),
		mcp.WithNumber("from", mcp.Description("可选：开始时间下限（Unix 秒）")),
		mcp.WithNumber("to", mcp.Description("可选：开始时间上限（Unix 秒）")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		list := ctx.FilterProgress(optionalString(args, "status"), optionalString(args, "name"))
		from := optionalInt64(args, "from")
		to := optionalInt64(args, "to")
		filtered := make([]svc.TaskProgress, 0, len(list))
		summary := map[string]int{"running": 0, "cancelled": 0, "done": 0}
		for _, p := range list {
			if from > 0 && p.StartedAt < from {
				continue
			}
			if to > 0 && p.StartedAt > to {
				continue
			}
			summary[p.Status()]++
			filtered = append(filtered, p)
		}
		return jsonResult(map[string]any{
			"summary": map[string]any{"total": len(filtered), "byStatus": summary},
			"tasks":   filtered,
		}), nil
	})

	s.AddTool(mcp.NewTool("dt_get_progress",
		mcp.WithDescription("查询单个任务的进度详情"),
		mcp.WithString("taskId", mcp.Required(), mcp.Description("任务 ID")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := mustString(req.GetArguments(), "taskId")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		p, ok := ctx.GetProgress(taskID)
		if !ok {
			return mcp.NewToolResultError("task not found: " + taskID), nil
		}
		return jsonResult(p), nil
	})

	s.AddTool(mcp.NewTool("dt_cancel_progress",
		mcp.WithDescription("取消一个进行中的任务（任务会在下一个步骤边界退出）"),
		mcp.WithString("taskId", mcp.Required(), mcp.Description("任务 ID")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := mustString(req.GetArguments(), "taskId")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		// 与 HTTP 通道一致：CancelActiveTask 会原子检查任务是否已结束，
		// 并把 cancelled / cancelRequestedAt 落库；旧的 CancelTask 只 cancel 不记账，
		// 导致 MCP 返回"已取消"但列表里仍是 done。
		ok, msg := ctx.CancelActiveTask(taskID)
		if !ok {
			return mcp.NewToolResultError("task not cancellable: " + taskID + "（" + msg + "）"), nil
		}
		return textResult("cancel requested: " + taskID), nil
	})
}
