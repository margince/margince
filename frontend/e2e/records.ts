import type { Screen } from "../src/app/router";

/**
 * Every record page in the product, once, with the id the seed fixture mocks.
 *
 * The record SHELL — the chrome its header draws, the strip its tabs sit in,
 * the column both are measured against — belongs to no single screen, so the
 * specs that measure it sweep all of them. Two specs did, from two hand-kept
 * copies of the same five routes, and a copy is a census that goes stale
 * silently: a sixth record page joins the product and both sweeps keep passing
 * over five.
 *
 * `screen` is what makes this list a census rather than a habit. It is held
 * against `GRIDDED_RECORD_SCREENS` — the shell's own answer to which screens
 * have a record page — by `src/app/recordcensus.test.ts`, which fails when a
 * screen the shell caps has no route here. That check is a vitest test rather
 * than a test in these specs because `app/nav.ts` reaches `app/custom.ts` and
 * its `import.meta.glob`, which only Vite compiles: importing the set here
 * takes down Playwright's transform before a single spec is collected.
 *
 * Pure data, therefore, and no import that reaches a React screen or a
 * stylesheet — the constraint `src/screens/settingscatalog.ts` states for the
 * same reason. The `Screen` import above is type-only and erases.
 */
export type RecordPage = {
  readonly screen: Screen;
  readonly name: string;
  readonly route: string;
};

export const RECORDS: readonly RecordPage[] = [
  { screen: "contacts", name: "contact", route: "/#/contacts/p-anna" },
  { screen: "leads", name: "lead", route: "/#/leads/l-1" },
  { screen: "companies", name: "company", route: "/#/companies/o-brandt" },
  { screen: "deals", name: "deal", route: "/#/deals/d-fleet" },
  { screen: "projects", name: "project", route: "/#/projects/pr-fleet" },
];
