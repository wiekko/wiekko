# Page Tree & Navigation

This document defines the UI page tree, navigation structure, and related design decisions.

## Design Principles

1. **Three-layer depth**: global > product-scoped > entity detail. No page is more than three clicks from the dashboard.
2. **Product as context, not destination**: selecting a product sets the sidebar context; everything inside is product-relative.
3. **Tabs over pages for related data**: detail pages use tabs for comments, attachments, linked items, and history rather than separate routes.
4. **URL = state**: filters, sort order, and active tab are reflected in the URL so views are bookmarkable and shareable.
5. **Consistent CRUD pattern**: every entity follows list > detail (tabbed) > create/edit.

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
| General        | Name, description, estimation mode, workflow mode (kanban/scrum) |
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

### Product switcher dropdown with section preservation

This supersedes an earlier decision to switch products via the dashboard only. That decision rested on two premises that did not hold: that a dropdown means loading the product list into every page, and that switching is a low-frequency action. The panel is fetched lazily over htmx on first open (`GET /products/switcher`), so ordinary page renders cost nothing; and for a QC/Jira replacement, someone covering several products switches many times a day.

The switcher **preserves the current section**: from `/products/A/defects`, picking product B lands on `/products/B/defects`, not B's overview. This is the capability the dashboard route cannot offer, and it is what justifies the dropdown. `patterns.SwitchTargetURL` resolves the layout's `ActiveSection` against the target product:

- 11 sections map straight onto `/products/{id}/<section>`
- Detail and form pages set the same `ActiveSection` as their list page, so they degrade to the list — the entity id is meaningless in another product
- `overview`, non-product sections (`dashboard`, `admin-*`) and unrecognised input fall back to the target's overview
- `board`/`sprints` resolve against the **target's** workflow mode, so the two map onto each other: a scrum target lands on `/sprints`, a kanban target on `/board`
- Query parameters are dropped, so `/milestones?view=timeline` resets to the default table view

The section arrives as a query parameter, so the URL is always built from a whitelisted section name rather than from the raw input.

The dashboard remains a product selector, reachable as **All products** in the panel.

### Tabs on detail pages (not separate routes)

Entity detail pages use in-page tabs loaded via htmx (`hx-get` to tab content endpoint, `hx-target` to tab panel). This means:

- Related data is one click away without full page navigation
- The URL updates to reflect the active tab (e.g. `?tab=comments`) for bookmarking
- Tab content loads lazily, keeping initial page load fast
- The sidebar active state doesn't change when switching tabs

### Board views as alternate routes (not tab)

Requirements and defects have both a table view (`/requirements`) and a board view (`/requirements/board`). These are separate routes rather than tabs because:

- They represent fundamentally different interaction patterns
- The sidebar link can indicate which view is active
- Users can bookmark their preferred view

### List state lives in the URL, on the list's own route

Sortable, filterable tables (UC-104..107) keep their whole state in one canonical
parameter set: `sort`, `dir`, `q`, the facet names, and `page`. The rules are:

- Changing the sort or a filter resets to page 1 — the row that was 51st under
  the old order has nothing to do with the row that is 51st under the new one.
- A column header cycles ascending → descending → unsorted. The third step is
  what makes the default order reachable again; a two-state toggle leaves a
  table permanently sorted once it has been sorted at all. Clicking a different
  column always starts it ascending.
- Clearing the sort clears only the sort. A single control clears the search and
  every facet in one step while leaving the sort order alone, since re-sorting
  is not what the user is undoing.
- Changing the page preserves everything else.
- Sorting follows a column's meaning, not its stored value: severities sort by
  severity, and workflow statuses by the column order the product owner
  configured. Sort keys are resolved against a per-entity allowlist, so an
  unknown or stale key renders the default order rather than an error.
- Server-side paging needs a unique tiebreaker in the ORDER BY. Without one,
  rows that tie on the sort key have no defined order between them and paging
  can show the same row twice while never showing another.

Unlike the tab fragments, a list does **not** get a separate fragment route. Its
controls push their request URL into the address bar, so that URL has to be the
page's own; and the search box cannot precompute a page URL to push instead,
because its URL depends on what has been typed. The list route therefore serves
the full page normally and just the table fragment when `HX-Request` is set.

### Theme preference on user profile

The theme toggle in the header provides quick access for in-session switching (persisted to `localStorage`). The user profile page is where the preference is formally configured and persisted to the database, so it follows the user across devices.

### Sprint board as sprint detail

The sprint detail page defaults to the board tab (kanban view of items in that sprint). This avoids a separate top-level "Board" nav entry and makes it clear which sprint's board the user is looking at.

Because it is the default tab, the board is rendered server-side with the page; the other three
tabs load lazily over htmx like every other detail page.

### Sprint lifecycle: planned → active → completed

A sprint carries an explicit `status` rather than deriving its state from its dates. Dates say when
a sprint is *scheduled*; status says whether the team has actually started it, and real sprints slip.
Owners move a sprint with the **Start Sprint** / **Close Sprint** actions on the detail page.

**At most one sprint per product may be active**, mirroring the single-active-campaign rule
(FR-322). Starting a second one fails and names the sprint already running. The dashboard's
"current sprint" prefers the active sprint and only falls back to the date range when none is started.

### One board implementation, three surfaces

The product board, the defects board and the sprint board tab all render `patterns.BoardSurface`
over `templates.BoardSurfaceProps`, built by `buildBoardSurface` in `internal/controller/board_view.go`.
They differ only in what they load: everything, defects only, or the items assigned to one sprint.

Card moves always PATCH the product-level `/products/{id}/board/…/status` endpoints, whichever
board they came from — a status change is the same operation regardless of the surface. The sprint
planning lists reuse the same drag machinery with `data-move-field="sprint_id"`, so `board.js`
stays one implementation.

### Sprint planning requires owner role

Assigning an item to a sprint is a full requirement update, which the service layer restricts to
owners (developers may only change status and activation, FR-407). The Planning tab therefore gates
drag on owner-or-admin, unlike the board, which developers can also use.

### Settings pages use auto-save (UC-006)

Product settings (statuses, environments, pyramid levels) and admin instance settings use auto-save on change. Entity create/edit forms use explicit submit buttons. This distinction is reflected in the form patterns used on each page.
