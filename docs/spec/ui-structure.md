# Page Tree & Navigation

This document defines the UI page tree, navigation structure, and related design decisions.

## Design Principles

1. **Three-layer depth**: global > product-scoped > entity detail. No page is more than three clicks from the dashboard.
2. **Product as context, not destination**: selecting a product sets the sidebar context; everything inside is product-relative.
3. **Tabs over pages for related data**: detail pages use tabs for comments, attachments, linked items, and history rather than separate routes.
4. **URL = state**: filters, sort order, and active tab are reflected in the URL so views are bookmarkable and shareable.
5. **Consistent CRUD pattern**: every entity follows list > detail (tabbed) > create/edit.
6. **Show only what the role can do**: actions the user's role does not allow are not offered. A create or edit form opened without the role to save it (a bookmark, a typed URL) redirects to what the user can read instead: the list for a create form, the detail page for an edit form. That page shows a notice naming the role required.

## Navigation Structure

### Sidebar

The sidebar is visible on all authenticated pages. Its contents adapt to context:

- **Always visible**: Dashboard link, Admin section (admin-only)
- **Product context active**: Backlog, Testing, and Structure sections appear
- **Product switcher**: mounted at the top of the sidebar below `md` only, where the sidebar is the mobile drawer. From `sm` up the header carries it instead.

### Header

The header bar contains:

- **Left**: logo, then the product switcher when a product is in context (hidden below `sm`)
- **Right**: theme toggle (quick access), user dropdown (profile, API tokens, admin link, sign out)

Breadcrumbs are injected per page into the content area, not the header.

### Breadcrumbs

Every page below the dashboard renders breadcrumbs for upward navigation. Pattern:

```
Dashboard > [Product Name] > [Section] > [Entity Title]
```

## Page Tree

### Authentication (unauthenticated, uses AuthLayout)

| Route                    | Page              | Purpose                           |
|--------------------------|-------------------|-----------------------------------|
| `/login`                 | Login             | Username/password + optional TOTP |
| `/register`              | Register          | Self-registration (if enabled)    |
| `/accept-invite/:token`  | Accept Invitation | Register via invitation link      |
| `/forgot-password`       | Forgot Password   | Request password reset email      |
| `/reset-password/:token` | Reset Password    | Set new password via reset link   |

### Global (authenticated, uses AppLayout without product context)

| Route           | Page           | Purpose                                                              |
|-----------------|----------------|----------------------------------------------------------------------|
| `/`             | Dashboard      | Product cards with key metrics; doubles as product switcher          |
| `/products/new` | Create Product | New product form (if globally enabled or admin)                      |
| `/user/profile` | User Profile   | Display name, email, password, TOTP setup, theme preference (UC-009) |
| `/user/tokens`  | API Tokens     | List, create, revoke personal access tokens                          |

The **Dashboard** shows all products the user has access to. Each product card displays:

- Product name and description
- Open requirements / active defects count
- Current sprint (if scrum mode)
- User's role in the product

### Product-Scoped (authenticated, uses AppLayout with product context)

#### Product Overview & Settings

| Route                    | Page             | Purpose                                           |
|--------------------------|------------------|---------------------------------------------------|
| `/products/:id`          | Product Overview | Summary dashboard for the selected product        |
| `/products/:id/settings` | Product Settings | Tabbed settings page (see below)                  |
| `/products/:id/team`     | Team Management  | Product roles: list members, invite, change roles |

**Product Settings tabs:**

| Tab            | Content                                                          |
|----------------|------------------------------------------------------------------|
| General        | Name, description, estimation mode, workflow mode (kanban/scrum), defect and document identifier prefixes, 2FA requirement |
| Categories     | Requirement, task and constraint categories: name, identifier prefix, order |
| Statuses       | Requirement and defect status columns (drag to reorder)          |
| Environments   | Runtime environment labels for test matrix                       |
| Pyramid Levels | Test pyramid configuration (hierarchical, max depth 2)           |

#### Backlog

