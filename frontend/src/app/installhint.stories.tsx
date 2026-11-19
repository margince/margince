// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useRef } from "react";
import { expect, userEvent, within } from "storybook/test";
import { en } from "../i18n/en";
import { StoryProviders } from "../testing/story-utils";
import { type HintedInstall, InstallHintNotice } from "./installhint";

const AVAILABLE: HintedInstall = {
  kind: "available",
  prompt: () => new Promise(() => undefined),
};
const MANUAL_IOS: HintedInstall = { kind: "manual-ios" };

function Hint({ install }: Readonly<{ install: HintedInstall }>) {
  const scroller = useRef<HTMLDivElement>(null);
  return (
    <StoryProviders>
      <InstallHintNotice
        install={install}
        onDismiss={() => undefined}
        scroller={scroller}
      />
      <div ref={scroller} tabIndex={-1} />
    </StoryProviders>
  );
}

const meta: Meta<typeof InstallHintNotice> = {
  title: "Shell/Install hint",
  component: InstallHintNotice,
};
export default meta;
type Story = StoryObj<typeof InstallHintNotice>;

// Chromium on a phone kept its install offer for the hint to present.
export const Available: Story = { render: () => <Hint install={AVAILABLE} /> };

// The browser's dialog is open: Install waits for the reader's answer.
export const Pending: Story = {
  render: () => <Hint install={AVAILABLE} />,
  play: async ({ canvasElement }) => {
    const install = await within(canvasElement).findByRole("button", {
      name: en["settings.installAppAction"],
    });
    await userEvent.click(install);
    await expect(install).toHaveAttribute("aria-busy", "true");
  },
};

// Safari on an iPhone or iPad installs from its Share sheet.
export const ManualIos: Story = { render: () => <Hint install={MANUAL_IOS} /> };

export const AvailablePhone: Story = {
  ...Available,
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const ManualIosPhone: Story = {
  ...ManualIos,
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const AvailablePhoneDark: Story = {
  ...AvailablePhone,
  globals: { viewport: { value: "phone" }, theme: "dark" },
};
