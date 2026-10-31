package storage_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"agent-office/internal/domain"
	"agent-office/internal/storage"
)

// share records a version as the engine would after copying a file.
func share(t *testing.T, c *company, nv storage.NewDocumentVersion, content string) (storage.SharedDocument, storage.DocumentVersion) {
	t.Helper()
	nv.SourceKind, nv.SourcePath, nv.FileName, nv.Path, nv.Hash, nv.Size = "workspace", "plan.md", "plan.md", "projects/x/documents/plan.md", "h-"+content, int64(len(content))
	doc, v, err := c.db.AddDocumentVersion(context.Background(), c.project.ID, nv)
	if err != nil {
		t.Fatal(err)
	}
	return doc, v
}

func deliveriesFor(t *testing.T, c *company, seq int64) []string {
	t.Helper()
	all, _ := c.db.Deliveries(context.Background(), c.project.ID, storage.DeliveryFilter{})
	names := map[string]string{c.mkt.ID: "mkt", c.dev.ID: "dev", c.fin.ID: "fin"}
	var out []string
	for _, d := range all {
		if d.EventSequence == seq {
			out = append(out, names[d.DepartmentID]+":"+d.Purpose)
		}
	}
	slices.Sort(out)
	return out
}

func eventSeq(t *testing.T, c *company, kind string) int64 {
	t.Helper()
	all, _ := c.db.Deliveries(context.Background(), c.project.ID, storage.DeliveryFilter{})
	var seq int64
	for _, d := range all {
		if d.EventKind == kind && d.EventSequence > seq {
			seq = d.EventSequence
		}
	}
	return seq
}

func TestShareAndReviseDocument(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	subscribe(t, c, c.dev, []string{domain.EventDocumentShared, domain.EventDocumentRevised})
	subscribe(t, c, c.fin, []string{domain.EventDocumentShared}, "비용")
	plan, _ := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: c.project.ID, DepartmentID: c.mkt.ID, Title: "기획안 작성"})
	doc, v1 := share(t, c, storage.NewDocumentVersion{Title: "캠페인 기획안", WorkItemID: plan.ID, Categories: []string{"기획", "비용"}}, "1판")
	if doc.OwnerDepartmentID != c.mkt.ID || v1.Number != 1 || v1.Stage != "draft" {
		t.Fatalf("doc=%+v v1=%+v", doc, v1)
	}
	if got := deliveriesFor(t, c, eventSeq(t, c, domain.EventDocumentShared)); !slices.Equal(got, []string{"dev:judge", "fin:judge"}) {
		t.Fatalf("shared went to %v", got)
	}

	// Development and finance use the first version.
	devWork, _ := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: c.project.ID, DepartmentID: c.dev.ID, Title: "구현 검토"})
	finWork, _ := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: c.project.ID, DepartmentID: c.fin.ID, Title: "예산 검토"})
	for _, w := range []domain.WorkItem{devWork, finWork, plan} {
		if err := c.db.LinkDocument(ctx, c.project.ID, w.ID, v1.ID); err != nil {
			t.Fatal(err)
		}
	}
	// The same content again is not a new version.
	if _, _, err := c.db.AddDocumentVersion(ctx, c.project.ID, storage.NewDocumentVersion{DocumentID: doc.ID, SourceKind: "workspace",
		SourcePath: "plan.md", FileName: "plan.md", Path: "p", Hash: v1.Hash, Size: 1}); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("same content: %v", err)
	}
	_, v2 := share(t, c, storage.NewDocumentVersion{DocumentID: doc.ID}, "2판")
	if v2.Number != 2 || v2.Stage != "draft" {
		t.Fatalf("v2 = %+v", v2)
	}
	// Subscribers judge the revision; finance, which only follows new
	// shares, is told because its work used version 1.
	if got := deliveriesFor(t, c, eventSeq(t, c, domain.EventDocumentRevised)); !slices.Equal(got, []string{"dev:judge", "fin:notify"}) {
		t.Fatalf("revised went to %v", got)
	}
	checks, _ := c.db.ImpactChecks(ctx, c.project.ID, "", "open")
	var who []string
	for _, ic := range checks {
		who = append(who, ic.WorkItemID)
		if ic.Cause != "revised" || ic.RefID != v2.ID || !strings.Contains(ic.Summary, "2판") {
			t.Fatalf("check = %+v", ic)
		}
	}
	slices.Sort(who)
	want := []string{devWork.ID, finWork.ID}
	slices.Sort(want)
	if !slices.Equal(who, want) { // the document's own work is not asked
		t.Fatalf("impact checks for %v, want %v", who, want)
	}
	links, _ := c.db.WorkDocuments(ctx, c.project.ID, devWork.ID)
	if len(links) != 1 || links[0].Version.ID != v1.ID || links[0].Newer != 2 {
		t.Fatalf("dev links = %+v", links)
	}
	if err := c.db.CloseImpactCheck(ctx, c.project.ID, checks[0].ID, "resolved", "2판 확인함"); err != nil {
		t.Fatal(err)
	}
	if err := c.db.CloseImpactCheck(ctx, c.project.ID, checks[0].ID, "dismissed", ""); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("closed twice: %v", err)
	}
}

