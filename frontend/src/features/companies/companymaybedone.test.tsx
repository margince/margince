/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { installFetchStub, meRoute } from "../../testing/story-utils";
import { MayBeDoneVerb } from "./companytodayverbs";

// The account card's "Done?" settles the claim it asks about; nothing else.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type Moment = NonNullable<components["schemas"]["Company360"]["moment"]>;

const QUESTION: Moment = {
  claim_key: "moment:account_may_be_done:c-3",
  evidence_fingerprint: "fp-1",
  rule: "open_promise",
  headline: "You may have done this — you wrote to them on 2026-09-13",
  why_now:
    "You owe Lena Kunde: Send the listing details. Promised with no due date.",
  confidence: "medium",
  evidence: [
    { type: "activity", id: "e-0", label: "Send the listing details" },
    { type: "activity", id: "e-1", label: "Re: Listing details" },
  ],
  recommended_action: {
    kind: "complete_task",
    label: "Done",
    state: "available",
  },
  may_be_done: {
    promise_type: "claim",
    promise_id: "c-3",
    email_activity_id: "e-1",
    wrote_at: "2026-09-13T09:00:00Z",
  },
};

function show(moment: Moment) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <MayBeDoneVerb moment={moment} companyId="co-1" />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("the account card asking whether a later reply kept a promise", () => {
  it("settles the claim as done on Done", async () => {
    const settled = vi.fn();
    installFetchStub({
      "GET /me": meRoute({ contact: ["read", "update"] }),
      "POST /claims/c-3/settle": (body) => {
        settled(body);
        return new Response(null, { status: 204 });
      },
    });
    show(QUESTION);
    await userEvent.setup().click(screen.getByRole("button", { name: "Done" }));
    await waitFor(() =>
      expect(settled).toHaveBeenCalledWith({ outcome: "done" }),
    );
  });

  it("offers no Done on a card that asks nothing", () => {
    installFetchStub({ "GET /me": meRoute({ contact: ["read"] }) });
    show({ ...QUESTION, may_be_done: undefined });
    expect(screen.queryByRole("button", { name: "Done" })).toBeNull();
  });
});
