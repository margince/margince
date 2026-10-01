/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { formatDayMonth, formatTimeOfDay } from "../format/format";
import { viewerZone } from "../format/timezone";
import { BriefChanges } from "./brief.changes";
import { jsonResponse, render, stubApi, writes } from "./brief.testkit";
import { ReceiptReview } from "./worklist.receiptreview";
import { automaticStageReceipt } from "./worklist.receiptreview.fixtures";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("accepts the exact recorded stage change and shows its persisted answer", async () => {
  let accepted = false;
  const change = automaticStageReceipt;
  const path = `/deals/${change.subject?.id}/applied-changes/${change.id}/accept`;
  const calls = stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    "GET /worklist/handled": () =>
      jsonResponse({
        as_of: change.occurred_at,
        truncated: false,
        receipts: [{ ...change, review: { ...change.review, accepted } }],
      }),
    [`GET /deals/${change.subject?.id}`]: () =>
      jsonResponse({ name: "PIM Rollout" }),
    [`POST ${path}`]: () => {
      accepted = true;
      return new Response(null, { status: 204 });
    },
  });
  const user = userEvent.setup();
  render(<BriefChanges />);
  await user.click(await screen.findByRole("button", { name: "Accept" }));
  await screen.findByText("Accepted");
  expect(writes(calls).map((call) => call.path)).toEqual([path]);
  expect(screen.queryByRole("button", { name: "Accept" })).toBeNull();
});

it("draws no group at all when no change waits for a word", async () => {
  stubApi({
    "GET /worklist/handled": () =>
      jsonResponse({
        as_of: "2026-09-13T08:00:00Z",
        truncated: false,
        receipts: [],
      }),
  });
  const { container, client } = render(<BriefChanges />);
  // It draws nothing while loading too, so the empty answer has to have come
  // back before "nothing" says anything about it.
  await waitFor(() =>
    expect(
      client
        .getQueryCache()
        .getAll()
        .map((query) => query.state.status),
    ).toEqual(["success"]),
  );
  // The receipt's summary already answers for a quiet day, so an empty group
  // would be a heading over nothing.
  expect(container.innerHTML).toBe("");
  expect(
    screen.queryByRole("heading", { name: "Changes made for you" }),
  ).toBeNull();
});

it("dates a change from today by its hour and an older one by its day", async () => {
  const today = automaticStageReceipt;
  const older = {
    ...automaticStageReceipt,
    id: "01a00000-0000-7000-8000-000000000013",
    occurred_at: "2026-09-09T14:00:00Z",
  };
  stubApi({
    "GET /worklist/handled": () =>
      jsonResponse({
        as_of: today.occurred_at,
        truncated: false,
        receipts: [today, older],
      }),
    [`GET /deals/${today.subject?.id}`]: () =>
      jsonResponse({ name: "PIM Rollout" }),
  });
  const { container } = render(<BriefChanges />);
  await screen.findByRole("list", { name: "Changes made for you" });
  const zone = viewerZone();
  expect(
    [...container.querySelectorAll("time")].map((time) => time.textContent),
  ).toEqual([
    formatTimeOfDay(today.occurred_at, "en", zone),
    formatDayMonth(older.occurred_at, "en", zone),
  ]);
});

it("uses the stage reversal route rather than restoring a stage field", async () => {
  const change = automaticStageReceipt;
  const path = `/deals/${change.subject?.id}/stage-progressions/${change.id}/revert`;
  const calls = stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    [`POST ${path}`]: () => jsonResponse({}),
  });
  const user = userEvent.setup();
  render(<ReceiptReview receipt={change} />);
  await user.click(await screen.findByRole("button", { name: "Undo" }));
  await waitFor(() =>
    expect(writes(calls).map((call) => call.path)).toEqual([path]),
  );
});

it("offers no write for a superseded change", () => {
  stubApi({});
  render(
    <ReceiptReview
      receipt={{
        ...automaticStageReceipt,
        review: {
          kind: "stage",
          accepted: false,
          reversed: false,
          version: 8,
          can_accept: false,
          writable: true,
          can_undo: false,
        },
      }}
    />,
  );
  expect(screen.queryByRole("button")).toBeNull();
});
