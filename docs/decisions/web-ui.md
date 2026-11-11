# Web UI decisions

Decisions behind the [UI structure](../spec/ui-structure.md): what was chosen, and why.

## Product switcher dropdown with section preservation

This supersedes an earlier decision to switch products via the dashboard only. That decision rested on two premises that did not hold: that a dropdown means loading the product list into every page, and that switching is a low-frequency action. The panel is fetched lazily over htmx on first open (`GET /products/switcher`), so ordinary page renders cost nothing; and for a QC/Jira replacement, someone covering several products switches many times a day.

The switcher **preserves the current section**: from `/products/A/defects`, picking product B lands on `/products/B/defects`, not B's overview. This is the capability the dashboard route cannot offer, and it is what justifies the dropdown. `patterns.SwitchTargetURL` resolves the layout's `ActiveSection` against the target product:

- 11 sections map straight onto `/products/{id}/<section>`
- Detail and form pages set the same `ActiveSection` as their list page, so they degrade to the list — the entity id is meaningless in another product
- `overview`, non-product sections (`dashboard`, `admin-*`) and unrecognised input fall back to the target's overview
- `board`/`sprints` resolve against the **target's** workflow mode, so the two map onto each other: a scrum target lands on `/sprints`, a kanban target on `/board`
- Query parameters are dropped, so `/milestones?view=timeline` resets to the default table view

The section arrives as a query parameter, so the URL is always built from a whitelisted section name rather than from the raw input.

The dashboard remains a product selector, reachable as **All products** in the panel.

## Tabs on detail pages (not separate routes)

Entity detail pages use in-page tabs loaded via htmx (`hx-get` to tab content endpoint, `hx-target` to tab panel). This means:

- Related data is one click away without full page navigation
- The URL updates to reflect the active tab (e.g. `?tab=comments`) for bookmarking
- Tab content loads lazily, keeping initial page load fast
- The sidebar active state doesn't change when switching tabs

## Board views as alternate routes (not tab)

The backlog and defects have both table views (`/requirements`, `/tasks`, `/defects`) and board views (`/board` for the whole backlog in kanban mode, `/defects/board`). These are separate routes rather than tabs because:

- They represent fundamentally different interaction patterns
- The sidebar link can indicate which view is active
- Users can bookmark their preferred view

## List state lives in the URL, on the list's own route

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

## Theme preference on user profile

The theme toggle in the header provides quick access for in-session switching (persisted to `localStorage`). The user profile page is where the preference is formally configured and persisted to the database, so it follows the user across devices.

## Sprint board as sprint detail

The sprint detail page defaults to the board tab (kanban view of items in that sprint). This avoids a separate top-level "Board" nav entry and makes it clear which sprint's board the user is looking at.

Because it is the default tab, the board is rendered server-side with the page; the other three
tabs load lazily over htmx like every other detail page.

## Sprint lifecycle: planned → active → completed

A sprint carries an explicit `status` rather than deriving its state from its dates. Dates say when
a sprint is *scheduled*; status says whether the team has actually started it, and real sprints slip.
Owners move a sprint with the **Start Sprint** / **Close Sprint** actions on the detail page.

**At most one sprint per product may be active**, mirroring the single-active-campaign rule
(FR-322). Starting a second one fails and names the sprint already running. The dashboard's
"current sprint" prefers the active sprint and only falls back to the date range when none is started.

## One board implementation, three surfaces

The product board, the defects board and the sprint board tab all render `patterns.BoardSurface`
over `templates.BoardSurfaceProps`, built by `buildBoardSurface` in `internal/controller/board_view.go`.
They differ only in what they load: everything, defects only, or the items assigned to one sprint.

Card moves always PATCH the product-level `/products/{id}/board/…/status` endpoints, whichever
board they came from — a status change is the same operation regardless of the surface. The sprint
planning lists reuse the same drag machinery with `data-move-field="sprint_id"`, so `board.js`
stays one implementation.

## Sprint planning requires owner role

Assigning an item to a sprint is a full requirement update, which the service layer restricts to
owners (developers may only change status and activation, FR-407). The Planning tab therefore gates
drag on owner-or-admin, unlike the board, which developers can also use.

## Settings pages use auto-save (UC-006)

Product settings (statuses, environments, pyramid levels) and admin instance settings use auto-save on change. Entity create/edit forms use explicit submit buttons. This distinction is reflected in the form patterns used on each page.
