// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";

/**
 * The corner ladder, and the squircle that rides on it.
 *
 * Six rungs, in fours: `--r-xs` 4, `--r-sm` 8, `--r-control` 12, `--r-md` 16,
 * `--r-lg` 20, `--r-full` the pill. Every `border-radius` under `src/` names one
 * of them — the tree ran 3, 5, 6, 9, 10, 11 and 28px beside the tokens before
 * this was one ladder, and `design-system/corners.test.ts` now fails a seventh
 * value rather than leaving it to review.
 *
 * `tokens.css` also declares `corner-shape: squircle` on `:where(*)` inside an
 * `@supports` query, and DOUBLES each finite rung there: a superellipse of
 * radius R reads about as round as a circular corner of R/2, so a corner keeps
 * its apparent size and gives up the two points where a circular arc meets the
 * straight edge. Nothing at a call site opts in.
 *
 * A genuinely round thing opts OUT, beside its own radius:
 *
 * ```css
 * .badge {
 *   border-radius: var(--r-full);
 *   corner-shape: round;
 *  }
 * ```
 *
 * `:where(*)` carries no specificity, so that one line always wins. Write it
 * wherever you write `--r-full` or `50%`: a superellipse at full radius is a
 * lozenge, and on an avatar a squircle tile. The last story below shows both,
 * which is the comparison the rule exists for.
 *
 * In an engine without `corner-shape` every box here falls back to the plain
 * rung and nothing moves — that is what the doubling buys.
 */
const meta = {
  title: "Design System/Corners",
  parameters: { layout: "centered" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

const RUNGS = [
  {
    token: "--r-xs",
    plain: "4px",
    smooth: "8px",
    role: "a keycap, a scan tile, a 2px rule's own cap",
  },
  {
    token: "--r-sm",
    plain: "8px",
    smooth: "16px",
    role: "a chip, a nav row, a tooltip",
  },
  {
    token: "--r-control",
    plain: "12px",
    smooth: "24px",
    role: "a button, an input, the select trigger",
  },
  {
    token: "--r-md",
    plain: "16px",
    smooth: "32px",
    role: "a board card, the agent's row",
  },
  {
    token: "--r-lg",
    plain: "20px",
    smooth: "40px",
    role: "a pane, the details panel, a reading card",
  },
  {
    token: "--r-full",
    plain: "9999px",
    smooth: "not doubled",
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
const label: CSSProperties = {
  fontSize: "11px",
  letterSpacing: "0.08em",
  textTransform: "uppercase",
  color: "var(--textMeta)",
};
const note: CSSProperties = {
  fontSize: "var(--fs-sm)",
  color: "var(--textSecondary)",
};
const swatch = (radius: string, shape?: "round"): CSSProperties => ({
  width: "84px",
  height: "56px",
  flex: "none",
  borderRadius: radius,
  ...(shape ? { cornerShape: shape } : {}),
  background: "var(--accentLight)",
  border: "1px solid var(--accentMed)",
});

/**
 * The six rungs, drawn at the size a component would use them. Under an engine
 * with `corner-shape` each of these is a superellipse of twice the plain value;
 * without one it is the plain value as a circular arc. Both read as the same
 * corner, which is the whole claim.
 */
export const TheLadder: Story = {
  render: () => (
    <div style={column}>
      {RUNGS.map((rung) => (
        <div key={rung.token} style={row}>
          <div
            style={swatch(
              `var(${rung.token})`,
              rung.token === "--r-full" ? "round" : undefined,
            )}
          />
          <div>
            <div style={{ fontWeight: "var(--fw-semibold)" }}>{rung.token}</div>
            <div style={note}>
              {rung.plain} · squircle {rung.smooth} · {rung.role}
            </div>
          </div>
        </div>
      ))}
    </div>
  ),
};

/**
 * What the doubling is for. Left: the rung as a circular corner, the shape this
 * product drew before. Right: the same rung doubled and drawn as a superellipse
 * — the size a reader perceives is the same, and the two points on each corner
 * are gone. Read them at the edge rather than at the middle: that is where a
 * circular arc meets the straight side at a visible angle.
 *
 * An engine without the property draws both boxes the same, and this story then
 * says only that the fallback is honest.
 */
export const RoundAgainstSquircle: Story = {
  render: () => (
    <div style={column}>
      <span style={label}>--r-lg · round 20px, squircle 40px</span>
      <div style={row}>
        <div
          style={{
            ...swatch("var(--r-lg)", "round"),
            width: "180px",
            height: "110px",
          }}
        />
        <div
          style={{
            ...swatch("var(--r-lg)"),
            width: "180px",
            height: "110px",
          }}
        />
      </div>
      <span style={note}>
        Left keeps `corner-shape: round`; right takes the house default.
      </span>
    </div>
  ),
};

/**
 * The rule a call site has to remember, and the defect it prevents. Both of
 * these ask for a full radius; only the left one cancels the squircle. Under an
 * engine that has the property the right one is a lozenge, and the avatar
 * beside it a squircle tile — neither is what a pill or a face means.
 */
export const RoundThingsOptOut: Story = {
  render: () => (
    <div style={column}>
      <span style={label}>corner-shape: round · missing</span>
      <div style={row}>
        <div
          style={{
            ...swatch("var(--r-full)", "round"),
            width: "120px",
            height: "28px",
          }}
        />
        <div
          style={{
            ...swatch("var(--r-full)"),
            width: "120px",
            height: "28px",
          }}
        />
        <div
          style={{ ...swatch("50%", "round"), width: "44px", height: "44px" }}
        />
        <div style={{ ...swatch("50%"), width: "44px", height: "44px" }} />
      </div>
      <span style={note}>
        Pill, pill without the opt-out, monogram, monogram without it.
      </span>
    </div>
  ),
};
