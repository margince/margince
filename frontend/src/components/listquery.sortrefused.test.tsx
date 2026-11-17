/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ProblemError } from "../lib/problem";
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
function refusingSort(field: string, code: string) {
  return vi.fn(async (query: ListQuery, _cursor: string | null) => {
    if (query.sort === "bogus" || query.sort === "a,b") {
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

  it("still shows any other refusal with its Retry", async () => {
    globalThis.history.replaceState(null, "", "#/contacts?sort=bogus");
    const fetchPage = refusingSort("owner_id", "invalid_uuid");
    render(<ListTableHarness fetchPage={fetchPage} />);

    await screen.findByText("Could not load this view. Reload the page.");
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy();
  });
});
