# Requirements & Constraints

> **Superseded.** These requirements and constraints now live in Nexmap (see `CONTRIBUTING.md`), with new
> identifiers. Code and docs use the Nexmap identifiers; each item in Nexmap names its identifier from
> this file as its legacy ID. This file is no longer maintained.

This document defines both **requirements** (implementable work items) and **constraints** (ongoing rules that are either met or violated). Requirements represent discrete deliverables that move through a workflow. Constraints define boundaries and standards that must be maintained throughout the product lifecycle.

Both requirements and constraints belong to configurable **categories** (e.g. Functionality/FR, Architecture/AC). Categories are defined per product and per entity type, each with a name and prefix used for human-readable identifiers.

## Constraints

### Architecture

#### General

* **AC-001**: Generally, the MVCS pattern applies (model - view - controller - services)
* **AC-002**: Adhere to SOA: Wherever applicable, encapsulate tasks in dedicated services
* **AC-003**: Adhere to DRY: Don't repeat yourself
* **AC-004**: Adhere to SOLID
* **AC-005**: Wherever possible, code shall be covered by unit tests
* **AC-006**: Error values returned by a function shall always be handled, or explicitly discarded.

#### Deployment

* **AC-101**: The server component shall ship as single executable
* **AC-102**: The server component shall be deployed as a container image built from the repository's `Dockerfile` (e.g. on [Coolify](https://coolify.io/))
* **AC-103**: The server component shall adhere to the [twelve factor app methodology](https://12factor.net/).
* **AC-104**: The server component shall support sending transactional emails, configurable via either SMTP relay (host, port, credentials) or a bulk mailer API (e.g. Mailgun, SendGrid, AWS SES).

#### Components

##### Control

* **AC-201**: All Nexmap control & business logic shall live in a central server component.

##### Persistence

* **AC-211**: Nexmap shall use an SQL database backend

##### Interfaces

* **AC-221**: Nexmap shall provide a web interface for human interaction
* **AC-222**: Nexmap shall provide a CLI for command line interaction from the project repository, e.g. for scripting, integration into build systems, or AI agents.
* **AC-223**: Nexmap shall provide a REST API for third-party systems (e.g. web hooks for GitHub / GitLab / Forgejo)
* **AC-224**: The CLI and third-party integrations shall all use the same REST API
* **AC-225**: The Web UI shall use dedicated endpoints, rendering HTMX snippets.

#### Technologies

##### Server

* **AC-401**: The server shall be written in Go 1.26
* **AC-402**: The server shall use the Go standard library web server (`net/http`)

##### Database

* **AC-411**: The server component shall support SQLite for small setups.
* **AC-412**: The server component shall support PostgreSQL for large setups.

##### Web UI

* **AC-421**: The Web UI shall use [Tailwind CSS](https://tailwindcss.com/) for styling
* **AC-422**: The Web UI shall use [templ](https://templ.guide/) for server-side rendering
* **AC-423**: The Web UI shall use [HTMX](https://htmx.org/) for client-side rendering
* **AC-424**: The Web UI shall use [Lucide Go](https://github.com/kaugesaar/lucide-go) icons

### Security

#### Communication

* **SC-001**: All network communication shall be encrypted using TLS (HTTPS)
* **SC-002**: All communication shall be authenticated

#### Authentication

* **SC-101**: The built-in authentication provider shall use username and password stored in the database.
* **SC-102**: The web UI shall use cookie-based sessions with server-side session storage for authentication.
* **SC-103**: Session cookies shall be HttpOnly, Secure and SameSite=Strict.
* **SC-104**: The web UI shall implement CSRF protection for all state-changing requests.
* **SC-105**: The CLI shall use personal access tokens (PAT) to authenticate API requests.
* **SC-106**: Users shall be able to create, list and revoke personal access tokens via the web UI, optionally restricting them (SC-401 to SC-409).
* **SC-107**: Two-Factor-Authentication shall be an option, either available to the user, or enforced. Administrators shall be able to enforce it at instance level for everyone, for administrators, and for users holding a given product role (owner, developer, tester, viewer) in at least one product; users covered must set it up before using the instance. Product owners shall be able to require it for the members of their product; members without it cannot access that product until they set it up.
* **SC-108**: The authentication system shall use a pluggable provider interface to allow future integration of LDAP and OIDC (OpenID Connect) providers (e.g. Google, GitHub, OneLogin).
* **SC-109**: Invitation links (for user registration or product membership) shall be delivered via email.
* **SC-110**: Password reset shall be handled via a time-limited, single-use link delivered by email.
* **SC-111**: Email delivery shall be limited strictly to authentication-related transactional messages (invitations, password resets). General notifications shall not use email.

#### Roles

* **SC-201**: `users` can log in to the Nexmap installation
* **SC-202**: `administrators` have full access to all aspects of the Nexmap installation (superset of all other roles)
* **SC-203**: `product owners` have full access to all aspects of one product (role is bound to a product; superset of developer)
* **SC-204**: `developers` have restricted access to a product (role is bound to a product; superset of tester)
* **SC-205**: `testers` have restricted access to a product, limited to executing tests and reporting defects (role is bound to a product)
* **SC-206**: `viewers` have read-only access to a product (role is bound to a product); intended for stakeholders, managers and other observers

#### Data Security

* **SC-301**: All personal data (as defined by GDPR Art. 4(1), e.g. email addresses, display names) shall be encrypted at application level using AES-256-GCM, stored in a dedicated `personal_data` table with a blind index (HMAC) for equality lookups.
* **SC-302**: The encryption key shall be provided via environment variable (per AC-103). Key rotation shall be supported by storing a key version identifier alongside each ciphertext.
* **SC-303**: Log entries shall never contain personal or other sensitive data
* **SC-304**: Passwords shall be stored as salted hash (bcrypt or pbkdf2) with high computation cost
* **SC-305**: Data subject access (GDPR Art. 15) and right to erasure (GDPR Art. 17) shall be supported by centralising all personal data in the `personal_data` table.
* **SC-306**: Usernames of Nexmap's built-in authentication provider are considered as pseudonyms and thus excluded from application level encryption

#### Access Tokens

* **SC-401**: Personal access tokens can be restricted when they are created. An unrestricted token acts with all rights of its user, within the limits of SC-406.
* **SC-402**: A token can be restricted to a set of products. It shall not reach any other product, and product listings made with it shall show only the products it is restricted to.
* **SC-403**: A token can carry a role cap. On every product it reaches, the token acts with the lower of its user's role and the cap.
* **SC-404**: A token can be restricted to scopes, each granted as `read` or `write` (`write` includes `read`): `structure` (components, capabilities, milestones, sprints), `backlog` (requirements, tasks, constraints), `documents` (documents and their revisions), `defects`, `tests` (test cases, campaigns, results, test runs), `discussion` (comments, attachments) and `settings` (members, categories, statuses, pyramid levels, runtime environments).
* **SC-405**: A request needs the scope of the item it addresses. Side effects that an operation causes by design are covered by that scope: uploading a test run needs `tests: write` even though it files defects (FR-502). Comments and attachments need the `discussion` scope plus `read` on the item they belong to. Linking a document to a requirement, constraint or test case (FR-048) needs `documents: write` plus `read` on the scope of the linked item; restoring a document revision (FR-051) needs `documents: write`.
* **SC-406**: Account and token management and the administration endpoints shall be closed to tokens unless they carry the `account` or `admin` scope. A token can therefore never create a token with more rights than its own. An administrator's token without the `admin` scope acts with the administrator's product roles only.
* **SC-407**: Token restrictions shall be evaluated on every request against the user's current rights, so a token never grants more than its user holds at that moment.
* **SC-408**: The web UI shall offer restriction presets for common purposes, e.g. *CI ingestion* (`tests: write`) and *agent, read-only* (`read` on all product scopes).
* **SC-409**: Administrators shall be able to set a maximum token lifetime for the instance. Tokens shall record when they were last used.

### User Experience

#### General

* **UC-001**: The Web UI shall work on both, desktop and mobile devices
* **UC-002**: Interaction shall be seamless with both, mouse and touch screen
* **UC-003**: Do not restrict the viewport width for views where a wider screen is beneficial
* **UC-004**: On wide screens, the menu shall be placed on the left side
* **UC-005**: Use Tailwind CSS colors to define a consistent color scheme for the UI
* **UC-006**: Settings dialogs shall auto-save changes without requiring a save button. Forms for creating or editing entities (defects, requirements, etc.) shall use explicit submit buttons.
* **UC-007**: Common actions shall be accessible via keyboard shortcuts
* **UC-008**: Keyboard shortcuts shall be discoverable (e.g. via a help overlay)
* **UC-009**: The Web UI shall be available in light and dark mode (users can choose)

#### Backlog Management

* **UC-101**: Backlog shall be displayed on a kanban or sprint board, where columns represent the status
* **UC-102**: Moving an item from one column to another shall update its status
* **UC-103**: Backlog items shall also be manageable in a table format (ordering, selecting as active, assigning to a sprint)
* **UC-104**: All table views shall support dynamic filtering and sorting by any column
* **UC-104a**: Table filtering shall combine a single search box matching across the text columns with per-column facet filters for the enumerable ones. Columns holding numbers or dates shall be sortable but not searchable, since a substring match on them does not mean what the user sees
* **UC-104b**: Sorting shall follow a column's meaning rather than its stored value, so severities sort by severity and workflow statuses by their configured column order
* **UC-104c**: A sortable column shall cycle ascending → descending → unsorted, so the default order stays reachable, and the active search and filters shall be offered a single reset control
* **UC-105**: All table views shall support multi-select for bulk operations
* **UC-106**: Table views shall paginate server-side, with the page, sort order, search and filters all reflected in the URL
* **UC-107**: An empty table shall distinguish "nothing exists yet", which invites creating the first entity, from "nothing matches the current filter", which offers a way to clear it

#### Test Campaigns

* **UC-201**: Test campaigns shall be visualized as a matrix with `test cases` in rows, `runtime environments` in columns, and the result status in the cells
* **UC-202**: Test case status in a campaign shall be visualized using icons and colors
* **UC-203**: The campaign matrix shall be groupable by `component` or by `pyramid level` (user-toggled), using collapsible sections. `test cases` linked to neither a `component` nor a `capability` shall be grouped at product level.
* **UC-204**: Test progress metrics shall be displayed (e.g. percentage passed, failed, pending; progress over time)
* **UC-205**: Users shall be able to group and/or filter `test cases` by their `pyramid level` attribute (if defined)
* **UC-206**: Product owners shall select the `test cases` for a test campaign explicitly, assisted by filters (component, capability, pyramid level, execution mode) and a select-all action over the filtered list
* **UC-207**: Campaign progress over time shall be derived from recorded campaign result events (FR-042)

## Requirements

### Functionality

#### Model

* **FR-001**: The top level model entity shall be the `product`
* **FR-002**: A `product` shall have `components`
* **FR-003**: A `product` shall have `capabilities`
* **FR-004**: `capabilities` can be linked to `components`, or exist directly under the `product` (top level)
* **FR-005**: A `product` shall have `requirements`. All not yet fulfilled `requirements` are the `backlog`
* **FR-005a**: `requirements` can optionally be linked to `capabilities` (many-to-many), establishing traceability from specification to testable feature areas
* **FR-006**: `requirements` shall be grouped into configurable categories (defined per product). Default categories are Functionality, Architecture, Security and UX. Architecture, security and UX rules are modeled as `constraints` (a separate entity).
* **FR-007**: `requirements` shall have a `definition of done`.
* **FR-008**: `requirements` can have parent-child relations, allowing hierarchical decomposition without prescribing specific agile terminology (e.g. epic, story, task)
* **FR-008a**: A `product` shall have `tasks`: backlog items that carry no statement about the product (maintenance, tooling, spikes, technical debt). `tasks` and `requirements` share one table, one status set and one board, distinguished by a `kind` discriminator.
* **FR-008b**: `tasks` shall be grouped into their own configurable categories, separate from the requirement categories. Default task categories are Maintenance (MT), Tooling (TL) and Spike (SP).
* **FR-008c**: A `requirement`'s parent must be a `requirement`. Consequently a `task` may hang off a `requirement` or another `task`, but a `requirement` never hangs off a `task`. Parent links shall be rejected when they cross products or would close a cycle.
* **FR-008d**: `tasks` shall not participate in traceability: they cannot be linked to `capabilities` or `test cases`, they have no `definition of done`, and they shall be excluded from the requirement list and from every requirement picker.
* **FR-008e**: `statuses` shall remain defined for exactly two entity types (`requirement` and `defect`). `tasks` use the `requirement` status set; they shall never get a status set of their own.
* **FR-009**: `requirements`, `tasks` and `defects` can be assigned to one or more users
* **FR-010**: `requirements`, `tasks` and `defects` can optionally have an estimation (e.g. story points or time-based); the estimation mechanism shall be configurable per product
* **FR-011**: A `product` shall have a `roadmap` with `milestones`
* **FR-012**: `milestones` shall have a `type` (e.g. planning, release, internal) to distinguish regular planning checkpoints from shippable releases
* **FR-013**: `components`, `capabilities`, `documents`, `requirements` and `constraints` can be linked to `milestones`
* **FR-014**: A `product` shall have `test cases`
* **FR-015**: `test cases` can be linked to `components`, `capabilities` or exist directly under the `product`
* **FR-016**: `test cases` can be linked to `requirements` or their `definition of done`
* **FR-017**: `test cases` can either be automatic or user-driven tests
* **FR-018**: Automatic `test cases` shall be linked to a specific test identifier in code (e.g. a Go test function name), allowing results to be ingested from CI pipelines or the CLI
* **FR-019**: `test cases` can have a matrix of `runtime environments`
* **FR-020**: `runtime environments` shall be freely definable labels (e.g. OS, browser, third-party software) configurable per product
* **FR-021**: `test cases` have a status (pending, passed, failed, blocked, skipped, re-test) in conjunction with a `testing campaign`, tracked per `runtime environment` where the test case defines one
* **FR-022**: A `product` shall have `defects`
* **FR-023**: A `defect` can be linked to `components`, `capabilities` or a `test case`
* **FR-024**: A `defect` shall have a `criticality` and a `status`
* **FR-025**: A `defect` can be linked to a `milestone`
* **FR-026**: A `defect` can have `comments` and `attachments`
* **FR-027**: `requirements`, `tasks`, `constraints`, `test cases`, `components`, `capabilities`, `documents` and `milestones` can also have `comments` and `attachments`
* **FR-028**: A `sprint` has a start and end date, and can contain `requirements`, `tasks` and `defects`.
* **FR-029**: A `sprint` can be linked to a `milestone` ("leads towards" / "enables")
* **FR-030**: All text fields (descriptions, comments, definitions of done, bodies of `markdown` documents) shall support Markdown formatting
* **FR-031**: All changes to `products`, `components`, `capabilities`, `requirements`, `constraints`, `documents`, `test cases`, `defects`, `milestones` and `sprints` shall be recorded in an audit trail (who changed what, when)
* **FR-032**: `test cases` can have one `pyramid level` (or none if feature isn't used).
* **FR-033**: `pyramid levels` are defined per product (default: "unit", "integration", "system", "acceptance")
* **FR-034**: `pyramid levels` can be hierarchical, i.e. have a parent level (optional, max depth: 2)
* **FR-035**: `pyramid levels` are orthogonal to execution mode (FR-017)
* **FR-036**: A `product` shall have `constraints` as a separate entity from `requirements`. Constraints represent ongoing rules or boundaries (e.g. architecture, security, UX standards) that are either met or violated, rather than implementable work items.
* **FR-037**: `constraints` shall have a `compliance_state` (not assessed, met, violated)
* **FR-038**: `constraints` can be linked to `capabilities` (many-to-many)
* **FR-039**: `constraints` shall support CRUD operations (create, read, update, delete) with the same role-based access as requirements
* **FR-040**: `categories` shall be configurable per product, with separate sets for requirements, tasks and constraints. Each category has a name and a prefix used in human-readable identifiers (e.g. FR-1, MT-18, AC-42). Identifier sequence numbers are drawn from one counter per product, shared across requirements and tasks, so identifiers can never collide.
* **FR-041**: A `testing campaign` is complete when none of its results is pending, blocked or awaiting re-test. Skipped results do not influence the campaign outcome. A campaign passes when every non-skipped result is passed.
* **FR-042**: Every change to a campaign result status shall be recorded as an immutable event capturing the user, timestamp, previous status, new status and any linked `defect`.
* **FR-043**: `defects` shall have a human-readable identifier (e.g. DF-17), with a prefix configurable per product (default DF) and sequence numbers from a per-product counter of their own. The REST API and the CLI shall accept it wherever a defect ID is expected.
* **FR-044**: A `product` shall have `documents`: long-form reference texts that define part of the product and outlive the work that creates or changes them, such as a protocol, a data schema, a file format or an interface contract. A `document` is part of the product's definition, not a backlog item: it has no workflow status, no assignees and no estimation.
* **FR-045**: Nexmap shall be the authoritative source of a `document`. Its content shall be complete in Nexmap, so that reading the product in Nexmap (web UI, REST API, CLI or MCP) is sufficient to implement it or to validate an implementation against it. Copies in repositories are derived from Nexmap (FR-907), never the other way round.
* **FR-046**: A `document` shall have a title, a body and a `format`. The format is either `markdown` (the default, rendered like all other text, FR-030) or a verbatim machine-readable format (e.g. JSON Schema, OpenAPI, Protocol Buffers, SQL, XML Schema, plain text), which is stored, displayed and exported byte for byte, without Markdown processing.
* **FR-047**: A `document` can belong to a `component` or a `capability`, or exist directly under the `product` (top level), like `capabilities` (FR-004).
* **FR-048**: `documents` can be linked to `requirements`, `constraints` and `test cases` (many-to-many). A requirement links the documents it introduces or changes, a constraint the documents the product must conform to, a test case the documents it verifies. `tasks` cannot be linked to documents (FR-008d).
* **FR-049**: `documents` shall have a `state` (draft, current, superseded). Only `current` documents are binding for the product. A superseded document can name the document that replaces it.
* **FR-050**: `documents` shall have a human-readable identifier (e.g. DOC-3), with a prefix configurable per product (default DOC) and sequence numbers from a per-product counter of their own. The prefix shall differ from every category prefix and from the defect prefix. The REST API and the CLI shall accept it wherever a document ID is expected.
* **FR-051**: Every change to a `document`'s title, body or format shall create a new numbered `revision`. Earlier revisions shall remain readable, and any two revisions shall be comparable as a diff. Restoring an earlier revision creates a new revision with its content; history is never rewritten.
* **FR-052**: A `document` can be linked to a `milestone` (FR-013).

#### Administration

* **FR-101**: Administrators shall be able to enable or disable creation of new products
* **FR-102**: Administrators shall be able to enable or disable user self-registration
* **FR-103**: Administrators shall be able to enable or disable moderation for self-registration
* **FR-104**: Administrators shall be able to lock other user accounts
* **FR-105**: Administrators shall be able to remove other user accounts and their data
* **FR-106**: Administrators shall be able to assign a new password to any user account
* **FR-107**: Administrators shall be able to create, view, edit or delete any `product`, `component`, `capability`, `requirement`, `constraint`, `document`, `test case` or `defect`

#### User Accounts

* **FR-201**: If globally enabled, non-users shall be able to register a user account without invitation link.
* **FR-202**: If self-registration is globally disabled, users shall be able to register only with an invitation link issued by a product owner.
* **FR-203**: If moderation is enabled, self-registered accounts will remain pending until approved by an administrator.
* **FR-204**: If moderation is enabled, accounts created from an invitation link shall be exempted from moderation.
* **FR-205**: Users shall be able to request deletion of their account.
* **FR-206**: If moderation is enabled, requests for account deletion shall require approval by an administrator.
* **FR-207**: If moderation is disabled, requests for account deletion shall be processed instantaneously.

#### Product Ownership

* **FR-301**: Users shall be able to create a new product if product creation is globally enabled
* **FR-302**: Administrators shall always be able to create a new product, no matter if creation of new products is enabled or not
* **FR-303**: The user who created a new product automatically becomes its product owner
* **FR-304**: Product owners shall be able to add further users to the product, and assign a role for them
* **FR-305**: Product owners shall be able to invite not-yet existing users (with a token link) to join their product, with the pre-assigned role
* **FR-306**: Product owners shall be able to transfer ownership to another user account. If the account has not yet been created (pending invitation), the transfer shall be pending as well, and only be completed once the new account is fully established.
* **FR-307**: Product owners shall be able to create, update and delete components and capabilities
* **FR-308**: Product owners shall be able to create, update and delete requirements (including definition of done).
* **FR-308a**: Product owners shall be able to create, update and delete tasks. Tasks carry the same authorization as requirements: owners for full edits and activation (FR-318), developers for status only (FR-407).
* **FR-309**: Product owners shall be able to update the product roadmap (create, update, delete milestones).
* **FR-310**: Product owners shall be able to create, update and delete test cases
* **FR-311**: Product owners shall define which conditions re-set a test case (mainly for the webhook API, e.g. a new push to branch main or similar)
* **FR-312**: Product owners shall be able to manually re-set a test case (slate for re-test after a change)
* **FR-313**: Product owners shall be able to link a Git repository to the product
* **FR-314**: Product owners shall be able to link a branch from the repository to a feature or milestone
* **FR-315**: Product owners shall be able to create, update and delete defects.
* **FR-316**: Product owners shall be able to prioritize requirements and defects
* **FR-317**: Product owners shall be able to schedule sprints (optional feature)
* **FR-318**: Product owners shall be able to activate requirements, tasks and defects. In scrum mode, adding an item to a sprint activates it (and taking it out deactivates it); product owners may also activate an item directly. In kanban mode, an item is active exactly while it sits in an active column (FR-323a), so product owners activate it by moving it into one. Only product owners activate items.
* **FR-319**: Product owners shall be able to export the product specification via the CLI (FR-905). Importing a specification is not provided (FR-906).
* **FR-320**: Product owners shall be able to schedule a testing campaign. Campaigns have a status (draft, active, closed) and optional planned start and end dates.
* **FR-321**: When creating a testing campaign, the product owner explicitly selects the `test cases` in scope. The selection is a snapshot taken at creation; test cases created afterwards do not join a running campaign. The scope remains editable while the campaign is draft and is locked once it is active.
* **FR-322**: At most one testing campaign per product may be active at a time
* **FR-323**: Product owners shall be able to define status (columns) for requirements and defects
* **FR-323a**: Product owners shall be able to mark status columns as active (kanban). Items in an active column count as activated for work. New products start with the default (entry) column and the closed columns inactive, and the columns between them active.
* **FR-324**: Product owners can decide whether they want to use sprints (scrum) or a continuous flow (kanban)
* **FR-325**: Product owners shall be able to mark backlog items as coarse (requiring refinement)
* **FR-326**: Product owners shall be able to refine a requirement (break down into child requirements or into child tasks)
* **FR-327**: Product owners shall be able to assign requirements and defects to users
* **FR-328**: Product owners shall be able to configure the estimation mechanism for their product (e.g. story points, t-shirt sizes, hours) or disable it entirely
* **FR-329**: Product owners shall be able to configure the pyramid levels for their product
* **FR-330**: Product owners shall be able to create, update and delete constraints
* **FR-331**: Product owners shall be able to configure categories for requirements, tasks and constraints
* **FR-332**: Closing a testing campaign shall require an explicit action by a product owner. Closing records the acting user in the audit trail and freezes all results against further changes.
* **FR-333**: Closing a testing campaign that still has incomplete results (FR-041) shall require confirmation, but shall not be prevented
* **FR-334**: Product owners shall be able to create, update and delete `documents`, change their `state`, link them to requirements, constraints and test cases, and restore earlier revisions (FR-051).
* **FR-335**: Product owners shall be able to configure the document identifier prefix (FR-050).

#### Development

* **FR-401**: Developers shall be able to add comments to components, capabilities, requirements and documents
* **FR-402**: Developers shall be able to create, update and delete test cases
* **FR-403**: Developers shall be able to define which conditions re-set a test case (mainly for the webhook API)
* **FR-404**: Developers shall be able to manually re-set a test case (slate for re-testing after a change)
* **FR-405**: Developers shall be able to create, update, close and reject defects. Changing a defect's status, which includes closing and rejecting it, requires it to be activated first (FR-407).
* **FR-406**: Developers shall be able to add comments and attachments to defects.
* **FR-407**: Developers shall be able to update the status of defects, requirements and tasks if they have been activated upfront (FR-318). In kanban mode, a developer may move an item out of an active column into any column, including an inactive one, but not out of an inactive column.
* **FR-408**: Developers shall be able to mark backlog items as coarse (requiring refinement)
* **FR-409**: Developers shall be able to ingest unit test results via the CLI (e.g. `go test -v ./... | nexmap utest`) or via CI pipeline webhook
* **FR-409a**: Developers shall be able to create automatic `test cases` from a unit test report via the CLI (`nexmap test sync`), for every test identifier that has no test case yet. Subtests roll up to their parent and get no test case of their own. New test cases can be filed under a `component` or `capability`, given a `pyramid level`, and linked to `requirements`. A dry run shall list the test cases without creating them.

#### Testing

* **FR-501**: Testers shall be able to create defects and link them to a test case, requirement, component or capability
* **FR-502**: Testers shall be able to set the status of any result in an active test campaign. Marking a result as failed requires a `defect`: the tester either links an existing one or creates one inline. The defect is linked to the `test case` (FR-023) and recorded on the result.
* **FR-503**: Testers shall be able to add comments and attachments to defects.
* **FR-504**: When a `defect` recorded on a failed campaign result is closed, that result shall move to re-test. Only an explicit tester action can mark it as passed.
* **FR-505**: Results shall be writable only while their testing campaign is active. Results of a draft or closed campaign are read-only.

#### Notifications

* **FR-601**: Users shall receive in-app notifications ("toast") for relevant events (e.g. assignment, status change, new comment)
* **FR-602**: Users shall be able to configure which events trigger notifications
* **FR-603**: Outgoing webhooks shall be supported for integration with external systems (second priority)

#### Search

* **FR-701**: Full-text search shall be available across requirements, documents, defects, test cases, comments and attachments
* **FR-702**: Search results shall indicate the entity type and link directly to the item

#### Bulk Operations

* **FR-801**: Bulk status changes shall be supported for requirements and defects (e.g. multi-select in table view)
* **FR-802**: Bulk assignment to users, sprints or milestones shall be supported
* **FR-803**: Bulk operations shall respect the same role-based permissions as individual operations

#### Command Line Interface

* **FR-901**: Everything a role can create, update or delete in the web UI shall also be possible via the REST API (AC-224), with the same role-based permissions.
* **FR-902**: The CLI shall support creating, updating and deleting `components` and `capabilities` (FR-307), and linking `requirements`, `constraints` and `test cases` to `capabilities`, `test cases` to `requirements`, and `requirements`, `constraints` and `test cases` to `documents` (FR-005a, FR-016, FR-038, FR-048).
* **FR-903**: The CLI shall support creating and updating `constraints`, including setting their `compliance_state` (FR-037).
* **FR-904**: The CLI's MCP server shall offer the same create, update and link operations as the CLI. Deletions shall not be offered via MCP.
* **FR-905**: The CLI shall export a product's structure (`components`, `capabilities`), `constraints` and `requirements` as a Markdown specification in the style of this document: grouped by category, each item carrying its identifier, in a stable order so that exports committed to a repository diff cleanly. `tasks` shall be included only on request.
* **FR-906**: Importing a specification is out of scope. Users turn their own documents into `requirements` and `constraints` with their own tooling (e.g. an AI agent) via the CLI or REST API. Since exports carry identifiers (FR-905), such tooling can update existing items instead of duplicating them.
* **FR-907**: The specification export (FR-905) shall include `current` documents (and `draft` ones only on request). The specification lists each document with its identifier, title, revision and links; the body of each document is written to a file of its own, named by its identifier and an extension matching its `format` (e.g. `DOC-3.md`, `DOC-4.proto`). Verbatim formats are written byte for byte, so tooling in the repository (code generators, schema validators) can consume the exported file directly. Documents are exported only when the token holds `documents: read`; without it, the export leaves them out instead of failing.
* **FR-908**: The CLI shall support creating, updating and reading `documents`, including setting a document's body from a local file and writing a document or one of its revisions to a local file. Setting a body from a file is an ordinary update (FR-051); it does not make the file the source of the document (FR-045), and no automatic synchronisation from a repository takes place.
* **FR-909**: When the REST API, the CLI or MCP returns a requirement, constraint or test case, it shall name the documents linked to it, so that an agent working on an item can find the definitions that apply to it. A token without `documents: read` receives only the identifiers of the linked documents, not their titles or content.
* **FR-910**: The CLI shall be downloadable as a prebuilt binary for Linux, macOS and Windows on amd64 and arm64, with checksums, and installable by a script per platform that verifies them. Every release shall stay downloadable, so that CI pipelines can pin a version. The CLI shall update itself to the latest or a given release on request, and shall point out a newer release at most once a day, without delaying commands noticeably and never in scripts, CI pipelines or the MCP server.
