package main

import (
	"encoding/json"
	"errors"

	"agent-office/internal/domain"
	"agent-office/internal/engine"
	"agent-office/internal/storage"
	"agent-office/internal/work"
)

// ---- department templates (company) ----

func (a *App) ListDepartmentTemplates() ([]domain.DepartmentTemplate, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.db.DepartmentTemplates(a.ctx)
	return list, uiErr(err)
}

// SaveDepartmentTemplate creates or updates a template. Departments
// already copied from it into services do not change.
func (a *App) SaveDepartmentTemplate(d domain.DepartmentTemplate) (domain.DepartmentTemplate, error) {
	if err := a.ready(); err != nil {
		return d, err
	}
	saved, err := a.db.SaveDepartmentTemplate(a.ctx, d)
	return saved, uiErr(err)
}

func (a *App) DeleteDepartmentTemplate(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.db.DeleteDepartmentTemplate(a.ctx, id))
}

// ---- departments (inside a service) and categories ----

func (a *App) ListDepartments(projectID string) ([]domain.Department, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.db.Departments(a.ctx, projectID)
	return list, uiErr(err)
}

// SaveDepartment creates or updates a department of a service. MemberIDs
// replaces its members (the service's assignments); a member listed here
// leaves the department it was in.
func (a *App) SaveDepartment(d domain.Department) (domain.Department, error) {
	if err := a.ready(); err != nil {
		return d, err
	}
	saved, err := a.db.SaveDepartment(a.ctx, d)
	return saved, uiErr(err)
}

// ImportDepartmentTemplate copies a company template into the service as
// a new department.
func (a *App) ImportDepartmentTemplate(projectID, templateID string) (domain.Department, error) {
	if err := a.ready(); err != nil {
		return domain.Department{}, err
	}
	d, err := a.db.ImportDepartmentTemplate(a.ctx, projectID, templateID)
	return d, uiErr(err)
}

// SaveDepartmentAsTemplate copies a service department into a new
// company template named name (the department's name when empty).
func (a *App) SaveDepartmentAsTemplate(projectID, departmentID, name string) (domain.DepartmentTemplate, error) {
	if err := a.ready(); err != nil {
		return domain.DepartmentTemplate{}, err
	}
	t, err := a.db.SaveDepartmentAsTemplate(a.ctx, projectID, departmentID, name)
	return t, uiErr(err)
}

func (a *App) ProjectCategories(projectID string) ([]string, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.db.ProjectCategories(a.ctx, projectID)
	return list, uiErr(err)
}

func (a *App) SetProjectCategories(projectID string, names []string) ([]string, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.db.SetProjectCategories(a.ctx, projectID, names)
	return list, uiErr(err)
}

// ---- work items ----

func (a *App) ListWorkItems(projectID string, filter storage.WorkFilter) ([]domain.WorkItem, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.db.WorkItems(a.ctx, projectID, filter)
	return list, uiErr(err)
}

// WorkDetail is one work item with its history, and for a goal the work
// under it.
type WorkDetail struct {
	Item     domain.WorkItem   `json:"item"`
	Notes    []domain.WorkNote `json:"notes"`
	Children []domain.WorkItem `json:"children"`
}

func (a *App) GetWorkItem(projectID, id string) (WorkDetail, error) {
	if err := a.ready(); err != nil {
		return WorkDetail{}, err
	}
	item, err := a.db.WorkItem(a.ctx, projectID, id)
	if err != nil {
		return WorkDetail{}, uiErr(err)
	}
	d := WorkDetail{Item: item, Children: []domain.WorkItem{}}
	if d.Notes, err = a.db.WorkNotes(a.ctx, projectID, id); err != nil {
		return WorkDetail{}, uiErr(err)
	}
	if item.Kind == domain.WorkGoal {
		if d.Children, err = a.db.WorkItems(a.ctx, projectID, storage.WorkFilter{ParentID: id}); err != nil {
			return WorkDetail{}, uiErr(err)
		}
	}
	return d, nil
}

func (a *App) CreateWorkItem(w domain.WorkItem) (domain.WorkItem, error) {
	if err := a.ready(); err != nil {
		return w, err
	}
	saved, err := a.db.CreateWorkItem(a.ctx, w)
	return saved, uiErr(err)
}

