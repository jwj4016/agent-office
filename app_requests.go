package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"agent-office/internal/domain"
	"agent-office/internal/engine"
	"agent-office/internal/storage"
)

// Company requests (docs/plan/company-requests/plan.md): the person asks
// the company or a service in words; its intake desk decides what to do.

// EventRequestsChanged tells open screens that requests changed. Company
// requests have no service, so service events do not cover them.
const EventRequestsChanged = "requests:changed"

// DeskView is a desk's setup, and for the company desk this month's use.
type DeskView struct {
	Desk   domain.IntakeDesk `json:"desk"`
	Exists bool              `json:"exists"`
	// MonthUsage is the company desk's use since the start of the month.
	MonthUsage *storage.AIAttemptUsage `json:"monthUsage,omitempty"`
	// Assignee is a service desk's AI assignee (with its instructions).
	Assignee *domain.Assignment `json:"assignee,omitempty"`
	// LastStaffing is the last time an AI wrote this desk.
	LastStaffing *storage.Staffing `json:"lastStaffing,omitempty"`
	// SuggestedConnectionID and SuggestedModel are what the app would give
	// a new desk: the verified connection the assignees use most.
	SuggestedConnectionID string `json:"suggestedConnectionId"`
	SuggestedModel        string `json:"suggestedModel"`
}

// GetIntakeDesk returns the company desk (projectID "") or a service's.
func (a *App) GetIntakeDesk(projectID string) (DeskView, error) {
	if err := a.ready(); err != nil {
		return DeskView{}, err
	}
	d, ok, err := a.db.IntakeDesk(a.ctx, projectID)
	if err != nil {
		return DeskView{}, uiErr(err)
	}
	v := DeskView{Desk: d, Exists: ok}
	if ok && projectID != "" {
		list, err := a.db.Assignments(a.ctx, projectID)
		if err != nil {
			return v, uiErr(err)
		}
		for i := range list {
			if list[i].ID == d.AssignmentID {
				v.Assignee = &list[i]
			}
		}
	}
	if s, found, err := a.db.LatestStaffing(a.ctx, projectID); err != nil {
		return v, uiErr(err)
	} else if found {
		v.LastStaffing = &s
	}
	if c, model, found, err := a.db.PreferredConnection(a.ctx, projectID); err != nil {
		return v, uiErr(err)
	} else if found {
		v.SuggestedConnectionID, v.SuggestedModel = c.ID, model
	}
	if projectID == "" {
		now := time.Now()
		u, err := a.db.CompanyIntakeUsage(a.ctx, time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()))
		if err != nil {
			return v, uiErr(err)
		}
		v.MonthUsage = &u
	}
	return v, nil
}

// SaveIntakeDesk sets up a desk; requests waiting for it go ahead.
func (a *App) SaveIntakeDesk(d domain.IntakeDesk) (domain.IntakeDesk, error) {
	if err := a.ready(); err != nil {
		return d, err
	}
	out, err := a.db.SaveIntakeDesk(a.ctx, d)
	if err == nil {
		a.intake.Wake()
	}
	return out, uiErr(err)
}

// StaffIntakeDesk has an AI look at the service (projectID "" for the
// company) and set up a new desk for it. It waits for the AI (up to a few
// minutes) and uses its connection's quota.
func (a *App) StaffIntakeDesk(projectID string) (domain.IntakeDesk, error) {
	if err := a.ready(); err != nil {
		return domain.IntakeDesk{}, err
	}
	if projectID != "" {
		if err := a.db.CheckScope(a.ctx, projectID); err != nil {
			return domain.IntakeDesk{}, uiErr(err)
		}
	}
	d, err := a.intake.StaffDesk(a.ctx, projectID, "")
	if err == nil {
		a.intake.Wake()
	}
	return d, uiErr(err)
}

// RewriteIntakeDesk has an AI write a desk's instructions again.
func (a *App) RewriteIntakeDesk(projectID string) (domain.IntakeDesk, error) {
	if err := a.ready(); err != nil {
		return domain.IntakeDesk{}, err
	}
	if projectID != "" {
		if err := a.db.CheckScope(a.ctx, projectID); err != nil {
			return domain.IntakeDesk{}, uiErr(err)
		}
	}
	d, err := a.intake.RewriteDesk(a.ctx, projectID)
	return d, uiErr(err)
}

