// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../testing/story-utils";
import { ShortcutsDialog } from "./shortcutsdialog";
import "./account.css";

/**
 * The keyboard shortcuts, opened from Help and feedback in the account menu.
 * Two groups: the chords that work anywhere, and the keys of the Brief's
 * decision deck, named in the deck's own words. The caps follow the reader's
 * platform, so a Mac shows ⌘ and every other keyboard Ctrl.
 */
const meta: Meta<typeof ShortcutsDialog> = {
  title: "Shell/Keyboard shortcuts",
  component: ShortcutsDialog,
  render: () => (
    <StoryProviders>
      <ShortcutsDialog
        open
        onClose={() => undefined}
        returnFocusTo={() => null}
      />
    </StoryProviders>
  ),
};
export default meta;
type Story = StoryObj<typeof ShortcutsDialog>;

export const Default: Story = {};

export const Dark: Story = {
  globals: { theme: "dark" },
};

export const Phone: Story = {
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