func (a *App) UpdateWorkItem(w domain.WorkItem) (domain.WorkItem, error) {
	if err := a.ready(); err != nil {
		return w, err
	}
	saved, err := a.db.UpdateWorkItem(a.ctx, w)
	return saved, uiErr(err)
}

// SetWorkStatus changes a work item's status; waiting and blocked need a
// reason. Done and cancelled close the item, any other status reopens it.
func (a *App) SetWorkStatus(projectID, id, status, reason string) (domain.WorkItem, error) {
	if err := a.ready(); err != nil {
		return domain.WorkItem{}, err
	}
	saved, err := a.db.SetWorkStatus(a.ctx, projectID, id, status, reason)
	return saved, uiErr(err)
}

func (a *App) AddWorkNote(projectID, id, body string) (domain.WorkNote, error) {
	if err := a.ready(); err != nil {
		return domain.WorkNote{}, err
	}
	n, err := a.db.AddWorkNote(a.ctx, projectID, id, body)
	return n, uiErr(err)
}

// ---- work events, subscriptions and department inboxes ----

// ListWorkEvents lists a work item's events and where each went.
func (a *App) ListWorkEvents(projectID, workItemID string) ([]storage.WorkEventView, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.db.WorkEvents(a.ctx, projectID, workItemID)
	return list, uiErr(err)
}

func (a *App) ListSubscriptions(projectID, departmentID string) ([]domain.Subscription, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.db.Subscriptions(a.ctx, projectID, departmentID)
	return list, uiErr(err)
}

// SaveSubscription creates or updates what a department hears about.
func (a *App) SaveSubscription(s domain.Subscription) (domain.Subscription, error) {
	if err := a.ready(); err != nil {
		return s, err
	}
	saved, err := a.db.SaveSubscription(a.ctx, s)
	return saved, uiErr(err)
}

// ListDeliveries lists a service's department inbox entries.
func (a *App) ListDeliveries(projectID string, filter storage.DeliveryFilter) ([]storage.Delivery, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.db.Deliveries(a.ctx, projectID, filter)
	return list, uiErr(err)
}

// RequestDepartmentCheck asks a department to look for work to do.
func (a *App) RequestDepartmentCheck(projectID, departmentID, note string) error {
	if err := a.ready(); err != nil {
		return err
	}
	_, err := a.db.RequestDepartmentCheck(a.ctx, projectID, departmentID, note)
	return uiErr(err)
}

// RetryDelivery puts a failed or held delivery back in the queue, or
// retries its judgment.
func (a *App) RetryDelivery(projectID, deliveryID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	if err := a.db.RetryDelivery(a.ctx, projectID, deliveryID); err != nil {
		return uiErr(err)
	}
	a.work.Wake()
	for _, l := range []string{storage.LaneInteractive, storage.LaneBackground} {
		a.ai.Wake(l) // a retried judgment job
	}
	return nil
}

// SkipDelivery closes a delivery the person decided not to act on.
func (a *App) SkipDelivery(projectID, deliveryID, reason string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.db.SkipDelivery(a.ctx, projectID, deliveryID, reason))
}

// ---- running work ----

// WorkPlanView is a work item's plan (Saved false when none is stored
// yet: Plan then holds a starting point) and what a run would be told.
type WorkPlanView struct {
	Plan    domain.WorkPlan `json:"plan"`
	Saved   bool            `json:"saved"`
	Preview string          `json:"preview"`
	Problem string          `json:"problem"`
}

// GetWorkPlan returns the plan of a work item. Without a saved plan it
// suggests the work's department lead as the assignee.
func (a *App) GetWorkPlan(projectID, workItemID string) (WorkPlanView, error) {
	if err := a.ready(); err != nil {
		return WorkPlanView{}, err
	}
	plan, saved, err := a.db.WorkPlan(a.ctx, projectID, workItemID)
	if err != nil {
		return WorkPlanView{}, uiErr(err)
	}
	v := WorkPlanView{Plan: plan, Saved: saved}
	if !saved {
		v.Plan.ProjectID, v.Plan.Outputs = projectID, domain.DefaultWorkOutputs()
		if w, err := a.db.WorkItem(a.ctx, projectID, workItemID); err == nil && w.DepartmentID != "" {
			if d, err := a.db.Department(a.ctx, projectID, w.DepartmentID); err == nil {
				v.Plan.AssignmentID = d.LeadAssignmentID
			}
		}
		return v, nil
	}
	docs, _, err := a.eng.WorkDocumentsSection(a.ctx, projectID, workItemID)
	if err != nil {
		return v, uiErr(err)
	}
	spec, err := a.db.ComposeWorkSpec(a.ctx, projectID, workItemID, docs)
	if err != nil {
		v.Problem = uiErr(err).Error()
	} else {
		v.Preview = spec.Nodes[0].Instructions
	}
	return v, nil
}

