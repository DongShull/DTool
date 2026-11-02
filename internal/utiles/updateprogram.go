package utiles

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/DongShull/DTool/internal/config"
	"github.com/DongShull/DTool/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// updateProgramMu 串行化自更新：并发两次调用会交错写同一个
// DTool.tar.gz / DTool-new，可能装上损坏或错版的包。
var updateProgramMu sync.Mutex

// ErrUpdateProgramRunning 已有一轮自更新在执行时返回。
var ErrUpdateProgramRunning = errors.New("已有一轮自更新在执行，请稍后再试")

// UpdateOptions 自更新的可选参数。
type UpdateOptions struct {
	// Target 为空时更新到远端 version 文件指向的最新版；
	// 非空时强制更新到指定版本（允许跨版本、降级）。版本号经 SanitizeUpdateTarget 校验。
	Target string
	// Manual 手动触发（HTTP/MCP）；自动更新传 false，用于历史记录区分。
	Manual bool
}

// UpdateProgram 执行一轮程序自更新：检查远端版本 → 下载 → SHA256 校验 → 解压换入。
// 任一步失败都会记录失败历史并发送失败通知（如已配置 webhook）。
func UpdateProgram(ctx *svc.ServiceContext, opts UpdateOptions) (err error) {
	fromVersion := config.Version
	targetVersion := ""
	defer func() {
		// 统一记录更新历史：成功失败都记，手动/自动区分；失败时额外发通知。
		AppendUpdateHistory(UpdateHistoryEntry{
			FromVersion: fromVersion,
			ToVersion:   targetVersion,
			OK:          err == nil,
			Manual:      opts.Manual,
			Message:     updateResultMessage(err),
		})
		if err != nil && strings.Contains(err.Error(), "FNOS") {
			// FNOS 的拒绝属于配置约束，不算更新失败，不进历史不发通知。
			RemoveLastUpdateHistory()
			return
		}
		if err != nil {
			NotifyTaskDone("program-update", fromVersion, "", false, "程序自更新失败: "+err.Error())
		}
	}()
	if !updateProgramMu.TryLock() {
		return ErrUpdateProgramRunning
	}
	defer updateProgramMu.Unlock()

	if platform, managed := ManagedUpdatePlatform(config.Version); managed {
		// 专用版二进制与通用版互不兼容：被通用包覆盖后无法自行回退。
		return fmt.Errorf("%s 版本请使用专用发布包更新，禁止覆盖为通用版", platform)
	}
	if platform, managed := ManagedUpdatePlatform(opts.Target); managed {
		return fmt.Errorf("目标版本 %s 属于 %s 专用包，禁止用通用版覆盖（如需切换请用专用发布包）", opts.Target, platform)
	}
	if opts.Target != "" {
		v, sanitizeErr := SanitizeUpdateTarget(opts.Target)
		if sanitizeErr != nil {
			return sanitizeErr
		}
		targetVersion = v
	}
	githubProxy := os.Getenv("githubProxy")
	if githubProxy != "" {
		githubProxy = strings.TrimRight(githubProxy, "/") + "/"
	}
	versionURL := githubProxy + "https://raw.githubusercontent.com/" + UpdateRepo() + "/" + UpdateBranch() + "/version"
	releaseBaseURL := githubProxy + "https://github.com/" + UpdateRepo() + "/releases/download"
	version := targetVersion
	if version == "" {
		// 未指定目标版本：按远端 version 文件更新到最新。
		logx.Infof("版本检查地址: %s", versionURL)
		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Get(versionURL)
		if err != nil {
			logx.Info("没有获取到最新版本信息:", err)
			return err
		}
		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				logx.Error("关闭resp.Body失败:", err)
			}
		}(resp.Body)

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("获取版本信息失败: HTTP %s", resp.Status)
		}
		versionData, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		logx.Infof("远端版本文件内容: %s", versionData)
		if err != nil {
			logx.Info("没有获取到最新版本信息:", err)
			return err
		}
		version = strings.TrimSpace(string(versionData))
		if v, sanitizeErr := SanitizeUpdateTarget(version); sanitizeErr != nil {
			return fmt.Errorf("远端版本号非法: %w", sanitizeErr)
		} else {
			version = v
		}
		targetVersion = version
	}
	logx.Info("目标版本：", version, "（当前 ", fromVersion, "）")
	// 2. 构造下载链接
	downloadURL := fmt.Sprintf("%s/%s/DTool-%s.tar.gz", releaseBaseURL, version, runtime.GOARCH)
	logx.Info("下载链接：", downloadURL)
	workDir, err := updateWorkDir()
	if err != nil {
		return err
	}
	dest := filepath.Join(workDir, "DTool.tar.gz")
	checksumDest := dest + ".sha256"
	defer func() {
		_ = os.Remove(dest)
		_ = os.Remove(checksumDest)
	}()

	if err := downloadFile(downloadURL, dest); err != nil {
		logx.Error("下载失败:", err)
		return err
	}
	if os.Getenv("UPDATE_SKIP_CHECKSUM") != "true" {
		if err := downloadFile(downloadURL+".sha256", checksumDest); err != nil {
			return fmt.Errorf("下载校验文件失败（如为旧版本可临时设置 UPDATE_SKIP_CHECKSUM=true）: %w", err)
		}
		checksumData, readErr := os.ReadFile(checksumDest)
		if readErr != nil {
			return readErr
		}
		fields := strings.Fields(string(checksumData))
		if len(fields) == 0 {
			return fmt.Errorf("校验文件为空")
		}
		if err := verifySHA256(dest, fields[0]); err != nil {
			return fmt.Errorf("安装包 SHA256 校验失败: %w", err)
		}
		logx.Info("SHA256 校验通过")
	}
	logx.Info("下载成功")

	// 先解压到临时目录，成功后再原子替换；避免解压失败留下半个 DTool-new
	// 被 start.sh 在下次启动时误当成新版本。
	tmpDir, err := os.MkdirTemp(workDir, ".dc-update-")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()
	if err := decompressTarGz(dest, tmpDir); err != nil {
		logx.Info("解压缩失败:", err)
		return err
	}

	candidates := []string{
		filepath.Join(tmpDir, "DTool-new"),
		filepath.Join(tmpDir, "dist", "linux", runtime.GOARCH, "DTool-new"),
	}
	for _, candidate := range candidates {
		if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() {
			// start.sh 在 WORKDIR（默认 /app）找 ./DTool-new，所以必须落在二进制同目录，
			// 否则会出现“历史/审计/通知都说成功、实际没换版”的静默空更新。
			if err := os.Rename(candidate, filepath.Join(workDir, "DTool-new")); err != nil {
				return err
			}
			logx.Info("解压缩成功")
			return nil
		}
	}
	return fmt.Errorf("解压完成但未找到 DTool-new")
}

