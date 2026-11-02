package utiles

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// updateWorkDir 返回自更新换版操作的工作目录：二进制所在目录。
// start.sh 在 WORKDIR（默认 /app）里找 ./DTool-new，所以下载包、临时解压目录、
// 新二进制都必须落在同一目录；用进程 CWD（"."）会在非 Docker 部署下写错位置。
func updateWorkDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("无法定位程序自身路径: %w", err)
	}
	// /proc/self/exe 之类可能是符号链接，解析到真实二进制所在目录。
	if resolved, evalErr := filepath.EvalSymlinks(exe); evalErr == nil {
		exe = resolved
	}
	dir := filepath.Dir(exe)
	if info, statErr := os.Stat(dir); statErr != nil || !info.IsDir() {
		return "", fmt.Errorf("程序所在目录不可用: %s", dir)
	}
	return dir, nil
}

// ScheduleProcessExit 自更新成功后延迟退出，交给容器 restart 策略 / systemd / start.sh
// 拉起新版本。
//
// 退出码必须是 0：start.sh 会把「非 0 且运行不足 30 秒」的退出计为启动失败，
// 连续 3 次就会回退掉刚换上的新版本（原实现用 os.Exit(1)，连点三次更新即误回退）。
func ScheduleProcessExit(delay time.Duration) {
	go func() {
		time.Sleep(delay)
		logx.Info("自更新完成，进程退出以换入新版本")
		os.Exit(0)
	}()
}
