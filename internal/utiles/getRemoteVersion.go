package utiles

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/DongShull/DTool/internal/config"
	"github.com/zeromicro/go-zero/core/logx"
)

// ErrFnosManagedUpdate 保留旧名以兼容既有调用方；新代码请用 ErrPlatformManagedUpdate。
var ErrFnosManagedUpdate = ErrPlatformManagedUpdate

// ErrPlatformManagedUpdate 表示该平台版本（FnOS / UGREEN）由专用发布包管理，
// 不走通用的 GitHub 在线版本检查与自更新链。
var ErrPlatformManagedUpdate = errors.New("该平台版本由专用发布包管理，不支持在线版本检查与自更新")

// platformManagedUpdateError 携带平台名，同时可被 errors.Is(ErrPlatformManagedUpdate) 命中。
type platformManagedUpdateError struct {
	Platform string
}

func (e *platformManagedUpdateError) Error() string {
	return e.Platform + " 版本由专用发布包管理，不支持在线版本检查与自更新"
}

func (e *platformManagedUpdateError) Is(target error) bool {
	return target == ErrPlatformManagedUpdate
}

// ManagedUpdatePlatform 判断版本号是否属于"专用发布包管理"的平台构建。
// 同时覆盖 FNOS、UGREEN，并且大小写不敏感（旧实现只判大写 FNOS，
// v2.x-fnos 这类格式会漏判，导致专用版被通用版覆盖）。
func ManagedUpdatePlatform(version string) (string, bool) {
	upper := strings.ToUpper(version)
	switch {
	case strings.Contains(upper, "FNOS"):
		return "FnOS", true
	case strings.Contains(upper, "UGREEN"):
		return "UGREEN", true
	}
	return "", false
}

func GetRemoteVersion() (remoteVersion string, err error) {
	localVersion := config.Version
	if platform, managed := ManagedUpdatePlatform(localVersion); managed {
		// 专用版使用独立发布包，不需要也不应该访问通用版 version 文件。
		logx.Infof("%s 版本，不支持在线版本检查", platform)
		return "", &platformManagedUpdateError{Platform: platform}
	}

	githubProxy := os.Getenv("githubProxy")
	if githubProxy != "" {
		githubProxy = strings.TrimRight(githubProxy, "/") + "/"
	}
	versionURL := githubProxy + "https://raw.githubusercontent.com/" + UpdateRepo() + "/" + UpdateBranch() + "/version"
	remoteVersion, err = fetchVersionFromURL(versionURL)
	if err != nil {
		return "0.0.0", err
	}
	if localVersion == remoteVersion {
		logx.Info("版本一致:", localVersion)
		return remoteVersion, nil
	} else {
		logx.Infof("版本不一致! 本地: %s, 远程: %s\n", localVersion, remoteVersion)
		return remoteVersion, nil
	}

}

func fetchVersionFromURL(url string) (string, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}

	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logx.Error("关闭Body失败:", err)
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("获取版本信息失败：HTTP %s（检查 UPDATE_REPO=%s、UPDATE_BRANCH=%s）", resp.Status, UpdateRepo(), UpdateBranch())
	}

	versionData, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(versionData)), nil
}
