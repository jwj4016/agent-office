package domain

import (
	"fmt"
	"slices"
	"strings"
)

// Organization plans (company-requests plan, 4th revision): the
// departments a service starts with, and its intake desk. The designer AI
// proposes one with a new service; for an existing service an AI proposes
// one from its assignees. Members and leads are assignment refs in a
// design, assignment ids for an existing service.

// OrgPlan is a proposed organization for one service.
type OrgPlan struct {
	Departments []PlannedDepartment `json:"departments"`
	// IntakeDesk is nil when no desk is proposed (one already exists).
	IntakeDesk *PlannedDesk `json:"intakeDesk"`
}

// PlannedDepartment is a department to create.
type PlannedDepartment struct {
	Name           string         `json:"name"`
	Responsibility string         `json:"responsibility"`
	Members        []string       `json:"members"`
	Lead           string         `json:"lead"`
	Watches        []PlannedWatch `json:"watches"`
}

// PlannedWatch is a subscription: events the department judges, and the
// viewpoint it judges them from.
type PlannedWatch struct {
	EventKinds []string `json:"eventKinds"`
	Guidance   string   `json:"guidance"`
}

// PlannedDesk is the service's intake desk to create.
type PlannedDesk struct {
	Name         string `json:"name"`
	Instructions string `json:"instructions"`
}

// WatchableEvents are the events a planned department may watch.
var WatchableEvents = []string{EventWorkCreated, EventWorkCompleted, EventDocumentShared, EventDocumentRevised}

// Check reports what is wrong with a plan. members maps every known
// member ref (or id) to whether it is an AI; taken names departments that
// already exist (skipped, not an error).
func (p OrgPlan) Check(members map[string]bool) []string {
	var out []string
	names := map[string]bool{}
	placed := map[string]string{}
	for i, d := range p.Departments {
		label := fmt.Sprintf("부서 %d", i+1)
		name := strings.TrimSpace(d.Name)
		if name == "" {
			out = append(out, label+"에 이름이 없습니다")
			continue
		}
		label = "부서 ‘" + name + "’"
		if names[name] {
			out = append(out, label+"의 이름이 겹칩니다")
		}
		names[name] = true
		if len(d.Members) == 0 {
			out = append(out, label+"에 구성원이 없습니다")
		}
		for _, m := range d.Members {
			if _, ok := members[m]; !ok {
				out = append(out, fmt.Sprintf("%s의 구성원 %q를 담당자 목록에서 찾을 수 없습니다", label, m))
				continue
			}
			if other, ok := placed[m]; ok && other != name {
				out = append(out, fmt.Sprintf("담당자 %q가 ‘%s’와 ‘%s’ 두 부서에 들어 있습니다", m, other, name))
			}
			placed[m] = name
		}
		if !slices.Contains(d.Members, d.Lead) {
			out = append(out, label+"의 대표가 구성원이 아닙니다")
		} else if !members[d.Lead] {
			out = append(out, label+"의 대표는 AI 담당자여야 합니다")
		}
		for _, w := range d.Watches {
			if len(w.EventKinds) == 0 {
				out = append(out, label+"의 받아 볼 사건이 비어 있습니다")
			}
			for _, k := range w.EventKinds {
				if !slices.Contains(WatchableEvents, k) {
					out = append(out, fmt.Sprintf("%s가 받아 볼 수 없는 사건입니다: %q", label, k))
				}
			}
		}
	}
	if d := p.IntakeDesk; d != nil && (strings.TrimSpace(d.Name) == "" || strings.TrimSpace(d.Instructions) == "") {
		out = append(out, "접수 담당의 이름이나 지침이 비어 있습니다")
	}
	return out
}