// NewDeskInput is a desk assignee the person writes themselves.
type NewDeskInput struct {
	ProjectID    string `json:"projectId"`
	Name         string `json:"name"`
	ConnectionID string `json:"connectionId"`
	Model        string `json:"model"`
	Instructions string `json:"instructions"`
}

// CreateIntakeAssignee makes a new AI assignee in the shared desk role and
// sets it as the service's desk.
func (a *App) CreateIntakeAssignee(in NewDeskInput) (domain.IntakeDesk, error) {
	if err := a.ready(); err != nil {
		return domain.IntakeDesk{}, err
	}
	d, _, err := a.db.CreateDeskAssignee(a.ctx, in.ProjectID, in.Name, in.ConnectionID, in.Model, in.Instructions)
	if err == nil {
		a.intake.Wake()
	}
	return d, uiErr(err)
}

// RemoveIntakeDesk takes a service's desk away.
func (a *App) RemoveIntakeDesk(projectID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.db.RemoveIntakeDesk(a.ctx, projectID))
}

// SubmitRequest asks the company (projectID "") or a service. previousID
// continues an earlier request of the same place.
func (a *App) SubmitRequest(projectID, body, previousID string) (domain.Request, error) {
	if err := a.ready(); err != nil {
		return domain.Request{}, err
	}
	r, err := a.db.SubmitRequest(a.ctx, domain.Request{ProjectID: projectID, Body: body, PreviousID: previousID})
	if err == nil {
		a.intake.Wake()
	}
	return r, uiErr(err)
}

// ListRequests lists requests with their items, newest first.
func (a *App) ListRequests(f storage.RequestFilter) ([]domain.Request, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.db.Requests(a.ctx, f)
	return list, uiErr(err)
}

// GetRequest returns one request of a service ("" for the company).
func (a *App) GetRequest(projectID, id string) (domain.Request, error) {
	if err := a.ready(); err != nil {
		return domain.Request{}, err
	}
	r, err := a.db.Request(a.ctx, projectID, id)
	return r, uiErr(err)
}

// AnswerRequest answers the desk's question; the desk decides again.
func (a *App) AnswerRequest(projectID, id, reply string) error {
	if err := a.ready(); err != nil {
		return err
	}
	err := a.db.AnswerRequest(a.ctx, projectID, id, reply)
	if err == nil {
		a.intake.Wake()
	}
	return uiErr(err)
}

// CloseRequest ends a request the person handles themselves.
func (a *App) CloseRequest(projectID, id, note string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.db.CloseRequest(a.ctx, projectID, id, note))
}

// CancelRequest stops a request and what it was passed on as.
func (a *App) CancelRequest(projectID, id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.db.CancelRequest(a.ctx, projectID, id))
}

// RetryRequest gives a failed request one more attempt.
func (a *App) RetryRequest(projectID, id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	err := a.db.RetryRequest(a.ctx, projectID, id)
	if err == nil {
		a.intake.Wake()
	}
	return uiErr(err)
}

// AcceptRequestWork registers work a desk proposed, with the person's
// edits.
func (a *App) AcceptRequestWork(projectID, itemID string, edit domain.WorkProposalPayload) (domain.WorkItem, error) {
	if err := a.ready(); err != nil {
		return domain.WorkItem{}, err
	}
	w, err := a.db.AcceptRequestWork(a.ctx, projectID, itemID, edit)
	return w, uiErr(err)
}

// RejectRequestItem turns down a proposal or an action.
func (a *App) RejectRequestItem(projectID, itemID, note string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.db.RejectRequestItem(a.ctx, projectID, itemID, note))
}

// ConfirmRequestAction closes an action the person checked after a stop.
func (a *App) ConfirmRequestAction(projectID, itemID, note string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.db.ConfirmRequestAction(a.ctx, projectID, itemID, note))
}

// ActionInput is the person's approval of an action, with what they
// chose: how and where code is delivered.
type ActionInput struct {
	ProjectID      string `json:"projectId"`
	ItemID         string `json:"itemId"`
	Method         string `json:"method,omitempty"`
	Folder         string `json:"folder,omitempty"`
	UseAsWorkspace bool   `json:"useAsWorkspace,omitempty"`
	Note           string `json:"note,omitempty"`
}

// ActionOutcome says what an approved action did. Cancelled is set when
// the person closed the save dialog: nothing ran.
type ActionOutcome struct {
	Item      domain.RequestItem `json:"item"`
	Cancelled bool               `json:"cancelled"`
}

