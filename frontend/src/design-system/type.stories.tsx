// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";

/**
 * The type ramp, and the tracking family that goes with it.
 *
 * Nine fixed rungs and two fluid ones, declared in `tokens.css`. Every
 * `font-size` under `src/` names one of them — the tree wrote 25 other values
 * beside the scale before this was one ramp, `12.5px` in 25 places and `11px`
 * in 22, none of them a size anybody had chosen. `design-system/type.test.ts`
 * fails a tenth value now, and reads inline `fontSize` in a component as well
 * as a stylesheet's.
 *
 * The rung is picked by ROLE, not by how big the text should look: a card title
 * is `--fs-h3` on every screen that has one, so the day the card title moves it
 * moves everywhere at once. Reading a rung off this page and hard-coding its
 * number is the drift the gate exists to catch.
 *
 * A `clamp()` is the ramp made fluid, and its ends are rungs too —
 * `clamp(var(--fs-h1), 2.2vw, var(--fs-display))` is the sign-in heading. A
 * clamp with a raw length at either end is a rung nobody declared wearing a
 * responsive coat, and fails the same way a bare `27px` does.
 *
 * Flip the theme in the toolbar: the ramp does not move, but the ink does, and
 * the eyebrow is the one rung whose legibility depends on both.
 */
const meta = {
  title: "Design System/Type",
  parameters: { layout: "centered" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

const RUNGS = [
  { token: "--fs-eyebrow", px: "10.5px", role: "uppercase kickers, a monogram" },
  { token: "--fs-meta", px: "12px", role: "counts, timestamps, provenance" },
  { token: "--fs-sm", px: "13px", role: "chips, table cells, helper text" },
  { token: "--fs-body", px: "13.5px", role: "the default reading size" },
  { token: "--fs-lead", px: "15px", role: "the paragraph under a heading; inputs" },
  { token: "--fs-h3", px: "17px", role: "a card title" },
  { token: "--fs-h2", px: "20px", role: "a section title" },
  { token: "--fs-h1", px: "24px", role: "a step title, a record's head" },
  { token: "--fs-display", px: "32px", role: "a full-viewport moment" },
];

const TRACKING = [
  { token: "--tracking-eyebrow", value: "0.08em", sample: "WAITING ON YOU" },
  { token: "--tracking-display", value: "-0.03em", sample: "Globex Renewal" },
  { token: "--tracking-normal", value: "0", sample: "Everything else" },
];

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
  fontSize: "var(--fs-meta)",
  fontFamily: "var(--f-mono)",
  color: "var(--textMeta)",
  minWidth: "170px",
  flex: "none",
};
const note: CSSProperties = {
  fontSize: "var(--fs-sm)",
  color: "var(--textSecondary)",
};

/** Each rung at its own size, with the role that picks it. */
export const TheRamp: Story = {
  render: () => (
    <div style={column}>
      {RUNGS.map((rung) => (
        <div key={rung.token} style={line}>
          <span style={key}>
            {rung.token} · {rung.px}
          </span>
          <span style={{ fontSize: `var(${rung.token})` }}>
            Margince keeps the record
          </span>
          <span style={note}>{rung.role}</span>
        </div>
      ))}
    </div>
  ),
};

/**
 * The two fluid rungs, for the first-run and signed-out surfaces where a
 * heading is sized against the viewport rather than against the rows beside it.
 * Resize the preview to see them move; they stop at a fixed rung either end.
 */
export const TheFluidRungs: Story = {
  render: () => (
    <div style={column}>
      <div style={line}>
        <span style={key}>--fs-display-fluid</span>
        <span style={{ fontSize: "var(--fs-display-fluid)" }}>
          Let’s set up your workspace
        </span>
      </div>
      <div style={line}>
        <span style={key}>--fs-hero-fluid</span>
        <span style={{ fontSize: "var(--fs-hero-fluid)" }}>Welcome back</span>
      </div>
      <span style={note}>
        clamp(24px, 2.6vw, 32px) and clamp(30px, 6vw, 46px).
      </span>
    </div>
  ),
};

/** The three trackings, on the faces each is drawn for. */
export const TheTrackingFamily: Story = {
  render: () => (
    <div style={column}>
      {TRACKING.map((entry) => (
        <div key={entry.token} style={line}>
          <span style={key}>
            {entry.token} · {entry.value}
          </span>
          <span
            style={{
              letterSpacing: `var(${entry.token})`,
              fontSize:
                entry.token === "--tracking-eyebrow"
                  ? "var(--fs-eyebrow)"
                  : "var(--fs-h2)",
              fontFamily:
                entry.token === "--tracking-display"
                  ? "var(--f-display)"
                  : "var(--f-body)",
            }}
          >
            {entry.sample}
          </span>
        </div>
      ))}
    </div>
  ),
};
