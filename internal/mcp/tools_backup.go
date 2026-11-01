package mcp

import (
	"context"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/DongShull/DTool/internal/utiles"
)

func registerBackupTools(s *server.MCPServer, ctx *svc.ServiceContext) {
	s.AddTool(mcp.NewTool("dt_backup_containers",
		mcp.WithDescription("备份容器配置为 JSON 文件，写入生效的备份目录（BACKUP_DIR > 设置 > /data/backups）；不传 include 备份全部容器，传了则只备份命中名单的容器；单个容器失败不中止整体备份，失败清单在返回的 failures 中"),
		mcp.WithArray("include", mcp.Items(map[string]any{"type": "string"}), mcp.Description("可选：只备份这些容器（名字或 ID，精确匹配）；不传备份全部")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var include []string
		if raw, ok := req.GetArguments()["include"]; ok && raw != nil {
			arr, ok := raw.([]any)
			if !ok {
				return mcp.NewToolResultError("include 必须是字符串数组"), nil
			}
			for _, item := range arr {
				s, ok := item.(string)
				if !ok {
					return mcp.NewToolResultError("include 必须是字符串数组"), nil
				}
				include = append(include, s)
			}
		}
		failures, backupErr := utiles.BackupContainer(ctx, include...)
		if backupErr != nil {
			return mcp.NewToolResultError(backupErr.Error()), nil
		}
		utiles.AuditLog("container.backup", "all", "backup completed")
		result := map[string]any{
			"result":   "backup completed",
			"dir":      utiles.EffectiveBackupDir(),
			"failures": failures,
		}
		if len(failures) > 0 {
			result["result"] = "backup completed with failures"
		}
		return jsonResult(result), nil
	})

	s.AddTool(mcp.NewTool("dt_backup_compose",
		mcp.WithDescription("把容器配置导出为 docker-compose YAML 备份"),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := utiles.Backup2Compose(ctx); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		utiles.AuditLog("container.backup2compose", "all", "compose backup completed")
		return textResult("compose backup completed, dir=" + utiles.EffectiveBackupDir()), nil
	})

	s.AddTool(mcp.NewTool("dt_list_backups",
		mcp.WithDescription("列出备份目录下的 .json / .yaml 备份文件"),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		list, err := utiles.BackupList(ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return jsonResult(map[string]any{
			"dir":   utiles.EffectiveBackupDir(),
			"files": list,
		}), nil
	})

	s.AddTool(mcp.NewTool("dt_compare_backups",
		mcp.WithDescription("对比两个 JSON 配置备份的差异：新增/移除的容器，以及镜像、环境变量、挂载、端口、重启策略、网络的变化；恢复前预览用它确认两个备份差在哪"),
		mcp.WithString("old", mcp.Required(), mcp.Description("旧备份文件名（基线，仅 .json）")),
		mcp.WithString("new", mcp.Required(), mcp.Description("新备份文件名（当前，仅 .json）")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		oldFile, err := mustString(args, "old")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		newFile, err := mustString(args, "new")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		result, cmpErr := utiles.CompareBackups(oldFile, newFile)
		if cmpErr != nil {
			return mcp.NewToolResultError(cmpErr.Error()), nil
		}
		utiles.AuditLog("container.backup.compare", oldFile+" → "+newFile, "diff computed")
		return textResult(utiles.FormatBackupDiff(result)), nil
	})

	s.AddTool(mcp.NewTool("dt_restore_container",
		mcp.WithDescription("从 JSON 备份恢复容器；force=true 时同名容器会被替换（改名旧容器→创建新容器→成功后删除旧容器，失败自动回滚）；nameMap 可把备份里的容器重定向到新名字恢复，不动原容器"),
		mcp.WithString("filename", mcp.Required(), mcp.Description("备份文件名（仅 .json）")),
		mcp.WithBoolean("start", mcp.Description("是否启动；不传则按备份记录的状态恢复")),
		mcp.WithBoolean("force", mcp.Description("是否替换同名容器，默认 false（同名则跳过）")),
		mcp.WithObject("nameMap", mcp.Description("可选：容器名重定向映射，键=备份中的容器名，值=恢复后的容器名；如 {\"nginx\":\"nginx-copy\"} 以副本方式恢复")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		filename, err := mustString(args, "filename")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		fullPath, err := utiles.ResolveBackupPath(filename, ".json")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		var startOverride *bool
		if _, ok := args["start"]; ok {
			v := optionalBool(args, "start", false)
			startOverride = &v
		}
		force := optionalBool(args, "force", false)
		var nameMap map[string]string
		if raw, ok := args["nameMap"]; ok && raw != nil {
			m, convErr := optionalStringMap(raw)
			if convErr != nil {
				return mcp.NewToolResultError("nameMap 非法: "+convErr.Error()), nil
			}
			nameMap = m
		}

		taskID := uuid.New().String()
		taskCtx := ctx.RegisterTask(taskID)
		defer ctx.FinishTask(taskID)

		restoreErr := utiles.RestoreContainer(taskCtx, ctx, filepath.Base(fullPath), taskID, utiles.RestoreOptions{
			StartOverride: startOverride,
			Force:         force,
			NameMap:       nameMap,
		})
		final, _ := ctx.GetProgress(taskID)
		message := final.Message
		if message == "" {
			message = "恢复任务已结束"
		}
		detail := final.DetailMsg
		if detail == "" {
			detail = message
		}
		utiles.NotifyTaskDone("restore", filename, taskID, restoreErr == nil, message)
		utiles.AuditLog("container.restore", filename, detail)
		if restoreErr != nil {
			return mcp.NewToolResultError(restoreErr.Error()), nil
		}
		return jsonResult(final), nil
	})

	s.AddTool(mcp.NewTool("dt_delete_backup",
		mcp.WithDescription("删除一个备份文件（只删备份文件，不动容器）"),
		mcp.WithString("filename", mcp.Required(), mcp.Description("备份文件名")),
	), func(ctx2 context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filename, err := mustString(req.GetArguments(), "filename")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		fullPath, err := utiles.ResolveBackupPath(filename, ".json", ".yaml")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err = os.Remove(fullPath); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		utiles.AuditLog("container.backup.delete", filename, "deleted")
		return textResult("deleted backup: " + filename), nil
	})
}
