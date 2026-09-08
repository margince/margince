// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import {
  type CompanyDraft,
  type CompanyFieldName,
  EMPTY_DRAFT,
} from "./onboarding";
import { CompanyStep } from "./onboarding-company-form";
import { StoryProviders } from "./story-utils";
import "./onboarding.css";

// The reviewable company form: the field groups at the house rhythm, the
// origin line above them, and the collapsed fact list under them.
//
// The two lines these stories exist for are the ones a reader believes without
// checking. The origin line states WHERE the values came from — a read of the
// company's own pages, or the reader's own typing — and the facts summary is
// the kicker over a hundred evidence cards nobody has opened. Both are the
// smallest text on the surface, so both are rendered here at their real size
// rather than inside the conversational shell that hosts them.
//
// Dates in the read are fixed: `make fe-clock-drift` runs at +200 days.

type CompanySiteRead = components["schemas"]["CompanySiteRead"];
type CompanySiteReadFact = components["schemas"]["CompanySiteReadFact"];

// A draft the read filled in, so the grounded adornments beside the labels are
// present rather than an empty form's absence. Built from the screen's own
// empty form: a story that spells the whole shape stops compiling the day the
// company gains a field, and says nothing about the story either way.
const grounded: CompanyDraft = {
  values: {
    ...EMPTY_DRAFT.values,
    display_name: "Brandt Automotive",
    website: "https://brandt-automotive.example",
    legal_name: "Brandt Automotive GmbH",
    registered_address: "Werkstraße 14, 70565 Stuttgart",
    industry: "Automotive tier-one supply",
    offer_summary:
      "Retrofit lines for assembly plants, sold as a programme rather than a machine.",
    icp: "Tier-one suppliers running two or more plants on ageing lines.",
  },
  grounded: {
    legal_name: {
      field: "legal_name",
      value: "Brandt Automotive GmbH",
      evidence_snippet: "Brandt Automotive GmbH · HRB 21 447 Stuttgart",
      source_kind: "url",
      source_url: "https://brandt-automotive.example/impressum",
      confidence: 0.94,
    },
  },
  edited: new Set<CompanyFieldName>(["icp"]),
};

const FACTS: readonly CompanySiteReadFact[] = [
  {
    category: "company",
    field: "founded_year",
    value_key: "company:founded_year:1998",
    value: "Founded 1998",
    evidence_snippet: "Family-owned in Stuttgart since 1998.",
    evidence_url: "https://brandt-automotive.example/about",
    confidence: 0.91,
  },
  {
    category: "offering",
    field: "service",
    value_key: "offering:service:retrofit",
    value: "Assembly-line retrofit programmes",
    evidence_snippet: "We retrofit ageing lines without stopping production.",
    evidence_url: "https://brandt-automotive.example/retrofit",
    confidence: 0.88,
  },
];

// The read as the API serves it. `pages_read` is the count the origin line
// says out loud, and it is the server's number — the pages array a story
// carries is not it.
function read(facts: readonly CompanySiteReadFact[]): CompanySiteRead {
  return {
    id: "11111111-1111-4111-8111-111111111111",
    target_kind: "onboarding",
    root_url: "https://brandt-automotive.example",
    status: "ready",
    status_code: null,
    status_detail: null,
    next_attempt_at: null,
    pages_read: 14,
    pages: [],
    profile_fields: [],
    facts: [...facts],
    comparisons: [],
    people: [],
    warnings: [],
    draft_version: 1,
    proposal_hash: "hash",
    created_at: "2026-07-01T09:00:00Z",
    updated_at: "2026-07-01T09:05:00Z",
  };
}

const meta: Meta<typeof CompanyStep> = {
  title: "Onboarding/Company form",
  component: CompanyStep,
};
export default meta;
type Story = StoryObj<typeof CompanyStep>;

function form(
  draft: CompanyDraft,
  site: CompanySiteRead | null,
  locale?: "de",
) {
  return () => (
    <StoryProviders locale={locale}>
      <CompanyStep
        draft={draft}
        setField={() => {}}
        onPickEntity={() => {}}
        read={site}
        saved={false}
        saveError={null}
        missingRequired={[]}
        selectedFactKeys={FACTS.map((fact) => fact.value_key)}
        setSelectedFactKeys={() => {}}
        onFieldBlur={() => {}}
      />
    </StoryProviders>
  );
}

/** After a read: the origin line names how many pages it was grounded in, and
 *  the fact list's kicker sits over the selected-of-total count. */
export const GroundedInARead: Story = {
  render: form(grounded, read(FACTS)),
};

/** No site to read, so the reader typed it. The origin line changes what it
 *  claims — human assertions, not evidence — and the fact list is absent
 *  because there was nothing to find. */
export const TypedByHand: Story = {
  render: form({ ...grounded, grounded: {} }, null),
};

/** The German form, where the origin sentence runs a third longer and has to
 *  stay one readable line above the fields. */
export const German: Story = {
  render: form(grounded, read(FACTS), "de"),
};

/** The same two lines in the dark theme: both sit on the muted role, which is
 *  where the dark accent lift shows first. */
export const GroundedInAReadDark: Story = {
  globals: { theme: "dark" },
  render: form(grounded, read(FACTS)),
};

/** At 390px the field rhythm stacks and the facts summary's two halves — the
 *  kicker and the count — must not collide. */
export const Phone: Story = {
  tags: ["uat-phone"],
  render: form(grounded, read(FACTS)),
};
