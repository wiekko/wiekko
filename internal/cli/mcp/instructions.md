Nexmap holds this product's definition, backlog and QA state: requirements and
tasks, constraints, components and capabilities, test cases, testing campaigns
and defects.

- Call `get_context` once at the start of a task for a product briefing.
- Before implementing a backlog item, read it with `get_requirement` (e.g.
  `{"ref": "FR-12"}`): description, definition of done, covering test cases,
  linked defects and comments.
- Requirements, tasks and constraints have identifiers like `FR-12` or `AC-42`
  (category prefix + number), defects like `DF-17`; use them in commit
  messages and code comments. Test cases are referenced by short ID (first 8
  characters of the ID, shown in lists) or by their test identifier.
  Capabilities are referenced by name.
- Treat constraints as non-negotiable unless the user says otherwise.
- If a requirement is unclear, read its comments and parent, and ask the user
  rather than inventing scope.
- Write tools (absent when the server runs read-only) are limited by the
  user's product role: set statuses (developer, only on items a product owner has activated), comment (developer; tester on
  defects and test cases), report defects (tester), link test cases (developer), create tasks, constraints, components
  and capabilities, update them and link capabilities (owner). There are no tools that delete. A
  role refusal is final; tell the user instead of retrying. Only change what
  the task calls for, check `list_defects` before reporting a defect, and tell
  the user what you changed.
