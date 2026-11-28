/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { Receipt } from "../../../lib/handled";
import { jsonResponse, render, stubApi } from "../../../testing/brief.testkit";
import { meFixture } from "../../../testing/mefixture";
import { marginceLineOf } from "./dealmarginceline";
import { DealNoteUndo } from "./dealnoteundo";

const DEAL = "01a00000-0000-7000-8000-000000000011";
const CLOSING = "01a00000-0000-7000-8000-0000000000c1";
const AUDIT = "01a00000-0000-7000-8000-0000000000a1";
const NOTE = "01a00000-0000-7000-8000-0000000000b1";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function closeReceipt(
  review: Partial<NonNullable<Receipt["review"]>> = {},
): Receipt {
  return {
    id: AUDIT,
    kind: "deal_close_lost",
    summary: "No word from the customer for 196 days",
    occurred_at: "2026-11-20T02:00:00Z",
    subject: { type: "deal", id: DEAL, label: "PIM Rollout" },
    undo: { audit_log_id: AUDIT, version: 7, reversed: false },
    review: {
      kind: "close_lost",
      accepted: false,
      reversed: false,
      version: 7,
      can_undo: true,
      can_accept: true,
      writable: true,
      note_activity_id: NOTE,
      stage_history_id: CLOSING,
      ...review,
    },
  };
}

function line(receipts: Receipt[], closing: string | null = CLOSING) {
  return marginceLineOf({
    deal: { id: DEAL, version: 7, closing_occurrence_id: closing } as never,
    receipts,
    openStages: [],
  });
}

function session() {
  return stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    [`POST /records/deal/${DEAL}/history/${AUDIT}/restore`]: () =>
      jsonResponse({}),
  });
}

it("offers Undo under the header on a deal Margince closed, and presses the restore route Home presses", async () => {
  const calls = session();
  render(line([closeReceipt()]));
  expect(
    await screen.findByText(/Margince closed this deal as lost on/),
  ).toBeTruthy();
  await userEvent.click(await screen.findByRole("button", { name: "Undo" }));
  expect(
    calls.some(
      (call) =>
        call.method === "POST" &&
        call.path === `/records/deal/${DEAL}/history/${AUDIT}/restore`,
    ),
  ).toBe(true);
});

it("offers Reopen and no Undo once the server no longer allows the Undo", async () => {
  session();
  render(
    marginceLineOf({
      deal: { id: DEAL, version: 7, closing_occurrence_id: CLOSING } as never,
      receipts: [closeReceipt({ can_undo: false })],
      openStages: [{ id: "s-1", name: "Discovery" } as never],
    }),
  );
  expect(await screen.findByRole("button", { name: "Reopen" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
});

it("shows no line on a deal closed by hand", () => {
  session();
  expect(line([closeReceipt()], "01a00000-0000-7000-8000-0000000000c2")).toBe(
    undefined,
  );
  expect(line([])).toBe(undefined);
});

it("shows a reader who may not write the line with neither verb", async () => {
  session();
  render(line([closeReceipt({ writable: false, can_undo: false })]));
  expect(
    await screen.findByText(/Margince closed this deal as lost on/),
  ).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
  expect(screen.queryByRole("button", { name: "Reopen" })).toBeNull();
});

function openedReceipt(canUndo: boolean): Receipt {
  return {
    ...closeReceipt({
      kind: "deal_open",
      stage_history_id: null,
      can_undo: canUndo,
    }),
    kind: "deal_open",
  };
}

it("says an opened deal's line only while its Undo is open", async () => {
  session();
  render(line([openedReceipt(true)], null));
  expect(await screen.findByText(/Margince opened this deal on/)).toBeTruthy();
  cleanup();
  expect(line([openedReceipt(false)], null)).toBe(undefined);
});

it("puts Undo on the note that reports the change and on no other note", async () => {
  session();
  render(
    <>
      <DealNoteUndo
        activityId={NOTE}
        dealId={DEAL}
        dealName="PIM Rollout"
        receipts={[closeReceipt()]}
      />
      <DealNoteUndo
        activityId="01a00000-0000-7000-8000-0000000000b2"
        dealId={DEAL}
        dealName="PIM Rollout"
        receipts={[closeReceipt()]}
      />
    </>,
  );
  expect(await screen.findAllByRole("button", { name: "Undo" })).toHaveLength(
    1,
  );
});

it("sends one bundle Undo for a note that reports several changes", async () => {
  const second = "01a00000-0000-7000-8000-0000000000a2";
  const calls = stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    "POST /deal-corrections/undo": () =>
      jsonResponse({ undone: ["deal_amount", "deal_probability"], left: [] }),
  });
  const amount = closeReceipt({ kind: "amount", stage_history_id: null });
  const probability = {
    ...closeReceipt({ kind: "probability", stage_history_id: null }),
    id: second,
    kind: "deal_probability",
    undo: { audit_log_id: second, version: 7, reversed: false },
  } as Receipt;
  render(
    <DealNoteUndo
      activityId={NOTE}
      dealId={DEAL}
      dealName="PIM Rollout"
      receipts={[{ ...amount, kind: "deal_amount" }, probability]}
    />,
  );
  await userEvent.click(
    await screen.findByRole("button", { name: "Undo all" }),
  );
  const press = calls.find((call) => call.path === "/deal-corrections/undo");
  expect(JSON.stringify(press?.body)).toContain(AUDIT);
  expect(JSON.stringify(press?.body)).toContain(second);
});
