// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { AmbientWaves } from "./ambient-waves";

// The ground behind the first-run welcome, never on its own. A canvas with
// nothing over it says nothing about the one thing this component exists to
// prove: that it stays BEHIND a host's own content rather than painting over
// it. Every story here reproduces the two declarations `.auth-surface`
// carries for exactly this (`position: relative; isolation: isolate;`,
// `auth.css`) so a reader sees the real contract, not a simplified one that
// happens to work in Storybook and fails on the screen it is copied from.
//
// WATCH IT RATHER THAN READ THE CAPTURE. The field moves too slowly for a
// still frame to show it moving at all: that slowness is deliberate (see
// `ambient-waves-shader.ts`, "WHY IT SAYS NOTHING"). What a capture DOES show
// is the two themes and that the copy over it stays legible in both.
const meta = {
  title: "Signed out/Ambient waves",
  component: AmbientWaves,
  parameters: { layout: "padded" },
} satisfies Meta<typeof AmbientWaves>;
export default meta;

type Story = StoryObj<typeof meta>;

// The host contract in full: a box with a definite height, opening its own
// stacking context, with a greeting-shaped cluster of copy sitting on top of
// the ground rather than beside it, which is the composition first run
// actually uses.
export const BehindAWelcome: Story = {
  render: () => (
    <div
      style={{
        position: "relative",
        isolation: "isolate",
        display: "grid",
        placeItems: "center",
        height: "420px",
        borderRadius: "var(--radius-lg)",
        border: "1px solid var(--borderSubtle)",
        background: "var(--bgElevated)",
        textAlign: "center",
        padding: "var(--space-6)",
      }}
    >
      <div style={{ display: "grid", gap: "var(--space-3)", maxWidth: "32ch" }}>
        <h2 style={{ margin: 0, fontSize: "var(--fs-h2)" }}>
          Welcome to Margince
        </h2>
        <p style={{ margin: 0, color: "var(--textSecondary)" }}>
          Your agent reads what you already do and stages what it finds.
        </p>
      </div>
      <AmbientWaves />
    </div>
  ),
};

// The minimum host a caller can get away with: one short line, no card, no
// composition, which is the case that catches the ground reading as an image
// placed on the page instead of light behind a line of text.
export const BehindAShortLine: Story = {
  render: () => (
    <div
      style={{
        position: "relative",
        isolation: "isolate",
        display: "grid",
        placeItems: "center",
        height: "220px",
        background: "var(--bgElevated)",
      }}
    >
      <p style={{ margin: 0, fontSize: "var(--fs-lead)" }}>
        Reading your first fortnight of activity.
      </p>
      <AmbientWaves />
    </div>
  ),
};
