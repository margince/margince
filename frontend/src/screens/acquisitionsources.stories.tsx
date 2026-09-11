// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { AcquisitionSourcesCard } from "./acquisitionsources";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// Settings › the business channels a deal is attributed to.
//
// The state worth seeing beyond admin-versus-reader is the RETIRED entry: it
// stays in the list with its switch off, because deals still carry its key and
// a catalog that hid it would leave those deals naming something the settings
// page says does not exist. There is no delete verb here at all, which is the
// other thing a reader should be able to see at a glance.

function source(
  key: string,
  label: string,
  extra: Record<string, unknown> = {},
) {
  return {
    id: `acq-${key}`,
    key,
    label,
    sort_order: 10,
    active: true,
    system: true,
    version: 1,
    created_at: "2026-08-01T08:00:00Z",
    updated_at: "2026-08-01T08:00:00Z",
    ...extra,
  };
}

const SOURCES = {
  data: [
    source("inbound", "Inbound"),
    source("outbound", "Outbound"),
    source("referral", "Referral"),
    source("partner", "Partner"),
    source("event", "Event"),
    source("existing_customer", "Existing customer"),
    source("employee_referral", "Employee referral"),
    source("other", "Other"),
    // Added by this workspace and since withdrawn. Still listed, because deals
    // booked while it was live still carry the key.
    source("roadshow", "Roadshow", { system: false, active: false }),
  ],
};

const ADMIN = { custom_field: ["read", "create", "update", "delete"] } as const;
const READER = { custom_field: ["read"] } as const;

function story(allow: Parameters<typeof meRoute>[0]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /acquisition-sources": () => jsonResponse(SOURCES),
    });
    return (
      <StoryProviders>
        <AcquisitionSourcesCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof AcquisitionSourcesCard> = {
  title: "Settings/Sales/Acquisition sources",
  component: AcquisitionSourcesCard,
};
export default meta;
type Story = StoryObj<typeof AcquisitionSourcesCard>;

export const Admin: Story = { render: story(ADMIN) };

// Every control refused, the list still readable: the page is a report to a
// holder who may not change it, not a wall.
export const Reader: Story = { render: story(READER) };

export const AdminDark: Story = {
  globals: { theme: "dark" },
  render: story(ADMIN),
};

// The add form is a dialog behind the header verb rather than a row under the
// list, where its own label would have read as one of the sources.
export const AddingSource: Story = {
  render: story(ADMIN),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "New source" }),
    );
  },
};
