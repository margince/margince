/** @vitest-environment happy-dom */

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ReactNode, useState } from "react";
import { flushSync } from "react-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../../api/client";
import {
  ACTION_TOAST_MS,
  ToastProvider,
  ToastRegion,
} from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import type { Approval } from "../../lib/approvals.queries";
import { magicKey, worklistKey } from "../../lib/workkeys";
import {
  isPreviewDoor,
  refusedPreview,
} from "../../testing/sendpermission.testkit";
import { steppedClock } from "../../testing/steppedclock";
import { ApprovalRow } from "./approvalrow";

// What a reader sees after approving, and where the button goes.
//
// The toast is the only signpost to a reversal that already exists, so what
// matters is that it APPEARS and that pressing it lands on the record whose
// history holds the change.
//
// The region is the application's, mounted once in `main.tsx`, so this harness
// mounts it the same way. The row used to render its own when no surface handed
// one down, which is what let a row shown on its own say anything at all; with
// one region for the whole app that fallback — and the prop that fed it — is
// gone, and a suite that renders the row alone has to supply the region the
// shell would.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function render(
  ui: ReactNode,
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  }),
) {
  return rtlRender(ui, {
    wrapper: ({ children }) => (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <ToastProvider>
            {children}
            <ToastRegion />
          </ToastProvider>
        </LocaleProvider>
      </QueryClientProvider>
    ),
  });
}

// How many times the row posted a decision, by verb. The negative tests wait
// on these: an absence asserted before the request went out is an absence that
// survives the interaction being deleted.
function decisionCalls(fetched: ReturnType<typeof vi.fn>, verb: string) {
  // The client hands fetch a Request, not a url string, so the path is read
  // off the object rather than by stringifying the argument.
  return fetched.mock.calls.filter(([input]) =>
    input instanceof Request
      ? new URL(input.url).pathname.endsWith(verb)
      : String(input).endsWith(verb),
  ).length;
}
const approveCalls = (f: ReturnType<typeof vi.fn>) =>
  decisionCalls(f, "/approve");
const rejectCalls = (f: ReturnType<typeof vi.fn>) =>
  decisionCalls(f, "/reject");

function closeDateApproval(overrides: Partial<Approval> = {}): Approval {
  return {
    id: "ap1",
    kind: "close_date_correction",
    status: "pending",
    summary: "Confirm the real close date",
    proposed_by: "system:close-date",
    proposed_change: {
      deal_id: "d1",
      expected_close_date: "2026-11-15",
      basis: "Nobody has answered since 5 August.",
    },
    created_at: "2026-08-20T09:00:00Z",
    target_entity_type: "deal",
    target_entity_id: "d1",
    ...overrides,
  } as Approval;
}

