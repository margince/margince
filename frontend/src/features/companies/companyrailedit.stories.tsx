// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { FieldGrid, FieldRow } from "../../design-system/fieldgrid";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { type CompoundField, CompoundFieldValue } from "./companyrailedit";

// A company value made of several inputs or a search: the reading opens the
// record's edit form for that one field.

const meta: Meta = {
  title: "Records/Company 360/Compound field",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;
type Company = components["schemas"]["Company"];

const PARENT = "o-parent";

const company: Company = {
  writable: true,
  id: "o-1",
  workspace_id: "w-1",
  display_name: "Brandt Automotive GmbH",
  parent_company_id: PARENT,
  address: {
    line1: "Werkstraße 12",
    line2: null,
    city: "Munich",
    region: "Bavaria",
    postal_code: "80331",
    country: "DE",
  },
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

function Row({
  which,
  label,
  reading,
  link,
  canEdit = true,
}: Readonly<{
  which: CompoundField;
  label: string;
  reading?: ReactNode;
  link?: boolean;
  canEdit?: boolean;
}>) {
  installFetchStub({
    "GET /companies": () =>
      jsonResponse({
        data: [{ id: PARENT, display_name: "Brandt Holding AG" }],
        page: { has_more: false, next_cursor: null },
      }),
  });
  return (
    <StoryProviders>
      <div style={{ maxWidth: 340 }}>
        <FieldGrid>
          <FieldRow label={label}>
            <CompoundFieldValue
              company={company}
              which={which}
              label={label}
              reading={reading}
              link={link}
              canEdit={canEdit}
              readOnlyReason="This company is archived."
            />
          </FieldRow>
        </FieldGrid>
      </div>
    </StoryProviders>
  );
}

export const Address: Story = {
  render: () => <Row which="address" label="Address" reading="Munich, DE" />,
};

export const Unset: Story = {
  render: () => <Row which="address" label="Address" />,
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByRole("button", {
        name: "Change Address Not set",
      }),
    ).toBeVisible();
  },
};

// The parent is a link to its record; the pill around it opens the search.
export const ParentLink: Story = {
  render: () => (
    <Row
      which="parent_company_id"
      label="Parent company"
      reading={<EntityRef kind="company" id={PARENT} />}
      link
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("link", { name: /Brandt Holding AG/ }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: /^Change Parent company/ }),
    ).toBeVisible();
  },
};

// A reader who may not edit sees the reading plain, with the reason on hover.
export const ReadOnly: Story = {
  render: () => (
    <Row which="address" label="Address" reading="Munich, DE" canEdit={false} />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("Munich, DE")).toBeVisible();
    await expect(canvas.queryByRole("button")).toBeNull();
  },
};

export const AddressDialog: Story = {
  render: () => <Row which="address" label="Address" reading="Munich, DE" />,
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    await user.click(
      await within(canvasElement).findByRole("button", {
        name: "Change Address Munich, DE",
      }),
    );
    const dialog = await screen.findByRole("dialog", { name: "Address" });
    await waitFor(() =>
      expect(within(dialog).getByDisplayValue("Werkstraße 12")).toBeVisible(),
    );
  },
};

export const AddressDialogPhone: Story = {
  ...AddressDialog,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
