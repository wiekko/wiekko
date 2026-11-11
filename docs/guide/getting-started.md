# Getting started

This page walks a product owner through setting up a product: creating it, adjusting its settings,
bringing in the team and describing its structure. The terms are explained in [Concepts](concepts.md).

## Create a product

On the dashboard, choose **Create Product** (`/products/new`). Any user can do this if the administrator has
enabled product creation; administrators always can. You become the product's owner.

| Field | Meaning |
|---|---|
| Name, description | Shown on the dashboard and the product overview |
| Estimation mode | How backlog items are estimated: story points, t-shirt sizes, hours, or disabled |
| Workflow mode | **Kanban** (one continuous board) or **scrum** (work planned into sprints); see [Backlog](backlog.md#kanban-or-scrum) |

## Adjust the settings

**Settings** in the product's sidebar has five tabs. Changes are saved as you make them.

- **General:** name, description, estimation mode and workflow mode.
- **Categories:** the categories of requirements, tasks and constraints, each with a name and an
  identifier prefix. The defaults are listed in [Concepts](concepts.md#requirements-tasks-and-constraints).
  Settle the prefixes early, before identifiers like `FR-12` appear in commit messages.
- **Statuses:** the workflow columns. New products start with *New*, *In Progress*, *In Review* and
  *Done*. One status is the default for new items; statuses marked **closed** (like *Done*) count as
  finished, so their items drop out of "open" lists. At first, requirements, tasks and defects share one
  set of statuses. If defects need a different workflow, split the sets; **splitting cannot be undone**.
- **Environments:** the runtime environments tests run in, such as `linux`, `windows` or `firefox`.
  Name them the way your CI jobs will (see [CI integration](../cli/ci-integration.md)).
- **Pyramid levels:** the levels of the test pyramid, by default Unit, Integration and End-to-End.

## Bring in the team

On the **Team** page, search for existing users, add them with a role, and change or remove roles later.
The roles are explained in [Concepts](concepts.md#roles).

> **Not available yet in the web UI:** pages for accepting an invitation or registering an account.
> The **Invite New User** form on the Team page creates an invitation link, but the page it points to does
> not exist yet, and nexmap cannot send emails. Until that is fixed, people need an account before you can
> add them. If the administrator has enabled self-registration, they can create one through the REST API:
>
> ```sh
> curl -X POST https://nexmap.example.com/api/v1/auth/register \
>   -H 'Content-Type: application/json' \
>   -d '{"username": "jdoe", "email": "jdoe@example.com", "display_name": "Jane Doe", "password": "…"}'
> ```
>
> With moderation enabled, an administrator then approves the account under `/admin/users`.

To hand the product over, use **Transfer Ownership** next to a member. They become the owner and you
become a developer.

## Describe the structure

Under **Structure** in the sidebar:

1. Create the **components** the product consists of.
2. Create **capabilities**, each under a component or at product level.
3. Optionally, lay out the roadmap as **milestones**.

The structure can grow over time; nothing else depends on it being complete.

## Fill the backlog and the test catalogue

- Write down the rules that always apply as **constraints**, and what the product must do as
  **requirements**, each with a definition of done. Link both to the capabilities they concern. See
  [Backlog](backlog.md).
- Describe how requirements are verified as **test cases**, linked to the requirements they cover. For
  automated tests, `nexmap test sync` can create the test cases from a test run instead; see
  [CI integration](../cli/ci-integration.md#setting-up).
- When it's time to test, run a **testing campaign**; see [Testing](testing.md).

## Your own account

Under your user menu:

- **Profile** (`/user/profile`): display name, email address, password, theme, two-factor authentication,
  and a request to delete your account.
- **API Tokens** (`/user/tokens`): personal access tokens for the [CLI](../cli/setup.md).
