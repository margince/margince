// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";

/**
 * Root type: one declaration, and everything under it inherits.
 *
 * Every size, leading and paragraph spacing is rem, on the browser's own
 * 1rem = 16px, so a reader who enlarges that moves the whole document. The
 * three levels live in `tokens.css` as `font` shorthands — `--fontBodyLarge`,
 * `--fontBody`, `--fontSmall` — each paired with the paragraph spacing that
 * belongs to it, and `app.css` declares the middle pair on `body`. Size,
 * leading, weight, face and neutral ink all arrive from there, so a heading, a
 * label, a field, a code sample and a figure are the same text until a role
 * rule says otherwise — and no role rule exists yet.
 *
 * Flip the theme in the toolbar: nothing here changes size, but the ink does.
 */
const meta = {
  title: "Design System/Type",
  parameters: { layout: "centered" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

const column: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "var(--space-4)",
  minWidth: "520px",
};
const line: CSSProperties = {
  display: "flex",
  alignItems: "baseline",
  gap: "var(--space-4)",
};
const key: CSSProperties = {
  minWidth: "170px",
  flex: "none",
};

const levels: CSSProperties = {
  display: "flex",
  alignItems: "flex-start",
  gap: "var(--space-8)",
};
const levelColumn: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "var(--space-2)",
  maxWidth: "240px",
};

/**
 * Declared rather than asserted onto `CSSProperties`: React's own type carries
 * the CSS properties it knows, and a cast to it would say this one is among
 * them. A level that is not the root's own has to hand its paragraph spacing
 * down as well, because the sibling rule in `app.css` reads one name.
 */
type TypeLevel = CSSProperties &
  Readonly<{
    "--paragraphSpacing"?: string;
  }>;

const LEVELS: ReadonlyArray<{ token: string; style: TypeLevel }> = [
  {
    token: "--fontBodyLarge",
    style: {
      font: "var(--fontBodyLarge)",
      "--paragraphSpacing": "var(--paragraphSpacingLarge)",
    },
  },
  { token: "--fontBody", style: { font: "var(--fontBody)" } },
  {
    token: "--fontSmall",
    style: {
      font: "var(--fontSmall)",
      "--paragraphSpacing": "var(--paragraphSpacingSmall)",
    },
  },
];

/**
 * The three levels side by side, each drawn as two paragraphs. No rule here
 * sets a size: a column wears one `font` shorthand and the gap between its two
 * paragraphs arrives from the spacing token paired with it, so what separates
 * prose is the prose's own level rather than a margin chosen at the call site.
 */
export const Root: Story = {
  render: () => (
    <div style={levels}>
      {LEVELS.map((level) => (
        <div key={level.token} style={levelColumn}>
          <span>{level.token}</span>
          <div style={level.style}>
            <p>
              Margince keeps the record: what was agreed, who agreed it, and
              when it changed.
            </p>
            <p>
              The gap above this line is the paragraph spacing of the level the
              column wears.
            </p>
          </div>
        </div>
      ))}
    </div>
  ),
};

const FIGURES = ["€1,284,500.00", "€48,000.00", "€711.10", "€9,999.99"];

const figureColumn: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  alignItems: "flex-end",
};

/**
 * Mono is for code, and only for code. A column of figures lines up in the body
 * face through `.t-num` (`font-variant-numeric: tabular-nums`) — the mono face
 * is never the way to align digits. The column beside it is what a figure
 * without `.t-num` does: every digit its own width, so the column drifts. Code
 * — and only an element that IS code — takes the code face, from one base rule.
 * `design-system/mono.test.ts` and `check-font-lock.sh` fail mono anywhere else.
 */
export const MonoIsForCode: Story = {
  render: () => (
    <div style={column}>
      <div style={line}>
        <span style={key}>.t-num · tabular digits</span>
        <div style={figureColumn}>
          {FIGURES.map((figure) => (
            <span key={figure} className="t-num">
              {figure}
            </span>
          ))}
        </div>
        <span style={key}>not this · proportional</span>
        <div style={figureColumn}>
          {FIGURES.map((figure) => (
            <span key={figure}>{figure}</span>
          ))}
        </div>
      </div>
      <div style={line}>
        <span style={key}>an id · body face</span>
        <span>psp_7Q3fa91</span>
      </div>
      <div style={line}>
        <span style={key}>code · samp</span>
        <span>
          Call <code>deals.update</code> and expect <samp>HTTP 200</samp>
        </span>
      </div>
      <div style={line}>
        <span style={key}>pre.code-block</span>
        <pre className="code-block">{'{ "stage": "proposal" }'}</pre>
      </div>
    </div>
  ),
};
