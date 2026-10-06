// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { TableScroll } from "./atoms";

const meta: Meta<typeof TableScroll> = {
  title: "Components/Text and data display/Table scroll",
  component: TableScroll,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof TableScroll>;

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul"];

function InvoiceCounts({ columns }: Readonly<{ columns: readonly string[] }>) {
  return (
    <table className="table">
      <thead>
        <tr>
          <th scope="col">Account</th>
          {columns.map((column) => (
            <th key={column} scope="col">
              {column}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {["Acme GmbH", "Globex AG"].map((account) => (
          <tr key={account}>
            <th scope="row">{account}</th>
            {columns.map((column) => (
              <td key={column} className="t-num">
                3
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  );
}

// A table that fits takes no tab stop: most tables fit, and every keyboard
// reader would pay for the few that do not.
export const Fits: Story = {
  render: () => (
    <div style={{ maxInlineSize: "28rem" }}>
      <TableScroll label="Recent invoices">
        <InvoiceCounts columns={MONTHS.slice(0, 2)} />
      </TableScroll>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("table");
    await expect(canvas.queryByRole("region")).toBeNull();
  },
};

export const ScrollsSideways: Story = {
  render: () => (
    <div style={{ maxInlineSize: "20rem" }}>
      <TableScroll label="Recent invoices">
        <InvoiceCounts columns={MONTHS} />
      </TableScroll>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const region = await within(canvasElement).findByRole("region", {
      name: "Recent invoices",
    });
    await expect(region).toHaveAttribute("tabindex", "0");
  },
};
