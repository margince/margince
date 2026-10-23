/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import {
  jsonResponse,
  render,
  stubApi,
  writes,
} from "../../testing/brief.testkit";
import { meFixture } from "../../testing/mefixture";
import {
  automaticDateReceipt,
  automaticStageReceipt,
} from "../../testing/worklist.receiptreview.fixtures";
import { LineReview } from "../sign-in/magic.review";
import { useHandledForYou } from "./worklist.queries";
import { ReceiptReview } from "./worklist.receiptreview";

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
  const { client } = render(<FirstHandledReview />);
  client.setQueryData(["magic"], { needs_you: [] });
  await user.click(await screen.findByRole("button", { name: "Accept" }));
  expect(await screen.findByText(en["brief.changes.accepted"])).toBeTruthy();
  expect(client.getQueryState(["magic"])?.isInvalidated).toBe(true);
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

it("keeps an undo-only applied change visible until it is reversed", async () => {
  const { review, ...change } = automaticDateReceipt;
  stubApi({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    "GET /worklist/handled": () =>
      jsonResponse({
        as_of: change.occurred_at,
        truncated: false,
        receipts: [change],
      }),
    [`GET /deals/${change.subject?.id}`]: () =>
      jsonResponse({ name: "PIM Rollout" }),
  });
  render(<FirstHandledReview />);
  expect(await screen.findByRole("button", { name: "Undo" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Keep date" })).toBeNull();
});

// The review controls as a Done for you line draws them, over the receipt the
// worklist read returned.
function FirstHandledReview() {
  const receipt = useHandledForYou().data?.receipts?.[0];
  return receipt ? <LineReview receipt={receipt} /> : null;
}
