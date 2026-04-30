package daemon

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseOSC7(t *testing.T) {
	data := []byte("\x1b]7;file://localhost/home/user/project\x1b\\")
	result := ParseOSC(data)
	assert.Len(t, result, 1)
	assert.Equal(t, OSCTypeCwd, result[0].Type)
	assert.Equal(t, "/home/user/project", result[0].Value)
}

func TestParseOSC9(t *testing.T) {
	data := []byte("\x1b]9;Build complete\x1b\\")
	result := ParseOSC(data)
	assert.Len(t, result, 1)
	assert.Equal(t, OSCTypeNotification, result[0].Type)
	assert.Equal(t, "Build complete", result[0].Value)
}

func TestParseOSC99(t *testing.T) {
	data := []byte("\x1b]99;d=0:p=body;Test done\x1b\\")
	result := ParseOSC(data)
	assert.Len(t, result, 1)
	assert.Equal(t, OSCTypeNotification, result[0].Type)
	assert.Equal(t, "Test done", result[0].Value)
}

func TestParseOSC777(t *testing.T) {
	data := []byte("\x1b]777;notify;Title;Body text\x1b\\")
	result := ParseOSC(data)
	assert.Len(t, result, 1)
	assert.Equal(t, OSCTypeNotification, result[0].Type)
	assert.Equal(t, "Body text", result[0].Value)
}

func TestParseOSC_NoOSC(t *testing.T) {
	data := []byte("just plain text with no escape sequences")
	result := ParseOSC(data)
	assert.Empty(t, result)
}

func TestParseOSC_Multiple(t *testing.T) {
	data := []byte("\x1b]7;file:///tmp\x1b\\\x1b]9;Done\x1b\\")
	result := ParseOSC(data)
	assert.Len(t, result, 2)
}

func TestParseOSC_BELTerminator(t *testing.T) {
	data := []byte("\x1b]9;Alert\x07")
	result := ParseOSC(data)
	assert.Len(t, result, 1)
	assert.Equal(t, "Alert", result[0].Value)
}

func TestParseOSC_ShellReady(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []OSCResult
	}{
		{
			name:  "shell-ready with ST",
			input: "\x1b]777;wmux;shell-ready\x1b\\",
			want:  []OSCResult{{Type: OSCTypeShellReady, Value: "shell-ready"}},
		},
		{
			name:  "shell-ready with BEL",
			input: "\x1b]777;wmux;shell-ready\x07",
			want:  []OSCResult{{Type: OSCTypeShellReady, Value: "shell-ready"}},
		},
		{
			name:  "regular 777 notification not shell-ready",
			input: "\x1b]777;notify;title;body\x1b\\",
			want:  []OSCResult{{Type: OSCTypeNotification, Value: "body"}},
		},
		{
			name:  "shell-ready embedded in other output",
			input: "prompt$ \x1b]777;wmux;shell-ready\x1b\\more output",
			want:  []OSCResult{{Type: OSCTypeShellReady, Value: "shell-ready", Offset: 8}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			results := ParseOSC([]byte(tc.input))
			assert.Equal(t, tc.want, results)
		})
	}
}

func TestParseOSC_OffsetAccuracy(t *testing.T) {
	data := []byte("abc\x1b]7;file:///tmp/x\x1b\\xyz\x1b]9;hello\x07")
	results := ParseOSC(data)
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Offset != 3 {
		t.Errorf("results[0].Offset = %d, want 3", results[0].Offset)
	}
	if results[1].Offset <= results[0].Offset {
		t.Errorf("offsets must be strictly increasing: %d, %d", results[0].Offset, results[1].Offset)
	}
}

func TestParseOSC_NoMatch_ReturnsNil(t *testing.T) {
	results := ParseOSC([]byte("plain text without any OSC"))
	if results != nil {
		t.Errorf("results = %v, want nil", results)
	}
}

func TestParseOSC_133_A(t *testing.T) {
	data := []byte("\x1b]133;A\x07")
	results := ParseOSC(data)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Type != OSCType133 {
		t.Errorf("Type = %v, want OSCType133", results[0].Type)
	}
	if results[0].Value != "A" {
		t.Errorf("Value = %q, want A", results[0].Value)
	}
}

func TestParseOSC_133_D_WithExit(t *testing.T) {
	data := []byte("\x1b]133;D;42\x07")
	results := ParseOSC(data)
	if len(results) != 1 {
		t.Fatalf("got %d, want 1", len(results))
	}
	if results[0].Value != "D;42" {
		t.Errorf("Value = %q, want D;42", results[0].Value)
	}
}

func TestParseOSC_133_D_NoExit(t *testing.T) {
	data := []byte("\x1b]133;D\x07")
	results := ParseOSC(data)
	if len(results) != 1 {
		t.Fatalf("got %d, want 1", len(results))
	}
	if results[0].Value != "D" {
		t.Errorf("Value = %q, want D", results[0].Value)
	}
}

func TestParseOSC_133_ExtraSegments(t *testing.T) {
	data := []byte("\x1b]133;D;0;extra\x07")
	results := ParseOSC(data)
	if results[0].Value != "D;0;extra" {
		t.Errorf("Value = %q, want D;0;extra (extra segments preserved for daemon parsing)", results[0].Value)
	}
}

func TestParseOSC_Mixed_133_7_777(t *testing.T) {
	data := []byte("output\x1b]7;file:///x\x1b\\\x1b]133;C\x07ls\n\x1b]133;D;0\x07\x1b]777;wmux;shell-ready\x07")
	results := ParseOSC(data)
	if len(results) != 4 {
		t.Fatalf("got %d, want 4: %+v", len(results), results)
	}
	wantTypes := []OSCType{OSCTypeCwd, OSCType133, OSCType133, OSCTypeShellReady}
	for i, w := range wantTypes {
		if results[i].Type != w {
			t.Errorf("results[%d].Type = %v, want %v", i, results[i].Type, w)
		}
	}
}

func TestParseOSC_133_OffsetAccuracy(t *testing.T) {
	prefix := []byte("hello world ")
	osc := []byte("\x1b]133;C\x07")
	data := make([]byte, 0, len(prefix)+len(osc))
	data = append(data, prefix...)
	data = append(data, osc...)
	results := ParseOSC(data)
	if len(results) != 1 {
		t.Fatalf("got %d, want 1", len(results))
	}
	if results[0].Offset != len(prefix) {
		t.Errorf("Offset = %d, want %d (start of ESC])", results[0].Offset, len(prefix))
	}
}

func TestParseOSC_BackwardCompat_OSC7(t *testing.T) {
	data := []byte("\x1b]7;file:///home/user\x1b\\")
	results := ParseOSC(data)
	if len(results) != 1 {
		t.Fatalf("got %d, want 1", len(results))
	}
	if results[0].Type != OSCTypeCwd {
		t.Errorf("Type = %v, want OSCTypeCwd", results[0].Type)
	}
	if results[0].Value != "/home/user" {
		t.Errorf("Value = %q, want /home/user", results[0].Value)
	}
}

func TestParseOSC_BackwardCompat_OSC777_ShellReady(t *testing.T) {
	data := []byte("\x1b]777;wmux;shell-ready\x07")
	results := ParseOSC(data)
	if len(results) != 1 {
		t.Fatalf("got %d, want 1", len(results))
	}
	if results[0].Type != OSCTypeShellReady {
		t.Errorf("Type = %v, want OSCTypeShellReady", results[0].Type)
	}
}
