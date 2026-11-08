package ops

import (
	"context"
)

// contextDefectLimit caps the open defects listed in the briefing; the rest
// are reachable through `defect list --open`.
const contextDefectLimit = 15

// Context assembles the one-shot product briefing: what the product is, how
// it is structured, which rules apply, what is being worked on, and the QA
// state. It is sized for an agent's context window: descriptions of
// individual items are left to the show operations.
func (s *Session) Context(ctx context.Context) (*ContextView, error) {
	p, err := s.Product(ctx)
	if err != nil {
		return nil, err
	}
	v := &ContextView{Workflow: []string{}, Constraints: []ConstraintSummary{}, InProgress: []RequirementSummary{}}
	v.Product.ID, v.Product.Name, v.Product.Description = p.ID, p.Name, p.Description
	v.Product.WorkflowMode, v.Product.EstimationMode = p.WorkflowMode, p.EstimationMode

	wf, err := s.workflow(ctx, "requirement")
	if err != nil {
		return nil, err
	}
	for _, st := range wf {
		v.Workflow = append(v.Workflow, st.Name)
	}

	structure, err := s.Structure(ctx)
	if err != nil {
		return nil, err
	}
	v.Structure = *structure

	if v.Constraints, err = s.ListConstraints(ctx, ConstraintQuery{}); err != nil {
		return nil, err
	}

	sprints, err := s.sprints(ctx)
	if err != nil {
		return nil, err
	}
	for _, sp := range sprints {
		if sp.Status == "active" {
			v.ActiveSprint = &SprintView{Name: sp.Name, StartDate: sp.StartDate, EndDate: sp.EndDate}
			break
		}
	}

	open, err := s.ListRequirements(ctx, RequirementQuery{Open: true})
	if err != nil {
		return nil, err
	}
	for _, r := range open {
		if r.Kind == "task" {
			v.Backlog.OpenTasks++
		} else {
			v.Backlog.OpenRequirements++
		}
		if r.Coarse {
			v.Backlog.Coarse++
		}
		if r.Active {
			v.InProgress = append(v.InProgress, r)
		}
	}

	if v.OpenDefects, err = s.ListDefects(ctx, DefectQuery{Open: true, Limit: contextDefectLimit}); err != nil {
		return nil, err
	}

	campaigns, err := s.campaigns(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range campaigns {
		if c.Status == "active" {
			if v.ActiveCampaign, err = s.campaignView(ctx, c); err != nil {
				return nil, err
			}
			break
		}
	}
	return v, nil
}
