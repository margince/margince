// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import type { GrantSpec } from "../app/mefixture";
import { CompanyHeaderActions } from "./companyheaderactions";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";

// The account header's verbs on their own: write, then the pair that records
// what already happened. What the stories hold is where a refusal lands — a
// caption under the row, never a sentence that widens it.
const meta: Meta = {
  title: "Records/Company 360/Header actions",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Company = components["schemas"]["Company"];

const company: Company = {
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
  writable: true,
  version: 1,
};

function Row({
  record = company,
  activity = true,
}: Readonly<{ record?: Company; activity?: boolean }>) {
  const grants: GrantSpec = activity
    ? { company: ["read", "update"], activity: ["create"] }
    : { company: ["read", "update"] };
  installFetchStub({ "GET /me": meRoute(grants) });
  return (
    <StoryProviders>
      {/* The header's own row class, so the verbs and any caption sit at the
          interval and measure the record page gives them. */}
      <div className="record-actions record-actions-inline">
        <CompanyHeaderActions
          company={record}
          composerOpen={false}
          onComposerOpen={() => undefined}
          drawer={null}
          onDrawer={() => undefined}
        />
      </div>
    </StoryProviders>
  );
}

export const Live: Story = { render: () => <Row /> };

// A role without the activity grant: both log verbs point at one sentence.
export const LogRefused: Story = { render: () => <Row activity={false} /> };

// An archived account: every verb here refuses on the record's state, and the
// sentence is the component's own when the page has not stated it.
export const Archived: Story = {
  render: () => (
    <Row record={{ ...company, archived_at: "2026-07-13T00:00:00Z" }} />
  ),
};
