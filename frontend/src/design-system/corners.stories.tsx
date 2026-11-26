// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";

/**
 * The corner ladder, in fours: `--r-xs` 4, `--r-sm` 8, `--r-full` the pill, and
 * `--r-control`, `--r-md` and `--r-lg`, which read `--r-sm` so a control, a card
 * and a pane round alike. `census/rules/corners.census.ts` fails a
 * `border-radius` under `src/` that names none of them.
 */
const meta = {
  title: "Foundations/Radius",
  parameters: { layout: "centered" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

const RUNGS = [
  {
    token: "--r-xs",
    size: "4px",
    role: "a keycap, a scan tile, a 2px rule's own cap",
  },
  {
    token: "--r-sm",
    size: "8px",
    role: "a chip, a nav row, a tooltip",
  },
  {
    token: "--r-control",
    size: "8px",
    role: "a button, an input, the select trigger",
  },
  {
    token: "--r-md",
    size: "8px",
    role: "a board card, the agent's row",
  },
  {
    token: "--r-lg",
    size: "8px",
    role: "a pane, the details panel, a reading card",
  },
  {
    token: "--r-full",
    size: "9999px",
    role: "a pill, a monogram, a count",
  },
];

const column: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "var(--space-4)",
  minWidth: "460px",
};
const row: CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: "var(--space-4)",
};
const swatch = (radius: string): CSSProperties => ({
  width: "84px",
  height: "56px",
  flex: "none",
  borderRadius: radius,
  background: "var(--accentLight)",
  border: "1px solid var(--accentMed)",
});

/** The six rungs, drawn at the size a component would use them. */
export const TheLadder: Story = {
  render: () => (
    <div style={column}>
      {RUNGS.map((rung) => (
        <div key={rung.token} style={row}>
          <div style={swatch(`var(${rung.token})`)} />
          <div>
            <div>{rung.token}</div>
            <div>
              {rung.size} · {rung.role}
            </div>
          </div>
        </div>
      ))}
    </div>
  ),
};
