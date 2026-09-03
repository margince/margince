// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Evidence } from "../../design-system/trust";
import type { CompanyFieldName } from "../onboarding";
import { StoryProviders } from "../story-utils";
import type { ReviewRow, RowState } from "./company-review-state";
import { ProfileDigest, type ProfileDigestRead } from "./profile-digest";
// `.ob-scene` is a container (`conversation.css`), and the document's
// two-column fold answers to it; without the sheet the story shows one column
// at every width and the fold is never reviewed.
import "./conversation.css";

// The digest's two faces: the deck's narrow companion, and the whole-record
// document a reader reaches through "Read the whole profile". The document
// is what carries the states worth reviewing — open lines, a settled record,
// a sparse read, and a page cited from every direction — because that is
// where the header's figures, the dashed unanswered rows and the sidebar all
// live.

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

function cited(page: string, snippet: string): Evidence {
  return { source: `${SITE}${page}`, snippet };
}

// A full board: every field of the four groups, some answered from the read,
// some typed, five left open (`usp` required, the rest advisory) so "Settle
// it" has something to point at.
const OPEN_ROWS: ReviewRow[] = [
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
    "registered_address",
    "Registered address",
    "Lindwurmstraße 12, 80337 Munich",
    "quoted",
    cited("/impressum", "Lindwurmstraße 12, 80337 Munich."),
  ),
  row("register_vat", "VAT number", "", "empty"),
  row(
    "legal_form",
    "Legal form",
    "GmbH",
    "quoted",
    cited("/impressum", "Acme Freight GmbH."),
  ),
  row("register_court", "Register court", "", "empty"),
  row(
    "register_number",
    "Register number",
    "HRB 123456",
    "quoted",
    cited("/impressum", "HRB 123456, Amtsgericht München."),
  ),
  row(
    "industry",
    "Industry",
    "Freight forwarding",
    "quoted",
    cited("/about", "We plan road freight for European manufacturers."),
  ),
  row(
    "history",
    "Company history",
    "Founded in 2014 by two logistics planners frustrated with spreadsheets.",
    "quoted",
    cited("/about", "Founded in 2014 by two logistics planners."),
  ),
  row(
    "offer_summary",
    "What do you sell?",
    "Route planning software for freight forwarders.",
    "quoted",
    cited("/product", "Route planning software for freight forwarders."),
  ),
  row(
    "value_proposition",
    "Value proposition",
    "Cuts empty-leg miles by a third within the first quarter.",
    "typed",
  ),
  row("usp", "What makes you different?", "", "required"),
  row(
    "icp",
    "Ideal customer",
    "Mid-market freight forwarders running 40-200 trucks.",
    "quoted",
    cited("/customers", "Built for forwarders running 40 to 200 trucks."),
  ),
  row(
    "buying_center",
    "Who is in the room?",
    "Fleet ops lead and the finance director who signs off.",
    "typed",
  ),
  row("customer_pains", "What are they struggling with?", "", "empty"),
  row(
    "desired_outcomes",
    "What does winning look like?",
    "Fewer empty legs, no more spreadsheet planning.",
    "quoted",
    cited("/customers", "Fewer empty legs, no spreadsheets."),
  ),
  row(
    "buying_intents",
    "What signals interest?",
    "A fleet growing past 40 trucks with route planning still on paper.",
    "typed",
  ),
  row("common_objections", "What do they push back on?", "", "empty"),
  row(
    "sales_motion",
    "How do deals happen?",
    "Free route audit, then a 90-day pilot on one depot.",
    "quoted",
    cited("/product", "We start with a free route audit."),
  ),
];

const READ_FULL: ProfileDigestRead = {
  root_url: SITE,
  pages: [
    { url: `${SITE}/`, status: "fetched", kind: "home" },
    { url: `${SITE}/about`, status: "fetched", kind: "about" },
    { url: `${SITE}/product`, status: "fetched", kind: "services" },
    { url: `${SITE}/customers`, status: "fetched", kind: "other" },
    { url: `${SITE}/impressum`, status: "fetched", kind: "impressum" },
    { url: `${SITE}/team`, status: "fetched", kind: "team" },
  ],
  facts: [
    {
      category: "company",
      field: "founded_year",
      value: "2014",
      value_key: "founded_year:2014",
      evidence_snippet: "Founded in 2014.",
      evidence_url: `${SITE}/about`,
      confidence: 0.91,
    },
    {
      category: "company",
      field: "employee_range",
      value: "50-100",
      value_key: "employee_range:50-100",
      evidence_snippet: "A team of around eighty.",
      evidence_url: `${SITE}/about`,
      confidence: 0.72,
    },
    {
      category: "company",
      field: "location",
      value: "Munich",
      value_key: "location:munich",
      evidence_snippet: "Our head office is in Munich.",
      evidence_url: `${SITE}/about`,
      confidence: 0.85,
    },
    {
      category: "company",
      field: "location",
      value: "Rotterdam",
      value_key: "location:rotterdam",
      evidence_snippet: "A second desk in Rotterdam covers the ports.",
      evidence_url: `${SITE}/team`,
      confidence: 0.63,
    },
    {
      category: "signal",
      field: "certification",
      value: "ISO 9001",
      value_key: "certification:iso-9001",
      evidence_snippet: "ISO 9001 certified since 2019.",
      evidence_url: `${SITE}/about`,
      confidence: 0.8,
    },
    {
      category: "market",
      field: "named_customer",
      value: "Nordfracht",
      value_key: "named_customer:nordfracht",
      evidence_snippet: "Nordfracht runs their whole depot on Acme.",
      evidence_url: `${SITE}/customers`,
      confidence: 0.58,
    },
  ],
  people: [
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
  ],
  legal_entities: [
    {
      name: "Acme Freight GmbH",
      registered_address: "Lindwurmstraße 12, 80337 Munich",
      register_number: "HRB 123456",
      vat_number: "DE123456789",
      evidence_snippet: "Acme Freight GmbH, HRB 123456, Amtsgericht München.",
      source_url: `${SITE}/impressum`,
    },
  ],
};

