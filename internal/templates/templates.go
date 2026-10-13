// Package templates holds the built-in starting workflows. Assignments are
// left empty: the user picks who does each step.
package templates

import (
	"embed"
	"encoding/json"
)

//go:embed *.json
var files embed.FS

type Template struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Workflow    json.RawMessage `json:"workflow"`
}

var catalog = []struct{ id, file, title, desc string }{
	{"blank", "blank.json", "빈 흐름", "첫 업무부터 직접 만듭니다."},
	{"service-dev", "service-dev.json", "서비스 개발", "기획 → 승인 → 설계 → 백엔드·프론트엔드 → 통합 → 코드 리뷰 → QA → 전달"},
	{"game-launch", "game-launch.json", "게임 출시", "기획·시장·법률 쟁점·비용 분석 → 방향 승인 → 개발·마케팅 준비 → 통합·QA → 출시 승인 → 배포 패키지"},
	{"legal-review", "legal-review.json", "법률 검토", "범위·지역·사실 자료 → 근거 조사 → 의견 초안 → 별도 검토 → 사람 결정 → 문서 전달"},
	{"real-estate", "real-estate.json", "부동산 분석", "대상·목적 → 자료 확보·품질 점검 → 시장·비용·쟁점 분석 → 비교·검토 → 보고서"},
}

// List returns all templates.
func List() []Template {
	out := make([]Template, 0, len(catalog))
	for _, c := range catalog {
		data, _ := files.ReadFile(c.file)
		out = append(out, Template{ID: c.id, Title: c.title, Description: c.desc, Workflow: data})
	}
	return out
}

// Get returns one template's workflow JSON.
func Get(id string) (json.RawMessage, bool) {
	for _, t := range List() {
		if t.ID == id {
			return t.Workflow, true
		}
	}
	return nil, false
}
