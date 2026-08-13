package procfmt

import (
	"reflect"
	"testing"
)

func TestParseCmdlineSplitsOnNUL(t *testing.T) {
	data := []byte("node\x00server.js\x00--port\x003000\x00")

	got := ParseCmdline(data)
	want := []string{"node", "server.js", "--port", "3000"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseCmdlineEmptyReturnsNil(t *testing.T) {
	if got := ParseCmdline(nil); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
	if got := ParseCmdline([]byte{}); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func TestParseCmdlineWithoutTrailingNUL(t *testing.T) {
	got := ParseCmdline([]byte("go\x00run\x00."))
	want := []string{"go", "run", "."}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