function Digest({
  rows,
  read,
}: Readonly<{ rows: readonly ReviewRow[]; read?: ProfileDigestRead }>) {
  return (
    <StoryProviders>
      <div className="ob-scene">
        <ProfileDigest
          rows={rows}
          read={read}
          onSettle={() => {}}
          onField={() => {}}
        />
      </div>
    </StoryProviders>
  );
}

const meta: Meta<typeof Digest> = {
  title: "Onboarding/Profile digest",
  component: Digest,
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj<typeof Digest>;

// The deck's own narrow companion: a flat list of record lines, the field
// being decided marked in it, no sections, no sidebar.
export const Companion: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: "26rem" }}>
        <ProfileDigest
          rows={OPEN_ROWS}
          active="usp"
          identity={{ rootUrl: SITE }}
          onReadWhole={() => {}}
        />
      </div>
    </StoryProviders>
  ),
};

// The whole-record document with lines still open — `usp` (required), the
// two legal register lines and two customer lines (advisory) — each drawn as
// its own dashed row with "Settle it".
export const OpenItemsPresent: Story = {
  render: () => <Digest rows={OPEN_ROWS} read={READ_FULL} />,
};

// Every line filled: the header's second figure reads zero and drops the
// warn colour, and no dashed row remains in the article.
export const NoneOpen: Story = {
  render: () => {
    const filled = OPEN_ROWS.map((line) =>
      line.value.trim() === ""
        ? {
            ...line,
            value: `${line.label} — settled in the pilot call.`,
            state: "typed" as const,
          }
        : line,
    );
    return <Digest rows={filled} read={READ_FULL} />;
  },
};

// A read that came back thin: no facts, no legal entities, one page. The
// sidebar keeps only the pairs a record field backs (Legal name,
// Headquarters, Industry, Website) and drops Founded, Offices, Employees and
// Certifications entirely rather than printing them empty.
export const SidebarValuesMissing: Story = {
  render: () => (
    <Digest
      rows={OPEN_ROWS}
      read={{
        root_url: SITE,
        pages: [{ url: `${SITE}/`, status: "fetched", kind: "home" }],
        facts: [],
        people: [],
        legal_entities: [],
      }}
    />
  ),
};

// The kind of each of the ten pages the ManyReferences story cites, in the
// same order they are listed below.
const MANY_PAGE_KINDS: Readonly<
  Record<string, "home" | "about" | "impressum" | "team" | "contact" | "other">
> = {
  "/": "home",
  "/about": "about",
  "/impressum": "impressum",
  "/team": "team",
  "/contact": "contact",
};

// A dense read: many distinct pages, several cited from more than one line,
// for a References list long enough to show the numbering holding together.
export const ManyReferences: Story = {
  render: () => {
    const pages = [
      "/",
      "/about",
      "/product",
      "/customers",
      "/impressum",
      "/team",
      "/services",
      "/contact",
      "/careers",
      "/blog/route-planning",
    ];
    const read: ProfileDigestRead = {
      ...READ_FULL,
      pages: pages.map((path) => ({
        url: `${SITE}${path}`,
        status: "fetched" as const,
        kind: MANY_PAGE_KINDS[path] ?? "other",
      })),
      facts: [
        ...READ_FULL.facts,
        {
          category: "offering",
          field: "product",
          value: "Route optimiser",
          value_key: "product:route-optimiser",
          evidence_snippet: "Our route optimiser replans a depot in minutes.",
          evidence_url: `${SITE}/services`,
          confidence: 0.77,
        },
        {
          category: "signal",
          field: "partner",
          value: "Fleetbase",
          value_key: "partner:fleetbase",
          evidence_snippet: "Built on Fleetbase's telematics feed.",
          evidence_url: `${SITE}/blog/route-planning`,
          confidence: 0.51,
        },
      ],
    };
    return <Digest rows={OPEN_ROWS} read={read} />;
  },
};
