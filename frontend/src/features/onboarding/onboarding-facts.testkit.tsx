import { type RenderResult, render as rtlRender } from "@testing-library/react";
import type { ReactNode } from "react";
import { vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { EMPTY_DRAFT } from "./onboarding";
import { CompanyConfirmCard } from "./onboarding-conversation/confirm-card";

// The fact surface is prop-driven, so every case built on these fixtures is a
// fixture in and a claim out. No fetch, no clock.

export type CompanySiteReadFact = components["schemas"]["CompanySiteReadFact"];

export function fact(
  over: Partial<CompanySiteReadFact> & { value_key: string; value: string },
): CompanySiteReadFact {
  return {
    category: "company",
    field: "founded_year",
    evidence_snippet: "Founded in Hamburg in 2011.",
    evidence_url: "https://acme.test/about",
    confidence: 0.9,
    ...over,
  };
}

export const FOUNDED = fact({
  value_key: "company:founded_year:2011",
  value: "Founded 2011",
  confidence: 0.95,
});

export const SERVICE = fact({
  value_key: "offering:service:k8s",
  value: "Managed Kubernetes",
  category: "offering",
  field: "service",
  evidence_snippet: "We run Kubernetes for logistics operators.",
  evidence_url: "https://acme.test/services/kubernetes",
  confidence: 0.88,
});

export const SUPPORT = fact({
  value_key: "offering:service:support",
  value: "24/7 support desk",
  category: "offering",
  field: "service",
  evidence_snippet: "The desk answers around the clock.",
  evidence_url: "https://acme.test/services/support",
  confidence: 0.71,
});

export function render(ui: ReactNode): RenderResult {
  return rtlRender(<LocaleProvider initial="en">{ui}</LocaleProvider>);
}

// Fixtures for the triage layout: a summary line before any row, and work rows
// ahead of the settled fields in each group. The long offer exercises a prose
// field's preview and full-text swap.

export type Proposal = components["schemas"]["OnboardingCompanyProposal"];

export const LONG_OFFER =
  "We help logistics operators run their fleets more efficiently by combining live telemetry with route optimization, and the platform plugs directly into dispatch software to remove the manual planning steps that used to take hours every single day.";

export function triageProposal(): Proposal {
  return {
    ready: true,
    fields: [
      {
        field: "legal_name",
        value: "Gradion GmbH",
        confidence: 0.9,
        evidence_snippet: "Legal name on the imprint page.",
        source_url: "https://gradion.com/impressum",
      },
      {
        field: "offer_summary",
        value: LONG_OFFER,
        confidence: 0.9,
        evidence_snippet: "About page copy.",
        source_url: "https://gradion.com/about",
      },
    ],
    facts: [],
    open_questions: [],
    remaining_required_fields: ["icp"],
    draft_version: 1,
    proposal_hash: "hash",
  };
}

export function triageDraft() {
  return {
    ...EMPTY_DRAFT,
    values: {
      ...EMPTY_DRAFT.values,
      legal_name: "Gradion GmbH",
      offer_summary: LONG_OFFER,
    },
  };
}

export function renderTriage(
  missingRequired: readonly "icp"[],
  read: components["schemas"]["CompanySiteRead"] | null = null,
) {
  render(
    <CompanyConfirmCard
      proposal={triageProposal()}
      draft={triageDraft()}
      answers={[]}
      read={read}
      selectedFactKeys={[]}
      setSelectedFactKeys={vi.fn()}
      missingRequired={missingRequired}
      setField={vi.fn()}
      onAcceptAll={vi.fn()}
      pending={false}
      authorizing={false}
      error={null}
    />,
  );
}

// The read the edit form needs to offer facts at all. Only the fact list is
// under test, so everything else is the read's empty shape.
export function readWith(
  facts: readonly CompanySiteReadFact[],
): components["schemas"]["CompanySiteRead"] {
  return {
    id: "11111111-1111-4111-8111-111111111111",
    target_kind: "onboarding",
    root_url: "https://acme.test",
    status: "ready",
    status_code: null,
    status_detail: null,
    next_attempt_at: null,
    pages: [],
    profile_fields: [],
    facts: [...facts],
    comparisons: [],
    contacts: [],
    warnings: [],
    draft_version: 1,
    proposal_hash: "hash",
    created_at: "2026-07-01T09:00:00Z",
    updated_at: "2026-07-01T09:05:00Z",
  };
}
