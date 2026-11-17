// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { BuildRelease } from "../lib/buildrelease";
import { StoryProviders } from "../testing/story-utils";
import { AboutMargince, AboutMarginceDialog } from "./aboutmargince";

const SHA = "0123456789abcdef0123456789abcdef01234567";

/** Opened from Help and feedback; `ContentOnly` is the body a page draws. */
const meta: Meta<typeof AboutMarginceDialog> = {
  title: "Shell/About Margince",
  component: AboutMarginceDialog,
  args: {
    build: { kind: "core", release: "2026.2.1", revision: SHA },
    open: true,
    onClose: () => undefined,
    returnFocusTo: () => null,
  },
  render: (args) => (
    <StoryProviders>
      <AboutMarginceDialog {...args} />
    </StoryProviders>
  ),
};
export default meta;
type Story = StoryObj<typeof AboutMarginceDialog>;

const phone: Story = {
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

const instance: BuildRelease = {
  kind: "instance",
  instance: "v0.4.1",
  core: "v0.0.5",
  revision: SHA,
};

const longVersion: BuildRelease = {
  kind: "instance",
  instance:
    "2026.11.17-rc.4+acme-emea-tenant-integration-build.20261117T080711Z",
  core: "2026.2.1-hotfix.3+backport.security-roll-up-november",
  revision: SHA,
};

export const CoreBuild: Story = {
  name: "Core build",
  play: async ({ canvasElement }) => {
    const dialog = within(canvasElement.ownerDocument.body).getByRole(
      "dialog",
      { name: "About Margince" },
    );
    await expect(
      within(dialog).getByRole("link", { name: /^0123456/ }),
    ).toHaveAttribute(
      "href",
      `https://github.com/margince/margince/commit/${SHA}`,
    );
  },
};

export const CoreBuildDark: Story = {
  name: "Core build (dark)",
  globals: { theme: "dark" },
};

export const CoreBuildPhone: Story = { name: "Core build (phone)", ...phone };

export const InstanceBuild: Story = {
  name: "Instance build",
  args: { build: instance },
};

export const InstanceBuildDark: Story = {
  name: "Instance build (dark)",
  args: { build: instance },
  globals: { theme: "dark" },
};

export const InstanceCoreOnly: Story = {
  name: "Instance build, core release only",
  args: { build: { kind: "instance", core: "v0.0.5", revision: SHA } },
};

export const DevelopmentBuild: Story = {
  name: "Development build",
  args: { build: { kind: "unstamped", revision: SHA } },
};

export const DevelopmentBuildNoRevision: Story = {
  name: "Development build, no revision",
  args: { build: { kind: "unstamped" } },
};

export const LongVersion: Story = {
  name: "Long version",
  args: { build: longVersion },
};

export const LongVersionPhone: Story = {
  name: "Long version (phone)",
  args: { build: longVersion },
  ...phone,
};

export const LongVersionPhoneDark: Story = {
  name: "Long version (phone, dark)",
  args: { build: longVersion },
  ...phone,
  globals: { ...phone.globals, theme: "dark" },
};

/** The content alone, as a page that is not a dialog draws it. */
export const ContentOnly: Story = {
  name: "Content without the dialog",
  render: (args) => (
    <StoryProviders>
      <AboutMargince build={args.build} />
    </StoryProviders>
  ),
};
