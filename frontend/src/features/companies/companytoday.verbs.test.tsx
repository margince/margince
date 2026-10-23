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
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { CompanyNextBar } from "./companynextbar";

// The Next bar's own promise: whenever it names something the reader owes, it
// also offers the verb that answers it. That holds even where the moment's own
// recommended action names no destination for `onOpenRecord` to open.
// Both tests below fire on that shape. One is a moment the server sent with
// no destination. The other is a fact of the account's own (a named task, a
// reply we owe). The page backs it with a control it already holds.

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

// A moment whose own recommended action names no destination. A rule that
// fires on a promise, not on one record, sends this shape. The bar then falls
// to the page's own verb.
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
        <CompanyNextBar
          view={view}
          onOpenTasks={() => {}}
          onOpenTask={opts.onOpenTask}
          onDraftTo={opts.onDraftTo}
          onLogActivity={opts.onLogActivity}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// Scoped to the bar's own row, found by the headline it leads with.
function leadingCard(headline: string) {
  const card = screen.getByText(headline).closest(".next-bar-row");
  if (!(card instanceof HTMLElement)) {
    throw new Error("the Next bar did not render");
  }
  return within(card);
}

describe("the Next bar's own fallback verb", () => {
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
    const verb = leadingCard(MOMENT_NO_DESTINATION.headline).getByRole(
      "button",
      { name: en["today.moment.act.openTask"] },
    );
    fireEvent.click(verb);
    expect(opened).toHaveBeenCalledWith("a-9");
  });

  it("drafts a reply to the strongest contact when we owe one and no task is named", () => {
    const drafted = vi.fn();
    show(
      {
        ...BASE,
        moment: MOMENT_NO_DESTINATION,
        state_strip: {
          account: { lifecycle: "customer", relationship_types: [] },
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
    const verb = leadingCard(MOMENT_NO_DESTINATION.headline).getByRole(
      "button",
      { name: en["today.draft.act"] },
    );
    fireEvent.click(verb);
    expect(drafted).toHaveBeenCalledWith("p-1");
  });
});