func TestConfirmVersionWithoutChangingIt(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	subscribe(t, c, c.dev, []string{domain.EventDocumentShared})    // follows shares: told
	subscribe(t, c, c.fin, []string{domain.EventDocumentConfirmed}) // chose confirmations: judges
	doc, v1 := share(t, c, storage.NewDocumentVersion{Title: "기획안", OwnerDepartmentID: c.mkt.ID}, "1판")
	got, err := c.db.ConfirmDocumentVersion(ctx, c.project.ID, v1.ID, "최종 승인")
	if err != nil {
		t.Fatal(err)
	}
	if got.Stage != "final" || got.ConfirmedBy != domain.LocalOwner || got.ConfirmationNote != "최종 승인" || got.Hash != v1.Hash || got.Number != 1 {
		t.Fatalf("confirmed = %+v", got)
	}
	seq := eventSeq(t, c, domain.EventDocumentConfirmed)
	if d := deliveriesFor(t, c, seq); !slices.Equal(d, []string{"dev:notify", "fin:judge"}) {
		t.Fatalf("confirmed went to %v", d)
	}
	// Confirming again changes nothing and emits nothing.
	c.db.ConfirmDocumentVersion(ctx, c.project.ID, v1.ID, "다시")
	if again := eventSeq(t, c, domain.EventDocumentConfirmed); again != seq {
		t.Fatalf("second confirmation emitted %d", again)
	}
	// A new version is a draft again.
	_, v2 := share(t, c, storage.NewDocumentVersion{DocumentID: doc.ID}, "2판")
	if v2.Stage != "draft" {
		t.Fatalf("v2 = %+v", v2)
	}
	if err := c.db.WithdrawDocument(ctx, c.project.ID, doc.ID, v2.ID, "오류"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.ConfirmDocumentVersion(ctx, c.project.ID, v2.ID, ""); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("confirmed a withdrawn version: %v", err)
	}
}

func TestDocumentScopeLimitsWhoSeesIt(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	subscribe(t, c, c.dev, []string{domain.EventDocumentShared})
	subscribe(t, c, c.fin, []string{domain.EventDocumentShared})
	doc, v1 := share(t, c, storage.NewDocumentVersion{Title: "예산 초안", OwnerDepartmentID: c.mkt.ID, Scope: "departments",
		ScopeDepartments: []string{c.fin.ID}}, "예산")
	if got := deliveriesFor(t, c, eventSeq(t, c, domain.EventDocumentShared)); !slices.Equal(got, []string{"fin:judge"}) {
		t.Fatalf("shared went to %v", got)
	}
	devWork, _ := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: c.project.ID, DepartmentID: c.dev.ID, Title: "x"})
	if err := c.db.LinkDocument(ctx, c.project.ID, devWork.ID, v1.ID); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("out-of-scope link: %v", err)
	}
	visible, _ := c.db.Documents(ctx, c.project.ID, storage.DocumentFilter{VisibleTo: c.dev.ID})
	forFin, _ := c.db.Documents(ctx, c.project.ID, storage.DocumentFilter{VisibleTo: c.fin.ID})
	if len(visible) != 0 || len(forFin) != 1 || forFin[0].ID != doc.ID {
		t.Fatalf("visible to dev=%d fin=%d", len(visible), len(forFin))
	}
	other, _ := c.db.CreateProject(ctx, domain.Project{Name: "다른 서비스"})
	if _, err := c.db.Document(ctx, other.ID, doc.ID); !errors.Is(err, storage.ErrNotInProject) {
		t.Fatalf("read from another service: %v", err)
	}
}