func (a *App) SaveWorkPlan(p domain.WorkPlan) (domain.WorkPlan, error) {
	if err := a.ready(); err != nil {
		return p, err
	}
	saved, err := a.db.SaveWorkPlan(a.ctx, p)
	return saved, uiErr(err)
}

// StartWork runs a work item's saved plan. With feedback, the run is a
// change request on the latest result. Finishing a run never completes
// the work: a person checks the result first.
func (a *App) StartWork(projectID, workItemID, feedback string) (StartResult, error) {
	if err := a.ready(); err != nil {
		return StartResult{}, err
	}
	id, err := a.eng.StartWork(a.ctx, projectID, workItemID, feedback)
	var nr *engine.NotRunnableError
	if errors.As(err, &nr) {
		return StartResult{Issues: nr.Issues}, nil
	}
	return StartResult{RunID: id, Issues: []domain.Issue{}}, uiErr(err)
}

// ListWorkRuns lists a work item's runs, newest first.
func (a *App) ListWorkRuns(projectID, workItemID string) ([]engine.RunDetail, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	runs, err := a.eng.WorkRuns(a.ctx, projectID, workItemID)
	return runs, uiErr(err)
}

// ---- AI work plans ----

// PlanJobView is one AI plan proposal: the job, the proposal when ready,
// and whether the work changed since it was made (then it cannot apply).
type PlanJobView struct {
	Job      storage.AIJob      `json:"job"`
	Proposal *work.ProposedPlan `json:"proposal,omitempty"`
	Stale    bool               `json:"stale"`
}

// RequestWorkPlan asks the work's AI to propose a plan. requestID is made
// by the screen for each click, so a repeated click is one request.
func (a *App) RequestWorkPlan(projectID, workItemID, requestID string) (storage.AIJob, error) {
	if err := a.ready(); err != nil {
		return storage.AIJob{}, err
	}
	job, err := work.RequestPlan(a.ctx, a.db, projectID, workItemID, requestID)
	if err == nil {
		a.ai.Wake(job.Lane)
	}
	return job, uiErr(err)
}

// ListPlanJobs lists a work item's plan proposals, newest first.
func (a *App) ListPlanJobs(projectID, workItemID string) ([]PlanJobView, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	jobs, err := a.db.AIJobs(a.ctx, projectID, workItemID, storage.JobPlan)
	if err != nil {
		return nil, uiErr(err)
	}
	out := []PlanJobView{}
	for _, j := range jobs {
		v := PlanJobView{Job: j}
		if j.Status == storage.JobReady {
			var p work.ProposedPlan
			if json.Unmarshal(j.Result, &p) == nil {
				v.Proposal = &p
			}
			v.Stale, _ = work.PlanStale(a.ctx, a.db, j)
		}
		out = append(out, v)
	}
	return out, nil
}

// ApplyPlanJob saves a ready proposal as the work's plan. It does not
// start a run: the person checks the plan and runs it.
func (a *App) ApplyPlanJob(projectID, jobID string) (domain.WorkPlan, error) {
	if err := a.ready(); err != nil {
		return domain.WorkPlan{}, err
	}
	p, err := work.ApplyPlan(a.ctx, a.db, projectID, jobID)
	return p, uiErr(err)
}

// RetryAIJob queues a failed or held AI job again.
func (a *App) RetryAIJob(projectID, jobID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	job, err := a.db.AIJob(a.ctx, projectID, jobID)
	if err != nil {
		return uiErr(err)
	}
	if err := a.db.RetryAIJob(a.ctx, projectID, jobID); err != nil {
		return uiErr(err)
	}
	a.ai.Wake(job.Lane)
	return nil
}

// ---- shared documents ----

func (a *App) ListDocuments(projectID string, filter storage.DocumentFilter) ([]storage.SharedDocument, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.db.Documents(a.ctx, projectID, filter)
	return list, uiErr(err)
}

