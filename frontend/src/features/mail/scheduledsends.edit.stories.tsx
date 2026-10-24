// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, waitFor, within } from "storybook/test";
import { viewerZone } from "../../format/timezone";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import type { ScheduledSend } from "./scheduledqueue";
import { EditComposer } from "./scheduledsends.edit";

// Edit opens from the message's own read; these are the answers that keep the
// composer shut and say why in its place.
const meta: Meta = {
  title: "Records/Scheduled message edit",
};
export default meta;

type Story = StoryObj;

const message: ScheduledSend = {
  id: "019f7e65-fbf7-7114-b114-40af4af63ae8",
  status: "scheduled",
  scheduled_at: new Date(Date.now() + 86_400_000).toISOString(),
  scheduled_tz: viewerZone(),
  subject: "The renewal quote",
  body: "Hi, attached is the renewal quote.",
  to: ["ceo@acme.example"],
  links: [{ entity_type: "company", entity_id: "company-1" }],
  version: 3,
  created_at: "2026-08-20T09:00:00Z",
  updated_at: "2026-08-20T09:00:00Z",
};

const detailRoute = `GET /scheduled-sends/${message.id}`;

function Editing() {
  return (
    <StoryProviders>
      <EditComposer send={message} onClose={() => {}} onReplaced={() => {}} />
    </StoryProviders>
  );
}

/** A frozen file the send would now refuse: the edit cannot carry it or drop
 *  it unseen, so the message is withdrawn and written again instead. */
export const FilesRefused: Story = {
  render: () => {
    installFetchStub({
      [detailRoute]: () =>
        jsonResponse({ ...message, attachments_unavailable: 2 }),
    });
    return <Editing />;
  },
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    const title = await dialog.findByText("Message not editable");
    await waitFor(() => expect(title).toBeVisible());
  },
};

export const FilesRefusedPhone: Story = {
  ...FilesRefused,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

/** The message's own read failed: nothing opens until a retry lands. */
export const LoadFailed: Story = {
  render: () => {
    installFetchStub({
      [detailRoute]: () =>
        jsonResponse(
          { type: "about:blank", title: "Service Unavailable", status: 503 },
          503,
        ),
    });
    return <Editing />;
  },
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    const retry = await dialog.findByRole("button", { name: "Retry" });
    await waitFor(() => expect(retry).toBeVisible());
  },
};

/** The message went or changed after the list was read: nothing to edit. */
export const Stale: Story = {
  render: () => {
    installFetchStub({
      [detailRoute]: () => jsonResponse({ ...message, status: "sent" }),
    });
    return <Editing />;
  },
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    const title = await dialog.findByText("Message changed");
    await waitFor(() => expect(title).toBeVisible());
  },
};

/** The message's own read is still out: the composer waits in its place. */
export const Loading: Story = {
  render: () => {
    installFetchStub({ [detailRoute]: () => new Promise<Response>(() => {}) });
    return <Editing />;
  },
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await expect(await dialog.findByRole("status")).toBeInTheDocument();
  },
};

export const LoadingPhone: Story = {
  ...Loading,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const StalePhone: Story = {
  ...Stale,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
