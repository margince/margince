// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { Panel, PanelBody } from "../design-system/panel";
import { FactRow } from "./companyfactrow";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";
// The row wears the contact page's meta-row shape, which lives with the
// composed primitives rather than with this screen — so a row drawn on its own
// has to load it the way the facts panel above it does.
import "../design-system/composed.css";

// One stated or read fact, on the row the facts panel is built out of.
//
// The three frames are about PROVENANCE and WRITE STANDING rather than about
// which field a fact names: a value a colleague typed, a value a site read with
// the sentence it came from, and a value whose shape contradicts the field it
// was filed under all draw a different meta line and a different verb column —
// and a reader who may not correct any of them sees the same facts with the
// verbs gone.

type CompanyFact = components["schemas"]["CompanyFact"];

const COMPANY = "01a04298-1971-7076-8076-8064da20fdff";

function fact(over: Partial<CompanyFact> = {}): CompanyFact {
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
  } as CompanyFact;
}

const siteRead = fact({
  id: "01a04298-2000-7000-8000-000000000002",
  field: "employee_range",
  value: "201-500",
  value_key: "201-500",
  source: "site_read",
  captured_by: "margince",
  evidence_snippet: "Over 300 contacts across four sites in Bavaria.",
  source_url: "https://brandt-automotive.example/about",
  confidence: 0.82,
  retrieved_at: "2026-08-29T04:12:00Z",
});

const misfiled = fact({
  id: "01a04298-2000-7000-8000-000000000003",
  field: "location",
  value: "+49 89 1234 5678",
  value_key: "49891234567",
  source: "site_read",
  captured_by: "margince",
  suspect_reason: "phone_shaped_location",
  evidence_snippet: "Ingolstadt · +49 89 1234 5678",
  retrieved_at: "2026-08-29T04:12:00Z",
});

function Rows({
  facts,
  canEdit,
}: Readonly<{ facts: readonly CompanyFact[]; canEdit: boolean }>) {
  installFetchStub({
    "GET /me": meRoute({ company: ["read", "update"] }),
  });
  return (
    <StoryProviders>
      <div style={{ maxWidth: 560 }}>
        <Panel title="What we know">
          <PanelBody>
            {facts.map((one) => (
              <FactRow
                key={one.id}
                companyId={COMPANY}
                fact={one}
                label={one.field === "founded_year" ? "Founded" : undefined}
                canEdit={canEdit}
                onOpenHistory={() => {}}
              />
            ))}
          </PanelBody>
        </Panel>
      </div>
    </StoryProviders>
  );
}

const meta: Meta = {
  title: "Records/Company record/Fact row",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// A colleague's word and a site read, side by side: the first names the human
// who stated it, the second carries the sparkle, the snippet and the verdict
// verbs that let a reader disagree with what was read.
export const StatedAndRead: Story = {
  render: () => <Rows facts={[fact(), siteRead]} canEdit />,
};

// A phone number filed as a location. The fact is still shown with its
// evidence — hiding it would be the worse answer — with the badge saying what
// contradicts what, because the reader is the one who can tell.
export const ValueContradictsItsField: Story = {
  render: () => <Rows facts={[misfiled]} canEdit />,
};

// The same facts for a reader who may not correct them: the meta line and the
// evidence stay, the verdict verbs and removal go.
export const ReadOnly: Story = {
  render: () => <Rows facts={[fact(), siteRead]} canEdit={false} />,
};