beforeEach(() => {
  localStorage.setItem("margince.workspaceSlug", "acme");
  globalThis.location.hash = "";
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

const HOLD_TIMERS = [
  "setTimeout",
  "clearTimeout",
  "setInterval",
  "clearInterval",
  "Date",
] as const;

function stubDecisions(answer = () => jsonResponse({ status: "rejected" })) {
  const fetched = vi.fn(async (_input: RequestInfo | URL) => answer());
  vi.stubGlobal("fetch", fetched);
  return fetched;
}

const REJECTED = en["decision.status.rejected"];
const DETAIL = "This proposal can no longer be rejected.";
const UNDO = en["common.undo"];

function refused(detail: string) {
  return jsonResponse({ title: "Refused", status: 422, detail }, 422);
}

function TwoRows({ second }: Readonly<{ second: Approval }>) {
  return (
    <>
      <ApprovalRow approval={closeDateApproval()} />
      <ApprovalRow approval={second} />
    </>
  );
}

describe("a rejection held for its undo", () => {
  it("sends once the window closes and keeps the bar, focused, until the card is decided", async () => {
    const user = steppedClock({ toFake: [...HOLD_TIMERS] });
    const fetched = stubDecisions();
    render(<ApprovalRow approval={closeDateApproval()} />);

    await user.click(screen.getByRole("button", { name: "Reject" }));
    expect(screen.getByText(REJECTED)).toHaveFocus();
    await vi.advanceTimersByTimeAsync(ACTION_TOAST_MS - 1);
    expect(rejectCalls(fetched)).toBe(0);

    await vi.advanceTimersByTimeAsync(1);
    await vi.waitFor(() => expect(rejectCalls(fetched)).toBe(1));
    await vi.advanceTimersByTimeAsync(0);
    expect(screen.getByText(REJECTED)).toHaveFocus();
    expect(screen.queryByRole("button", { name: UNDO })).toBeNull();
    expect(screen.queryByRole("button", { name: "Reject" })).toBeNull();
  });

  it("puts the verbs back and sends nothing when undone", async () => {
    const user = steppedClock({ toFake: [...HOLD_TIMERS] });
    const fetched = stubDecisions();
    render(<ApprovalRow approval={closeDateApproval()} />);

    await user.click(screen.getByRole("button", { name: "Reject" }));
    await user.click(screen.getByRole("button", { name: UNDO }));

    expect(screen.getByRole("button", { name: "Reject" })).toHaveFocus();
    expect(screen.getByRole("button", { name: "Approve" })).toBeEnabled();
    await vi.advanceTimersByTimeAsync(ACTION_TOAST_MS * 2);
    expect(rejectCalls(fetched)).toBe(0);
  });

  it("waits while focus is on Undo and sends the rest of the window after it leaves", async () => {
    const user = steppedClock({ toFake: [...HOLD_TIMERS] });
    const fetched = stubDecisions();
    render(<ApprovalRow approval={closeDateApproval()} />);

    await user.click(screen.getByRole("button", { name: "Reject" }));
    await vi.advanceTimersByTimeAsync(3000);
    await user.tab();
    expect(screen.getByRole("button", { name: UNDO })).toHaveFocus();
    await vi.advanceTimersByTimeAsync(ACTION_TOAST_MS * 2);
    expect(rejectCalls(fetched)).toBe(0);

    await user.tab();
    await vi.advanceTimersByTimeAsync(ACTION_TOAST_MS - 3000 - 1);
    expect(rejectCalls(fetched)).toBe(0);
    await vi.advanceTimersByTimeAsync(1);
    await vi.waitFor(() => expect(rejectCalls(fetched)).toBe(1));
  });

  it("waits while the pointer is on Undo and sends the rest after it leaves", async () => {
    const user = steppedClock({ toFake: [...HOLD_TIMERS] });
    const fetched = stubDecisions();
    render(<ApprovalRow approval={closeDateApproval()} />);

    await user.click(screen.getByRole("button", { name: "Reject" }));
    const undo = screen.getByRole("button", { name: UNDO });
    await user.hover(undo);
    await vi.advanceTimersByTimeAsync(ACTION_TOAST_MS * 2);
    expect(rejectCalls(fetched)).toBe(0);

    await user.unhover(undo);
    await vi.advanceTimersByTimeAsync(ACTION_TOAST_MS - 1);
    expect(rejectCalls(fetched)).toBe(0);
    await vi.advanceTimersByTimeAsync(1);
    await vi.waitFor(() => expect(rejectCalls(fetched)).toBe(1));
  });

  it("sends at once when the card leaves the page", async () => {
    const user = steppedClock({ toFake: [...HOLD_TIMERS] });
    const fetched = stubDecisions();
    const { unmount } = render(<ApprovalRow approval={closeDateApproval()} />);

    await user.click(screen.getByRole("button", { name: "Reject" }));
    unmount();

    await vi.waitFor(() => expect(rejectCalls(fetched)).toBe(1));
  });

  it("sends at once when the page is hidden", async () => {
    const user = steppedClock({ toFake: [...HOLD_TIMERS] });
    const fetched = stubDecisions();
    render(<ApprovalRow approval={closeDateApproval()} />);

    await user.click(screen.getByRole("button", { name: "Reject" }));
    globalThis.dispatchEvent(new Event("pagehide"));

    await vi.waitFor(() => expect(rejectCalls(fetched)).toBe(1));
  });

  it("sends at once when the tab is hidden", async () => {
    const user = steppedClock({ toFake: [...HOLD_TIMERS] });
    const fetched = stubDecisions();
    render(<ApprovalRow approval={closeDateApproval()} />);
    await user.click(screen.getByRole("button", { name: "Reject" }));

    const shown = vi.spyOn(document, "visibilityState", "get");
    shown.mockReturnValue("visible");
    document.dispatchEvent(new Event("visibilitychange"));
    await vi.advanceTimersByTimeAsync(0);
    expect(rejectCalls(fetched)).toBe(0);
    shown.mockReturnValue("hidden");
    document.dispatchEvent(new Event("visibilitychange"));

    await vi.waitFor(() => expect(rejectCalls(fetched)).toBe(1));
    shown.mockRestore();
  });

  it.each([
    ["approving", "Approve"],
    ["rejecting", "Reject"],
  ])(
    "sends the held rejection at once when another card is answered by %s",
    async (_how, verb) => {
      const user = steppedClock({ toFake: [...HOLD_TIMERS] });
      const fetched = stubDecisions();
      render(
        <TwoRows
          second={closeDateApproval({ id: "ap2", summary: "Second" })}
        />,
      );

      const [first] = screen.getAllByRole("button", { name: "Reject" });
      if (!first) throw new Error("the first row drew no Reject");
      await user.click(first);
      await user.click(screen.getByRole("button", { name: verb }));

      await vi.waitFor(() =>
        expect(decisionCalls(fetched, "/ap1/reject")).toBe(1),
      );
      expect(decisionCalls(fetched, "/ap2/reject")).toBe(0);
    },
  );

  it("sends the held rejection at once when another card is approved after an edit", async () => {
    const user = steppedClock({ toFake: [...HOLD_TIMERS] });
    const fetched = stubDecisions();
    render(
      <TwoRows
        second={closeDateApproval({
          id: "ap2",
          kind: "advance_deal",
          summary: "Move Harbor to lost",
          proposed_change: { lost_reason: "Old reason" },
        })}
      />,
    );

    const [first] = screen.getAllByRole("button", { name: "Reject" });
    if (!first) throw new Error("the first row drew no Reject");
    await user.click(first);
    await user.click(screen.getByRole("button", { name: en["trust.edit"] }));
    await user.click(
      screen.getByRole("button", { name: en["decision.approveEdited"] }),
    );

    await vi.waitFor(() => {
      expect(decisionCalls(fetched, "/ap1/reject")).toBe(1);
      expect(decisionCalls(fetched, "/ap2/approve")).toBe(1);
    });
  });

  it("asks the browser to finish the request even if the page goes", async () => {
    const user = steppedClock({ toFake: [...HOLD_TIMERS] });
    stubDecisions();
    const post = vi.spyOn(api, "POST");
    render(<ApprovalRow approval={closeDateApproval()} />);

    await user.click(screen.getByRole("button", { name: "Reject" }));
    await vi.advanceTimersByTimeAsync(ACTION_TOAST_MS);

    expect(post).toHaveBeenCalledWith(
      "/approvals/{id}/reject",
      expect.objectContaining({ keepalive: true }),
    );
    post.mockRestore();
  });

  it("sends nothing once the card turns decided elsewhere", async () => {
    const user = steppedClock({ toFake: [...HOLD_TIMERS] });
    const fetched = stubDecisions();
    function Deciding() {
      const [decided, setDecided] = useState(false);
      return (
        <>
          <ApprovalRow approval={closeDateApproval()} decided={decided} />
          <button type="button" onClick={() => setDecided(true)}>
            Decided elsewhere
          </button>
        </>
      );
    }
    render(<Deciding />);
    await user.click(screen.getByRole("button", { name: "Reject" }));
    await user.click(screen.getByRole("button", { name: "Decided elsewhere" }));
    await vi.advanceTimersByTimeAsync(ACTION_TOAST_MS * 2);
    cleanup();
    await vi.advanceTimersByTimeAsync(0);

    expect(rejectCalls(fetched)).toBe(0);
  });

  it("keeps the bar on a card that runs out inside the window, and still sends", async () => {
    const user = steppedClock({ toFake: [...HOLD_TIMERS] });
    const fetched = stubDecisions();
    render(
      <ApprovalRow
        approval={closeDateApproval({
          expires_at: new Date(Date.now() + 3000).toISOString(),
        })}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Reject" }));
    await vi.advanceTimersByTimeAsync(5000);
    expect(screen.getByText(REJECTED)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: UNDO })).toBeInTheDocument();

    await vi.advanceTimersByTimeAsync(ACTION_TOAST_MS - 5000);
    await vi.waitFor(() => expect(rejectCalls(fetched)).toBe(1));
  });

  it("brings the verbs back with the refusal, focus on Reject", async () => {
    const user = steppedClock({ toFake: [...HOLD_TIMERS] });
    stubDecisions(() => refused(DETAIL));
    render(<ApprovalRow approval={closeDateApproval()} />);

    await user.click(screen.getByRole("button", { name: "Reject" }));
    await vi.advanceTimersByTimeAsync(ACTION_TOAST_MS);

    await vi.waitFor(() =>
      expect(screen.getByRole("button", { name: "Reject" })).toHaveFocus(),
    );
    expect(screen.queryByText(REJECTED)).toBeNull();
  });

  it.each([
    [
      "a refusal as a danger toast that stays",
      refused(DETAIL),
      DETAIL,
      "danger",
    ],
    [
      "an answer someone else gave as a quiet toast that goes",
      jsonResponse(
        { title: "Conflict", status: 409, code: "already_decided" },
        409,
      ),
      en["decision.alreadyDecided"],
      "info",
    ],
  ])("says %s once the card has gone", async (_what, answer, said, tone) => {
    const user = steppedClock({ toFake: [...HOLD_TIMERS] });
    stubDecisions(() => answer.clone());
    const { rerender } = render(<ApprovalRow approval={closeDateApproval()} />);

    await user.click(screen.getByRole("button", { name: "Reject" }));
    rerender(<p>Elsewhere</p>);

    const toast = (await vi.waitFor(() => screen.getByText(said))).closest(
      "output",
    );
    expect(toast?.querySelector(`.toast-dot-${tone}`)).not.toBeNull();
    const sticky = toast?.querySelector(".toast-dismiss") !== null;
    expect(sticky).toBe(tone === "danger");
  });

  it("sends nothing when the row is decided before it leaves", async () => {
    const user = steppedClock({ toFake: [...HOLD_TIMERS] });
    const fetched = stubDecisions();
    const view = render(<ApprovalRow approval={closeDateApproval()} />);
    await user.click(screen.getByRole("button", { name: "Reject" }));

    act(() => {
      flushSync(() =>
        view.rerender(<ApprovalRow approval={closeDateApproval()} decided />),
      );
      view.unmount();
    });
    await vi.advanceTimersByTimeAsync(ACTION_TOAST_MS * 2);

    expect(rejectCalls(fetched)).toBe(0);
  });

  it("holds a second Reject for its whole window after an Undo by keyboard", async () => {
    const user = steppedClock({ toFake: [...HOLD_TIMERS] });
    const fetched = stubDecisions();
    render(<ApprovalRow approval={closeDateApproval()} />);

    await user.click(screen.getByRole("button", { name: "Reject" }));
    await user.tab();
    await user.keyboard("{Enter}");
    await user.click(screen.getByRole("button", { name: "Reject" }));
    await vi.advanceTimersByTimeAsync(ACTION_TOAST_MS - 1);
    expect(rejectCalls(fetched)).toBe(0);
    await vi.advanceTimersByTimeAsync(1);

    await vi.waitFor(() => expect(rejectCalls(fetched)).toBe(1));
  });

  it("stays paused while focus is on Undo even after the pointer leaves", async () => {
    const user = steppedClock({ toFake: [...HOLD_TIMERS] });
    const fetched = stubDecisions();
    render(<ApprovalRow approval={closeDateApproval()} />);

    await user.click(screen.getByRole("button", { name: "Reject" }));
    const undo = screen.getByRole("button", { name: UNDO });
    await user.hover(undo);
    await user.tab();
    await user.unhover(undo);
    await vi.advanceTimersByTimeAsync(ACTION_TOAST_MS * 2);

    expect(undo).toHaveFocus();
    expect(rejectCalls(fetched)).toBe(0);
  });
});

describe("the offer to put an approved change back", () => {
  it("appears after approving, and lands on the record that changed", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => jsonResponse({ id: "ap1", status: "approved" })),
    );
    render(<ApprovalRow approval={closeDateApproval()} />);

    await userEvent.click(screen.getByRole("button", { name: "Approve" }));

    const undo = await screen.findByRole("button", {
      name: "Undo on record",
    });
    await userEvent.click(undo);
    expect(globalThis.location.hash).toBe("#/deals/d1");
  });

  // A rejection changed nothing, so there is nothing to put back and no offer.
  //
  // The absence is asserted only AFTER the reject call has actually gone out.
  // Before it, the button is absent for the uninteresting reason that nothing
  // has happened yet — an assertion that would survive the interaction being
  // removed entirely, and therefore proves nothing about rejecting.
  it("stays silent after a rejection", async () => {
    const user = steppedClock({ toFake: [...HOLD_TIMERS] });
    const fetched = stubDecisions();
    render(<ApprovalRow approval={closeDateApproval()} />);

    await user.click(screen.getByRole("button", { name: "Reject" }));
    await vi.advanceTimersByTimeAsync(ACTION_TOAST_MS);

    await vi.waitFor(() => expect(rejectCalls(fetched)).toBe(1));
    expect(screen.queryByRole("button", { name: "Undo on record" })).toBeNull();
  });

  // A step-up names no record, so there is no history to send anyone to. Same
  // rule as above: the approve call must have gone out before the absence means
  // anything.
  it("stays silent when the approval names no record", async () => {
    const fetched = vi.fn(async () =>
      jsonResponse({ id: "ap1", status: "approved" }),
    );
    vi.stubGlobal("fetch", fetched);
    render(
      <ApprovalRow
        approval={closeDateApproval({
          kind: "volume_release",
          target_entity_type: null,
          target_entity_id: null,
        })}
      />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Approve" }));
    await waitFor(() => expect(approveCalls(fetched)).toBe(1));
    expect(screen.queryByRole("button", { name: "Undo on record" })).toBeNull();
  });
});

