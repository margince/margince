/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { jsonResponse, line, receipt, renderMagic } from "./magic.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const LINE = "00000000-0000-7000-8000-000000000001";

const RECORDS = {
  data: [
    {
      audit_id: "00000000-0000-7000-8000-0000000000a1",
      occurred_at: "2026-09-13T07:30:00Z",
      entity: {
        type: "company",
        id: "00000000-0000-7000-8000-0000000000c1",
        label: "GEM",
      },
      changes: [{ field: "industry", after: "Software" }],
      undo: {
        undoable: true,
        audit_id: "00000000-0000-7000-8000-0000000000a1",
        version: 3,
      },
    },
    {
      audit_id: "00000000-0000-7000-8000-0000000000a2",
      occurred_at: "2026-09-13T07:29:00Z",
      entity: {
        type: "company",
        id: "00000000-0000-7000-8000-0000000000c2",
        label: "Kandu GmbH",
      },
      changes: [{ field: "industry", before: "Retail", after: "Software" }],
      undo: { undoable: false, reason: "already_undone" },
    },
  ],
  page: { has_more: false, total: 2 },
};

function stubOpenedLine() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      const path = url.split("?")[0];
      if (path.endsWith(`/magic/lines/${LINE}/records`)) {
        return jsonResponse(RECORDS);
      }
      if (path.endsWith("/magic")) {
        return jsonResponse(
          receipt({
            done: [
              line({
                summary: {
                  key: "magic.action.fields_changed",
                  values: { fields: "industry" },
                },
                entity: {
                  type: "company",
                  id: "00000000-0000-7000-8000-0000000000c1",
                  label: "GEM",
                },
                count: 2,
                undo: undefined,
              }),
            ],
            totals: {
              done: 1,
              needs_you: 0,
              could_not_complete: 0,
              watching: 0,
            },
          }),
        );
      }
      return jsonResponse({ data: [] });
    }),
  );
}

describe("a done line standing for many records", () => {
  it("opens to each record, what changed on it, and its own undo", async () => {
    stubOpenedLine();
    renderMagic();

    await userEvent.click(
      await screen.findByRole("button", { name: "GEM and 1 more" }),
    );

    const dialog = await screen.findByRole("dialog");
    expect(await within(dialog).findByText("Kandu GmbH")).toBeTruthy();
    expect(within(dialog).getByText("empty → Software")).toBeTruthy();
    expect(within(dialog).getByText("Retail → Software")).toBeTruthy();
    // One press per record: the undoable one offers it, the other says why not.
    expect(
      within(dialog).getAllByRole("button", { name: "Undo" }),
    ).toHaveLength(1);
    expect(
      within(dialog).getByText("This change was already undone."),
    ).toBeTruthy();

    await waitFor(() => {
      const opened = vi
        .mocked(fetch)
        .mock.calls.map(([input]) => String((input as Request).url))
        .find((url) => url.includes(`/magic/lines/${LINE}/records`));
      expect(opened).toContain("since=");
    });
  });
});
