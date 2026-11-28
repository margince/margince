// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { SkipReason } from "./bulkwords";

type BulkVerb = components["schemas"]["BulkVerb"];

// A record a change left as it was is named in the direction the change ran.
// A tag it already has is not a tag it lacks.

afterEach(cleanup);

function reasonOf(verb: BulkVerb, undo = false): string | null {
  render(
    <LocaleProvider initial="en">
      <SkipReason
        skip={{ id: "c-1", reason: "no_change" }}
        verb={verb}
        undo={undo}
      />
    </LocaleProvider>,
  );
  const text = screen.getByText(/./).textContent;
  cleanup();
  return text;
}

it("says which way a tag or Shortlist change found the record already", () => {
  expect(reasonOf("add_tag")).toBe(en["bulk.reason.no_change_tagged"]);
  expect(reasonOf("remove_tag")).toBe(en["bulk.reason.no_change_untagged"]);
  expect(reasonOf("add_to_list")).toBe(en["bulk.reason.no_change_listed"]);
  expect(reasonOf("remove_from_list")).toBe(
    en["bulk.reason.no_change_unlisted"],
  );
  expect(reasonOf("complete")).toBe(en["bulk.reason.no_change_done"]);
});

it("says an undo skips a record the change never reached, whatever its verb", () => {
  for (const verb of ["add_tag", "remove_from_list", "archive"] as const) {
    expect(reasonOf(verb, true)).toBe(en["bulk.reason.no_change_undo"]);
  }
});
