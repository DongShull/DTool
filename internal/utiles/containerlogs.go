package utiles

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/DongShull/DTool/internal/svc"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// ---- 日志 ----

// ContainerLogs 读取容器最近 N 行日志（不跟随）。tail<=0 默认 200，上限 5000。
func ContainerLogs(ctx context.Context, serviceContext *svc.ServiceContext, id string, tail int, timestamps bool, since string) ([]string, error) {
	return ContainerLogsRange(ctx, serviceContext, id, tail, timestamps, since, "")
}

// ContainerLogsRange 在 ContainerLogs 基础上支持 until 参数，形成 since..until 闭区间查询
// （follow=false 一次性读取）。until 为空表示不设上界，行为与 ContainerLogs 完全一致。
func ContainerLogsRange(ctx context.Context, serviceContext *svc.ServiceContext, id string, tail int, timestamps bool, since, until string) ([]string, error) {
	if err := requireDocker(serviceContext); err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("容器 ID 不能为空")
	}
	tail = normalizeTail(tail)
	callCtx, cancel := dockerTimeout(ctx, dockerListTimeout)
	defer cancel()

	reader, err := serviceContext.DockerClient.ContainerLogs(callCtx, id, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       strconv.Itoa(tail),
		Timestamps: timestamps,
		Since:      since,
		Until:      until,
	})
	if err != nil {
		return nil, fmt.Errorf("获取容器日志失败: %w", err)
	}
	defer func() { _ = reader.Close() }()

	return decodeLogStream(reader, tail)
}

// normalizeTail 归一化 tail 参数：<=0 取默认 200，上限 5000。
func normalizeTail(tail int) int {
	if tail <= 0 {
		return 200
	}
	if tail > 5000 {
		return 5000
	}
	return tail
}

// decodeLogStream 把 docker 多路复用日志流解复用成行切片，并按 tail 截断保留最后 N 行。
// 容器的日志流是 docker 的多路复用格式（每帧 8 字节头：stream + 长度），
// 直接按行读会把帧头当成正文（表现为行首出现 \x01/\x02 与乱码）。
// 必须用 stdcopy 解复用：stdout/stderr 都汇到同一个 buffer。
func decodeLogStream(reader io.Reader, tail int) ([]string, error) {
	var buf bytes.Buffer
	if _, err := stdcopy.StdCopy(&buf, &buf, reader); err != nil {
		return nil, fmt.Errorf("读取容器日志失败: %w", err)
	}
	raw := strings.TrimRight(buf.String(), "\n")
	if raw == "" {
		return []string{}, nil
	}
	all := strings.Split(raw, "\n")
	if len(all) > tail {
		all = all[len(all)-tail:]
	}
	lines := make([]string, 0, len(all))
	for _, line := range all {
		lines = append(lines, cleanLogLine(line))
	}
	return lines, nil
}

// cleanLogLine 去掉 docker 多路复用流留下的 0x01/0x02 流标记与行尾 \r。
func cleanLogLine(line string) string {
	line = strings.TrimRight(line, "\r")
	return strings.TrimPrefix(strings.TrimPrefix(line, "\x01"), "\x02")
}
