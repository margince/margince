/** @vitest-environment jsdom */

// That every case starts on an empty document.
//
// Testing Library unmounts after a case only when the runner has put
// `afterEach` on the global, and vitest does that only under `globals: true`,
// which this project does not set. So RTL's own registration never armed, and
// the suite ran with every render outliving the case that made it: a tree still
// mounted, its query observers still subscribed, and `notifyManager` still
// flushing renders into it after the file's own teardown. vitest.setup.ts arms
// it now, and this is what fails if that stops.
//
// What makes the guard necessary is that the failure is SILENT. Nothing in
// vitest, RTL or the type system reports a hook that was never registered, and
// a suite whose cases pile up in one document mostly still passes — until a
// page-wide query finds the previous case's copy of what it was looking for.
// So the guard is behavioural rather than a check that some hook exists: it
// renders, and the next case asks what is left behind.
//
// `createElement` rather than JSX so this file is `.ts`, which is what
// tsconfig.node.json's `scripts/**/*.ts` typechecks — a `.tsx` here would
// compile in the suite and be checked by nothing.

import { render, screen } from "@testing-library/react";
import { createElement } from "react";
import { expect, it } from "vitest";

const MARKER = "the case before this one rendered me";

it("renders something that would survive an unarmed teardown", () => {
  render(createElement("div", null, MARKER));

  expect(screen.getByText(MARKER)).toBeTruthy();
});

it("starts on a document the case before it did not leave behind", () => {
  // Asked of the BODY rather than of RTL's own bookkeeping: what breaks a suite
  // is what a page-wide query can still find, and that is the document.
  expect(document.body.textContent).toBe("");
  expect(screen.queryByText(MARKER)).toBeNull();
});
