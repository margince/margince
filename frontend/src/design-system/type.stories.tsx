// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";

/**
 * Root type: one declaration, and everything under it inherits.
 *
 * `--fs-base`, `--lh-base` and `--fw-base` live in `tokens.css`, and `app.css`'s
 * `html` rule is the only place they are read. Size, leading, weight, face and
 * neutral ink all arrive from there, so a heading, a label, a field, a code
 * sample and a figure are the same text until a role rule says otherwise — and
 * no role rule exists yet. The ramp this page used to draw is gone.
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

/**
 * A paragraph, a heading, a label, a code sample and a `.t-num` figure in a
 * plain `<div>`, with no rule styling any of them. Every one reads the root,
 * which is why they all look alike — that sameness IS what this story asserts.
 */
export const Root: Story = {
  render: () => (
    <div style={column}>
      <div style={line}>
        <span style={key}>p</span>
        <p>
          Margince keeps the record: what was agreed, who agreed it, and when it
          changed.
        </p>
      </div>
      <div style={line}>
        <span style={key}>h2</span>
        <h2>Globex renewal</h2>
      </div>
      <div style={line}>
        <span style={key}>label · input</span>
        <label>
          Close date <input defaultValue="31 Mar 2026" />
        </label>
      </div>
      <div style={line}>
        <span style={key}>code</span>
        <code>deals.update</code>
      </div>
      <div style={line}>
        <span style={key}>.t-num</span>
        <span className="t-num">€1,284,500.00</span>
      </div>
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