func (a *App) GetDocument(projectID, documentID string) (storage.SharedDocument, error) {
	if err := a.ready(); err != nil {
		return storage.SharedDocument{}, err
	}
	d, err := a.db.Document(a.ctx, projectID, documentID)
	return d, uiErr(err)
}

// ReadDocumentVersion returns a shared copy's text and whether its hash
// still matches.
func (a *App) ReadDocumentVersion(projectID, versionID string) (engine.DocumentContent, error) {
	if err := a.ready(); err != nil {
		return engine.DocumentContent{}, err
	}
	c, err := a.eng.ReadDocumentVersion(a.ctx, projectID, versionID)
	return c, uiErr(err)
}

// ShareArtifact shares a run result as a new document, or as a new
// version when doc.documentId is set. The work does not have to be done.
func (a *App) ShareArtifact(projectID, artifactID string, doc storage.NewDocumentVersion) (storage.DocumentVersion, error) {
	if err := a.ready(); err != nil {
		return storage.DocumentVersion{}, err
	}
	v, err := a.eng.ShareArtifact(a.ctx, projectID, artifactID, doc)
	return v, uiErr(err)
}

// ShareWorkspaceFile shares a file of the service's work folder, given by
// its path inside that folder.
func (a *App) ShareWorkspaceFile(projectID, relPath string, doc storage.NewDocumentVersion) (storage.DocumentVersion, error) {
	if err := a.ready(); err != nil {
		return storage.DocumentVersion{}, err
	}
	v, err := a.eng.ShareWorkspaceFile(a.ctx, projectID, relPath, doc)
	return v, uiErr(err)
}

// ConfirmDocumentVersion marks a version final without changing it.
func (a *App) ConfirmDocumentVersion(projectID, versionID, note string) (storage.DocumentVersion, error) {
	if err := a.ready(); err != nil {
		return storage.DocumentVersion{}, err
	}
	v, err := a.db.ConfirmDocumentVersion(a.ctx, projectID, versionID, note)
	return v, uiErr(err)
}

// WithdrawDocument withdraws one version, or the whole document when
// versionID is "".
func (a *App) WithdrawDocument(projectID, documentID, versionID, reason string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.db.WithdrawDocument(a.ctx, projectID, documentID, versionID, reason))
}

func (a *App) LinkDocument(projectID, workItemID, versionID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.db.LinkDocument(a.ctx, projectID, workItemID, versionID))
}

func (a *App) UnlinkDocument(projectID, workItemID, versionID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.db.UnlinkDocument(a.ctx, projectID, workItemID, versionID))
}

// ListWorkDocuments lists the shared versions a work item links.
func (a *App) ListWorkDocuments(projectID, workItemID string) ([]storage.LinkedDocument, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.db.WorkDocuments(a.ctx, projectID, workItemID)
	return list, uiErr(err)
}

// ListRunDocuments lists the shared versions a run was given.
func (a *App) ListRunDocuments(projectID, runID string) ([]storage.DocumentVersion, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.db.RunDocuments(a.ctx, projectID, runID)
	return list, uiErr(err)
}

// ListImpactChecks lists changes to check, for a work item or the whole
// service ("" ), with a status or "" for all.
func (a *App) ListImpactChecks(projectID, workItemID, status string) ([]storage.ImpactCheck, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.db.ImpactChecks(a.ctx, projectID, workItemID, status)
	return list, uiErr(err)
}

// CloseImpactCheck records that a change was checked (resolved) or does
// not matter (dismissed).
func (a *App) CloseImpactCheck(projectID, id, status, note string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.db.CloseImpactCheck(a.ctx, projectID, id, status, note))
}

// ---- department judgments and proposals ----

// ListJudgments lists a service's judgments ("" for every service);
// questionStatus filters, e.g. "open,manual" for questions to answer.
func (a *App) ListJudgments(projectID, questionStatus string) ([]storage.Judgment, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.db.Judgments(a.ctx, projectID, questionStatus)
	return list, uiErr(err)
}

// AnswerJudgment answers a department's question; the department judges
// once more with the answer.
func (a *App) AnswerJudgment(projectID, judgmentID, answer string) error {
	if err := a.ready(); err != nil {
		return err
	}
	job, err := a.db.AnswerJudgment(a.ctx, projectID, judgmentID, answer)
	if err != nil {
		return uiErr(err)
	}
	a.ai.Wake(job.Lane)
	return nil
}