// RunRequestAction carries out an action a desk proposed, once the person
// approves it (plan §4.5). Save dialogs come first, so closing one leaves
// the action as it was; the result or error is kept on the item.
func (a *App) RunRequestAction(in ActionInput) (ActionOutcome, error) {
	if err := a.ready(); err != nil {
		return ActionOutcome{}, err
	}
	r, err := a.db.Request(a.ctx, in.ProjectID, requestOf(a, in.ItemID))
	if err != nil {
		return ActionOutcome{}, uiErr(err)
	}
	var item domain.RequestItem
	for _, it := range r.Items {
		if it.ID == in.ItemID {
			item = it
		}
	}
	if item.Kind != domain.ItemAction {
		return ActionOutcome{}, errors.New("조치 항목을 찾을 수 없습니다")
	}
	var p domain.ActionPayload
	json.Unmarshal(item.Payload, &p)
	var path string
	switch p.Action {
	case domain.ActionSaveResult:
		if path, err = a.artifactDialog(in.ProjectID, p.ArtifactID); err != nil || path == "" {
			return ActionOutcome{Item: item, Cancelled: err == nil}, uiErr(err)
		}
	case domain.ActionExportService:
		if path, err = a.exportDialog(in.ProjectID); err != nil || path == "" {
			return ActionOutcome{Item: item, Cancelled: err == nil}, uiErr(err)
		}
	case domain.ActionDeliverCode, domain.ActionStartWorkflow:
	default:
		return ActionOutcome{}, fmt.Errorf("알 수 없는 조치입니다: %s", p.Action)
	}
	if _, err := a.db.StartRequestAction(a.ctx, in.ProjectID, in.ItemID, in.Note); err != nil {
		return ActionOutcome{}, uiErr(err)
	}
	var result any
	switch p.Action {
	case domain.ActionDeliverCode:
		result, err = a.eng.DeliverCode(a.ctx, engine.DeliverInput{ProjectID: in.ProjectID, RunID: p.RunID, Method: in.Method,
			Folder: in.Folder, UseAsWorkspace: in.UseAsWorkspace, RequestItemID: in.ItemID})
	case domain.ActionSaveResult:
		err = a.writeArtifactTo(in.ProjectID, p.ArtifactID, path)
		result = map[string]string{"path": path}
	case domain.ActionExportService:
		result, err = a.exportTo(in.ProjectID, path)
	case domain.ActionStartWorkflow:
		var res StartResult
		if res, err = a.StartRun(in.ProjectID, p.VersionID); err == nil && len(res.Issues) > 0 {
			err = fmt.Errorf("흐름을 실행할 수 없습니다: %s", res.Issues[0].Message)
		}
		result = res
	}
	errText := ""
	if err != nil {
		errText = uiErr(err).Error()
	}
	if ferr := a.db.FinishRequestAction(a.ctx, in.ItemID, result, errText); ferr != nil {
		return ActionOutcome{}, uiErr(ferr)
	}
	out, gerr := a.db.Request(a.ctx, in.ProjectID, r.ID)
	if gerr == nil {
		for _, it := range out.Items {
			if it.ID == in.ItemID {
				item = it
			}
		}
	}
	return ActionOutcome{Item: item}, uiErr(err)
}

// requestOf is the request an item belongs to ("" when unknown).
func requestOf(a *App, itemID string) string {
	var id string
	a.db.Read().QueryRowContext(a.ctx, `SELECT request_id FROM request_items WHERE id = ?`, itemID).Scan(&id)
	return id
}

// OrgView is a service's organization proposal with what the person needs
// to judge it: assignee names, and whether auto mode could start the new
// departments' work (connections allowed, a budget).
type OrgView struct {
	Proposal *storage.OrgProposal `json:"proposal,omitempty"`
	Names    map[string]string    `json:"names"`
	AutoMode bool                 `json:"autoMode"`
	// Allowed are the connections auto mode may use now; SuggestAllow are
	// the ones the proposal's AI members use that it may not yet.
	Allowed      []ConnectionChoice `json:"allowed"`
	SuggestAllow []ConnectionChoice `json:"suggestAllow"`
	HasBudget    bool               `json:"hasBudget"`
}