const (
	maxUpdateAssetSize      = 512 << 20 // 512MB：下载包大小上限，防止异常响应写满磁盘
	maxUpdateFileSize       = 256 << 20 // 256MB：解压后单文件上限
	maxUpdateTotalSize      = 1 << 30   // 1GB：解压后累计大小上限（正常发布包 ~30MB）
	maxUpdateArchiveEntries = 10000
)

func downloadFile(url string, dest string) error {
	client := &http.Client{
		Timeout: 10 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("重定向次数过多")
			}
			// 禁止 HTTPS 被重定向到明文 HTTP：发布包与校验文件走同一条链，
			// 降级重定向会让中间人同时替换两者。
			if req.URL.Scheme != "https" && req.URL.Scheme != "http" {
				return fmt.Errorf("不支持的重定向协议: %s", req.URL.Scheme)
			}
			if req.URL.Scheme == "http" && len(via) > 0 && via[len(via)-1].URL.Scheme == "https" {
				return fmt.Errorf("拒绝 HTTPS 到 HTTP 的降级重定向")
			}
			return nil
		},
	}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logx.Error("关闭resp.Body失败:", err)
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载 %s 失败: HTTP %s", url, resp.Status)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func(out *os.File) {
		err := out.Close()
		if err != nil {
			logx.Error("关闭out失败:", err)
		}
	}(out)

	written, err := io.Copy(out, io.LimitReader(resp.Body, maxUpdateAssetSize+1))
	if err != nil {
		return err
	}
	if written > maxUpdateAssetSize {
		return fmt.Errorf("下载文件超过大小限制 %d 字节", maxUpdateAssetSize)
	}
	return nil
}

func verifySHA256(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return err
	}
	actual := fmt.Sprintf("%x", h.Sum(nil))
	if !strings.EqualFold(actual, strings.TrimSpace(expected)) {
		return fmt.Errorf("期望 %s，实际 %s", expected, actual)
	}
	return nil
}

func decompressTarGz(gzFilePath string, dest string) error {
	file, err := os.Open(gzFilePath)
	if err != nil {
		return err
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			logx.Error("关闭file失败:", err)
		}
	}(file)

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func(gzr *gzip.Reader) {
		err := gzr.Close()
		if err != nil {
			logx.Error("关闭gzr失败:", err)
		}
	}(gzr)

	tarReader := tar.NewReader(gzr)

	// 只有压缩包大小上限（maxUpdateAssetSize）挡不住 gzip 炸弹：
	// 解压侧必须自带条目数、单文件与累计解压量限制，否则能写满容器 overlay。
	var totalWritten int64
	entryCount := 0

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		entryCount++
		if entryCount > maxUpdateArchiveEntries {
			return fmt.Errorf("压缩包条目数超过限制 %d", maxUpdateArchiveEntries)
		}
		target := filepath.Join(dest, filepath.FromSlash(header.Name))
		// 防止 zip-slip：用绝对路径判断，避免 dest="." 时把正常文件全部误判。
		absDest, err := filepath.Abs(dest)
		if err != nil {
			return err
		}
		absTarget, err := filepath.Abs(target)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(absDest, absTarget)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("非法的文件路径: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if header.Size > maxUpdateFileSize {
				return fmt.Errorf("包内文件过大: %s (%d 字节)", header.Name, header.Size)
			}
			if totalWritten+header.Size > maxUpdateTotalSize {
				return fmt.Errorf("解压总量超过限制 %d 字节", maxUpdateTotalSize)
			}
			outFile, err := os.Create(target)
			if err != nil {
				return err
			}
			written, copyErr := io.Copy(outFile, io.LimitReader(tarReader, maxUpdateFileSize+1))
			totalWritten += written
			if copyErr != nil {
				err := outFile.Close()
				if err != nil {
					return err
				}
				return copyErr
			}
			if written > maxUpdateFileSize {
				_ = outFile.Close()
				return fmt.Errorf("包内文件超过大小限制: %s", header.Name)
			}
			err = outFile.Close()
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("未知类型: %v in %s", header.Typeflag, header.Name)
		}
	}

	return nil
}
