package engine

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// The related-conversation section stays bounded however many notes a
// step has, and says what was left out.
func TestWriteContextIsBounded(t *testing.T) {
	var notes []ContextNote
	for i := 0; i < 40; i++ {
		notes = append(notes, ContextNote{Label: "전달 메모", Ref: "메시지 m", Text: strings.Repeat("가", 3000)})
	}
	var b strings.Builder
	writeContext(&b, notes)
	out := b.String()
	if n := utf8.RuneCountInString(out); n > contextTotalLimit+2000 {
		t.Fatalf("context is %d runes", n)
	}
	if !strings.Contains(out, "생략") || !strings.Contains(out, "실행 기록") {
		t.Fatalf("no note about omitted entries:\n%s", out[len(out)-300:])
	}
}