| Route                                  | Page               | Purpose                                                   |
|----------------------------------------|--------------------|-----------------------------------------------------------|
| `/products/:id/requirements`           | Requirements List  | Table view with filters, sort, multi-select (UC-103..105) |
| `/products/:id/requirements/new`       | Create Requirement | Form                                                      |
| `/products/:id/requirements/:rid`      | Requirement Detail | Tabbed detail page (see below)                            |
| `/products/:id/requirements/:rid/edit` | Edit Requirement   | Form                                                      |
| `/products/:id/defects`                | Defects List       | Table view with search, facet filters, sort, pagination (UC-104..107); returns the table fragment for htmx requests |
| `/products/:id/defects/board`          | Defects Board      | Kanban board view                                         |
| `/products/:id/defects/new`            | Create Defect      | Form                                                      |
| `/products/:id/defects/:did`           | Defect Detail      | Tabbed detail page                                        |
| `/products/:id/defects/:did/edit`      | Edit Defect        | Form                                                      |
| `/products/:id/tasks`                  | Tasks List         | Table view; work that is neither requirement nor defect   |
| `/products/:id/tasks/new`              | Create Task        | Form (no definition of done)                              |
| `/products/:id/tasks/:tid`             | Task Detail        | Tabbed detail page (see below)                            |
| `/products/:id/tasks/:tid/edit`        | Edit Task          | Form                                                      |
| `/products/:id/constraints`            | Constraints List   | Table with category filter                                |
| `/products/:id/constraints/new`        | Create Constraint  | Form                                                      |
| `/products/:id/constraints/:cid`       | Constraint Detail  | Tabbed detail page                                        |
| `/products/:id/constraints/:cid/edit`  | Edit Constraint    | Form                                                      |
| `/products/:id/board`                  | Backlog Board      | Kanban board view (only in kanban mode) (UC-101..102)     |
| `/products/:id/sprints`                | Sprints List       | Table (only in scrum mode)                                |
| `/products/:id/sprints/new`            | Create Sprint      | Form                                                      |
| `/products/:id/sprints/:sid`           | Sprint Detail      | Tabbed detail page, board first (see below)               |
| `/products/:id/sprints/:sid/edit`      | Edit Sprint        | Form                                                      |

**Requirement Detail tabs:**

| Tab          | Content                                                                |
|--------------|------------------------------------------------------------------------|
| Overview     | Fields, definition of done, status, assignments, child requirements    |
| Capabilities | Linked capabilities                                                    |
| Test Cases   | Linked test cases (requirement coverage + definition of done coverage) |
| Defects      | Linked defects                                                         |
| Comments    | Comment thread                                                         |
| Attachments | File list + upload                                                     |
| History     | Audit trail                                                            |

**Task Detail tabs:**

Tasks deliberately have no Capabilities, Test Cases or Defects tab: they make no
statement about the product, so there is nothing to trace or acceptance-test.

| Tab         | Content                                                  |
|-------------|----------------------------------------------------------|
| Overview    | Fields, status, assignments, parent, child tasks         |
| Comments    | Comment thread                                           |
| Attachments | File list + upload                                       |
| History     | Audit trail                                              |

**Defect Detail tabs:**

| Tab          | Content                                              |
|--------------|------------------------------------------------------|
| Overview     | Fields, status, assignments, criticality, milestone  |
| Components   | Linked components                                    |
| Capabilities | Linked capabilities                                  |
| Test Cases   | Linked test cases (link existing, or create a regression test) |
| Requirements | Linked requirements                                  |
| Comments     | Comment thread                                       |
| Attachments  | File list + upload                                   |
| History      | Audit trail                                          |

**Constraint Detail tabs:**

| Tab          | Content                                          |
|--------------|--------------------------------------------------|
| Overview     | Fields, category, compliance state, milestone    |
| Capabilities | Linked capabilities                              |
| Comments     | Comment thread                                   |
| Attachments  | File list + upload                               |
| History      | Audit trail                                      |

#### Testing

| Route                                | Page             | Purpose                                                                              |
|--------------------------------------|------------------|--------------------------------------------------------------------------------------|
| `/products/:id/test-cases`           | Test Cases List  | Table with filters (component, capability, pyramid level, execution mode)            |
| `/products/:id/test-cases/new`       | Create Test Case | Form                                                                                 |
| `/products/:id/test-cases/:tid`      | Test Case Detail | Tabbed detail page                                                                   |
| `/products/:id/test-cases/:tid/edit` | Edit Test Case   | Form                                                                                 |
| `/products/:id/campaigns`            | Campaigns List   | Table of testing campaigns with status and progress summary                          |
| `/products/:id/campaigns/new`        | Create Campaign  | Form (name, dates, test case selection with filters and select-all) (UC-206)         |
| `/products/:id/campaigns/:cid`       | Campaign Detail  | Tabbed detail page; execution matrix grouped by component or pyramid level (UC-201..205) |

**Campaign Detail tabs:**

| Tab      | Content                                                                   |
|----------|---------------------------------------------------------------------------|
| Matrix   | Execution matrix (test cases x environments) with grouping toggle (UC-201) |
| Progress | Status breakdown, percentages, progress over time (UC-204, UC-207)        |
| History  | Audit trail                                                               |

Campaigns move through draft > active > closed (FR-320). Scope is editable while draft and locked once
active (FR-321); results are writable only while active (FR-505). Closing is an explicit product owner
action and warns about incomplete results rather than blocking (FR-332, FR-333).

**Test Case Detail tabs:**

| Tab          | Content                                                |
|--------------|--------------------------------------------------------|
| Overview     | Fields, execution mode, test identifier, pyramid level |
| Requirements | Linked requirements and definitions of done            |
| Campaigns    | Campaign history (pass/fail across campaigns)          |
| Comments     | Comment thread                                         |
| Attachments  | File list + upload                                     |
| History      | Audit trail                                            |

#### Structure

