// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { DataTable } from "../design-system/datatable";
import { BarFigure, CountLink, columnScale } from "./analytics.cells";
import { StoryProviders } from "./story-utils";

// The cells every report table is drawn from: a count that opens the set it
// names, and a figure with its bar in one cell, the bar on the column's own
// scale. The third row has no figure and draws no bar, which would read as
// zero.
const meta: Meta = { title: "Records/Reports/Report cells" };
export default meta;

type Story = StoryObj;

type DemoRow = {
  id: string;
  name: string;
  deals: number;
  value: number | null;
  part: number | null;
  text: string;
};

const ROWS: DemoRow[] = [
  {
    id: "a",
    name: "Qualify",
    deals: 14,
    value: 96_400,
    part: 19_280,
    text: "€96,400.00",
  },
  {
    id: "b",
    name: "Proposal sent",
    deals: 7,
    value: 88_500,
    part: 53_100,
    text: "€88,500.00",
  },
  { id: "c", name: "Unpriced", deals: 2, value: null, part: null, text: "—" },
];

export const FigureWithItsBar: Story = {
  render: () => {
    const scale = columnScale(ROWS.map((row) => row.value));
    return (
      <StoryProviders>
        <DataTable
          label="Open deals by stage"
          columns={[
            {
              key: "name",
              header: "Stage",
              render: (row: DemoRow) => row.name,
            },
            {
              key: "deals",
              header: "Open deals",
              align: "end",
              render: (row: DemoRow) => (
                <CountLink
                  count={row.deals}
                  href="#/deals"
                  title={`Open the deals in ${row.name}`}
                />
              ),
            },
            {
              key: "value",
              header: "Unweighted",
              align: "end",
              grow: true,
              render: (row: DemoRow) => (
                <BarFigure
                  value={row.value}
                  part={row.part}
                  scale={scale}
                  label={row.name}
                >
                  {row.text}
                </BarFigure>
              ),
            },
          ]}
          rows={ROWS}
          rowKey={(row) => row.id}
        />
      </StoryProviders>
    );
  },
};
