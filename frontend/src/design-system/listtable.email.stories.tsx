// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { ListTable } from "./listtable";

// A generic component takes no `component` here: Storybook would have to infer
// `Row` from nothing to derive the args table.
const meta: Meta = {
  title: "Components/Text and data display/List table email column",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

type ContactRow = { id: string; name: string; email: string; city: string };

const LISTED: readonly ContactRow[] = Array.from({ length: 6 }, (_, index) => ({
  id: `c${index + 1}`,
  name: `Contact ${index + 1}`,
  email: `contact-${index + 60}@harbour-logistics.example.com`,
  city: index % 2 === 0 ? "Hamburg" : "Rotterdam",
}));

// An address column takes room beside the name before the plain columns grow,
// so it ends wider than a plain column of the same row.
export const EmailBesideTheName: Story = {
  render: () => (
    <ListTable<ContactRow>
      rows={[...LISTED]}
      columns={[
        { key: "name", header: "Name", fixed: true, cell: (row) => row.name },
        {
          key: "email",
          header: "Email",
          holds: "email",
          cell: (row) => row.email,
        },
        { key: "city", header: "City", cell: (row) => row.city },
        { key: "tags", header: "Tags", cell: () => null },
      ]}
      rowKey={(row) => row.id}
      unit="contacts"
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const email = await canvas.findByRole("columnheader", { name: /Email/ });
    const tags = canvas.getByRole("columnheader", { name: /Tags/ });
    await expect(email.getBoundingClientRect().width).toBeGreaterThan(
      tags.getBoundingClientRect().width,
    );
  },
};
