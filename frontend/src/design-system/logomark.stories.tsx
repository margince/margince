// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Logomark } from "./logomark";

const meta: Meta<typeof Logomark> = {
  title: "Design System/Logomark",
  component: Logomark,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof Logomark>;

/**
 * The mark draws in `currentColor`, so it takes the ink of whatever it sits in
 * rather than carrying a colour of its own — which is what lets one file serve
 * the top bar, the sign-in page and a dark theme without a second asset. It is
 * `aria-hidden` and `role="presentation"` on purpose: wherever it appears the
 * product's name is already in the accessible name beside it, and a mark that
 * announced itself would say it twice.
 */
export const Default: Story = { args: { size: 19 } };

/** The sizes the shell actually asks for, on one line so the strokes can be
 * compared as they scale. */
export const Sizes: Story = {
  render: () => (
    <span
      style={{ display: "flex", alignItems: "flex-end", gap: "var(--space-5)" }}
    >
      {[19, 28, 44, 72].map((size) => (
        <Logomark key={size} size={size} />
      ))}
    </span>
  ),
};

/** On the ink it is most often drawn in. `currentColor` is the whole contract:
 * nothing here sets a fill. */
export const OnInk: Story = {
  render: () => (
    <span
      style={{ display: "flex", gap: "var(--space-5)", alignItems: "center" }}
    >
      <span style={{ color: "var(--textPrimary)" }}>
        <Logomark size={44} />
      </span>
      <span style={{ color: "var(--accent)" }}>
        <Logomark size={44} />
      </span>
      <span style={{ color: "var(--textMeta)" }}>
        <Logomark size={44} />
      </span>
    </span>
  ),
};
