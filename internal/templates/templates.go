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
	{"service-dev", "service-dev.json", "서비스 개발 (예시)", "기획 → 승인 → 설계 → 백엔드·프론트엔드 → 통합 → 내 코드 리뷰 → QA → 전달"},
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
