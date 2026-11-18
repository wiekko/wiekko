Nexmap holds this product's definition, backlog and QA state: requirements and
tasks, constraints, documents (protocols, schemas and other definitions),
components and capabilities, test cases, testing campaigns and defects.

- Call `get_context` once at the start of a task for a product briefing.
- Before implementing a backlog item, read it with `get_requirement` (e.g.
  `{"ref": "FR-12"}`): description, definition of done, covering test cases,
  linked documents, defects and comments. Read the documents it names with
  `get_document` (e.g. `{"ref": "DOC-3"}`): a current document is the
  definition to implement and check code against; drafts are not binding yet.
- Requirements, tasks and constraints have identifiers like `FR-12` or `AC-42`
  (category prefix + number), defects like `DF-17`, documents like `DOC-3`; use them in commit
  messages and code comments. Test cases are referenced by short ID (first 8
  characters of the ID, shown in lists) or by their test identifier.
  Capabilities are referenced by name.
- To write or refresh a spec document in the repository, use
  `export_specification` and save its text as is; compare its `sha256` with
  the file's to see whether the file is out of date.
- Treat constraints as non-negotiable unless the user says otherwise.
- If a requirement is unclear, read its comments and parent, and ask the user
  rather than inventing scope.
- Write tools (absent when the server runs read-only) are limited by the
  user's product role: set statuses (developer, only on items a product owner has activated), comment (developer; tester on
  defects and test cases), report defects (tester), link test cases and create them from a test run (developer), create tasks and requirements and change their parent, create constraints, components
  and capabilities, update them and link capabilities, write, link and restore documents (owner). There are no tools that delete. A
  role refusal is final; tell the user instead of retrying. Only change what
  the task calls for, check `list_defects` before reporting a defect, and tell
  the user what you changed.
