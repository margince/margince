// The frontend suite's time budget, derived rather than picked.
//
// Six screen suites failed intermittently with `Test timed out in 5000ms` —
// vitest's default `testTimeout`, which nothing overrode. They passed in
// isolation and failed when the machine was busy, so the same commit produced a
// different verdict run to run (#1144).
//
// It is NOT starvation, and it is not vague slowness either. Measured under
// deliberate load (8 spin loops on 10 cores) with the timeout lifted, the two
// tests observed failing completed in 609ms and 910ms, and the slowest test in
// the whole suite took 3437ms. Nothing is stuck, and nothing is close to five
// seconds of work.
//
// What is actually wrong is arithmetic between two budgets that were never
// compared. A test built from N sequential waits may legitimately spend N times
// the per-waiter budget (ASYNC_UTIL_TIMEOUT_MS below) without any single wait
// failing. `company-act.test.tsx`'s "re-arms Continue once a skew refetch
// actually lands a NEW hash" chains six of them. Against a five-second ceiling
// that test can fail while every assertion in it is passing, and the failure
// names the test rather than the wait that was slow.
//
// The repo had already found this once and written it down — in
// src/screens/company-context.test.tsx, which pins its own
// `TEST_MS = SETTLE_MS * 4` with the reason spelled out: "it must cover EVERY
// waiter in there: four run in sequence, each bounded by SETTLE_MS. A test whose
// own limit is smaller than the sum lets vitest fire while a waiter still has
// budget, and what surfaces then is an opaque timeout rather than the assertion
// the test was written to make." That is this defect exactly, fixed for one
// file. #1144 is the same arithmetic everywhere it was not fixed, so the ceiling
// belongs here, once, rather than as a per-file constant six more suites would
// have to remember to copy.
//
// That file keeps its own numbers and is deliberately untouched: it overrides
// the per-waiter budget as well as the per-test one, and the defect it is still
// carrying (issue 613) is starvation rather than arithmetic. Its two starved
// cases are exempted by name in scripts/test-budget.test.ts so they stay
// fast-red.
//
// So the ceiling has to clear the longest chain the suite legitimately composes,
// plus the render and act work sitting between those waits.

/**
 * The per-waiter budget: Testing Library's `asyncUtilTimeout` and
 * `vi.waitFor`'s default timeout, both set from this in vitest.setup.ts.
 *
 * Measured over every waiter that states no timeout (15,625 of them), with the
 * default lifted. The slowest, `App.test.tsx`'s "opens the message a search
 * address names", settled in 971ms under 48 spin loops on 18 cores and in 339ms
 * with no added load. The library default of 1000ms left it 29ms, and a CI
 * runner busier than that has expired it. That load multiplied the waiter
 * 2.86x, so this leaves room for a runner that much busier again:
 * 971 × 971 / 339 = 2782ms. A waiter that is slow by design (a poll, a settle
 * it must outlast) still states its own timeout.
 */
export const ASYNC_UTIL_TIMEOUT_MS = 2_782;

/**
 * The largest budget any test spends waiting, among the tests that run under
 * this ceiling: seven default waiters in sequence, in `import.test.tsx`,
 * `company360.test.tsx` and `ai-provider-sheet.test.tsx`. The two `clickRefresh`
 * cases in `company-context.test.tsx` spend more and are exempted by name in
 * scripts/test-budget.test.ts.
 *
 * Measured from the syntax tree by `scripts/test-budget.ts` rather than counted
 * by hand: a hand count over this tree read one 688-line file as a single test
 * with 39 waiters, and a ceiling built on that would have been ten times
 * anything real.
 *
 * A test that deliberately raises its OWN waiters above the default — a slow
 * settle, a poll it has to outlast — owes its own per-test ceiling and then
 * leaves this population. `read-conclusion.test.tsx`, `onboarding-restore.test.tsx`,
 * `voice-act.test.tsx`, `integrations-provider.test.tsx` and `onboarding.test.tsx`
 * all work that way. The guard in scripts/test-budget.test.ts is what keeps the
 * two populations honest: it holds EVERY test's waiter budget against the
 * ceiling that test actually runs under, so a suite cannot quietly join this one
 * while spending like the other.
 */
export const MAX_DEFAULT_WAITER_BUDGET_MS = ASYNC_UTIL_TIMEOUT_MS * 7;

/**
 * The slowest single test measured under deliberate load, in milliseconds —
 * `read-conclusion.test.tsx`'s "a long multi-snapshot run converges on the
 * review". Used whole as the allowance for the work BETWEEN the waits, which
 * deliberately over-counts: that figure already contains its own waits. An
 * over-estimate is the safe direction here, since a ceiling that is too high
 * costs a slower red and one too low costs this bug.
 */
export const SLOWEST_MEASURED_TEST_MS = 3_437;

/**
 * The per-test ceiling. Not a round number on purpose: a round one silently
 * disagrees with the bound it has to outlast, and there is no reading of 10s or
 * 15s that explains itself. This one is the sum of the two measurements above,
 * so it moves when they do.
 */
export const TEST_TIMEOUT_MS =
  MAX_DEFAULT_WAITER_BUDGET_MS + SLOWEST_MEASURED_TEST_MS;
