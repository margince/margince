/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
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
