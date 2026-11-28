/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { act, cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en as englishCopy } from "../i18n/en";
import { ProblemError } from "../lib/problem";
import { replaceDial } from "../lib/urlstate";
import {
  emptyPage,
  ListTableHarness,
  render,
} from "../testing/listquery.testkit";
import type { ListQuery } from "./listquery";

// A bookmark or a link from an older build can name a sort the server no
// longer offers. The list then opens in its own order instead of an error.

afterEach(() => {
  cleanup();
  globalThis.history.replaceState(null, "", "#/");
});

// The shape httperr.Validation writes for a storekit.SortError.
function refusingSort(
  field: string,
  code: string,
  refused: (sort: string) => boolean = (sort) =>
    sort === "bogus" || sort === "a,b",
) {
  return vi.fn(async (query: ListQuery, _cursor: string | null) => {
    if (refused(query.sort)) {
      throw new ProblemError({
        status: 422,
        code: "validation_error",
        detail: "refused",
        details: { errors: [{ field, code, message: "refused" }] },
      });
    }
    return emptyPage();
  });
}

describe("an address whose sort the server refuses", () => {
  it.each([
    ["sort_field_not_allowed", "bogus"],
    ["sort_unsupported", "a,b"],
  ])(
    "drops a %s sort and reads the list in its opening order",
    async (code, sort) => {
      globalThis.history.replaceState(null, "", `#/contacts?sort=${sort}`);
      const fetchPage = refusingSort("sort", code);
      render(<ListTableHarness fetchPage={fetchPage} />);

      await screen.findByRole("cell", { name: "No Contacts yet." });
      expect(fetchPage.mock.calls.at(-1)?.[0].sort).toBe("-created_at");
      await waitFor(() =>
        expect(globalThis.location.hash).toContain("sort=-created_at"),
      );
    },
  );

  // The opening sort's cached read keeps its rows and its refusal, so the
  // refusal does not clear on the way back to it.
  it("asks for no sort once its opening order was refused after it had loaded", async () => {
    globalThis.history.replaceState(null, "", "#/contacts");
    const refused = new Set(["bogus"]);
    const fetchPage = refusingSort("sort", "sort_field_not_allowed", (sort) =>
      refused.has(sort),
    );
    render(<ListTableHarness fetchPage={fetchPage} />);
    await screen.findByRole("cell", { name: "No Contacts yet." });

    refused.add("-created_at");
    act(() => {
      globalThis.dispatchEvent(new Event("visibilitychange"));
    });
    await waitFor(() => expect(fetchPage.mock.calls.at(-1)?.[0].sort).toBe(""));
    act(() => replaceDial("sort", "bogus"));

    await waitFor(() =>
      expect(globalThis.location.hash).not.toContain("bogus"),
    );
    await waitFor(() => expect(fetchPage.mock.calls.at(-1)?.[0].sort).toBe(""));
    expect(
      await screen.findByRole("cell", { name: "No Contacts yet." }),
    ).toBeTruthy();
  });

  // A refused filter is not dropped behind the reader's back: the list says it
  // cannot be shown with these filters and offers to clear them.
  it("still shows any other refusal, with Clear filters", async () => {
    globalThis.history.replaceState(null, "", "#/contacts?sort=bogus");
    const fetchPage = refusingSort("owner_id", "invalid_uuid");
    render(<ListTableHarness fetchPage={fetchPage} />);

    await screen.findByText(englishCopy["table.dialsRefused"]);
    expect(
      screen.getByRole("button", { name: englishCopy["table.clearFilters"] }),
    ).toBeTruthy();
  });
});
