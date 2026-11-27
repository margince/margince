// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor, within } from "storybook/test";
import type { components } from "../../../api/schema";
import { en as englishCopy } from "../../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../../testing/story-utils";
import { ArchivedDealBand } from "./dealrestore";

// The band over an archived deal: why it takes no changes, and the way back
// for a reader who may archive deals.

type Deal = components["schemas"]["Deal"];

const DEAL_ID = "01a03000-0000-7000-8000-000000000002";

const archived = {
  id: DEAL_ID,
  name: "Fleet telematics rollout",
  status: "open",
  source: "manual",
  version: 3,
  writable: false,
  archived_at: "2026-09-01T00:00:00Z",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-09-01T00:00:00Z",
} as Deal;

const meta: Meta = {
  title: "Records/Deal 360/Archived band",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const band = (
  grants: Parameters<typeof meRoute>[0],
  undoable: Record<string, unknown> = { undoable: true },
) => {
  installFetchStub({
    "GET /me": meRoute(grants),
    [`GET /records/deal/${DEAL_ID}/history`]: () =>
      jsonResponse({
        data: [
          {
            id: "audit-archive",
            action: "archive",
            occurred_at: "2026-09-01T00:00:00Z",
            undoable,
          },
        ],
        page: { next_cursor: null, has_more: false },
      }),
  });
  return (
    <StoryProviders>
      <ArchivedDealBand
        deal={archived}
        reason="This deal is archived and takes no changes."
        reasonId="deal-archived"
      />
    </StoryProviders>
  );
};

/** A reader who may archive deals: the sentence, and Restore deal beside it. */
export const MayRestore: Story = {
  render: () => band({ deal: ["read", "update", "delete"] }),
  play: async ({ canvasElement }) => {
    const restore = await within(canvasElement).findByRole("button", {
      name: englishCopy["deal.restore"],
    });
    await waitFor(() => expect(restore).toBeEnabled());
  },
};

/** A reader who may only read deals: the sentence alone. */
export const SentenceOnly: Story = {
  render: () => band({ deal: ["read"] }),
};

/** A rep who may edit the deal, when a manager made the archive: the reason, no button. */
export const ArchivedBySomeoneElse: Story = {
  render: () =>
    band(
      { deal: ["read", "update"] },
      { undoable: false, reason: "restore_archiver_required" },
    ),
};
