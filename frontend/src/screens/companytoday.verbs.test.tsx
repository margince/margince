/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { TodayOnThisAccount } from "./companytoday";

// The leading card's own promise: whenever it names something the reader
// owes, it also offers the verb that answers it — even where the moment's own
// recommended action names no destination for `onOpenRecord` to open. Both
// tests below fire on exactly that shape: a moment the server sent with no
// destination, and a different fact of the account's own (a named task, a
// reply we owe) that the page can back with a control it already holds.

afterEach(cleanup);

type Company360 = components["schemas"]["Company360"];

const BASE: Company360 = {
  as_of: "2026-08-07T09:00:00Z",
  company: {
    id: "o-1",
    display_name: "Acme",
    source: "manual",
    captured_by: "human:test",
    created_at: "2026-08-01T09:00:00Z",
    updated_at: "2026-08-01T09:00:00Z",
  },
  sections_omitted: [],
};

// A moment whose own recommended action names no destination — the ordinary
// shape for a rule that fires on a promise rather than on one specific
// record — so the leading card falls to the page's own verb.
const MOMENT_NO_DESTINATION: NonNullable<Company360["moment"]> = {
  claim_key: "moment:open_promise",
  evidence_fingerprint: "fp-1",
  rule: "open_promise",
  headline: "Confirm server booking status",
  why_now: "Promised on Monday, and nothing has gone out since.",
  confidence: "observed_fact",
  evidence: [],
  recommended_action: {
    kind: "log_activity",
    label: "Log it",
    state: "will_confirm",
  },
};

function show(
  view: Company360,
  opts: Readonly<{
    onOpenTask?: (activityId: string) => void;
    onDraftTo?: (contactId: string) => void;
    onLogActivity?: () => void;
  }> = {},
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <TodayOnThisAccount
          companyId="o-1"
          view={view}
          loading={false}
          failed={false}
          onOpenTask={opts.onOpenTask}
          onDraftTo={opts.onDraftTo}
          onLogActivity={opts.onLogActivity}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// Scoped to the moment's own row: the account may draw a second "Draft" verb
// of its own (manualMoveRows' generic "write to them" row), and a query
// against the whole page cannot tell the leading card's verb from that one.
function leadingCard() {
  const card = document.querySelector(".co-move-lead");
  if (!card) {
    throw new Error("the leading card did not render");
  }
  return within(card as HTMLElement);
}

describe("the leading card's own fallback verb", () => {
  it("opens the named task when one is already on the account's list", () => {
    const opened = vi.fn();
    show(
      {
        ...BASE,
        moment: MOMENT_NO_DESTINATION,
        next_steps: {
          data: [
            {
              activity_id: "a-9",
              subject: "Confirm the booking",
              overdue: false,
            },
          ],
          page: { has_more: false, next_cursor: null },
        },
      },
      { onOpenTask: opened, onLogActivity: vi.fn() },
    );
    const verb = leadingCard().getByRole("button", { name: "Open task" });
    fireEvent.click(verb);
    expect(opened).toHaveBeenCalledWith("a-9");
    // The secondary verb rides beside it once the caller offers a place to
    // log one — the same shape the contact page's moment column draws.
    expect(screen.getByRole("button", { name: "Log activity" })).toBeTruthy();
  });

  it("drafts a reply to the strongest contact when we owe one and no task is named", () => {
    const drafted = vi.fn();
    show(
      {
        ...BASE,
        moment: MOMENT_NO_DESTINATION,
        state_strip: {
          account: { status: "customer", relationship_types: [] },
          engagement: {
            state: "waiting_on_us",
            last_inbound_at: "2026-08-05T09:00:00Z",
            last_outbound_at: "2026-07-20T09:00:00Z",
          },
        },
        contacts: {
          data: [
            {
              contact_id: "p-1",
              full_name: "Dana Buyer",
              strength: {
                score: 71,
                bucket: "strong",
                factors: {
                  recency: 0.9,
                  frequency: 0.6,
                  reciprocity: 0.8,
                  direction: 0.8,
                },
              },
              deal_roles: [],
              consent: {},
            },
          ],
          page: { has_more: false, next_cursor: null },
        },
      },
      { onDraftTo: drafted },
    );
    const verb = leadingCard().getByRole("button", { name: "Draft" });
    fireEvent.click(verb);
    expect(drafted).toHaveBeenCalledWith("p-1");
  });
});
