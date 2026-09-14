// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Evidence } from "../../design-system/trust";
import type { CompanyFieldName } from "../onboarding";
import { StoryProviders } from "../story-utils";
import type { ReviewRow, RowState } from "./company-review-state";
import { ProfileArticle } from "./profile-digest-article";
import {
  type Citation,
  type Contact,
  citationsOf,
  type Fact,
  factsByCategory,
  type LegalEntity,
  type Page,
} from "./profile-digest-data";
// The article draws `pdigest-*`; the sheet is a side-effect import of
// profile-digest.tsx, which a story of this column alone never reaches.
import "./profile-digest.css";

// The whole-record document's LEFT COLUMN on its own — the record grouped into
// the four sections a reader recognises from reading it, then what the crawl
// found beyond those fields, then the numbered pages every line above cited.
// The header and the sidebar beside it are Onboarding/Profile digest.
//
// The column takes no data of its own, so every state it can be in comes from
// the props. The numbering and the fact grouping are derived through the
// document's own `citationsOf` and `factsByCategory` rather than written out
// here: a story that numbered its own references would agree with itself and
// prove nothing about the page.

const SITE = "https://acme.test";

function row(
  field: CompanyFieldName,
  label: string,
  value: string,
  state: RowState,
  evidence: Evidence | null = null,
): ReviewRow {
  return {
    field,
    label,
    value,
    multiline: false,
    state,
    evidence,
    confidence: null,
    emptyHintKey: "ob.conv.triage.emptyHint",
    omissionReasonKey: null,
  };
}

function cited(path: string, snippet: string): Evidence {
  return { source: `${SITE}${path}`, snippet };
}

// One row per section, so all four headings draw, plus one still open — the
// dashed line "Settle it" points at.
const ROWS: readonly ReviewRow[] = [
  row(
    "display_name",
    "Company name",
    "Acme Freight",
    "quoted",
    cited("/", "Acme Freight — European road freight, planned right."),
  ),
  row(
    "legal_name",
    "Registered legal name",
    "Acme Freight GmbH",
    "quoted",
    cited("/impressum", "Acme Freight GmbH, registered in Munich."),
  ),
  row(
    "offer_summary",
    "What do you sell?",
    "Route planning software for freight forwarders.",
    "quoted",
    cited("/product", "Route planning software for freight forwarders."),
  ),
  row(
    "icp",
    "Ideal customer",
    "Mid-market freight forwarders running 40-200 trucks.",
    "typed",
  ),
  row("usp", "What makes you different?", "", "required"),
  row(
    "sales_motion",
    "How do deals happen?",
    "Free route audit, then a 90-day pilot on one depot.",
    "quoted",
    cited("/product", "We start with a free route audit."),
  ),
];

const PAGES: readonly Page[] = [
  { url: `${SITE}/`, status: "fetched", kind: "home" },
  { url: `${SITE}/product`, status: "fetched", kind: "services" },
  { url: `${SITE}/impressum`, status: "fetched", kind: "impressum" },
  { url: `${SITE}/team`, status: "fetched", kind: "team" },
];

const LEGAL_ENTITIES: readonly LegalEntity[] = [
  {
    name: "Acme Freight GmbH",
    registered_address: "Lindwurmstraße 12, 80337 Munich",
    register_number: "HRB 123456",
    vat_number: "DE123456789",
    evidence_snippet: "Acme Freight GmbH, HRB 123456, Amtsgericht München.",
    source_url: `${SITE}/impressum`,
  },
];

const FACTS: readonly Fact[] = [
  {
    category: "company",
    field: "founded_year",
    value: "2014",
    value_key: "founded_year:2014",
    evidence_snippet: "Founded in 2014.",
    evidence_url: `${SITE}/`,
    confidence: 0.91,
  },
  {
    category: "offering",
    field: "product",
    value: "Route optimiser",
    value_key: "product:route-optimiser",
    evidence_snippet: "Our route optimiser replans a depot in minutes.",
    evidence_url: `${SITE}/product`,
    confidence: 0.77,
  },
];

const CONTACTS: readonly Contact[] = [
  {
    name: "Mara Voss",
    role: "Co-founder",
    published_email: "mara@acme.test",
    linkedin_url: null,
    evidence_snippet: "Mara Voss, co-founder, leads product.",
    evidence_url: `${SITE}/team`,
  },
  {
    name: "Devrim Aksoy",
    role: "Head of Operations",
    published_email: null,
    linkedin_url: "https://linkedin.com/in/devrim-aksoy",
    evidence_snippet: "Devrim Aksoy runs day-to-day operations.",
    evidence_url: `${SITE}/team`,
  },
];

function Article({
  legalEntities = [],
  facts = [],
  contacts = [],
}: Readonly<{
  legalEntities?: readonly LegalEntity[];
  facts?: readonly Fact[];
  contacts?: readonly Contact[];
}>) {
  const factGroups = factsByCategory(facts);
  // Rendering order, the same order the document hands them in, so a page
  // first seen behind a legal entity keeps the lower number.
  const cites: readonly Citation[] = citationsOf(ROWS, [
    ...legalEntities.map((entity) => entity.source_url),
    ...factGroups.flatMap((group) => group.facts.map((f) => f.evidence_url)),
    ...contacts.map((contact) => contact.evidence_url),
  ]);
  return (
    <StoryProviders>
      <ProfileArticle
        rows={ROWS}
        number={new Map(cites.map((cite) => [cite.url, cite.n]))}
        pages={PAGES}
        legalEntities={legalEntities}
        factGroups={factGroups}
        contacts={contacts}
        cites={cites}
        onSettle={() => {}}
        onField={() => {}}
      />
    </StoryProviders>
  );
}

const meta: Meta<typeof Article> = {
  title: "Onboarding/Profile article",
  component: Article,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof Article>;

// Everything the crawl found: the four record sections with a legal entity
// folded into Identity, the proof it gathered, the two contacts it named under
// Contacts, and a References list numbering every page cited above it.
export const WholeRecord: Story = {
  render: () => (
    <Article legalEntities={LEGAL_ENTITIES} facts={FACTS} contacts={CONTACTS} />
  ),
};

// A read that found nothing beyond the record's own lines: no legal entity, no
// proof, nobody named. Those three sections are absent rather than drawn empty
// — a heading over nothing claims the crawl looked and came back — and the
// References list still numbers the pages the record lines themselves cite.
export const NothingBeyondTheRecord: Story = {
  render: () => <Article />,
};
