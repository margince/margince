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
// The retired entry stays listed with its switch off, because deals still carry
// its key. Its menu holds Rename alone: there is no delete.

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
    deal_count: 0,
    version: 1,
    created_at: "2026-08-01T08:00:00Z",
    updated_at: "2026-08-01T08:00:00Z",
    ...extra,
  };
}

const SOURCES = {
  data: [
    source("inbound", "Inbound", { deal_count: 14 }),
    source("outbound", "Outbound", { deal_count: 6 }),
    source("referral", "Referral", { deal_count: 1 }),
    source("partner", "Partner", { deal_count: 3 }),
    source("event", "Event"),
    source("existing_customer", "Existing customer"),
    source("employee_referral", "Employee referral"),
    source("other", "Other"),
    // Added by this workspace and since withdrawn. Still listed, because deals
    // booked while it was live still carry the key.
    source("roadshow", "Roadshow", {
      system: false,
      active: false,
      deal_count: 2,
    }),
  ],
};

// A seat that may not read deals is sent no count, never a zero.
const UNCOUNTED = {
  data: SOURCES.data.map((row) => ({ ...row, deal_count: undefined })),
};

const DUPLICATE = {
  type: "about:blank",
  title: "Conflict",
  status: 409,
  code: "conflict",
  detail: "conflict",
};

const ADMIN = { custom_field: ["read", "create", "update", "delete"] } as const;
const READER = { custom_field: ["read"] } as const;

function story(
  allow: Parameters<typeof meRoute>[0],
  options: Readonly<{ counted?: boolean; refuseName?: boolean }> = {},
) {
  const { counted = true, refuseName = false } = options;
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /acquisition-sources": () =>
        jsonResponse(counted ? SOURCES : UNCOUNTED),
      ...(refuseName && {
        "POST /acquisition-sources": () => jsonResponse(DUPLICATE, 409),
      }),
    });
    return (
      <StoryProviders>
        <AcquisitionSourcesCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof AcquisitionSourcesCard> = {
  title: "Settings/Sales/Acquisition sources/Sources",
  component: AcquisitionSourcesCard,
};
export default meta;
type Story = StoryObj<typeof AcquisitionSourcesCard>;

export const Admin: Story = { render: story(ADMIN) };

// Every control refused, the list still readable: the page is a report to a
// holder who may not change it, not a wall.
export const Reader: Story = { render: story(READER) };

// No deal count came back, so each row says so rather than reading as zero.
export const WithoutDealCounts: Story = {
  render: story(READER, { counted: false }),
};

// At 390 each row folds: name and key over the count and the switch.
export const AdminPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(ADMIN),
};

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

export const RenamingSource: Story = {
  render: story(ADMIN),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Actions for Referral" }),
    );
    const page = within(canvasElement.ownerDocument.body);
    await userEvent.click(page.getByRole("button", { name: "Rename" }));
    await page.findByRole("dialog");
  },
};

const addADuplicate: Story["play"] = async ({ canvasElement }) => {
  const canvas = within(canvasElement);
  await userEvent.click(
    await canvas.findByRole("button", { name: "New source" }),
  );
  const page = within(canvasElement.ownerDocument.body);
  const dialog = within(await page.findByRole("dialog"));
  await userEvent.type(dialog.getByLabelText("Label"), "Partner{Enter}");
  await dialog.findByText(
    "A source with this name or key already exists. Choose another name.",
  );
};

// The server refuses a name whose key is taken: the dialog says so on the field.
export const DuplicateSource: Story = {
  render: story(ADMIN, { refuseName: true }),
  play: addADuplicate,
};

export const DuplicateSourcePhone: Story = {
  render: story(ADMIN, { refuseName: true }),
  play: addADuplicate,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
