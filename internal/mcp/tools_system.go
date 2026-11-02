package mcp

import (
	"context"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/utiles"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerSystemTools 注册只读控制面工具：系统概览、卷、网络、容器详情/日志/资源。
// 这些工具与 HTTP 侧 /api/system、/api/volumes、/api/networks、
// /api/container/{id}/{detail,logs,stats} 共用 utiles 实现，行为一致。
func registerSystemTools(s *server.MCPServer, ctx *svc.ServiceContext) {
	s.AddTool(mcp.NewTool("dt_system_info",
		mcp.WithDescription("Docker 系统概览：版本/API 版本、主机信息（CPU/内存/存储驱动/data-root）、磁盘占用与可回收空间"),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		overview, err := utiles.SystemOverview(ctx2, ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(overview), nil
	})

	s.AddTool(mcp.NewTool("dt_list_volumes",
		mcp.WithDescription("列出数据卷：驱动、挂载点、创建时间、是否被容器引用（inUse/usedBy），未被引用的排在前面"),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		volumes, err := utiles.ListVolumes(ctx2, ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(map[string]any{"count": len(volumes), "volumes": volumes}), nil
	})

	s.AddTool(mcp.NewTool("dt_remove_volume",
		mcp.WithDescription("删除一个数据卷（卷内数据不可恢复）；force=true 时由 daemon 强删"),
		mcp.WithString("name", mcp.Required(), mcp.Description("卷名")),
		mcp.WithBoolean("force", mcp.Description("是否强制删除，默认 false")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := mustString(req.GetArguments(), "name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		force := optionalBool(req.GetArguments(), "force", false)
		if err = utiles.RemoveVolume(ctx2, ctx, name, force); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(map[string]any{"removed": name, "force": force}), nil
	})

	s.AddTool(mcp.NewTool("dt_list_networks",
		mcp.WithDescription("列出 Docker 网络：驱动、子网/网关、是否 internal/attachable、每个网络上的容器数量"),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		networks, err := utiles.ListNetworks(ctx2, ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(map[string]any{"count": len(networks), "networks": networks}), nil
	})

	s.AddTool(mcp.NewTool("dt_container_detail",
		mcp.WithDescription("容器 inspect 摘要：状态/健康、端口映射、挂载、网络、compose 归属、资源限制；凭据类环境变量已打码"),
		mcp.WithString("id", mcp.Required(), mcp.Description("容器 ID 或名称")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := mustString(req.GetArguments(), "id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		detail, err := utiles.ContainerDetail(ctx2, ctx, id)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(detail), nil
	})

	s.AddTool(mcp.NewTool("dt_container_logs",
		mcp.WithDescription("读取容器最近 N 行日志（不跟随），用于排查启动失败原因"),
		mcp.WithString("id", mcp.Required(), mcp.Description("容器 ID 或名称")),
		mcp.WithNumber("tail", mcp.Description("行数，默认 200，上限 5000")),
		mcp.WithBoolean("timestamps", mcp.Description("是否带时间戳，默认 false")),
		mcp.WithString("since", mcp.Description("起始时间（RFC3339 或 Unix 秒），可选")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := mustString(req.GetArguments(), "id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		tail := int(optionalInt64(req.GetArguments(), "tail"))
		lines, err := utiles.ContainerLogs(ctx2, ctx, id, tail, optionalBool(req.GetArguments(), "timestamps", false), optionalString(req.GetArguments(), "since"))
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(map[string]any{"containerId": id, "lines": lines, "count": len(lines)}), nil
	})

	s.AddTool(mcp.NewTool("dt_container_stats",
		mcp.WithDescription("容器资源采样（一次性）：CPU%、内存占用/上限、网络收发、块设备读写、进程数"),
		mcp.WithString("id", mcp.Required(), mcp.Description("容器 ID 或名称")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := mustString(req.GetArguments(), "id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		stat, err := utiles.ContainerStatOnce(ctx2, ctx, id)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(stat), nil
	})
}
