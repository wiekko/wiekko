# Testing

Testing in Wiekko has two parts: a catalogue of **test cases** that describes how the product is
verified, and **testing campaigns** that record the results of one round of testing.

## Test cases

**Test Cases** in the sidebar lists the catalogue. Developers and product owners create and edit them.

| Field | Meaning |
|---|---|
| Title, description | What is checked and how; for manual tests, the steps to follow |
| Execution mode | **Manual** (a person carries it out) or **automatic** (a test in code) |
| Test identifier | Automatic tests only: the name that test results report, such as a Go test function name. It connects CI results to the test case |
| Component, capability | What part of the product the test case covers |
| Pyramid level | Unit, Integration, End-to-End, or the product's own levels |
| Environments | The runtime environments the test runs in. A campaign tracks one result per selected environment; with none selected, there is a single result |

On a test case's **Requirements** tab, link the requirements it verifies, either the requirement as a whole
or its definition of done. On the **Constraints** tab, link the constraints it verifies, or verifies in
part, such as a test that checks that session cookies are HttpOnly. The **Campaigns** tab shows how the
test case fared across campaigns.

A constraint's **Test Cases** tab lists the test cases linked to it, with each one's latest result, and
lets developers and product owners link more or create one. Below them, under **Via capabilities**, it
shows the test cases of the constraint's capabilities that are not linked to it. Passing tests are
evidence for a constraint's compliance; the compliance state itself is still set by a product owner.

Automated tests can be registered in bulk from a test run with `wiekko test sync`; see
[CI integration](../cli/ci-integration.md#setting-up).

## Testing campaigns

A campaign moves through three states:

1. **Draft:** a product owner creates it under **Campaigns** with a name, an optional description and
   planned dates, and selects the test cases in scope. Filters for component, capability, pyramid level
   and execution mode, plus a select-all over the filtered list, help to pick them. The scope can still
   change while the campaign is a draft.
2. **Active:** the product owner activates it. The scope is now fixed: test cases created later do not
   join. Results can only be recorded while a campaign is active, and **only one campaign per product can
   be active at a time**.
3. **Closed:** the product owner closes it, which freezes all results. Closing a campaign with results that
   are still pending, blocked or awaiting re-test asks for confirmation first.

## Recording results

The campaign's **Matrix** tab shows test cases in rows and environments in columns, with each result's
status in its cell. Rows can be grouped by component or by pyramid level. Testers, developers and product
owners click a cell to set its result:

| Status | Meaning |
|---|---|
| Pending | Not run yet; every result starts here |
| Passed | The check succeeded |
| Failed | The check failed; requires a defect |
| Blocked | The check could not be carried out, for example because something it depends on is broken |
| Skipped | Deliberately not run in this campaign |
| Re-test | The defect behind a failure was fixed; run the check again |

A small amber dot on a cell marks a result whose status has changed more than once in this campaign.

Marking a result as **failed** always requires a defect that explains it: link an existing one, or create
one on the spot. The defect is linked to the test case automatically.

Automated tests record their results from CI instead; see [CI integration](../cli/ci-integration.md).

## The defect loop

When a defect that explains failed results is moved into a closed status, those results move to
**re-test**. They are not marked as passed automatically: a tester runs the check again and records the
outcome. For automated tests, the next green CI run does this.

## Progress and verdict

The **Progress** tab shows how many results are in which status and how that changed over time; the
**History** tab lists every result change with who made it and when.

A campaign is **complete** when no result is pending, blocked or awaiting re-test. It **passes** when every
result that was not skipped has passed.
