package utiles

import (
	"regexp"
	"strings"

	dockerTypes "github.com/docker/docker/api/types"
)

// FormatPorts 把 docker 的端口映射列表格式化成前端易读的短字符串。
// 例：0.0.0.0:13000→3000/tcp、[::]:8080→80/tcp、1688/tcp（未发布到宿主）。
func FormatPorts(ports []dockerTypes.Port) []string {
	if len(ports) == 0 {
		return nil
	}
	out := make([]string, 0, len(ports))
	for _, p := range ports {
		proto := p.Type
		if proto == "" {
			proto = "tcp"
		}
		if p.PublicPort > 0 {
			host := p.IP
			if host == "" {
				host = "0.0.0.0"
			}
			// IPv6 需要方括号，否则 "::" + ":" 拼出来是 ":::48199"，前端没法直接当 URL 用。
			if strings.Contains(host, ":") {
				host = "[" + strings.Trim(host, "[]") + "]"
			}
			out = append(out, host+":"+itoa(int(p.PublicPort))+"→"+itoa(int(p.PrivatePort))+"/"+proto)
			continue
		}
		out = append(out, itoa(int(p.PrivatePort))+"/"+proto)
	}
	return out
}

// SplitImageRef 把 "registry.example.com:5000/app:v1" 拆成 ("registry.example.com:5000/app", "v1")。
// 无 tag 时返回 (ref, "<none>")；带 digest 时返回 (ref, "sha256:...") 归一为 digest 片段。
func SplitImageRef(ref string) (string, string) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "<none>"
	}
	// digest 形式：name@sha256:...
	if idx := strings.LastIndex(ref, "@"); idx > 0 {
		return ref[:idx], ref[idx+1:]
	}
	lastSlash := strings.LastIndex(ref, "/")
	lastColon := strings.LastIndex(ref, ":")
	if lastColon > lastSlash {
		return ref[:lastColon], ref[lastColon+1:]
	}
	return ref, "<none>"
}

var healthStatusRE = regexp.MustCompile(`\((?:health: )?([a-z]+)\)\s*$`)

// HealthFromStatus 从 docker 的 Status 文本里取健康状态：healthy / unhealthy / starting。
// 容器没有 HEALTHCHECK 时 docker 不写括号段，这里返回空串。
func HealthFromStatus(status string) string {
	m := healthStatusRE.FindStringSubmatch(strings.TrimSpace(status))
	if len(m) != 2 {
		return ""
	}
	switch m[1] {
	case "healthy", "unhealthy", "starting":
		return m[1]
	}
	return ""
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
