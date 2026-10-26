// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import type { GrantSpec } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ActivityFiles, ActivityFilesAction } from "./activityfiles";

// The files on one meeting or call. The activity panel lists them, and so does
// a history row's Files verb.

const meta: Meta = {
  title: "Records/Activity files",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Activity = components["schemas"]["Activity"];
type Attachment = components["schemas"]["Attachment"];

const FILES: Attachment[] = [
  {
    id: "att-price",
    entity_type: "activity",
    entity_id: "act-kickoff",
    filename: "price-list-2026.pdf",
    byte_size: 482_000,
    created_at: "2026-07-06T09:00:00Z",
    source: "upload",
    captured_by: "human:u1",
  },
  {
    id: "att-photo",
    entity_type: "activity",
    entity_id: "act-kickoff",
    filename: "whiteboard.jpg",
    byte_size: 2_300_000,
    created_at: "2026-07-06T09:05:00Z",
    source: "upload",
    captured_by: "human:u1",
  },
];

const KICKOFF = {
  writable: true,
  id: "act-kickoff",
  kind: "meeting",
  subject: "Kickoff with Brandt IT",
  occurred_at: "2026-07-06T09:00:00Z",
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-07-06T09:00:00Z",
  updated_at: "2026-07-06T09:00:00Z",
} as Activity;

function stub(
  data: Attachment[],
  allow: GrantSpec = { activity: ["read", "update"] },
) {
  installFetchStub({
    "GET /me": meRoute(allow),
    "GET /attachments": () =>
      jsonResponse({ data, page: { has_more: false, next_cursor: null } }),
  });
}

function Files({
  data,
  allow,
  archived,
  writable = true,
}: Readonly<{
  data: Attachment[];
  allow?: GrantSpec;
  archived?: boolean;
  writable?: boolean;
}>) {
  stub(data, allow);
  return (
    <StoryProviders>
      <div style={{ maxWidth: 560 }}>
        <ActivityFiles
          activityId="act-kickoff"
          writable={writable}
          archived={archived}
        />
      </div>
    </StoryProviders>
  );
}

export const Populated: Story = { render: () => <Files data={FILES} /> };
export const PopulatedDark: Story = {
  ...Populated,
  globals: { theme: "dark" },
};

export const Empty: Story = { render: () => <Files data={[]} /> };

// A read seat sees the files and is told why it cannot add one.
export const Refused: Story = {
  render: () => <Files data={FILES} allow={{ activity: ["read"] }} />,
};

// An archived activity keeps its files and takes no new one.
export const Archived: Story = {
  render: () => <Files data={FILES} archived />,
};

// The history row's verb, opened: the files read only once it is pressed.
export const RowAction: Story = {
  render: () => {
    stub(FILES);
    return (
      <StoryProviders>
        <ActivityFilesAction activity={KICKOFF} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: en["files.title"],
      }),
    );
    const dialog = await within(document.body).findByRole("dialog");
    const file = await within(dialog).findByRole("link", {
      name: /price-list-2026\.pdf/,
    });
    // The modal fades in, so the file is visible only once it has.
    await waitFor(() => expect(file).toBeVisible());
  },
};
export const RowActionDark: Story = {
  ...RowAction,
  globals: { theme: "dark" },
};
