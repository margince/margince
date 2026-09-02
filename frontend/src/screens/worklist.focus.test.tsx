// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { FocusCard, focusOf } from "./worklist.focus";

// worthActingOn's promises: a routable top row is promoted, and `acknowledge`
// never is — it names no record to route to, the same invariant
// WorklistRow's own VERB_DESTINATION table holds for it.

type WorklistItem = components["schemas"]["WorklistItem"];

function dealItem(over: Partial<WorklistItem> = {}): WorklistItem {
  return {
    id: "deal-1",
    source: "deal_at_risk",
    category: "deals_at_risk",
    level: 3,
    consequence: "deal_slips_past_close",
    title: "Acme Expansion",
    because: [{ kind: "quiet_days", value: { kind: "days", days: 83 } }],
    actions: ["open"],
    primary_action: "open",
    subject: {
      type: "deal",
      id: "01a05500-0000-7000-8000-000000000010",
      label: "Acme Expansion",
    },
    ...over,
  };
}

function renderCard(item: WorklistItem) {
  // The card's verb can be a real mutation, so it needs a query client — the
  // completion the `complete` action names is a PATCH, not a link.
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <FocusCard item={item} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// A task whose primary action is `complete` — the case that made the card's
// verb a component. `id` is the ACTIVITY id for a task row, which is what the
// completion PATCHes.
function taskItem(over: Partial<WorklistItem> = {}): WorklistItem {
  return dealItem({
    id: "01a05500-0000-7000-8000-000000000099",
    source: "task",
    category: "tasks",
    level: 4,
    consequence: "task_slips",
    title: "Call Alice back",
    actions: ["complete", "snooze"],
    primary_action: "complete",
    subject: {
      type: "person",
      id: "01a05500-0000-7000-8000-000000000011",
      label: "Alice Müller",
    },
    ...over,
  });
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the queue's top row, drawn as the one thing to do next", () => {
  it("draws the card for a routable action", () => {
    renderCard(dealItem());
    expect(screen.getByText("Acme Expansion")).toBeTruthy();
  });

  // THE case this card's verb exists for.
  //
  // The label said "Complete it" over a link to the task's record, so pressing
  // it navigated and completed nothing — the reader believed the work was done
  // and moved on. Asserting the MUTATION rather than the label is the point: a
  // test reading the button's text passed throughout the whole time the
  // promise was false.
  it("completing a task from the card actually completes it", async () => {
    const fetched = vi.fn(
      async (_input: RequestInfo | URL, _init?: RequestInit) =>
        new Response(null, { status: 204 }),
    );
    vi.stubGlobal("fetch", fetched);
    renderCard(taskItem());

    await userEvent.click(screen.getByRole("button", { name: "Done" }));

    await vi.waitFor(() => {
      expect(fetched).toHaveBeenCalled();
    });
    // The ADDRESS and the METHOD, not the label: this is the assertion that
    // would have failed for the whole time the card only navigated.
    const [input, init] = fetched.mock.calls[0];
    const request = input instanceof Request ? input : undefined;
    expect(String(request?.url ?? input)).toContain(
      "/activities/01a05500-0000-7000-8000-000000000099",
    );
    expect(request?.method ?? init?.method).toBe("PATCH");
  });

  it("draws nothing for a notice's acknowledge, even with a record to name", () => {
    const { container } = renderCard(
      dealItem({
        source: "notice",
        primary_action: "acknowledge",
        actions: ["acknowledge"],
      }),
    );
    expect(container.firstChild).toBeNull();
  });

  it("focusOf promotes a routable top row", () => {
    const top = dealItem();
    expect(focusOf([top])).toBe(top);
  });

  it("focusOf excludes acknowledge from the queue's top row", () => {
    const top = dealItem({
      source: "notice",
      primary_action: "acknowledge",
      actions: ["acknowledge"],
    });
    expect(focusOf([top])).toBeUndefined();
  });

  it("draws nothing for review work — that is judgement, not a rep's next action", () => {
    const { container } = renderCard(dealItem({ band: "review" }));
    expect(container.firstChild).toBeNull();
  });

  it("focusOf answers undefined for an empty queue", () => {
    expect(focusOf([])).toBeUndefined();
  });
});
