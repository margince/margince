// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, describe, expect, it, type MockInstance, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { MagicUndoButton } from "./magic.undo";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function answerRestoreWith(status: number, body: unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify(body), {
          status,
          headers: {
            "content-type":
              status >= 400 ? "application/problem+json" : "application/json",
          },
        }),
    ),
  );
}

function pressUndo(client: QueryClient) {
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider>
        <MagicUndoButton
          undo={{ undoable: true, audit_id: "a1", version: 3 }}
          entityType="company"
          entityId="c1"
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  fireEvent.click(
    screen.getByRole("button", { name: en["magic.undo.action"] }),
  );
}

function invalidatedKeys(
  spy: MockInstance<QueryClient["invalidateQueries"]>,
): string[] {
  return spy.mock.calls.map((call) => JSON.stringify(call[0]?.queryKey));
}

describe("one refresh after an Undo from a receipt", () => {
  it("says why a refused Undo was refused and reads the receipt and the record again", async () => {
    answerRestoreWith(409, {
      title: "Conflict",
      status: 409,
      code: "later_company_change",
    });
    const client = new QueryClient();
    const invalidate = vi.spyOn(client, "invalidateQueries");
    pressUndo(client);

    expect(
      await screen.findByText(en["history.undo.laterCompanyChange"]),
    ).toBeTruthy();
    await act(async () => {});
    const keys = invalidatedKeys(invalidate);
    expect(keys).toContain(JSON.stringify(["magic"]));
    expect(keys).toContain(JSON.stringify(["worklist"]));
    expect(keys).toContain(JSON.stringify(["company", "c1"]));
    expect(keys).toContain(JSON.stringify(["record-history", "company", "c1"]));
  });

  it("refreshes the same reads after an Undo that landed", async () => {
    answerRestoreWith(200, { id: "r1" });
    const client = new QueryClient();
    const invalidate = vi.spyOn(client, "invalidateQueries");
    pressUndo(client);

    expect(await screen.findByRole("status")).toBeTruthy();
    await act(async () => {});
    const keys = invalidatedKeys(invalidate);
    expect(keys).toContain(JSON.stringify(["magic"]));
    expect(keys).toContain(JSON.stringify(["company", "c1"]));
  });
});
