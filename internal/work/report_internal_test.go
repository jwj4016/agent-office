package work

import (
	"strings"
	"testing"
)

// A report's details are written as a readable list whether the model
// sent text, a list or an object.
func TestRenderReport(t *testing.T) {
	for in, want := range map[string]string{
		`"그대로"`:       "그대로",
		`["하나", "둘"]`: "- 하나\n- 둘",
		`{"남은 일": "없음", "결과": ["기준서", "계산"]}`: "- 남은 일: 없음\n- 결과\n  - 기준서\n  - 계산",
		`[{"업무": "검증", "결과": "통과"}]`:          "-\n  - 업무: 검증\n  - 결과: 통과",
	} {
		if got := strings.TrimSpace(renderReport([]byte(in), 0)); got != want {
			t.Errorf("renderReport(%s) =\n%s\nwant\n%s", in, got, want)
		}
	}
}
