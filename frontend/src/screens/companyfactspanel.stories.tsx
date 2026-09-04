// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { CompanyFactsPanel } from "./companyfactspanel";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

type OrganizationFact = components["schemas"]["OrganizationFact"];

// What we know about an account, and where each claim came from.
//
// The frames are about PROVENANCE and WRITE STANDING rather than about how many
// rows arrived: a fact a person typed, a fact a site read produced with a
// snippet behind it, and a value whose shape contradicts the field it was filed
// under all draw differently, and a reader who may not correct any of them sees
// the same facts with the verbs gone and one sentence saying why.
//
// The technical fields are deliberately absent from every frame: the technical
// card claims them off the same endpoint, and a fact drawn twice on one tab is
// a reader correcting one copy and watching the other disagree.
//
// Read every frame in BOTH themes with the toolbar's Theme control.

const ORG = "01a04298-1971-7076-8076-8064da20fdff";

function fact(over: Partial<OrganizationFact> = {}): OrganizationFact {
  return {
    id: "01a04298-2000-7000-8000-000000000001",
    category: "company",
    field: "founded_year",
    value: "1998",
    value_key: "1998",
    source: "human",
    captured_by: "Demo Admin",
    evidence_snippet: null,
    source_url: null,
    confidence: null,
    suspect_reason: null,
    retrieved_at: null,
    verified_at: null,
    verified_by: null,
    updated_at: "2026-08-30T10:00:00Z",
    version: 1,
    ...over,
  } as OrganizationFact;
}

const FACTS: readonly OrganizationFact[] = [
  fact(),
  fact({
    id: "01a04298-2000-7000-8000-000000000002",
    field: "employee_range",
    value: "201-500",
    value_key: "201-500",
    source: "site_read",
    captured_by: "margince",
    evidence_snippet: "Over 300 people across four sites in Bavaria.",
    source_url: "https://koreapartner.example/about",
    confidence: 0.82,
    retrieved_at: "2026-08-29T04:12:00Z",
  }),
  fact({
    id: "01a04298-2000-7000-8000-000000000003",
    category: "offering",
    field: "service",
    value: "Conveyor commissioning",
    value_key: "conveyor-commissioning",
    source: "connector",
    captured_by: "margince",
    confidence: 0.44,
  }),
  fact({
    id: "01a04298-2000-7000-8000-000000000004",
    category: "market",
    field: "served_industry",
    value: "Automotive tier one",
    value_key: "automotive-tier-one",
    source: "site_read",
    captured_by: "margince",
    confidence: 0.91,
    verified_at: "2026-08-31T08:00:00Z",
    verified_by: "Demo Admin",
  }),
];

/** A phone number filed as a location: the row is well-formed and still wrong.
 *  It is FLAGGED rather than hidden — a heuristic that dropped data would be
 *  worse than one that points at it. */
const SUSPECT: OrganizationFact = fact({
  id: "01a04298-2000-7000-8000-000000000005",
  field: "location",
  value: "+49 89 1234 5678",
  value_key: "49-89-1234-5678",
  source: "site_read",
  captured_by: "margince",
  suspect_reason: "phone_shaped_location",
  confidence: 0.6,
});

function frame(
  facts: readonly OrganizationFact[],
  canEdit: boolean,
  reasonId?: string,
) {
  const routes: RouteMap = {
    "GET /me": meRoute({}),
    [`GET /organizations/${ORG}/facts`]: () => jsonResponse({ data: facts }),
  };
  installFetchStub(routes);
  return (
    <StoryProviders>
      {reasonId && (
        <p id={reasonId} className="t-caption">
          Your seat may read this account and not correct it.
        </p>
      )}
      <CompanyFactsPanel orgId={ORG} canEdit={canEdit} reasonId={reasonId} />
    </StoryProviders>
  );
}

const meta: Meta<typeof CompanyFactsPanel> = {
  // Not "Facts": `companyfacts.stories.tsx` holds that node, and it documents
  // the STANDING box in the record header — a different card about a different
  // question. This one is the panel of captured claims.
  title: "Records/Company 360/Facts about this company",
  component: CompanyFactsPanel,
};
export default meta;

type Story = StoryObj<typeof CompanyFactsPanel>;

/** The ordinary card: four claims across three categories, each carrying who
 *  captured it and how sure we are. */
export const Default: Story = {
  render: () => frame(FACTS, true),
};

/** A value that contradicts the field it was filed under, beside the claims
 *  that do not. */
export const WithASuspectValue: Story = {
  render: () => frame([...FACTS, SUSPECT], true),
};

/** Nothing captured yet. The sentence is the card's answer to "there is none",
 *  in the same voice every other card gives it. */
export const Empty: Story = {
  render: () => frame([], true),
};

/** A reader who may not write. The facts stay — the read is granted — and the
 *  verbs point at ONE sentence saying why, rather than each carrying its own
 *  copy of it. */
export const ReadOnly: Story = {
  render: () => frame(FACTS, false, "facts-no-write"),
};