// The card says whether the engine would let the staged mail go, before the
// approver releases it. An approver decides on somebody else's behalf, and
// used to learn of a refusal the way the author would have: by pressing Accept
// and reading the effect's failure.
describe("whether the staged mail may go", () => {
  function heldDraft(over: Partial<Approval> = {}): Approval {
    return {
      id: "ap2",
      kind: "held_draft",
      status: "pending",
      summary: "Reply to Anna about the quote",
      proposed_by: "system:automation",
      proposed_change: {
        anchor_activity_id: "act-1",
        to: "anna@example.test",
        subject: "Re: the quote",
        body: "Attached, as promised.",
      },
      created_at: "2026-08-20T09:00:00Z",
      target_entity_type: "activity",
      target_entity_id: "act-1",
      ...over,
    };
  }

  // Routes the preview door and nothing else: the card's other reads answer
  // empty, so what appears can only have come from the engine's answer.
  function stubEngine(answer: () => Response) {
    const fetched = vi.fn(async (input: RequestInfo | URL) => {
      const path =
        input instanceof Request
          ? new URL(input.url).pathname
          : new URL(String(input), "https://test").pathname;
      return isPreviewDoor(path) ? answer() : jsonResponse({ data: [] });
    });
    vi.stubGlobal("fetch", fetched);
    return fetched;
  }

  it("names who decided against the message, and that nobody may lift it", async () => {
    const fetched = stubEngine(() =>
      jsonResponse(
        refusedPreview("anna@example.test", {
          reason_code: "marketing_objection",
          decided_by: "subject",
        }),
      ),
    );
    render(<ApprovalRow approval={heldDraft()} />);

    expect(
      await screen.findByText(/recipient objected to marketing/i),
    ).toBeInTheDocument();
    // Asked the way the release will ask: against the thread the draft answers.
    const asked = fetched.mock.calls.find(([input]) =>
      isPreviewDoor(
        input instanceof Request ? new URL(input.url).pathname : String(input),
      ),
    );
    expect(
      asked && asked[0] instanceof Request
        ? new URL(asked[0].url).pathname
        : "",
    ).toBe("/v1/activities/act-1/send-email:preview");
  });

  // A decided row has nothing left to release, so there is nothing to ask.
  it("asks nothing about a proposal already decided", async () => {
    const fetched = stubEngine(() => jsonResponse({}));
    render(
      <ApprovalRow approval={heldDraft({ status: "approved" })} decided />,
    );
    await screen.findByText("Reply to Anna about the quote");
    expect(
      fetched.mock.calls.some(([input]) =>
        isPreviewDoor(
          input instanceof Request
            ? new URL(input.url).pathname
            : String(input),
        ),
      ),
    ).toBe(false);
  });
});

