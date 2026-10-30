/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  jsonResponse,
  line,
  receipt,
  renderMagic,
} from "../../testing/magic.testkit";
import { meFixture } from "../../testing/mefixture";
import {
  automaticDateReceipt,
  automaticStageReceipt,
} from "../../testing/worklist.receiptreview.fixtures";

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
      changes: [{ field: "industry", after: "software" }],
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
      changes: [
        {
          field: "industry",
          before: "consumer_goods_retail",
          after: "software",
        },
      ],
      undo: { undoable: false, reason: "already_undone" },
    },
  ],
  page: { has_more: false, total: 2 },
};

function stubOpenedLine(
  records: unknown = RECORDS,
  handled: () => Response = () => jsonResponse({ data: [] }),
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      const path = url.split("?")[0];
      if (path.endsWith(`/magic/lines/${LINE}/records`)) {
        return jsonResponse(records);
      }
      if (path.endsWith("/me")) {
        return jsonResponse(meFixture({ allow: { deal: ["read", "update"] } }));
      }
      if (path.endsWith("/worklist/handled")) {
        return handled();
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
    expect(
      within(dialog).getByText("Consumer Goods and Retail → Software"),
    ).toBeTruthy();
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

  it("never opens a decision line through the audit records read", async () => {
    stubOpenedLine();
    vi.mocked(fetch).mockImplementation(async (input) => {
      const path = String(input instanceof Request ? input.url : input).split(
        "?",
      )[0];
      if (path.endsWith("/magic")) {
        return jsonResponse(
          receipt({
            done: [
              line({
                id: LINE,
                approval_kind: "send_email",
                summary: {
                  key: "magic.action.decided_summary",
                  values: { summary: "Sent it" },
                },
                entity: {
                  type: "deal",
                  id: "00000000-0000-7000-8000-0000000000d1",
                  label: "PIM Rollout",
                },
                undo: undefined,
              }),
            ],
          }),
        );
      }
      return jsonResponse({ data: [] });
    });
    renderMagic();
    const subject = await screen.findByRole("link", { name: "PIM Rollout" });
    expect(subject.getAttribute("href")).toContain(
      "00000000-0000-7000-8000-0000000000d1",
    );
    expect(screen.queryByRole("button", { name: "PIM Rollout" })).toBeNull();
    const asked = vi
      .mocked(fetch)
      .mock.calls.map(([input]) =>
        String(input instanceof Request ? input.url : input),
      );
    expect(asked.some((url) => url.includes("/magic/lines/"))).toBe(false);
  });
});

// One record per receipt fixture, each joined to its receipt by `receipt_id`.
function reviewedRecords() {
  return {
    data: [automaticDateReceipt, automaticStageReceipt].map((change, n) => ({
      audit_id: `00000000-0000-7000-8000-0000000000b${n}`,
      receipt_id: change.id,
      occurred_at: change.occurred_at,
      entity: {
        type: "deal",
        id: `00000000-0000-7000-8000-0000000000d${n}`,
        label: `Deal ${n}`,
      },
      changes: [],
      undo: { undoable: false, reason: "no_before_image" },
    })),
    page: { has_more: false, total: 2 },
  };
}

async function openTheLine() {
  renderMagic();
  await userEvent.click(
    await screen.findByRole("button", { name: "GEM and 1 more" }),
  );
  return screen.findByRole("dialog");
}

describe("a grouped record with a reviewed change", () => {
  it("keeps the review verbs its receipt offers on its own", async () => {
    stubOpenedLine(reviewedRecords(), () =>
      jsonResponse({
        as_of: "2026-09-13T08:00:00Z",
        truncated: false,
        receipts: [automaticDateReceipt, automaticStageReceipt],
      }),
    );
    const dialog = await openTheLine();

    expect(
      await within(dialog).findByRole("button", { name: "Keep date" }),
    ).toBeTruthy();
    expect(within(dialog).getByRole("button", { name: "Accept" })).toBeTruthy();
    expect(
      within(dialog).getAllByRole("button", { name: "Undo" }),
    ).toHaveLength(2);
  });

  it("offers no review verb off a receipts read that failed", async () => {
    stubOpenedLine(reviewedRecords(), () =>
      jsonResponse({ title: "Unavailable" }, 503),
    );
    const dialog = await openTheLine();

    expect(await within(dialog).findByText("Deal 1")).toBeTruthy();
    expect(within(dialog).queryByRole("button", { name: "Accept" })).toBeNull();
    expect(
      within(dialog).queryByRole("button", { name: "Keep date" }),
    ).toBeNull();
  });
});

describe("a long recorded value", () => {
  it("reads in full, so two values with one beginning stay apart", async () => {
    const opening = "Hosting and managed services for retail chains, ".repeat(
      4,
    );
    stubOpenedLine({
      data: [
        {
          ...RECORDS.data[0],
          changes: [
            {
              field: "offer_summary",
              before: `${opening}in Germany.`,
              after: `${opening}in Vietnam.`,
            },
          ],
        },
      ],
      page: { has_more: false, total: 1 },
    });
    const dialog = await openTheLine();

    expect(
      await within(dialog).findByText(
        `${opening}in Germany. → ${opening}in Vietnam.`,
      ),
    ).toBeTruthy();
  });
});
