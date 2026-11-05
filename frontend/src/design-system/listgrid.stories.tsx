// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { Badge } from "./atoms";
import { CellStrip } from "./listgrid";
import { type ListColumn, ListTable } from "./listtable";

// The grid a list table draws, through the one part a column renderer takes
// from it: `CellStrip`, a cell's values on one line.
const meta: Meta = {
  title: "Components/Text and data display/List grid",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

type Deal = { id: string; name: string; quiet: string };

const deals: Deal[] = [
  { id: "d1", name: "Fleet retrofit", quiet: "three weeks and two days" },
  { id: "d2", name: "Harbour lighting", quiet: "two days" },
];

const columns: ListColumn<Deal>[] = [
  { key: "name", header: "Deal", fixed: true, cell: (deal) => deal.name },
  {
    key: "signal",
    header: "Last signal",
    numeric: true,
    cell: (deal) => (
      <CellStrip>
        <span>{deal.quiet}</span>
        <Badge tone="warning">Stalled</Badge>
      </CellStrip>
    ),
  },
];

/**
 * A strip in a numeric column. A strip ends in an ellipsis by design, so its
 * text is no figure: the column keeps its default width and the strip is cut.
 */
export const StripInANumericColumn: Story = {
  render: () => (
    <LocaleProvider initial="en">
      <div style={{ maxWidth: "480px" }}>
        <ListTable
          rows={deals}
          columns={columns}
          rowKey={(deal) => deal.id}
          unit="deals"
        />
      </div>
    </LocaleProvider>
  ),
  play: async ({ canvasElement }) => {
    const cell = await within(canvasElement).findByRole("cell", {
      name: /three weeks and two days/,
    });
    const quiet = within(cell).getByText(/three weeks and two days/);
    await waitFor(() =>
      expect(quiet.scrollWidth).toBeGreaterThan(quiet.clientWidth),
    );
    // No floor widened the column to the text it cuts.
    await expect(cell.getBoundingClientRect().width).toBeLessThan(
      quiet.scrollWidth,
    );
  },
};
