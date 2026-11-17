package utiles

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/docker/docker/pkg/stdcopy"
)

// muxStream 构造 docker 多路复用格式的日志流：每帧 8 字节头（stream + 4 字节长度）。
func muxStream(t *testing.T, frames ...[]byte) io.Reader {
	t.Helper()
	var buf bytes.Buffer
	for i, frame := range frames {
		stream := stdcopy.Stdout
		if i%2 == 1 {
			stream = stdcopy.Stderr
		}
		var header [8]byte
		header[0] = byte(stream)
		header[4] = byte(len(frame) >> 24)
		header[5] = byte(len(frame) >> 16)
		header[6] = byte(len(frame) >> 8)
		header[7] = byte(len(frame))
		buf.Write(header[:])
		buf.Write(frame)
	}
	return &buf
}

// TestDecodeLogStreamDemux 验证 stdout/stderr 交错帧能被正确解复用，行首不带流标记。
func TestDecodeLogStreamDemux(t *testing.T) {
	r := muxStream(t,
		[]byte("stdout line 1\n"),
		[]byte("stderr line 1\n"),
		[]byte("stdout line 2\n"),
	)
	lines, err := decodeLogStream(r, 5000)
	if err != nil {
		t.Fatalf("decodeLogStream: %v", err)
	}
	want := []string{"stdout line 1", "stderr line 1", "stdout line 2"}
	if len(lines) != len(want) {
		t.Fatalf("行数 = %d, 期望 %d: %q", len(lines), len(want), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("lines[%d] = %q, 期望 %q", i, lines[i], want[i])
		}
	}
	for i, line := range lines {
		if strings.ContainsAny(line, "\x01\x02") {
			t.Errorf("lines[%d] 残留流标记: %q", i, line)
		}
	}
}

// TestDecodeLogStreamEmpty 空日志流应返回空切片（非 nil），避免前端拿到 null。
func TestDecodeLogStreamEmpty(t *testing.T) {
	lines, err := decodeLogStream(muxStream(t), 200)
	if err != nil {
		t.Fatalf("decodeLogStream: %v", err)
	}
	if lines == nil || len(lines) != 0 {
		t.Fatalf("期望空切片, 得到 %#v", lines)
	}
}

// TestDecodeLogStreamTailTruncation 解复用后行数超出 tail 时只保留最后 tail 行。
func TestDecodeLogStreamTailTruncation(t *testing.T) {
	var frames [][]byte
	const total = 10
	for i := 1; i <= total; i++ {
		frames = append(frames, []byte("line-"+string(rune('0'+i))+"\n"))
	}
	lines, err := decodeLogStream(muxStream(t, frames...), 3)
	if err != nil {
		t.Fatalf("decodeLogStream: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("行数 = %d, 期望 3: %q", len(lines), lines)
	}
	want := []string{"line-8", "line-9", "line-:"}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("lines[%d] = %q, 期望 %q", i, lines[i], want[i])
		}
	}
}

// TestDecodeLogStreamCR 清理行尾 \r，兼容 TTY 模式下的 CRLF 输出。
func TestDecodeLogStreamCR(t *testing.T) {
	lines, err := decodeLogStream(muxStream(t, []byte("with cr\r\n")), 200)
	if err != nil {
		t.Fatalf("decodeLogStream: %v", err)
	}
	if len(lines) != 1 || lines[0] != "with cr" {
		t.Fatalf("lines = %q, 期望 [with cr]", lines)
	}
}

// TestNormalizeTail tail 参数边界：<=0 回落默认 200，>5000 截断到 5000。
func TestNormalizeTail(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{-1, 200}, {0, 200}, {1, 1}, {200, 200}, {4999, 4999}, {5000, 5000}, {5001, 5000}, {100000, 5000},
	}
	for _, c := range cases {
		if got := normalizeTail(c.in); got != c.want {
			t.Errorf("normalizeTail(%d) = %d, 期望 %d", c.in, got, c.want)
		}
	}
}

// TestCleanLogLineStreamMarks 单独覆盖行级清理：流标记前缀与行尾 \r。
func TestCleanLogLineStreamMarks(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"\x01stdout", "stdout"},
		{"\x02stderr", "stderr"},
		{"\x01both\x02", "both\x02"}, // 只清理行首标记
		{"trailing\r", "trailing"},
		{"\x01messy\r", "messy"},
		{"", ""},
	}
	for _, c := range cases {
		if got := cleanLogLine(c.in); got != c.want {
			t.Errorf("cleanLogLine(%q) = %q, 期望 %q", c.in, got, c.want)
		}
	}
}