// ConnectionChoice names a connection.
type ConnectionChoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (a *App) orgView(projectID string, p *storage.OrgProposal) (OrgView, error) {
	v := OrgView{Proposal: p, Names: map[string]string{}, Allowed: []ConnectionChoice{}, SuggestAllow: []ConnectionChoice{}}
	proj, err := a.db.Project(a.ctx, projectID)
	if err != nil {
		return v, err
	}
	v.AutoMode = proj.Mode == domain.ModeAuto
	var b engine.Budget
	json.Unmarshal(proj.Budget, &b)
	v.HasBudget = b.MaxTokens != nil || b.MaxCostUSD != nil
	var policy struct {
		AllowedConnectionIDs []string `json:"allowedConnectionIds"`
	}
	json.Unmarshal(proj.AutoPolicy, &policy)
	people, err := a.db.Assignments(a.ctx, projectID)
	if err != nil {
		return v, err
	}
	connOf := map[string]string{}
	for _, x := range people {
		v.Names[x.ID] = x.DisplayName
		connOf[x.ID] = x.ConnectionID
	}
	name := func(id string) string {
		if c, err := a.db.Connection(a.ctx, id); err == nil {
			return c.Name
		}
		return id
	}
	for _, id := range policy.AllowedConnectionIDs {
		v.Allowed = append(v.Allowed, ConnectionChoice{ID: id, Name: name(id)})
	}
	if p != nil {
		seen := map[string]bool{}
		for _, d := range p.Plan.Departments {
			for _, m := range d.Members {
				c := connOf[m]
				if c == "" || seen[c] || slicesContains(policy.AllowedConnectionIDs, c) {
					continue
				}
				seen[c] = true
				if conn, err := a.db.Connection(a.ctx, c); err == nil && (conn.Provider == "test" || conn.Verified()) {
					v.SuggestAllow = append(v.SuggestAllow, ConnectionChoice{ID: c, Name: conn.Name})
				}
			}
		}
	}
	return v, nil
}

func slicesContains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// LatestOrgProposal returns a service's last organization proposal.
func (a *App) LatestOrgProposal(projectID string) (OrgView, error) {
	if err := a.ready(); err != nil {
		return OrgView{}, err
	}
	if err := a.db.CheckScope(a.ctx, projectID); err != nil {
		return OrgView{}, uiErr(err)
	}
	p, ok, err := a.db.LatestOrgProposal(a.ctx, projectID)
	if err != nil {
		return OrgView{}, uiErr(err)
	}
	if !ok {
		v, err := a.orgView(projectID, nil)
		return v, uiErr(err)
	}
	v, err := a.orgView(projectID, &p)
	return v, uiErr(err)
}

// ProposeOrganization has an AI propose the service's departments (and a
// desk when it has none). It waits for the AI and uses its quota.
func (a *App) ProposeOrganization(projectID string) (OrgView, error) {
	if err := a.ready(); err != nil {
		return OrgView{}, err
	}
	if err := a.db.CheckScope(a.ctx, projectID); err != nil {
		return OrgView{}, uiErr(err)
	}
	p, err := a.intake.ProposeOrg(a.ctx, projectID)
	if err != nil {
		return OrgView{}, uiErr(err)
	}
	v, err := a.orgView(projectID, &p)
	return v, uiErr(err)
}

// ApplyOrganization creates a proposal's departments and desk. allow are
// connections auto mode may now use (the person's choice).
func (a *App) ApplyOrganization(projectID, proposalID string, allow []string) (storage.OrgApplied, error) {
	if err := a.ready(); err != nil {
		return storage.OrgApplied{}, err
	}
	if err := a.db.CheckScope(a.ctx, projectID); err != nil {
		return storage.OrgApplied{}, uiErr(err)
	}
	out, err := a.intake.ApplyOrgProposal(a.ctx, projectID, proposalID, allow)
	if err == nil {
		a.auto.Wake()
		a.intake.Wake()
	}
	return out, uiErr(err)
}

// DiscardOrgProposal sets a proposal aside without applying it.
func (a *App) DiscardOrgProposal(projectID, proposalID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.db.CloseOrgProposal(a.ctx, projectID, proposalID, "discarded", nil))
}

// MarkReportRead records that the person read a request's completion
// report; it then leaves the to-dos.
func (a *App) MarkReportRead(projectID, id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.db.MarkReportRead(a.ctx, projectID, id))
}
