import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { JurisdictionReading } from "./settings.jurisdiction";
import { StoryProviders, stubWithSession } from "./story-utils";

// Settings → Governance → Jurisdiction. The reading is rendered directly: every
// state worth looking at is a state of the jurisdiction, and stubbing the query
// to reach it would put the fetch on trial instead of the page.

type Jurisdiction = components["schemas"]["InstallationJurisdiction"];

const SIX_YEARS = {
  name: "commercial_correspondence",
  keep: "P6Y",
  from_year_end: true,
};

const AVAILABLE: Jurisdiction["available"] = [
  {
    code: "de",
    unit_name: "de",
    unit_version: "1.0.0",
    description:
      "German jurisdiction pack: the statutory retention floors and the outbound-messaging rules.",
    correspondence_floor: SIX_YEARS,
  },
  {
    code: "vn",
    unit_name: "vn",
    unit_version: "1.0.0",
    description: "Vietnamese jurisdiction pack: the advertising-email rules.",
  },
];

const GDPR_DEADLINES: Jurisdiction["deadlines"] = (
  ["access", "rectify", "erasure"] as const
).map((kind) => ({ kind, rule: "gdpr", period: "P1M" }));

const UNSET: Jurisdiction = {
  selected: null,
  available: AVAILABLE,
  floor: {
    known: true,
    class: "commercial_correspondence",
    keep: "P6Y",
    from_year_end: true,
    packs: ["de", "vn"],
  },
  deadlines: GDPR_DEADLINES,
  documents: [],
};

const GERMANY: Jurisdiction = {
  ...UNSET,
  selected: "de",
  unit: AVAILABLE[0],
  retention_classes: [
    SIX_YEARS,
    { name: "accounting_records", keep: "P8Y", from_year_end: true },
  ],
  messaging: { version: 1, instruments: [] },
  documents: [
    {
      id: "mitarbeiterinformation",
      title: "Information for staff on mail capture",
      language: "de",
      audience: "employee",
      kind: "template",
      media_type: "text/markdown",
      size_bytes: 18_240,
    },
  ],
};

const VIETNAM_CARRIED: Jurisdiction = {
  ...UNSET,
  selected: "vn",
  unit: AVAILABLE[1],
  retention_classes: [],
  floor: {
    known: false,
    from_year_end: false,
    packs: ["de", "vn"],
    carried: {
      class: "commercial_correspondence",
      keep: "P6Y",
      from_year_end: true,
      before: "2026-10-02T09:00:00Z",
      binds_until: "2033-01-01T00:00:00Z",
    },
  },
  messaging: {
    version: 2,
    instruments: [
      { name: "Decree 91/2020/ND-CP", effective_from: "2020-10-01" },
    ],
  },
};

function story(jurisdiction: Jurisdiction) {
  return () => {
    stubWithSession({}, { installation_settings: ["read", "update"] });
    return (
      <StoryProviders>
        <JurisdictionReading jurisdiction={jurisdiction} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof JurisdictionReading> = {
  title: "Settings/Governance/Jurisdiction/Page",
  component: JurisdictionReading,
};
export default meta;
type Story = StoryObj<typeof JurisdictionReading>;

/** No jurisdiction set: the GDPR baseline and the strictest compiled-in floor. */
export const NotSet: Story = { render: story(UNSET) };
export const NotSetDark: Story = { ...NotSet, globals: { theme: "dark" } };

/** Germany, with its retention classes, rule set and one document. */
export const Germany: Story = { render: story(GERMANY) };
export const GermanyDark: Story = { ...Germany, globals: { theme: "dark" } };

/** Vietnam after a change from unset: no floor of its own, the old one carried. */
export const CarriedFloor: Story = { render: story(VIETNAM_CARRIED) };
export const CarriedFloorDark: Story = {
  ...CarriedFloor,
  globals: { theme: "dark" },
};
