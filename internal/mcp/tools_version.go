package mcp

import (
	"context"
	"errors"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/DongShull/DTool/internal/config"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/utiles"
)

func registerVersionTools(s *server.MCPServer, ctx *svc.ServiceContext) {
	s.AddTool(mcp.NewTool("dt_get_version",
		mcp.WithDescription("获取当前本地版本号与构建时间"),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return jsonResult(map[string]string{
			"version":   config.Version,
			"buildDate": config.BuildDate,
		}), nil
	})

	s.AddTool(mcp.NewTool("dt_check_program_update",
		mcp.WithDescription("检查程序是否有新版本（比对远端仓库版本号）；FNOS 版本由专用发布包管理，会返回 fnos=true"),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		remote, err := utiles.GetRemoteVersion()
		if errors.Is(err, utiles.ErrFnosManagedUpdate) {
			// FNOS 版本不走通用更新链：明确告知调用方，而不是伪装成“无更新”。
			return jsonResult(map[string]any{
				"localVersion": config.Version,
				"fnos":         true,
				"hasUpdate":    false,
				"hint":         err.Error(),
			}), nil
		}
		if err != nil {
			return mcp.NewToolResultError("获取远程版本失败: " + err.Error()), nil
		}
		return jsonResult(map[string]any{
			"localVersion":  config.Version,
			"remoteVersion": remote,
			"hasUpdate":     remote != config.Version,
		}), nil
	})

	s.AddTool(mcp.NewTool("dt_update_program",
		mcp.WithDescription("更新 DTool 自身：下载新版本、校验、解压并重启进程（会中断当前服务）；target 不传更新到最新版，传了则强制更新到指定版本（允许降级，版本号只允许字母数字与 . - _ +）"),
		mcp.WithString("target", mcp.Description("可选：目标版本号（如 v2.2.1）；不传更新到最新版")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if utiles.UpdateProgramDisabled() {
			return mcp.NewToolResultError(utiles.ErrUpdateProgramDisabled.Error()), nil
		}
		target := optionalString(req.GetArguments(), "target")
		if target != "" {
			if _, chkErr := utiles.SanitizeUpdateTarget(target); chkErr != nil {
				return mcp.NewToolResultError(chkErr.Error()), nil
			}
		}
		if err := utiles.UpdateProgram(ctx, utiles.UpdateOptions{Target: target, Manual: true}); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		utiles.AuditLog("program.update", "self", "program update triggered")
		return textResult("program update triggered, service will restart"), nil
	})

	s.AddTool(mcp.NewTool("dt_get_update_history",
		mcp.WithDescription("程序自更新历史：每次更新（成功或失败）一条记录，含前后版本、手动/自动、结果说明；最新在前"),
		mcp.WithNumber("limit", mcp.Description("可选：最多返回条数；不传返回全部")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		limit := int(optionalInt64(req.GetArguments(), "limit"))
		return jsonResult(map[string]any{
			"currentVersion": config.Version,
			"history":        utiles.UpdateHistory(limit),
		}), nil
	})
}
