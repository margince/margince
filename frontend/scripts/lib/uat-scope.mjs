// Which changed files the render gate (fe-uat.mjs) holds to "a component has a
// story". It lives here rather than in the gate because the gate is a
// straight-line script — it talks to git and to a browser as it loads — so the
// rule could not be asserted where it was written, and a coverage rule nothing
// tests is one nobody can change safely.

// A test-only module by NAME. `.testkit.` sits beside `.test.`: a testkit holds
// the fixtures and fetch fakes a suite shares, and nothing ships it. Keyed on
// the name rather than on "imported only by tests", which would also excuse a
// real component whose only importer so far is its own test — the exact case
// this gate exists to catch. Naming a shipped component `x.testkit.tsx` to dodge
// the gate would have to be deliberate.
const TEST_ONLY_NAME = /\.(test|testkit|stories)\./;

// A test-only module by LOCATION. A `testing/` directory holds the harnesses a
// suite mounts (src/app/testing/shellharness.tsx renders the shell and exports
// the queries its tests ask it) — the same thing the names above describe, said
// by where the file sits, so a story for one would document nothing a reader
// ships. Only a whole path segment counts, so `src/screens/testingground.tsx`
// is still a component the gate asks for a story.
const TESTING_DIRECTORY = /(?:^|\/)testing\//;

// needsStory answers whether a changed repo-relative path is a shipped
// component — a renderable source file, not a test, testkit, story or harness.
export function needsStory(file) {
  if (!/\.[tj]sx$/.test(file)) return false;
  if (TEST_ONLY_NAME.test(file)) return false;
  return !TESTING_DIRECTORY.test(file);
}
