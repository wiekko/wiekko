# Backlog

The backlog holds the product's requirements, tasks and defects, and the boards and sprints you work them
off with. Constraints live next to them. The differences between these are explained in
[Concepts](concepts.md#requirements-tasks-and-constraints).

## Requirements and tasks

**Requirements** and **Tasks** in the sidebar list them separately. Product owners create and edit them.

| Field | Meaning |
|---|---|
| Category | Decides the identifier prefix, such as `FR` or `MT` |
| Title, description | The description is written in Markdown |
| Definition of done | Requirements only: when the requirement counts as fulfilled. Test cases can be linked to it |
| Parent | Breaks work down: a requirement under a requirement, a task under either |
| Status | Position in the workflow; new items get the product's default status |
| Milestone | The milestone this item contributes to |
| Sprint | Scrum mode only: the sprint the item is planned into |
| Priority | A number; lower numbers mean higher priority |
| Estimate | Only when the product uses estimation, in the product's unit |
| Coarse | Marks an item that still needs breaking down |

A requirement's detail page shows its links in tabs: the capabilities it concerns, the test cases that
verify it, and the defects reported against it. Tasks have no such tabs, because they make no statement
about the product to verify.

## Constraints

**Constraints** are rules that always apply, such as "all traffic is encrypted". Instead of a workflow
status, each has a **compliance state**: *not assessed*, *met* or *violated*. Review constraints when
designing a change, and set a constraint to *violated* when the product breaks it, so the problem stays
visible until it is fixed. Constraints can be linked to capabilities and assigned to a milestone.

## Defects

**Defects** records what is broken. Testers, developers and product owners can report defects, testers
usually from a failed test result (see [Testing](testing.md#recording-results)). Editing a defect's fields
is up to product owners; developers change its status (see [Activation](#activation-what-developers-may-move)).

A defect has a **criticality** (low, medium, high, critical), a status, optionally the runtime
environment it occurs in, and links to the components, capabilities, requirements and test cases it
affects. From a defect's **Test Cases** tab you can link an existing test case or create a regression test
for it.

Moving a defect into a closed status (such as *Done*) puts every failed result it explains back to
*re-test* in its testing campaign; see [Testing](testing.md#the-defect-loop).

## Archiving

Finished work piles up: the lists and the board's last column grow with every closed item. Rather than
deleting closed items, which would tear them out of the test cases, campaigns and milestones they belong
to, product owners **archive** them with the *Archive* button on the item's page.

- Only items in a closed status can be archived. A requirement or task is archived together with all
  items below it, so those must be closed too.
- Archived items are hidden from the requirements, tasks and defects lists (switch on *Show archived* to
  see them), from the board and from the items offered when planning a sprint.
- Pages that trace the product state, such as capabilities, components, milestones, sprints and
  campaigns, and the specification export keep showing archived items, marked *Archived*.
- *Unarchive* brings an item back, with the items below it. Moving an archived item to an open status
  does the same, and so does making its status an open one or deleting it.

## Working with lists

All lists work the same way:

- The **search box** matches the text columns; **filters** narrow down columns with a fixed set of
  values, such as status or category. A single control resets search and filters.
- Clicking a column header sorts ascending, then descending, then back to the default order. Statuses sort
  in workflow order and criticalities by severity.
- Search, filters, sorting and the page are part of the URL, so a filtered view can be bookmarked or
  shared.

## Kanban or scrum

The product's workflow mode decides how work is organised.

### Kanban

**Board** shows all requirements, tasks and defects in one column per status. Drag a card to another
column to change its status. If the product's statuses have been split, requirements and defects get
separate boards. **Defects** also has a board of its own. Columns marked *Active* hold the work that is
activated for developers; see [Activation](#activation-what-developers-may-move) for who may move what.

### Scrum

Work is planned into **sprints**, each with a start and end date and optionally a milestone it leads to.
A sprint's detail page has four tabs:

- **Board:** the sprint's items in status columns, as on the kanban board.
- **Planning:** the product backlog next to the sprint's contents; drag items between them. Only product
  owners can plan.
- **Summary:** how many items are in which status, progress, and estimate totals.
- **History:** the sprint's changes.

A sprint is *planned* until its owner chooses **Start Sprint**, and *completed* after **Close Sprint**.
Only one sprint per product can be active at a time. Closing a sprint does not move unfinished items;
plan them into the next sprint.

## Activation: what developers may move

Developers may change the status of a requirement, task or defect only once a product owner has
**activated** it. This keeps the product owner in charge of what gets worked on. Only product owners
activate items, and they can always change statuses.

- In **kanban** mode, activation belongs to the board's columns. Under *Settings → Statuses*, each column
  can be marked **Active**; the board shows those columns with an *Active* badge. An item is active while
  it sits in an active column. A product owner activates an item by moving it into an active column, and
  from there developers can move it to any column, including back to an inactive one such as *Done*.
  Items in inactive columns show a lock to developers and cannot be dragged by them. A new product starts
  with *New* and *Done* inactive and *In Progress* and *In Review* active.
- In **scrum** mode, planning an item into a sprint activates it, and taking it out of the sprint
  deactivates it again. A product owner can also activate an item directly through the API. The column
  flags are not used. Switching a product from kanban to scrum resets activation to the sprint plan.

The detail page shows whether an item is active.
