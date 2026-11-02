package utiles

import "testing"

const (
	testSelfIDLong  = "4ea603ca3452ba8bfcc9049c66cf8267d984c8d17da606b62b910e6489bbcc26"
	testSelfIDShort = "4ea603ca3452"
)

func TestDetectSelfContainerIDPrefersMountPaths(t *testing.T) {
	// 真实容器 mountinfo 片段：注意 overlay2 的 upperdir 也是 64 位 hex（层缓存 ID），
	// 绝不能被误当成容器 ID —— 这正是第一版实现的 bug。
	mountinfo := `1743 1653 0:99 / / rw,relatime - overlay overlay rw,lowerdir=/vol2/docker/overlay2/l/Z2NQQZUEFA3VDD7NIBEDZB3BMQ,upperdir=/vol2/docker/overlay2/d3d9fa0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f/diff,workdir=/vol2/docker/overlay2/d3d9fa0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f/work
1750 1743 0:101 / /proc rw - proc proc rw
1850 1743 0:50 /var/lib/docker/containers/` + testSelfIDLong + `/hostname /etc/hostname rw - ext4 /dev/sda1 rw
1851 1743 0:50 /var/lib/docker/containers/` + testSelfIDLong + `/hosts /etc/hosts rw - ext4 /dev/sda1 rw
`
	if got := detectSelfContainerID(mountinfo, "", ""); got != testSelfIDLong {
		t.Fatalf("mountinfo 探测 = %q, want %q（不能取 overlay2 的 cache-id）", got, testSelfIDLong)
	}
}

func TestDetectSelfContainerIDFromCgroup(t *testing.T) {
	cases := map[string]string{
		"cgroup v1": "12:pids:/docker/" + testSelfIDLong + "\n11:memory:/docker/" + testSelfIDLong + "\n",
		"cgroup v2": "0::/system.slice/docker-" + testSelfIDLong + ".scope\n",
	}
	for name, cgroup := range cases {
		if got := detectSelfContainerID("", cgroup, ""); got != testSelfIDLong {
			t.Errorf("%s 探测 = %q, want %q", name, got, testSelfIDLong)
		}
	}
}

func TestDetectSelfContainerIDFallsBackToHostname(t *testing.T) {
	if got := detectSelfContainerID("", "0::/\n", testSelfIDShort); got != testSelfIDShort {
		t.Fatalf("hostname 回退 = %q, want %q", got, testSelfIDShort)
	}
}

func TestDetectSelfContainerIDEmptyOnBareMetal(t *testing.T) {
	mountinfo := "1743 1653 254:0 / / rw,relatime - ext4 /dev/nvme0n1p2 rw\n"
	if got := detectSelfContainerID(mountinfo, "0::/user.slice/session-3.scope", "DS-NAS"); got != "" {
		t.Fatalf("宿主裸跑应返回空，got %q", got)
	}
}

func TestIsSelfContainer(t *testing.T) {
	long := testSelfIDLong
	short := testSelfIDShort
	cases := []struct {
		name   string
		selfID string
		id     string
		want   bool
	}{
		{"长ID等于长ID", long, long, true},
		{"短自身匹配长目标", short, long, true},
		{"长自身匹配短目标", long, short, true},
		{"不同容器", long, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
		{"自身为空", "", long, false},
		{"目标为空", long, "", false},
		{"两边都空", "", "", false},
		{"大小写不敏感", short, "4EA603CA3452BA8BFCC9049C66CF8267", true},
		{"带空白", " " + short + " ", long, true},
	}
	for _, c := range cases {
		if got := IsSelfContainer(c.selfID, c.id); got != c.want {
			t.Errorf("%s: IsSelfContainer(%q, %q) = %v, want %v", c.name, c.selfID, c.id, got, c.want)
		}
	}
}