| Route                                     | Page                 | Purpose                                          |
|-------------------------------------------|----------------------|--------------------------------------------------|
| `/products/:id/components`                | Components List      | Table                                            |
| `/products/:id/components/new`            | Create Component     | Form                                             |
| `/products/:id/components/:cid`           | Component Detail     | Tabbed detail page                               |
| `/products/:id/components/:cid/edit`      | Edit Component       | Form                                             |
| `/products/:id/capabilities`              | Capabilities List    | Table (grouped by component optionally)          |
| `/products/:id/capabilities/new`          | Create Capability    | Form                                             |
| `/products/:id/capabilities/:fid`         | Capability Detail    | Tabbed detail page                               |
| `/products/:id/capabilities/:fid/edit`    | Edit Capability      | Form                                             |
| `/products/:id/milestones`                | Milestones List      | Table or timeline view                           |
| `/products/:id/milestones/new`            | Create Milestone     | Form                                             |
| `/products/:id/milestones/:mid`           | Milestone Detail     | Tabbed detail page                               |
| `/products/:id/milestones/:mid/edit`      | Edit Milestone       | Form                                             |
| `/products/:id/documents`                 | Documents List       | Table with state filter                          |
| `/products/:id/documents/new`             | Create Document      | Form with a monospace body editor                |
| `/products/:id/documents/:did`            | Document Detail      | Tabbed detail page                               |
| `/products/:id/documents/:did/edit`       | Edit Document        | Form                                             |
| `/products/:id/documents/:did/raw`        | Document content     | The body as a plain-text download, never rendered |

Documents sit in the sidebar's *Definition* group, next to components, capabilities, constraints and
milestones. A document body is user input of any format: Markdown is rendered only through the safe
Markdown renderer (raw HTML escaped, unsafe link schemes dropped), every other format and every diff
line is shown as escaped text, and the raw content is served as a `text/plain` attachment.

**Document Detail tabs:**

| Tab          | Content                                                                  |
|--------------|--------------------------------------------------------------------------|
| Overview     | Format, state, revision, placement, milestone, successor; the content (Markdown rendered, other formats verbatim) and its download |
| Links        | Linked requirements, constraints and test cases; one link modal with a type choice |
| Revisions    | Revision list with restore (owner), and the diff between two revisions    |
| Comments     | Comment thread                                                           |
| Attachments  | File list + upload                                                       |
| History      | Audit trail                                                              |

Requirement, constraint and test case overviews list the documents linked to them.

**Component Detail tabs:**

| Tab          | Content                  |
|--------------|--------------------------|
| Overview     | Fields, linked milestone |
| Capabilities | Child capabilities       |
| Test Cases   | Linked test cases        |
| Comments        | Comment thread           |
| Attachments     | File list + upload       |
| History         | Audit trail              |

**Capability Detail tabs:**

| Tab          | Content                                    |
|--------------|--------------------------------------------|
| Overview     | Fields, parent component, linked milestone |
| Requirements | Linked requirements                        |
| Test Cases   | Linked test cases                          |
| Defects      | Linked defects                             |
| Comments     | Comment thread                             |
| Attachments  | File list + upload                         |
| History      | Audit trail                                |

**Milestone Detail tabs:**

| Tab          | Content                           |
|--------------|-----------------------------------|
| Overview     | Fields, type, due date            |
| Components   | Components assigned to this milestone   |
| Capabilities | Capabilities assigned to this milestone |
| Requirements | Requirements assigned to this milestone |
| Constraints  | Constraints assigned to this milestone  |
| Defects      | Defects assigned to this milestone      |
| Sprints      | Sprints targeting this milestone        |
| Comments     | Comment thread                    |
| Attachments  | File list + upload                |
| History      | Audit trail                       |

Assignment is a single FK on the child, so an item belongs to at most one milestone.
The link picker therefore offers items already assigned elsewhere, labelled with their
current milestone; linking reassigns them.

**Sprint Detail tabs:**

| Tab      | Content                                                                        |
|----------|--------------------------------------------------------------------------------|
| Board    | Kanban view (columns = statuses, cards = this sprint's requirements, tasks and defects) |
| Planning | Two lists — product backlog and sprint contents — with drag to assign           |
| Summary  | Completion stats: item counts by status, progress, estimation totals            |
| History  | Audit trail                                                                    |

The Summary tab reports completion only. Velocity and burndown are **not** implemented:
neither has a data source yet, since nothing records work remaining over time. A burndown
would need per-day snapshots persisted alongside the sprint.

### Administration (admin-only)

| Route                | Page              | Purpose                                                                |
|----------------------|-------------------|------------------------------------------------------------------------|
| `/admin/users`       | Users List        | All users with status, filter by status                                |
| `/admin/users/:uid`  | User Detail       | Account status, roles, actions (lock, reset password, delete)          |
| `/admin/settings`    | Instance Settings | Registration, moderation, product creation toggles (UC-006: auto-save) |
| `/admin/invitations` | Invitations       | List + create new invitations                                          |

### Development (dev mode only)

| Route            | Page                | Purpose                                |
|------------------|---------------------|----------------------------------------|
| `/dev/catalogue` | Component Catalogue | Visual reference for all UI components |

## Decisions

The reasoning behind this structure is recorded in [Web UI decisions](../decisions/web-ui.md).
