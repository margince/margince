import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { meFixture } from "../app/mefixture";
import { AuditLogCard } from "./settings.auditlog";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The audit trail as a table: an agent's change made for the viewer, a
// person's change on a record with a page, a scheduled system write on a
// governance record, and an erase that reads in the danger tone.
const meta: Meta<typeof AuditLogCard> = {
  title: "Settings/Governance/Audit log/Audit trail",
  component: AuditLogCard,
};
export default meta;

type Story = StoryObj<typeof AuditLogCard>;

const auditLogPage = {
  data: [
    {
      id: "al-1",
      actor_type: "agent",
      actor_id: "agent:sdr",
      passport_id: "pp-9",
      on_behalf_of: "u-1",
      on_behalf_of_name: "Me",
      action: "update",
      entity_type: "contact",
      entity_id: "p-1",
      before: { stage: "new" },
      after: { stage: "qualified" },
      authorization_rule: "role:admin",
      evidence: { snippet: "Reply confirmed budget", source: "email:msg-1" },
      occurred_at: "2026-07-10T09:00:00Z",
    },
    {
      id: "al-2",
      actor_type: "human",
      actor_id: "human:u-2",
      actor_name: "Rep Two",
      action: "create",
      entity_type: "deal",
      entity_id: "d-1",
      before: null,
      after: { name: "Globex Renewal" },
      authorization_rule: "role[rep] deal.create",
      occurred_at: "2026-07-10T08:42:00Z",
    },
    {
      id: "al-3",
      actor_type: "system",
      actor_id: "system:scheduler",
      action: "update",
      entity_type: "forecast_capture_status",
      entity_id: "01a0fbe7-3c1d-7e2a-9b44-5d6e7f809a1b",
      before: { last_attempt_at: "2026-07-10T07:47:14Z" },
      after: { last_attempt_at: "2026-07-10T08:47:03Z" },
      authorization_rule: "system",
      occurred_at: "2026-07-10T08:47:03Z",
    },
    {
      id: "al-4",
      actor_type: "human",
      actor_id: "human:u-1",
      action: "erase",
      entity_type: "privacy_notice_case",
      entity_id: "01a0f7d7-1b2c-7d3e-8f40-516273849500",
      before: null,
      after: null,
      authorization_rule: "role:admin",
      occurred_at: "2026-07-09T16:22:00Z",
    },
  ],
  page: { next_cursor: "c-2", has_more: true },
};

// The grants the card asks for: `useCan("audit_log", "read")` is what draws
// the toolbar. The id stays `u-1`, the actor `on_behalf_of` names, so the
// agent's entry reads as the viewer's own.
const auditLogMe = () =>
  jsonResponse({
    ...meFixture({ roles: ["admin"], allow: { audit_log: ["read"] } }),
    user: { ...meFixture().user, id: "u-1", display_name: "Me" },
  });

function auditLogCard() {
  return () => {
    globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
    installFetchStub({
      "GET /me": auditLogMe,
      "GET /audit-log": () => jsonResponse(auditLogPage),
      "GET /users": () =>
        jsonResponse({
          data: [{ id: "u-2", display_name: "Rep Two" }],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /contacts/p-1": () =>
        jsonResponse({ id: "p-1", full_name: "Priya Shah" }),
      "GET /deals/d-1": () =>
        jsonResponse({ id: "d-1", name: "Globex Renewal" }),
    });
    return (
      <StoryProviders>
        <AuditLogCard />
      </StoryProviders>
    );
  };
}

export const AuditLog: Story = { render: auditLogCard() };

export const AuditLogDark: Story = {
  globals: { theme: "dark" },
  render: auditLogCard(),
};

// One change opened beside the table: who, what changed, why it was allowed,
// and the technical ids folded at the foot.
async function openFirstChange({
  canvasElement,
}: Readonly<{ canvasElement: HTMLElement }>) {
  const canvas = within(canvasElement);
  const [first] = await canvas.findAllByRole("button", {
    name: "Open details for this change",
  });
  await userEvent.click(first);
}

export const AuditLogDetail: Story = {
  name: "Audit log, a change opened",
  render: auditLogCard(),
  play: openFirstChange,
};

export const AuditLogDetailDark: Story = {
  name: "Audit log, a change opened (dark)",
  globals: { theme: "dark" },
  render: auditLogCard(),
  play: openFirstChange,
};
