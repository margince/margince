// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor, within } from "storybook/test";
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
    const table = await canvas.findByRole("table");
    await expect(canvas.queryByRole("region")).toBeNull();
    const box = table.parentElement;
    await expect(box).not.toBeNull();
    if (box) {
      await expect(endShade(box)).toBe("0px");
    }
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

const USAGE_COLUMNS = ["Calls", "Input", "Output", "Cached", "Estimated cost"];

function ModelUsage() {
  return (
    <table className="table">
      <thead>
        <tr>
          <th scope="col">Model</th>
          {USAGE_COLUMNS.map((column) => (
            <th key={column} scope="col" className="t-num">
              {column}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {["claude-sonnet", "claude-haiku"].map((model) => (
          <tr key={model}>
            <th scope="row">{model}</th>
            {USAGE_COLUMNS.map((column) => (
              <td key={column} className="t-num">
                1,204,310
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  );
}

const endShade = (region: HTMLElement) =>
  getComputedStyle(region, "::after").width;

// On a phone the end edge shades while columns lie past it, so a reader knows
// the table scrolls.
export const EndEdgeOnPhone: Story = {
  render: () => (
    <div style={{ padding: "var(--padCard)" }}>
      <TableScroll label="Usage by model" stickyFirst>
        <ModelUsage />
      </TableScroll>
    </div>
  ),
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    const region = await within(canvasElement).findByRole("region", {
      name: "Usage by model",
    });
    await waitFor(() => expect(endShade(region)).not.toBe("0px"));
  },
};

// Scrolled to the end, nothing lies past the edge, so the shade is gone.
export const ScrolledToTheEnd: Story = {
  ...EndEdgeOnPhone,
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    const region = await within(canvasElement).findByRole("region", {
      name: "Usage by model",
    });
    region.scrollLeft = region.scrollWidth;
    await waitFor(() => expect(endShade(region)).toBe("0px"));
  },
};

// The shade is drawn above the cells, so it holds against the dark plate and a
// hovered row alike.
export const EndEdgeOnPhoneDark: Story = {
  ...EndEdgeOnPhone,
  globals: { viewport: { value: "phone" }, theme: "dark" },
  tags: ["uat-phone"],
};

// A table with no width of its own, as a Markdown table has, keeps the width
// of its content rather than stretching across the box.
export const NarrowTableKeepsItsWidth: Story = {
  render: () => (
    <div style={{ inlineSize: "28rem" }}>
      <TableScroll label="Tiers">
        <table>
          <thead>
            <tr>
              <th scope="col">Tier</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td>Premium</td>
            </tr>
          </tbody>
        </table>
      </TableScroll>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const table = await within(canvasElement).findByRole("table");
    const box = table.parentElement;
    await expect(box).not.toBeNull();
    if (box) {
      await expect(table.getBoundingClientRect().width).toBeLessThan(
        box.getBoundingClientRect().width / 2,
      );
    }
  },
};
