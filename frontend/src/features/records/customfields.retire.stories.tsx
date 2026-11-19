// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen } from "storybook/test";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { RetireFieldConfirm } from "./customfields.retire";

type CustomField = components["schemas"]["CustomField"];

const field: CustomField = {
  id: "01a0f000-0000-7000-8000-0000000000f1",
  object: "contact",
  label: "Loyalty band",
  slug: "loyalty_band",
  type: "text",
  status: "active",
  column_name: "cf_loyalty_band",
  show_in_create: true,
  reportable: false,
  created_by: "00000000-0000-4000-8000-000000000001",
  created_at: "2026-09-01T00:00:00Z",
  updated_at: "2026-09-01T00:00:00Z",
  version: 1,
};

function retire(lists: readonly object[], unseenCount = 0) {
  installFetchStub({
    [`GET /custom-fields/${field.id}/lists`]: () =>
      jsonResponse({ lists, unseen_count: unseenCount }),
  });
  return (
    <StoryProviders>
      <RetireFieldConfirm
        field={field}
        onClose={() => undefined}
        onRetired={() => undefined}
      />
    </StoryProviders>
  );
}

const meta = {
  title: "Settings/Sales/Records and fields/Retire field",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

/** No Live List filters on the field: the confirm says so and archiving is open. */
export const NoLists: Story = {
  render: () => retire([]),
  play: async () => {
    await screen.findByRole("dialog", { name: /Loyalty band/ });
    await expect(
      await screen.findByText("No Live List filters on this field."),
    ).toBeInTheDocument();
  },
};

/** Lists that filter on the field are named, plus a count of lists the reader cannot see. */
export const FieldInUse: Story = {
  render: () =>
    retire(
      [
        { id: "l1", name: "Gold band customers", sharing: "team" },
        { id: "l2", name: "Renewals this quarter", sharing: "private" },
      ],
      2,
    ),
  play: async () => {
    await expect(await screen.findByText("Gold band customers")).toBeInTheDocument();
    await expect(screen.getByText("Renewals this quarter")).toBeInTheDocument();
  },
};
