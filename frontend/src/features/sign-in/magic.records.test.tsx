/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import {
  jsonResponse,
  line,
  receipt,
  renderMagic,
} from "../../testing/magic.testkit";
import { meFixture } from "../../testing/mefixture";
import { automaticStageReceipt } from "../../testing/worklist.receiptreview.fixtures";

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

function stubOpenedLine(
  records: unknown = RECORDS,
  receipts: readonly unknown[] = [],
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      const path = url.split("?")[0];
      if (path.endsWith(`/magic/lines/${LINE}/records`)) {
        return jsonResponse(records);
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
      if (path.endsWith("/worklist/handled")) {
        return jsonResponse({
          as_of: "2026-09-13T08:00:00Z",
          truncated: false,
          receipts,
        });
      }
      if (path.endsWith("/me")) {
        return jsonResponse(meFixture({ allow: { deal: ["read", "update"] } }));
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

  it("keeps each grouped deal change's own review and its full values", async () => {
    const shared = "x".repeat(150);
    const [first, second] = [
      { ...automaticStageReceipt, id: "00000000-0000-7000-8000-0000000000e1" },
      { ...automaticStageReceipt, id: "00000000-0000-7000-8000-0000000000e2" },
    ];
    const dealRecord = (receipt: typeof first, suffix: string) => ({
      audit_id: `${receipt.id.slice(0, -2)}f${suffix.length}`,
      receipt_id: receipt.id,
      occurred_at: "2026-09-13T07:30:00Z",
      entity: {
        type: "deal",
        id: receipt.subject?.id ?? "",
        label: `Deal ${suffix}`,
      },
      changes: [
        {
          field: "description",
          before: `${shared}old-${suffix}`,
          after: `${shared}new-${suffix}`,
        },
      ],
      undo: { undoable: false, reason: "not_a_replayable_verb" },
    });
    stubOpenedLine(
      {
        data: [dealRecord(first, "a"), dealRecord(second, "bb")],
        page: { has_more: false, total: 2 },
      },
      [first, second],
    );
    renderMagic();
    await userEvent.click(
      await screen.findByRole("button", { name: "GEM and 1 more" }),
    );
    const dialog = await screen.findByRole("dialog");
    expect(
      await within(dialog).findAllByRole("button", {
        name: en["brief.changes.accept"],
      }),
    ).toHaveLength(2);
    const text = dialog.textContent ?? "";
    for (const suffix of ["old-a", "new-a", "old-bb", "new-bb"]) {
      expect(text).toContain(`${shared}${suffix}`);
    }
    expect(
      within(dialog).getAllByText(en["magic.records.fullValues"]),
    ).toHaveLength(2);
  });
});
