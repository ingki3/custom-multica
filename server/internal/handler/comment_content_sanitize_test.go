package handler

import "testing"

func TestSanitizeCommentContent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "normal content unchanged", in: "hello 세계", want: "hello 세계"},
		{name: "embedded NUL stripped", in: "before\x00after", want: "beforeafter"},
		{name: "invalid UTF-8 stripped", in: "before\xffafter", want: "beforeafter"},
		{name: "all rejected bytes becomes empty", in: "\x00\xff", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := sanitizeCommentContent(tt.in); got != tt.want {
				t.Fatalf("sanitizeCommentContent(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