func TestWithdrawAndSearch(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	doc, v1 := share(t, c, storage.NewDocumentVersion{Title: "캠페인 기획안", OwnerDepartmentID: c.mkt.ID, Categories: []string{"기획"}}, "1판")
	share(t, c, storage.NewDocumentVersion{Title: "예산표", OwnerDepartmentID: c.fin.ID, Categories: []string{"비용"}}, "예산")
	w, _ := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: c.project.ID, DepartmentID: c.dev.ID, Title: "구현 검토"})
	c.db.LinkDocument(ctx, c.project.ID, w.ID, v1.ID)
	titles := func(f storage.DocumentFilter) string {
		list, _ := c.db.Documents(ctx, c.project.ID, f)
		var out []string
		for _, d := range list {
			out = append(out, d.Title)
		}
		slices.Sort(out)
		return strings.Join(out, ",")
	}
	if got := titles(storage.DocumentFilter{Query: "기획"}); got != "캠페인 기획안" {
		t.Fatalf("query = %s", got)
	}
	if got := titles(storage.DocumentFilter{Category: "비용"}); got != "예산표" {
		t.Fatalf("category = %s", got)
	}
	if got := titles(storage.DocumentFilter{OwnerDepartment: c.mkt.ID}); got != "캠페인 기획안" {
		t.Fatalf("owner = %s", got)
	}
	if err := c.db.WithdrawDocument(ctx, c.project.ID, doc.ID, "", " "); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("withdraw without reason: %v", err)
	}
	if err := c.db.WithdrawDocument(ctx, c.project.ID, doc.ID, "", "캠페인 취소"); err != nil {
		t.Fatal(err)
	}
	if got := titles(storage.DocumentFilter{}); got != "예산표" {
		t.Fatalf("after withdrawal = %s", got)
	}
	if got := titles(storage.DocumentFilter{IncludeWithdrawn: true}); got != "예산표,캠페인 기획안" {
		t.Fatalf("with withdrawn = %s", got)
	}
	checks, _ := c.db.ImpactChecks(ctx, c.project.ID, w.ID, "open")
	if len(checks) != 1 || checks[0].Cause != "withdrawn" || !strings.Contains(checks[0].Summary, "캠페인 취소") {
		t.Fatalf("checks = %+v", checks)
	}
	if err := c.db.LinkDocument(ctx, c.project.ID, w.ID, v1.ID); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("linked withdrawn material: %v", err)
	}
	if _, _, err := c.db.AddDocumentVersion(ctx, c.project.ID, storage.NewDocumentVersion{DocumentID: doc.ID, SourceKind: "workspace",
		SourcePath: "a", FileName: "a", Path: "a", Hash: "new", Size: 1}); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("new version of a withdrawn document: %v", err)
	}
}

func TestLinkingAnotherVersionReplacesTheLink(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	doc, v1 := share(t, c, storage.NewDocumentVersion{Title: "기획안", OwnerDepartmentID: c.mkt.ID}, "1판")
	_, v2 := share(t, c, storage.NewDocumentVersion{DocumentID: doc.ID}, "2판")
	w, _ := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: c.project.ID, DepartmentID: c.dev.ID, Title: "구현 검토"})
	c.db.LinkDocument(ctx, c.project.ID, w.ID, v1.ID)
	c.db.LinkDocument(ctx, c.project.ID, w.ID, v2.ID)
	links, _ := c.db.WorkDocuments(ctx, c.project.ID, w.ID)
	if len(links) != 1 || links[0].Version.ID != v2.ID || links[0].Newer != 0 {
		t.Fatalf("links = %+v", links)
	}
	if err := c.db.UnlinkDocument(ctx, c.project.ID, w.ID, v2.ID); err != nil {
		t.Fatal(err)
	}
	if links, _ = c.db.WorkDocuments(ctx, c.project.ID, w.ID); len(links) != 0 {
		t.Fatalf("after unlink = %+v", links)
	}
}