// CloseJudgmentQuestion closes a question the person decided themselves.
func (a *App) CloseJudgmentQuestion(projectID, judgmentID, note string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.db.CloseJudgmentQuestion(a.ctx, projectID, judgmentID, note))
}

// ListProposals lists proposed work of a service ("" for every service)
// with a status ("proposed", "accepted", "rejected") or "" for all.
func (a *App) ListProposals(projectID, status string) ([]storage.Proposal, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.db.Proposals(a.ctx, projectID, status)
	return list, uiErr(err)
}

// AcceptProposal registers a proposal as work, with the person's edits.
func (a *App) AcceptProposal(projectID, proposalID string, edit storage.ProposalEdit) (domain.WorkItem, error) {
	if err := a.ready(); err != nil {
		return domain.WorkItem{}, err
	}
	w, err := a.db.AcceptProposal(a.ctx, projectID, proposalID, edit)
	return w, uiErr(err)
}

// RejectProposal declines a proposal.
func (a *App) RejectProposal(projectID, proposalID, note string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.db.RejectProposal(a.ctx, projectID, proposalID, note))
}

// ProposalBatch is what deciding several proposals at once did.
type ProposalBatch struct {
	Done   int      `json:"done"`
	Errors []string `json:"errors"`
}

// DecideProposals accepts (unchanged) or rejects several proposals. Each
// is decided on its own, so one already decided does not stop the rest.
func (a *App) DecideProposals(projectID string, proposalIDs []string, accept bool, note string) (ProposalBatch, error) {
	if err := a.ready(); err != nil {
		return ProposalBatch{}, err
	}
	out := ProposalBatch{Errors: []string{}}
	for _, id := range proposalIDs {
		var err error
		if accept {
			_, err = a.db.AcceptProposal(a.ctx, projectID, id, storage.ProposalEdit{})
		} else {
			err = a.db.RejectProposal(a.ctx, projectID, id, note)
		}
		if err != nil {
			out.Errors = append(out.Errors, uiErr(err).Error())
			continue
		}
		out.Done++
	}
	return out, nil
}

// Decisions is what departments wait on a person for, across services:
// questions, proposals, collaboration requests to decide or answer, and
// required collaboration that changed after work was finished.
type Decisions struct {
	Questions []storage.Judgment       `json:"questions"`
	Proposals []storage.Proposal       `json:"proposals"`
	Collabs   []storage.CollabRequest  `json:"collabs"`
	Notices   []storage.RequiredNotice `json:"notices"`
}

// PendingDecisions lists what every active service waits on a person for,
// for the inbox.
func (a *App) PendingDecisions() (Decisions, error) {
	if err := a.ready(); err != nil {
		return Decisions{}, err
	}
	var d Decisions
	var err error
	if d.Questions, err = a.db.Judgments(a.ctx, "", "open,manual"); err != nil {
		return Decisions{}, uiErr(err)
	}
	if d.Proposals, err = a.db.Proposals(a.ctx, "", "proposed"); err != nil {
		return Decisions{}, uiErr(err)
	}
	if d.Collabs, err = a.db.CollabRequests(a.ctx, "", storage.CollabFilter{Open: true}); err != nil {
		return Decisions{}, uiErr(err)
	}
	d.Notices, err = a.db.OpenRequiredNotices(a.ctx)
	return d, uiErr(err)
}

// RequestDocumentReview asks a department to judge existing documents,
// for example when it was just created.
func (a *App) RequestDocumentReview(projectID, departmentID string, versionIDs []string, note string) (int, error) {
	if err := a.ready(); err != nil {
		return 0, err
	}
	n, err := a.db.RequestDocumentReview(a.ctx, projectID, departmentID, versionIDs, note)
	return n, uiErr(err)
}

// ---- collaboration requests ----

// WorkCollab is a work item's collaboration: what it asked of other
// departments (for a goal also what its work asked), the request it is
// the work of, the required requests that keep it from being completed,
// and open checks about required work that changed.
type WorkCollab struct {
	Requests []storage.CollabRequest `json:"requests"`
	Incoming []storage.CollabRequest `json:"incoming"`
	Blockers []storage.CollabRequest `json:"blockers"`
	Checks   []storage.ImpactCheck   `json:"checks"`
}

