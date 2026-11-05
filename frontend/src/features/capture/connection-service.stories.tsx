// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { StoryProviders } from "../../testing/story-utils";
import { AccountServices } from "./connection-service";

const meta: Meta = { title: "Settings/You/Connections/Sync evidence" };
export default meta;
type Story = StoryObj;
type Connection = components["schemas"]["CaptureConnection"];

const mail: Connection = {
  id: "demo-mailbox",
  provider: "imap",
  status: "connected",
  scopes: [],
  can_backfill: false,
  last_synced_at: "2026-08-24T12:00:00Z",
  last_success_at: "2026-08-23T12:00:00Z",
  last_sync_error_class: "unreachable",
};

function services(connection: Connection) {
  return () => (
    <StoryProviders>
      <AccountServices
        account={{ key: "imap", mail: connection, calendar: null }}
        vendor={null}
        label="Mailbox services"
        heldWindow={null}
        connectPendingFor={undefined}
        onConnect={() => undefined}
        onImport={() => undefined}
      />
    </StoryProviders>
  );
}

export const FailedAttemptAfterSuccess: Story = { render: services(mail) };
export const AttemptWithoutSuccess: Story = {
  render: services({ ...mail, last_success_at: null }),
};

export const SuccessTimeNotSupplied: Story = {
  render: services({ ...mail, last_success_at: undefined }),
};
