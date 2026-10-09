// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { AuditLogCard } from "./settings-audit";
import type { AuditLogEntry } from "./settings-audit.format";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

const VIEWER = "u-ada";

const ago = (minutes: number) =>
  new Date(Date.now() - minutes * 60_000).toISOString();

const base = {
  passport_id: null,
  on_behalf_of: null,
  on_behalf_of_name: null,
  actor_name: null,
  evidence: null,
  before: null,
  after: null,
  entity_label: null,
};

const ENTRIES: AuditLogEntry[] = [
  {
    ...base,
    id: "al-1",
    actor_type: "agent",
    actor_id: "agent:sdr",
    passport_id: "01a11ea4-9c2e-7d10-a1b2-3c4d5e6f7a8b",
    on_behalf_of: "u-lars",
    on_behalf_of_name: "Lars Brandt",
    action: "update",
    entity_type: "deal",
    entity_id: "01a11ea4-0eed-704d-b84d-89ffa4435b4a",
    entity_label: "Demo GmbH renewal 2027",
    before: { stage: "proposal", amount_minor: 4_200_000 },
    after: { stage: "negotiation", amount_minor: 4_800_000 },
    authorization_rule: "role[rep] deal.update row_scope=own",
    evidence: { snippet: "Reply confirmed budget", source: "email:msg-1" },
    occurred_at: ago(4),
  },
  {
    ...base,
    id: "al-2",
    actor_type: "human",
    actor_id: `human:${VIEWER}`,
    actor_name: "Ada Brandt",
    action: "create",
    entity_type: "onboarding_wizard_state",
    entity_id: "01a11ea4-37ab-720f-89a7-9008eb3f4280",
    after: { step: "complete", settings: { voice: false, connect: "later" } },
    authorization_rule:
      "role[individual] onboarding_wizard_state.create row_scope=own",
    occurred_at: ago(95),
  },
  {
    ...base,
    id: "al-3",
    actor_type: "human",
    actor_id: "human:u-dana",
    actor_name: "Dana Kessler",
    action: "delete",
    entity_type: "contact",
    entity_id: "01a11ea4-1286-7719-8719-3921cae3b269",
    before: { full_name: "Priya Shah", email: "priya@demo.example" },
    authorization_rule: "role[admin] contact.delete row_scope=all",
    occurred_at: ago(60 * 26),
  },
  {
    ...base,
    id: "al-4",
    actor_type: "connector",
    actor_id: "connector:gmail",
    on_behalf_of: "u-lars",
    on_behalf_of_name: "Lars Brandt",
    action: "import",
    entity_type: "activity",
    entity_id: "01a11ea4-10cb-7576-ad76-0e5a3f9bbfae",
    entity_label: "Re: Renewal terms for Demo GmbH",
    after: { kind: "email", subject: "Re: Renewal terms for Demo GmbH" },
    authorization_rule: "role[rep] activity.create row_scope=own",
    occurred_at: ago(60 * 24 * 3),
  },
  {
    ...base,
    id: "al-5",
    actor_type: "system",
    actor_id: "system",
    action: "expire",
    entity_type: "retention_policy",
    entity_id: "01a11ea4-36cd-707d-9d91-4a0b46440707",
    before: { state: "active" },
    after: { state: "expired" },
    authorization_rule: "system",
    occurred_at: ago(60 * 24 * 40),
  },
];

const LONG: AuditLogEntry = {
  ...base,
  id: "al-long",
  actor_type: "agent",
  actor_id: "agent:01a01740-c9c2-736d-a0b6-d3e3dcb13111",
  on_behalf_of: "u-max",
  on_behalf_of_name: "Maximiliane Theodora von Hohenzollern-Sigmaringen",
  action: "advance_stage",
  entity_type: "company",
  entity_id: "01a11ea4-362b-7411-b206-496f5055ce62",
  entity_label:
    "Brandt Industrieanlagen und Sondermaschinenbau GmbH und Co. Kommanditgesellschaft",
  before: { description: "Short note." },
  after: {
    description:
      "A long description that keeps going well past the width of the column so the value has to wrap onto several lines without pushing the table sideways.",
    custom_fields: {
      procurement_contact: "einkauf@brandt-industrieanlagen.example",
      framework_agreement_reference: "FA-2026-000123-DACH-SONDERMASCHINEN",
    },
  },
  authorization_rule:
    "role[manager,custom_regional_lead] company.update row_scope=team",
  occurred_at: ago(12),
};

type Answer = () => Response | Promise<Response>;

const page =
  (data: AuditLogEntry[], next: string | null = null): Answer =>
  () =>
    jsonResponse({ data, page: { next_cursor: next, has_more: !!next } });

const ADMIN: GrantSpec = { audit_log: ["read"] };

function story(auditLog: Answer, allow: GrantSpec = ADMIN) {
  return () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse({
          ...meFixture({ roles: ["admin"], allow }),
          user: { ...meFixture().user, id: VIEWER, display_name: "Ada Brandt" },
        }),
      "GET /audit-log": auditLog,
    });
    return (
      <StoryProviders>
        <AuditLogCard />
      </StoryProviders>
    );
  };
}

const openFirst = async ({ canvasElement }: { canvasElement: HTMLElement }) => {
  const canvas = within(canvasElement);
  const [toggle] = await canvas.findAllByRole("button", {
    name: /^Show change detail: /,
  });
  await userEvent.click(toggle);
};

const meta: Meta<typeof AuditLogCard> = {
  title: "Settings/Governance/Audit log/Recorded actions",
  component: AuditLogCard,
};
export default meta;
type Story = StoryObj<typeof AuditLogCard>;

// Every actor kind and action tone, labelled and unlabelled targets. Rows: an
// agent for a member, the viewer, a delete, a connector import, a system expiry.
export const Trail: Story = { render: story(page(ENTRIES)) };

export const Expanded: Story = {
  render: story(page(ENTRIES)),
  play: openFirst,
};

export const CreatedExpanded: Story = {
  render: story(page([ENTRIES[1]])),
  play: openFirst,
};

export const LongContent: Story = {
  render: story(page([LONG, ...ENTRIES.slice(0, 2)])),
  play: openFirst,
};

export const MorePages: Story = {
  render: story(page(ENTRIES.slice(0, 3), "c-2")),
};

export const FiltersOpen: Story = {
  render: story(page(ENTRIES)),
  play: async ({ canvasElement }) => {
    await userEvent.click(await within(canvasElement).findByText("Filters"));
  },
};

export const Empty: Story = { render: story(page([])) };

export const Loading: Story = {
  render: story(() => new Promise<Response>(() => {})),
};

export const Failed: Story = {
  render: story(() =>
    jsonResponse({ title: "Upstream is down", status: 500 }, 500),
  ),
};

export const Withheld: Story = { render: story(page(ENTRIES), {}) };

export const TrailDark: Story = {
  globals: { theme: "dark" },
  render: story(page(ENTRIES)),
  play: openFirst,
};

// Under 36rem the entry folds: target and toggle on line one, when, who and
// the verb under them.
export const TrailPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(page([LONG, ...ENTRIES])),
};

export const ExpandedPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(page(ENTRIES)),
  play: openFirst,
};
