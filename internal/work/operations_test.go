package work_test

import (
	"context"
	"errors"
	"testing"

	"agent-office/internal/domain"
	"agent-office/internal/storage"
)

// Stage 9 acceptance: the app can say which event made a department start
// a work item, follow it to the work that came of it, and count what each
// department has open and what waits on it.
func TestWorkLineageAndDepartmentLoads(t *testing.T) {
	f := newJudgeFixture(t)
	ctx := context.Background()
	goal, _ := f.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: f.p.ID, Kind: domain.WorkGoal, DepartmentID: f.mkt.ID, Title: "신규 고객 캠페인"})
	plan, _ := f.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: f.p.ID, ParentID: goal.ID, DepartmentID: f.mkt.ID, Title: "캠페인 기획"})
	f.answer = func(dept, _ string) (string, error) {
		switch dept {
		case "개발팀":
			return propose("랜딩 페이지 구현"), nil
		case "법무팀":
			return "", errors.New("연결 실패")
		}
		return none, nil
	}
	if _, _, err := f.db.AddDocumentVersion(ctx, f.p.ID, storage.NewDocumentVersion{Title: "캠페인 기획안", OwnerDepartmentID: f.mkt.ID,
		WorkItemID: plan.ID, Categories: []string{"기획"}, SourceKind: "workspace", SourcePath: "plan.md", FileName: "plan.md", Path: "p/h1", Hash: "h1", Size: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.RequestDepartmentCheck(ctx, f.p.ID, f.leg.ID, "약관을 본다"); err != nil {
		t.Fatal(err)
	}
	f.settle(t)
	props, _ := f.db.Proposals(ctx, f.p.ID, "proposed")
	if len(props) != 1 || props[0].EventWorkItemID != plan.ID || props[0].EventDocumentID == "" {
		t.Fatalf("proposals = %+v", props)
	}
	devWork, err := f.db.AcceptProposal(ctx, f.p.ID, props[0].ID, storage.ProposalEdit{})
	if err != nil {
		t.Fatal(err)
	}
	req, err := f.db.RequestCollab(ctx, f.p.ID, storage.NewCollabRequest{FromWorkItemID: plan.ID, ToDepartmentID: f.fin.ID, Question: "예산을 봐 주세요"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.AcceptCollab(ctx, f.p.ID, req.ID, storage.CollabAcceptance{Title: "캠페인 예산 검토"}); err != nil {
		t.Fatal(err)
	}
	finReq, _ := f.db.CollabRequest(ctx, f.p.ID, req.ID)

	// Why the development work started: development judged the shared plan.
	dev, err := f.db.WorkLineage(ctx, f.p.ID, devWork.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(dev.Causes) != 1 {
		t.Fatalf("development causes = %+v", dev.Causes)
	}
	c := dev.Causes[0]
	if c.Via != "proposal" || c.EventKind != domain.EventDocumentShared || c.DocumentTitle != "캠페인 기획안" || c.FromWorkTitle != "캠페인 기획" ||
		c.DepartmentName != "개발팀" || c.Reason == "" {
		t.Fatalf("development cause = %+v", c)
	}
	// Why the finance work started: marketing asked for it.
	fin, _ := f.db.WorkLineage(ctx, f.p.ID, finReq.ResultWorkItemID)
	if len(fin.Causes) != 1 || fin.Causes[0].Via != "collab" || fin.Causes[0].FromDepartmentName != "마케팅팀" ||
		fin.Causes[0].FromWorkItemID != plan.ID || fin.Causes[0].DecidedBy != "user" {
		t.Fatalf("finance causes = %+v", fin.Causes)
	}
	// What the plan led to, and that a person's own work has no cause.
	p, _ := f.db.WorkLineage(ctx, f.p.ID, plan.ID)
	if len(p.Causes) != 0 || len(p.Next) != 2 {
		t.Fatalf("plan lineage = %+v", p)
	}
	vias := map[string]string{}
	for _, n := range p.Next {
		vias[n.ID] = n.Via + ":" + n.DepartmentName
	}
	if vias[devWork.ID] != "proposal:개발팀" || vias[finReq.ResultWorkItemID] != "collab:재무팀" {
		t.Fatalf("next = %+v", p.Next)
	}
	if _, err := f.db.WorkLineage(ctx, "other", plan.ID); err == nil {
		t.Fatal("read another service's work")
	}

	// Loads: blocked work, open requests and failed deliveries per department.
	if _, err := f.db.SetWorkStatus(ctx, f.p.ID, devWork.ID, domain.WorkBlocked, "API 문서 없음"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.RequestCollab(ctx, f.p.ID, storage.NewCollabRequest{FromWorkItemID: plan.ID, ToDepartmentID: f.dev.ID, Question: "분석 도구"}); err != nil {
		t.Fatal(err)
	}
	loads, err := f.db.DepartmentLoads(ctx, f.p.ID)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]storage.DepartmentLoad{}
	for _, l := range loads {
		by[l.Name] = l
	}
	if d := by["개발팀"]; d.Open != 1 || d.Blocked != 1 || d.CollabIncoming != 1 || len(d.Attention) != 1 || d.Attention[0].Reason != "API 문서 없음" {
		t.Fatalf("development load = %+v", d)
	}
	if m := by["마케팅팀"]; m.Open != 1 || m.Blocked != 0 {
		t.Fatalf("marketing load = %+v", m)
	}
	if l := by["법무팀"]; l.FailedDeliveries != 1 {
		t.Fatalf("legal load = %+v", l)
	}
	troubles, total, err := f.db.DeliveryTroubles(ctx, f.p.ID, 5)
	if err != nil || total != 1 || len(troubles) != 1 || troubles[0].DepartmentName != "법무팀" || troubles[0].Reason == "" {
		t.Fatalf("troubles = %+v %d %v", troubles, total, err)
	}

	goals, err := f.db.GoalsOf(ctx, []string{plan.ID, goal.ID, devWork.ID})
	if err != nil || goals[plan.ID].Title != "신규 고객 캠페인" || goals[goal.ID].ID != goal.ID {
		t.Fatalf("goals = %+v %v", goals, err)
	}
}
