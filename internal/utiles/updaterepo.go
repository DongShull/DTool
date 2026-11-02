package utiles

import (
	"os"
	"strings"
)

// UpdateRepo 返回自更新与版本检查所使用的仓库（owner/repo）。
// 默认指向本项目的自维护 fork，可通过环境变量 UPDATE_REPO 覆盖。
func UpdateRepo() string {
	repo := strings.TrimSpace(os.Getenv("UPDATE_REPO"))
	if repo == "" {
		repo = "DongShull/DTool"
	}
	return repo
}

// UpdateBranch 返回版本检查读取的分支，默认 main，可用 UPDATE_BRANCH 覆盖。
//
// 历史包袱：默认值曾是 latest，但仓库默认分支已改为 main，且远程已无 latest 分支，
// 导致版本检查 404（"获取版本错误：HTTP 404 Not Found"）、自更新与发布流水线全断。
func UpdateBranch() string {
	branch := strings.TrimSpace(os.Getenv("UPDATE_BRANCH"))
	if branch == "" {
		branch = "main"
	}
	return branch
}
