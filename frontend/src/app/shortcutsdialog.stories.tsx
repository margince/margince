// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { setSingleKeyShortcutsOn } from "../lib/shortcuts";
import { StoryProviders } from "../testing/story-utils";
import { ShortcutsDialog } from "./shortcutsdialog";
import "./account.css";

/** Opened from Help and feedback or with `?`: the keymap's groups, then the
 *  deck's keys in the deck's own words. Caps follow the reader's platform. */
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

/** The reader turned single-key shortcuts off, and the dialog says so first. */
export const SingleKeysOff: Story = {
  render: () => {
    setSingleKeyShortcutsOn(false);
    return (
      <StoryProviders>
        <ShortcutsDialog
          open
          onClose={() => undefined}
          returnFocusTo={() => null}
        />
      </StoryProviders>
    );
  },
};

export const SingleKeysOffDark: Story = {
  ...SingleKeysOff,
  globals: { theme: "dark" },
};

export const Phone: Story = {
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
