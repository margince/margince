// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { LeadActions } from "./leads.actions";
import { useLeadPatch } from "./leads.patch";

type Lead = components["schemas"]["Lead"];

const meta = {
  title: "Records/Leads/Verbs",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;

type Story = StoryObj;

const LEAD: Lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  email: "jonas@nordwind.example",
  phone: "+49 40 1234 5678",
  company_name: "Nordwind Logistik",
  title: "Head of Fleet",
  status: "contacted",
  score: 72,
  source: "manual",
  captured_by: "human:u1",
  writable: true,
  version: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

// The page's one lead write is a hook, so it is raised inside the providers.
function Verbs() {
  const writer = useLeadPatch(LEAD, LEAD.id, () => {});
  return (
    <div style={{ display: "flex", gap: "var(--gapActions)" }}>
      <LeadActions
        lead={LEAD}
        id={LEAD.id}
        writer={writer}
        onQualify={() => {}}
        onDisqualify={() => {}}
        onLogged={() => {}}
        terminalReasonId="lead-terminal"
        emailOpen={false}
        onEmailOpenChange={() => {}}
      />
    </div>
  );
}

// A live lead's header verbs. Log activity opens the drawer on this lead.
export const Live: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute(
        { lead: ["read", "update"], activity: ["read", "create"] },
        { seat: "full" },
      ),
      "GET /leads/l-1": () => jsonResponse(LEAD),
    });
    return (
      <StoryProviders>
        <Verbs />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const log = await within(canvasElement).findByRole("button", {
      name: en["log.title"],
    });
    await waitFor(() => expect(log).toBeEnabled());
    await userEvent.click(log);
    const drawer = await within(document.body).findByRole("dialog", {
      name: en["log.title"],
    });
    await waitFor(() => expect(drawer).toBeVisible());
  },
};
