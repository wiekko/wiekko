# Requirements

## Architecture

### General

* **AR-001**: Generally, the MVCS pattern applies (model - view - controller - services)
* **AR-002**: Adhere to SOA: Wherever applicable, encapsulate tasks in dedicated services
* **AR-003**: Adhere to DRY: Don't repeat yourself
* **AR-004**: Adhere to SOLID

### Deployment

* **AR-101**: The server component shall ship as single executable
* **AR-102**: The server component shall be deployed via Docker (e.g. using [Nixpacks](https://nixpacks.com/) on [Coolify](https://coolify.io/))
* **AR-103**: The server component shall adhere to the [twelve factor app methodology](https://12factor.net/).

### Components

#### Control

* **AR-201**: All Nexmap control & business logic shall live in a central server component.

#### Persistence

* **AR-211**: Nexmap shall use an SQL database backend

#### Interfaces

* **AR-221**: Nexmap shall provide a web interface for human interaction
* **AR-222**: Nexmap shall provide a CLI for command line interaction from the project repository, e.g. for scripting, integration into build systems, or AI agents.
* **AR-223**: Nexmap shall provide an API for third-party systems (e.g. web hooks for GitHub / GitLab / Forgejo)

### Protocols

* **AR-301**: The CLI shall communicate with the server component using JSON-RPC 2.0

### Technologies

#### Server

* **AR-401**: The server shall be written in Go 1.24
* **AR-402**: The server shall use the Go standard library web server (`net/http`)

#### Database

* **AR-411**: The server component shall support SQlite for small setups.
* **AR-412**: The server component shall support PostgreSQL for large setups.

#### Web UI

* **AR-421**: The Web UI shall use [Tailwind CSS](https://tailwindcss.com/) for styling
* **AR-422**: The Web UI shall use [templ](https://templ.guide/) for server-side rendering
* **AR-423**: The Web UI shall use [HTMX](https://htmx.org/) for client-side rendering
* **AR-424**: The Web UI shall use [Lucide Go](https://github.com/kaugesaar/lucide-go) icons

## Security

### Communication

* **SR-001**: All network communication shall be encrypted using TLS (HTTPS)
* **SR-002**: All communication shall be authenticated

### Authentication

* **SR-101**: Authentication in general shall be orchestrated with OAuth 2.0
* **SR-102**: Default authentication provider shall be a built-in provider using username and password hash stored in the database.
* **SR-103**: Connection to alternate authentication providers (LDAP, social accounts) shall be possible.
* **SR-104**: Two-Factor-Authentication shall be an option, either available to the user, or enforced at instance level
* **SR-105**: The web ui shall use (short-lived) JWT tokens to authenticate requests
* **SR-106**: The CLI shall use (long-lived) application tokens to authenticate requests

### Roles

* **SR-201**: `users` can log in to the Nexmap installation
* **SR-202**: `administrators` have full access to all aspects of the Nexmap installation
* **SR-203**: `product owners` have full access to all aspects of one product (role is bound to a product)
* **SR-204**: `developers` have restricted access to a product (role is bound to a product)
* **SR-205**: `testers` have restricted access to a product (role is bound to a product)

### Data Security

* **SR-301**: All personal data shall be encrypted at rest (GDPR compliance)
* **SR-302**: Log entries shall never contain personal or other sensitive data
* **SR-303**: Passwords shall be stored as salted hash (bcrypt or pbkdf2) with high computation cost

## Functionality

### Model

* **FR-001**: The top level model entity shall be the `product`
* **FR-002**: A `product` shall have `components`
* **FR-003**: A `product` shall have `functionalities`
* **FR-004**: `functionalities` can be linked to `components`, or exist directly under the `product` (top level)
* **FR-005**: A `product` shall have `requirements`. All not yet fulfilled `requirements` are the `backlog`
* **FR-006**: `requirements` shall be grouped into *architecture*, *security*, *functionality* and *user experience* requirements
* **FR-007**: `requirements` shall have a `definition of done`.
* **FR-008**: A `product` shall have a `roadmap` with `milestones`
* **FR-009**: `components`, `functionalities` and `requirements` can be linked to `milestones`
* **FR-010**: A `product` shall have `test cases`
* **FR-011**: `test cases` can be linked to `components`, `functionalities` or exist directly under the `product`
* **FR-012**: `test cases` can be linked to `requirements` or their `definition of done`
* **FR-013**: `test cases` can either be automatic (unit tests) or user-driven tests
* **FR-014**: `test cases` can have a matrix of `runtime environments`
* **FR-015**: `test cases` have a status (pending, passed, failed) in conjunction with a `testing campaign`
* **FR-016**: A `product` shall have `defects`
* **FR-017**: A `defect` can be linked to `components`, `functionalities` or a `test case`
* **FR-018**: A `defect` shall have a `criticality` and a `status`
* **FR-019**: A `defect` can be linked to a `milestone`
* **FR-020**: A `defect` can have `comments` and `attachments`
* **FR-021**: A `sprint` has a start and end date, and can contain `requirements`, and `defects`.
* **FR-022**: A `sprint` can be linked to a `milestone` ("leads towards" / "enables")

### Administration

* **FR-101**: Administrators shall be able to enable or disable creation of new products
* **FR-102**: Administrators shall be able to enable or disable user self-registration
* **FR-103**: Administrators shall be able to enable or disable moderation for self-registration
* **FR-104**: Administrators shall be able to lock other user accounts
* **FR-105**: Administrators shall be able to remove other user accounts and their data
* **FR-106**: Administrators shall be able to assign a new password to any user account
* **FR-107**: Administrators shall be able to create, view, edit or delete any `product`, `component`, `functionality`, `requirement`, `test case` or `defect`

### User Accounts

* **FR-201**: If globally enabled, non-users shall be able to register a user account without invitation link.
* **FR-202**: If self-registration is globally disabled, users shall be able to register only with an invitation link issued by a product manager.
* **FR-203**: If moderation is enabled, self-registered accounts will remain pending until approved by an administrator.
* **FR-204**: If moderation is enabled, accounts created from an invitation link shall be exempted from moderation.
* **FR-205**: Users shall be able to request deletion of their account.
* **FR-206**: If moderation is enabled, requests for account deletion shall require approval by an administrator.
* **FR-207**: If moderation is disabled, requests for account deletion shall be processed instantaneously.

### Product Ownership

* **FR-301**: Users shall be able to create a new product if product creation is globally enabled
* **FR-302**: Administrators shall always be able to create a new product, no matter if creation of new products is enabled or not
* **FR-303**: The user who created a new product automatically becomes its product owner
* **FR-304**: Product owners shall be able to add further users to the product, and assign a role for them
* **FR-305**: Product owners shall be able to invite not-yet existing users (with a token link) to join their product, with the pre-assigned role
* **FR-306**: Product owners shall be able to transfer ownership to another user account. If the account has not yet been created (pending invitation), the transfer shall be pending as well, and only be completed once the new account is fully established.
* **FR-307**: Product owners shall be able to create, update and delete components and functionalities
* **FR-308**: Product owners shall be able to create, update and delete requirements (including definition of done).
* **FR-309**: Product owners shall be able to update the product roadmap (create, update, delete milestones).
* **FR-310**: Product owners shall be able to create, update and delete test cases
* **FR-311**: Product owners shall define which conditions re-set a test case (mainly for the webhook API, e.g. a new push to branch main or similar)
* **FR-312**: Product owners shall be able to manually re-set a test case (slate for re-test after a change)
* **FR-313**: Product owners shall be able to link a Git repository to the product
* **FR-314**: Product owners shall be able to link a branch from the repository to a feature or milestone
* **FR-315**: Product owners shall be able to create, update and delete defects.
* **FR-316**: Product owners shall be able to prioritize requirements and defects
* **FR-317**: Product owners shall be able to schedule sprints (optional feature)
* **FR-318**: Product owners shall be able to activate requirements and defects by adding them to a sprint, or by activating them directly (kanban approach)
* **FR-319**: Product owners shall be able to import and export requirements via the CLI ("To Do Markdown format")
* **FR-320**: Product owners shall be able to schedule a testing campaign
* **FR-321**: Product owners shall be able to define status (columns) for requirements and defects
* **FR-322**: Product owners can decide whether they want to use sprints (scrum) or a continuous flow (kanban)
* **FR-323**: Product owners shall be able to mark backlog items as coarse (requiring refinement)
* **FR-324**: Product owners shall be able to refine a requirement (break down into several new requirements)

### Development

* **FR-401**: Developers shall be able to add comments to components, functionalities and requirements
* **FR-402**: Developers shall be able to create, update and delete test cases
* **FR-403**: Developers shall be able to define which conditions re-set a test case (mainly for the webhook API)
* **FR-404**: Developers shall be able to manually re-set a test case (slate for re-testing after a change)
* **FR-405**: Developers shall be able to create, update, close and reject defects.
* **FR-406**: Developers shall be able to add comments and attachments to defects.
* **FR-407**: Developers shall be able to update the status of defects and requirements if they have been activated upfront
* **FR-408**: Developers shall be able to mark backlog items as coarse (requiring refinement)

### Testing

* **FR-501**: Testers shall be able to create defects and link them to a test case, requirement, component or functionality
* **FR-502**: Testers shall be able to mark test cases as processed. If no defect is recorded, they count as passed. A recorded (or linked) defect marks them as failed.
* **FR-503**: Testers shall be able to add comments and attachments to defects.

## User Experience

### General

* **UR-001**: The Web UI shall work on both, desktop and mobile devices
* **UR-002**: Interaction shall be seamless with both, mouse and touch screen
* **UR-003**: Do not restrict the viewport width for views where a wider screen is beneficial
* **UR-004**: On wide screens, the menu shall be placed on the left side
* **UR-005**: Use Tailwind CSS colors to define a consistent color scheme for the UI
* **UR-006**: Avoid save buttons where possible or semantically consistent (e.g. settings dialogs); instead, changes shall apply / be saved instantaneously.

### Backlog Management

* **UR-101**: Backlog shall be displayed on a kanban or sprint board, where columns represent the status
* **UR-102**: Moving an item from one column to another shall update its status
* **UR-103**: Backlog items shall also be manageable in a table format (ordering, selecting as active, assigning to a sprint)

### Test Campaigns

* **UR-201**: Test campaigns shall be visualized in tabular form (matrix)
* **UR-202**: Test case status in a campaign shall be visualized using icons and colors
* **UR-203**: Use collapsible sections by component to structure the test campaign status overview
