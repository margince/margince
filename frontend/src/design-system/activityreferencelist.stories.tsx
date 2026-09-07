// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";

import type { components } from "../api/schema";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { StoryProviders } from "../screens/story-utils";
import { ActivityReferenceList } from "./activityreferencelist";
import { Card } from "./atoms";

// The receipts behind a derived number.
//
// The states worth a picture are the KINDS, because they are what decides
// whether a row can be opened: an email takes the product's one email citation
// and opens the drawer, everything else is prose with no page to go to. The
// withheld row is the third, and the one a reader will ask about.

type ActivityReference = components["schemas"]["ActivityReference"];

const email: ActivityReference = {
  activity_id: "a-1",
  kind: "email",
  subject: "Depot slot confirmed",
  occurred_at: "2026-09-01T09:00:00Z",
  content_state: "available",
};

const call: ActivityReference = {
  activity_id: "a-2",
  kind: "call",
  subject: "Rang about the retrofit",
  occurred_at: "2026-08-28T14:30:00Z",
  content_state: "available",
};

const withheld: ActivityReference = {
  activity_id: "a-3",
  kind: "email",
  subject: null,
  occurred_at: "2026-08-20T08:00:00Z",
  content_state: "withheld",
};

const meta = {
  title: "Design System/ActivityReferenceList",
  component: ActivityReferenceList,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Card>
          <Story />
        </Card>
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof ActivityReferenceList>;

export default meta;
type Story = StoryObj<typeof meta>;

// Every kind at once, which is how a real score's receipts arrive.
export const Mixed: Story = {
  args: {
    references: [email, call, withheld],
    onOpenEmail: () => {},
    formatWhen: (when) => formatDateTime(when, "en", viewerZone()),
  },
};

// The host mounts no drawer, so the message is named and not pressable — the
// same arrangement every other surface that cites mail uses.
export const NoOpener: Story = {
  args: {
    references: [email, call],
    formatWhen: (when) => formatDateTime(when, "en", viewerZone()),
  },
};

// One receipt. The list draws the same row whether it stands alone or in ten.
export const Single: Story = {
  args: {
    references: [email],
    onOpenEmail: () => {},
    formatWhen: (when) => formatDateTime(when, "en", viewerZone()),
  },
};
