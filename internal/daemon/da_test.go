package daemon

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDAType_String(t *testing.T) {
	tests := []struct {
		dt   DAType
		want string
	}{
		{DATypeDA1, "DA1"},
		{DATypeDA2, "DA2"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.dt.String())
		})
	}
}

func TestParseDA_DA1(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  []DARequest
	}{
		{name: "ESC[c", input: []byte("\x1b[c"), want: []DARequest{{Type: DATypeDA1}}},
		{name: "ESC[0c", input: []byte("\x1b[0c"), want: []DARequest{{Type: DATypeDA1}}},
		{name: "embedded in output", input: []byte("some text\x1b[cmore text"), want: []DARequest{{Type: DATypeDA1}}},
		{name: "no DA sequence", input: []byte("just normal text"), want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseDA(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseDA_DA2(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  []DARequest
	}{
		{name: "ESC[>c", input: []byte("\x1b[>c"), want: []DARequest{{Type: DATypeDA2}}},
		{name: "ESC[>0c", input: []byte("\x1b[>0c"), want: []DARequest{{Type: DATypeDA2}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseDA(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseDA_Multiple(t *testing.T) {
	input := []byte("\x1b[c\x1b[>c")
	got := ParseDA(input)
	assert.Len(t, got, 2)
	assert.Equal(t, DATypeDA1, got[0].Type)
	assert.Equal(t, DATypeDA2, got[1].Type)
}

func TestDA1Response(t *testing.T) {
	resp := DA1Response()
	assert.Equal(t, []byte("\x1b[?62;22c"), resp)
}

func TestDA2Response(t *testing.T) {
	resp := DA2Response()
	assert.Equal(t, []byte("\x1b[>1;0;0c"), resp)
}
