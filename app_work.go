package main

import (
	"agent-office/internal/domain"
	"agent-office/internal/storage"
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

// RetryDelivery puts a failed or held delivery back in the queue.
func (a *App) RetryDelivery(projectID, deliveryID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	if err := a.db.RetryDelivery(a.ctx, projectID, deliveryID); err != nil {
		return uiErr(err)
	}
	a.work.Wake()
	return nil
}

// SkipDelivery closes a delivery the person decided not to act on.
func (a *App) SkipDelivery(projectID, deliveryID, reason string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.db.SkipDelivery(a.ctx, projectID, deliveryID, reason))
}
