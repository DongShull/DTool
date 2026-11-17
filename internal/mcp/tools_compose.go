package mcp

// Compose 工程级 MCP 工具：list / up / down / logs。
// 与 HTTP /api/compose/* 共用 utiles 实现；up/down 为写操作，
// 审计在 utiles.ComposeProjectAction 内统一落盘（含自身工程护栏）。

import (
	"context"
	"fmt"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/utiles"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerComposeTools(s *server.MCPServer, ctx *svc.ServiceContext) {
	s.AddTool(mcp.NewTool("dt_list_compose_projects",
		mcp.WithDescription("列出全部 Docker Compose 工程：项目名、服务数、workdir、运行/停止容器数与整体状态"),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projects, err := utiles.ListComposeProjects(ctx2, ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(map[string]any{"count": len(projects), "projects": projects}), nil
	})

	s.AddTool(mcp.NewTool("dt_compose_up",
		mcp.WithDescription("启动一个 Compose 工程（等价 docker compose up -d，工作目录取自容器标签）；DTool 自身所在工程禁止操作"),
		mcp.WithString("project", mcp.Required(), mcp.Description("compose 工程名（com.docker.compose.project 标签值）")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		project, err := mustString(req.GetArguments(), "project")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		output, err := utiles.ComposeProjectAction(ctx2, ctx, project, "up")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("%v %s", err, output)), nil
		}
		return jsonResult(map[string]any{"project": project, "action": "up", "output": output}), nil
	})

	s.AddTool(mcp.NewTool("dt_compose_down",
		mcp.WithDescription("停止并移除一个 Compose 工程（等价 docker compose down，会删除容器与默认网络）；DTool 自身所在工程禁止操作"),
		mcp.WithString("project", mcp.Required(), mcp.Description("compose 工程名（com.docker.compose.project 标签值）")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		project, err := mustString(req.GetArguments(), "project")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		output, err := utiles.ComposeProjectAction(ctx2, ctx, project, "down")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("%v %s", err, output)), nil
		}
		return jsonResult(map[string]any{"project": project, "action": "down", "output": output}), nil
	})

	s.AddTool(mcp.NewTool("dt_compose_logs",
		mcp.WithDescription("读取 Compose 工程日志：聚合各服务容器最近 N 行（service 为空时聚合全部服务，不跟随）"),
		mcp.WithString("project", mcp.Required(), mcp.Description("compose 工程名")),
		mcp.WithString("service", mcp.Description("只取该服务的日志，默认聚合全部")),
		mcp.WithNumber("tail", mcp.Description("行数，默认 200，上限 5000")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		project, err := mustString(req.GetArguments(), "project")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		service := optionalString(req.GetArguments(), "service")
		tail := int(optionalInt64(req.GetArguments(), "tail"))
		logs, err := utiles.ComposeProjectLogs(ctx2, ctx, project, service, tail)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(map[string]any{"project": project, "service": service, "services": logs}), nil
	})
}
