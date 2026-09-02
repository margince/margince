// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { FocusCard, focusOf } from "./worklist.focus";

// worthFocusing's promises: a routable top row is promoted, and `acknowledge`
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
  return render(
    <LocaleProvider initial="en">
      <FocusCard item={item} />
    </LocaleProvider>,
  );
}

afterEach(cleanup);

describe("the queue's top row, drawn as the one thing to do next", () => {
  it("draws the card for a routable action", () => {
    renderCard(dealItem());
    expect(screen.getByText("Acme Expansion")).toBeTruthy();
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
