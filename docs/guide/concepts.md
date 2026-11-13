# Concepts

nexmap keeps everything about a product in one place: what it is made of, what it must do, the work to
get there, how it is tested and what is broken. This page explains the building blocks and how they
relate.

## Products

Everything in nexmap belongs to a **product**. Each product has its own team, settings, structure,
backlog and tests; nothing is shared between products. The dashboard shows the products you are a member
of, and the product switcher in the header moves between them while staying in the same section.

## Structure: components and capabilities

- **Components** are the parts a product is built from, such as the server, the mobile app or the
  billing module.
- **Capabilities** are what the product can do, such as "export reports" or "sign in with 2FA". A
  capability belongs to a component, or directly to the product if it spans several.

Structure is optional, but it is what everything else hangs off: requirements, constraints, test cases and
defects can all be linked to capabilities, which tells you, per feature area, what is required, how it is
tested and what is broken.

## Requirements, tasks and constraints

These three look alike, but they are different things:

| | Requirement | Task | Constraint |
|---|---|---|---|
| What it is | A statement of what the product must do | Work that says nothing about the product: maintenance, tooling, research | A rule the product must keep following |
| Example | "Users can export a report as PDF" | "Upgrade the PDF library" | "All traffic is encrypted with TLS" |
| Done when | Its definition of done holds | It is finished | Never: it is met or violated, continuously |
| Moves through the workflow | Yes | Yes | No; it has a compliance state instead |
| Linked to capabilities and test cases | Yes | No | Capabilities only |

Each one gets an **identifier** from its **category**: `FR-12` is the 12th item in the product's
numbering, in the category with prefix `FR`. Requirements and tasks share one counter, so `FR-12` and
`MT-12` can never both exist. Use identifiers in conversation, commit messages and code comments.

New products start with these categories, which the product owner can change:

- **Requirements:** Functionality (`FR`), Architecture (`AR`), Security (`SR`), UX (`UR`)
- **Tasks:** Maintenance (`MT`), Tooling (`TL`), Spike (`SP`)
- **Constraints:** Architecture (`AC`), Security (`SC`), UX (`UC`)

Requirements and tasks can be broken down: a requirement can have child requirements or tasks, and a task
can have child tasks. A requirement never sits below a task. Items that still need breaking down can be
marked **coarse**.

Together, the open requirements and tasks are the product's **backlog**. See [Backlog](backlog.md).

## Defects

A **defect** is something that does not work as it should. It has a **criticality** (low, medium, high,
critical), moves through the same kind of workflow as the backlog, and can be linked to the components,
capabilities, requirements and test cases it affects.

Defects are numbered by a counter of their own and get identifiers like `DF-17`. The prefix `DF` can be
changed under **Settings → General**; it must differ from every category prefix.

## Test cases and testing campaigns

- A **test case** describes one check. It is **manual** (a person carries it out) or **automatic** (a test
  in code, identified by its **test identifier**, such as a Go test function name). It can run in several
  **runtime environments**, such as operating systems or browsers, and sit at a level of the **test
  pyramid** (by default Unit, Integration, End-to-End).
- A **testing campaign** is one round of testing: a selection of test cases, each with a result per
  environment. Results start as *pending* and become *passed*, *failed*, *blocked*, *skipped* or *re-test*.
  A failed result always names the defect that explains it.

See [Testing](testing.md).

## Milestones and sprints

- **Milestones** mark points on the product's roadmap. Their type says what they are: *planning*,
  *release* or *internal*. Components, capabilities, requirements, constraints, defects and sprints can
  be assigned to a milestone; an item belongs to at most one.
- **Sprints** are time boxes of work, used when a product works in scrum mode. See
  [Backlog](backlog.md#kanban-or-scrum).

## Comments, attachments and history

Requirements, tasks, constraints, defects, test cases, components, capabilities and milestones all have
comments and attachments, and every change to them is recorded in their **History** tab: who changed what,
and when. Descriptions and comments are written in Markdown.

## Roles

What you can do in a product depends on your role in it. Each role includes everything the roles below it
can do:

| Role | Can |
|---|---|
| **Product owner** | Everything in the product: settings, team, structure, backlog, campaigns |
| **Developer** | Create and edit test cases; comment; change the status of requirements, tasks and defects once they are activated |
| **Tester** | Record campaign results; create defects; comment on defects and test cases |
| **Viewer** | Read everything, change nothing |

**Administrators** manage the whole installation (users and instance settings) and can act in every
product.
