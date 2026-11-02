package utiles

import "testing"

func TestIsSelfContainer(t *testing.T) {
	long := "54461d49d7e031c0458cce22c39de6725350a92e5e199868fc19604633a24051"
	short := long[:12]
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
		{"大小写不敏感", short, "54461D49D7E031C0458CCE22", true},
		{"带空白", " " + short + " ", long, true},
	}
	for _, c := range cases {
		if got := IsSelfContainer(c.selfID, c.id); got != c.want {
			t.Errorf("%s: IsSelfContainer(%q, %q) = %v, want %v", c.name, c.selfID, c.id, got, c.want)
		}
	}
}

func TestDetectSelfContainerIDNotInContainer(t *testing.T) {
	// 单测进程通常不在 docker 容器里；这里只验证函数不 panic 且返回合法值。
	// （在容器内跑 go test 时可能返回非空，属正常。）
	id := DetectSelfContainerID()
	if id != "" && !selfContainerIDLong.MatchString(id) && !selfContainerIDShort.MatchString(id) {
		t.Fatalf("DetectSelfContainerID 返回了非法 ID: %q", id)
	}
}
