// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";

import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { FACT_PREVIEW_LIMIT, ReadEvidence } from "./onboarding-read";

// The evidence panel shows three kinds of finding — a legal entity, a profile
// field and a live fact — and each one is a card. They are cards through the
// design system's Card, not through three descriptions of a card kept in this
// screen's stylesheet, and the count below is what holds that: a finding that
// stops being a Card stops being counted, whatever it still looks like.

type CompanySiteRead = components["schemas"]["CompanySiteRead"];

const LEGAL_ENTITY = {
  name: "Gradion GmbH",
  registered_address: "Hafenstraße 1, Hamburg",
  register_number: "HRB 12345",
  source_url: "https://gradion.com/impressum",
};

function grounded(
  field: components["schemas"]["ColdStartField"]["field"],
  value: string,
  snippet: string,
): components["schemas"]["ColdStartField"] {
  return {
    field,
    value,
    evidence_snippet: snippet,
    source_kind: "url",
    source_url: "https://gradion.com",
    confidence: 0.9,
  };
}

function surplusFacts(
  count: number,
): components["schemas"]["CompanySiteReadFact"][] {
  return Array.from({ length: count }, (_unused, index) => ({
    category: "offering" as const,
    field: "service",
    value_key: `offering:service:${index}`,
    value: `Service ${index}`,
    evidence_snippet: `We run service ${index}.`,
    evidence_url: "https://gradion.com/services",
    confidence: 0.8,
  }));
}

const READ = {
  id: "11111111-1111-4111-8111-111111111111",
  target_kind: "onboarding",
  organization_id: null,
  root_url: "https://gradion.com",
  status: "ready",
  status_code: null,
  status_detail: null,
  next_attempt_at: null,
  phase: null,
  pages_read: 2,
  pages: [{ url: "https://gradion.com", status: "fetched", kind: "home" }],
  profile_fields: [
    grounded("legal_name", "Gradion GmbH", "© 2026 Gradion GmbH"),
    grounded("industry", "Manufacturing", "We serve manufacturers."),
  ],
  facts: [
    {
      category: "company",
      field: "founded_year",
      value_key: "company:founded_year:2011",
      value: "Founded 2011",
      evidence_snippet: "Founded in Hamburg in 2011.",
      evidence_url: "https://gradion.com/about",
      confidence: 0.95,
    },
    // Past the preview limit on purpose. A fixture that stays under it cannot
    // tell "one card per finding" from "one card per finding the panel shows",
    // and those stop agreeing at exactly the point the cap bites.
    ...surplusFacts(FACT_PREVIEW_LIMIT + 1),
  ],
  comparisons: [],
  people: [],
  legal_entities: [LEGAL_ENTITY],
  warnings: [],
  draft_version: 2,
  proposal_hash: "proposal-2",
  created_at: "2026-07-22T08:00:00Z",
  updated_at: "2026-07-22T08:00:01Z",
} satisfies CompanySiteRead;

function render(read: CompanySiteRead) {
  return rtlRender(
    <LocaleProvider initial="en">
      <ReadEvidence read={read} />
    </LocaleProvider>,
  );
}

afterEach(cleanup);

describe("the evidence panel", () => {
  it("shows a legal entity as printed, in a card", () => {
    const { container } = render(READ);

    const entity = container.querySelector(".legal-preview-card");
    expect(entity).toHaveClass("card");
    expect(entity?.tagName).toBe("ARTICLE");
    expect(entity).toHaveTextContent("Gradion GmbH");
    expect(entity).toHaveTextContent("Hafenstraße 1, Hamburg");
    expect(entity).toHaveTextContent("HRB 12345");
  });

  it("shows a profile field with its evidence, in a card named for the field", () => {
    render(READ);

    const finding = screen.getByTestId("finding-industry");
    expect(finding).toHaveClass("card");
    expect(finding).toHaveTextContent("Manufacturing");
    expect(finding).toHaveTextContent("We serve manufacturers.");
    // The confidence a reader acts on, rounded where they can see it.
    expect(finding).toHaveTextContent("90%");
  });

  it("shows a live fact with its evidence, in a card", () => {
    const { container } = render(READ);

    const fact = container.querySelector(".live-fact-preview .finding-card");
    expect(fact).toHaveClass("card");
    expect(fact).toHaveTextContent("Founded 2011");
    expect(fact).toHaveTextContent("Founded in Hamburg in 2011.");
  });

  // The count, both directions at once: a finding that is not a Card is missing
  // from the left side, and a card this panel did not put there is extra on the
  // right. A screen that grows its own card again fails here rather than in
  // somebody's eye, months later, over a shadow that is one pixel out.
  it("draws one card per finding it shows, and nothing else", () => {
    const { container } = render(READ);

    // Facts are capped, and the cap is part of the claim rather than an
    // exception to it: the panel draws a card for each finding it SHOWS.
    expect(READ.facts.length).toBeGreaterThan(FACT_PREVIEW_LIMIT);
    const shown =
      (READ.legal_entities?.length ?? 0) +
      READ.profile_fields.length +
      Math.min(READ.facts.length, FACT_PREVIEW_LIMIT);
    expect(container.querySelectorAll(".card")).toHaveLength(shown);
    expect(
      container.querySelectorAll(
        ".legal-preview-card:not(.card), .finding-card:not(.card)",
      ),
    ).toHaveLength(0);
  });

  it("shows nothing at all when the read found nothing", () => {
    const { container } = render({
      ...READ,
      profile_fields: [],
      facts: [],
      legal_entities: [],
      pages: [],
      warnings: [],
    });

    expect(container).toBeEmptyDOMElement();
  });
});