func (a *App) GetWorkCollab(projectID, workItemID string) (WorkCollab, error) {
	if err := a.ready(); err != nil {
		return WorkCollab{}, err
	}
	var c WorkCollab
	var err error
	if c.Requests, err = a.db.CollabRequests(a.ctx, projectID, storage.CollabFilter{WorkItemID: workItemID}); err != nil {
		return WorkCollab{}, uiErr(err)
	}
	if c.Incoming, err = a.db.CollabRequests(a.ctx, projectID, storage.CollabFilter{ResultWorkItemID: workItemID}); err != nil {
		return WorkCollab{}, uiErr(err)
	}
	if c.Blockers, err = a.db.CompletionBlockers(a.ctx, projectID, workItemID); err != nil {
		return WorkCollab{}, uiErr(err)
	}
	checks, err := a.db.ImpactChecks(a.ctx, projectID, workItemID, "open")
	c.Checks = []storage.ImpactCheck{}
	for _, ch := range checks {
		if ch.Cause == storage.CauseRequiredReopened {
			c.Checks = append(c.Checks, ch)
		}
	}
	return c, uiErr(err)
}

// ListCollabRequests lists a service's collaboration requests.
func (a *App) ListCollabRequests(projectID string, filter storage.CollabFilter) ([]storage.CollabRequest, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.db.CollabRequests(a.ctx, projectID, filter)
	return list, uiErr(err)
}

// RequestCollab asks another department for help on a work item. It
// reaches that department whatever it subscribes to.
func (a *App) RequestCollab(projectID string, n storage.NewCollabRequest) (storage.CollabRequest, error) {
	if err := a.ready(); err != nil {
		return storage.CollabRequest{}, err
	}
	r, err := a.db.RequestCollab(a.ctx, projectID, n)
	return r, uiErr(err)
}

// AcceptCollab takes a request on for the receiving department: new work
// is registered for it, or one of its open work items is chosen.
func (a *App) AcceptCollab(projectID, id string, acceptance storage.CollabAcceptance) (storage.CollabRequest, error) {
	if err := a.ready(); err != nil {
		return storage.CollabRequest{}, err
	}
	r, err := a.db.AcceptCollab(a.ctx, projectID, id, acceptance)
	return r, uiErr(err)
}

// AskCollab records the receiving department's question about a request.
func (a *App) AskCollab(projectID, id, question string) (storage.CollabRequest, error) {
	if err := a.ready(); err != nil {
		return storage.CollabRequest{}, err
	}
	r, err := a.db.AskCollab(a.ctx, projectID, id, question)
	return r, uiErr(err)
}

// AnswerCollab answers that question for the asking department.
func (a *App) AnswerCollab(projectID, id, answer string) (storage.CollabRequest, error) {
	if err := a.ready(); err != nil {
		return storage.CollabRequest{}, err
	}
	r, err := a.db.AnswerCollab(a.ctx, projectID, id, answer)
	return r, uiErr(err)
}

// DeclineCollab declines a request for the receiving department.
func (a *App) DeclineCollab(projectID, id, reason string) (storage.CollabRequest, error) {
	if err := a.ready(); err != nil {
		return storage.CollabRequest{}, err
	}
	r, err := a.db.DeclineCollab(a.ctx, projectID, id, reason)
	return r, uiErr(err)
}

// CancelCollab withdraws a request that was not taken on.
func (a *App) CancelCollab(projectID, id, reason string) (storage.CollabRequest, error) {
	if err := a.ready(); err != nil {
		return storage.CollabRequest{}, err
	}
	r, err := a.db.CancelCollab(a.ctx, projectID, id, reason)
	return r, uiErr(err)
}

// SetCollabRequirement makes a request required or reference; releasing a
// required one needs a reason.
func (a *App) SetCollabRequirement(projectID, id, requirement, reason string) (storage.CollabRequest, error) {
	if err := a.ready(); err != nil {
		return storage.CollabRequest{}, err
	}
	r, err := a.db.SetCollabRequirement(a.ctx, projectID, id, requirement, reason)
	return r, uiErr(err)
}

// ReplaceCollab hands a required request over to a new required request.
func (a *App) ReplaceCollab(projectID, id string, n storage.NewCollabRequest, reason string) (storage.CollabRequest, error) {
	if err := a.ready(); err != nil {
		return storage.CollabRequest{}, err
	}
	r, err := a.db.ReplaceCollab(a.ctx, projectID, id, n, reason)
	return r, uiErr(err)
}