describe("a decision that lost its race", () => {
  const skew = () =>
    jsonResponse({ title: "Conflict", status: 409, code: "version_skew" }, 409);

  it("asks for the staging again when its record moved", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => skew()),
    );
    render(<ApprovalRow approval={closeDateApproval()} />);

    await user.click(screen.getByRole("button", { name: "Approve" }));

    expect(await screen.findByText(en["decision.versionSkew"])).toBeVisible();
  });

  // Nobody stages a rate proposal by hand, so staging again is advice the
  // reader cannot follow.
  it("sends a rate proposal back to the refresh", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => skew()),
    );
    render(
      <ApprovalRow
        approval={closeDateApproval({
          kind: "fx_rate_proposal",
          summary: "Update the EUR rate",
          proposed_change: { from_currency: "EUR", rate: "1.08" },
          target_entity_type: "fx_rate",
        })}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Approve" }));

    expect(await screen.findByText(en["decision.fxRateMoved"])).toBeVisible();
    expect(screen.queryByText(en["decision.versionSkew"])).toBeNull();
  });
});

it("updates the visible approval change while its reason is edited", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse({ items: [] })),
  );
  render(
    <ApprovalRow
      approval={closeDateApproval({
        kind: "advance_deal",
        summary: "Move Harbor to lost",
        proposed_change: { lost_reason: "Old reason" },
        preview: {
          version: 1,
          state: "ready",
          reviewer_reason: "record_authority",
          changes: [
            {
              field: "lost_reason",
              before: { kind: "empty", text: "" },
              after: { kind: "text", text: "Old reason" },
            },
          ],
        },
      })}
    />,
  );
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: en["trust.edit"] }));
  const reason = screen.getByDisplayValue("Old reason");
  await user.clear(reason);
  await user.type(reason, "Corrected reason");
  expect(screen.getByText("Corrected reason")).toBeVisible();
  expect(screen.queryByText("Old reason")).toBeNull();
});

it.each(["Approve", "Reject"])(
  "refreshes Home after %s without a page reload",
  async (verdict) => {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    client.setQueryData(magicKey, { needs_you: [{ id: "ap1" }] });
    client.setQueryData([...worklistKey, "mine"], { total: 1 });
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          id: "ap1",
          status: verdict === "Approve" ? "approved" : "rejected",
        }),
      ),
    );
    const user = steppedClock({ toFake: [...HOLD_TIMERS] });
    render(<ApprovalRow approval={closeDateApproval()} />, client);
    await user.click(screen.getByRole("button", { name: verdict }));
    await vi.advanceTimersByTimeAsync(ACTION_TOAST_MS);
    await vi.waitFor(() => {
      expect(client.getQueryState(magicKey)?.isInvalidated).toBe(true);
      expect(
        client.getQueryState([...worklistKey, "mine"])?.isInvalidated,
      ).toBe(true);
    });
  },
);
