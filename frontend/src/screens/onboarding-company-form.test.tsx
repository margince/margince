// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider, translate } from "../i18n";
import type { CompanyDraft } from "./onboarding";
import { EMPTY_DRAFT } from "./onboarding";
import { CompanyStep } from "./onboarding-company-form";

type CompanySiteRead = components["schemas"]["CompanySiteRead"];

// Evidence-or-omit on the classic form: a grounded field shows the page's own
// words when the read captured them, and NOTHING when it did not. A chip drawn
// around an empty quote is the one failure this rule exists to prevent — it
// looks exactly like proof, and there is none behind it.

function render(ui: ReactNode) {
  return rtlRender(<LocaleProvider initial="en">{ui}</LocaleProvider>);
}

function groundedDraft(snippet: string | undefined): CompanyDraft {
  return {
    values: { ...EMPTY_DRAFT.values, legal_name: "Gradion Co., Ltd." },
    grounded: {
      legal_name: {
        field: "legal_name",
        value: "Gradion Co., Ltd.",
        evidence_snippet: snippet,
        source_kind: "url",
        source_url: "https://gradion.test/impressum",
      },
    },
    edited: new Set(),
  };
}

function siteRead(pagesRead: number): CompanySiteRead {
  return {
    id: "11111111-1111-4111-8111-111111111111",
    target_kind: "onboarding",
    root_url: "https://gradion.test",
    status: "ready",
    status_code: null,
    status_detail: null,
    next_attempt_at: null,
    pages_read: pagesRead,
    pages: [],
    profile_fields: [],
    facts: [],
    comparisons: [],
    contacts: [],
    warnings: [],
    draft_version: 1,
    proposal_hash: "hash",
    created_at: "2026-07-01T09:00:00Z",
    updated_at: "2026-07-01T09:05:00Z",
  };
}

function renderForm(
  snippet: string | undefined,
  read: CompanySiteRead | null = null,
) {
  render(
    <CompanyStep
      draft={groundedDraft(snippet)}
      setField={vi.fn()}
      onPickEntity={vi.fn()}
      read={read}
      saved={false}
      saveError={null}
      missingRequired={[]}
      selectedFactKeys={[]}
      setSelectedFactKeys={vi.fn()}
      onFieldBlur={vi.fn()}
    />,
  );
}

// The grounding label is asserted through its catalog key, not the sentence
// the en catalog happens to hold today: the wording belongs to the catalog,
// and what these tests are about is that the label renders at all.
const groundingLabel = translate("en", "ob.readFromSite");

afterEach(cleanup);

describe("a grounded field's evidence chip", () => {
  it("shows the page's own words when the read captured them", () => {
    renderForm("Gradion Co., Ltd. · HRB 12345 B");

    expect(
      screen.getByText(/Gradion Co\., Ltd\. · HRB 12345 B/),
    ).toBeInTheDocument();
    expect(document.querySelectorAll(".evidence-chip")).toHaveLength(1);
  });

  it("draws no chip at all when the read captured no quote", () => {
    renderForm(undefined);

    expect(document.querySelectorAll(".evidence-chip")).toHaveLength(0);
    // The grounding itself is still real and still says so: only the quote it
    // does not have is withheld.
    expect(screen.getByText(groundingLabel)).toBeInTheDocument();
  });

  it("treats a quote of nothing but whitespace as no quote", () => {
    renderForm("   ");

    expect(document.querySelectorAll(".evidence-chip")).toHaveLength(0);
    expect(screen.getByText(groundingLabel)).toBeInTheDocument();
  });
});

describe("the origin line", () => {
  it.each([
    [1, "Based on 1 public page."],
    [14, "Based on 14 public pages."],
  ])("counts %i page(s) the read was grounded in", (pages, sentence) => {
    renderForm("Gradion Co., Ltd.", siteRead(pages));
    expect(screen.getByText(new RegExp(sentence))).toBeInTheDocument();
  });
});
