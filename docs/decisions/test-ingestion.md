# Test ingestion decisions

Decisions behind recording automated test results in testing campaigns (`wiekko utest`,
`POST /products/{p}/test-runs`): what was chosen, and why. How to use it is described in
[CI integration](../cli/ci-integration.md).

## A dedicated upload endpoint

Campaign results could already be set one by one, but that endpoint requires a defect for every failure
(FR-121) and knows nothing about test identifiers. A whole run is therefore uploaded in one request to a
dedicated endpoint, which maps identifiers to automatic test cases on the server. Matching happens in
memory against the product's automatic test cases; manual test cases are never touched.

## Failures create or reuse a defect

FR-121 requires a defect for every failed result. For automated runs, the server creates one per failing
test, titled *Automated test failed: \<identifier\>*, with the run metadata and the test's captured
output (the last 64 KiB, where the assertion usually is). An open defect already recorded on the result is
reused instead. Defects are filed through the same service as manual ones, so they get the default
status, audit entries and validation.

This keeps the FR-123 loop working without anyone in the web UI: closing the defect moves the result to
re-test, and the next green run marks it passed.

## No active campaign: accept and discard

An upload when no campaign is active is accepted and nothing is written: no results, no events, no
defects. CI does not break between campaigns, and the data model stays unchanged (results exist only
inside a campaign, FR-124).

Defects are only created for results that are actually recorded. A failure that is discarded (no active
campaign, unknown identifier, test case outside the campaign) creates none, because an automated defect
exists to explain a campaign result; without one it would be an unlinked duplicate filed on every push.
Such failures are still reported back and printed in the build log.

## Unchanged results are not written again

A result whose status did not change since the last run is skipped, so repeated CI runs do not flood the
result history (FR-48) with identical events.

## No single transaction

The store has no transactions across stores, so results are applied one by one, and every step is
idempotent: unchanged statuses are skipped, and an open defect on the result is reused. A run that fails
half way is completed by sending it again. The accepted cost: two concurrent uploads failing the same new
test could both file a defect.

## Invalid input is rejected, not ignored

An unknown environment name, an explicitly named campaign that is not active, an unknown status (only
`passed`, `failed` and `skipped` exist) or an empty run is answered with an error rather than silently
recording nothing.

## Subtests roll up to their parent

A subtest (`TestX/case`) without a test case of its own counts towards `TestX`: any failing subtest fails
the parent, and the failing subtests' output goes into its defect. Test catalogues therefore do not need
an entry for every table-driven case.

`wiekko test sync` follows the same rule when it creates test cases: an identifier is a subtest only if
its parent is in the same report. A slash alone does not decide it, because package-qualified
identifiers (`example.com/app.TestX`) contain slashes of their own, and Go always reports the parent of
a subtest.
