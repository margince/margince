// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { ProblemError, WriteRefused } from "./common";
import { AiBindRefused } from "./installation-setup.notices";
import { StoryProviders } from "./story-utils";

// The cold start's refusals. Worth their own frames because the whole point of
// each heading is to name WHICH write failed, and a story is the only place the
// alternatives can be read side by side: on the screen itself they are the same
// three lines of the same step, one at a time.

const meta: Meta = {
  title: "Onboarding/First run/Refusals",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function refused(detail: string) {
  return new ProblemError({
    type: "about:blank",
    title: "Bad Request",
    status: 400,
    detail,
  });
}

const KEY_REFUSED = refused("That key was rejected by the provider.");
const BIND_REFUSED = refused("The lane could not be bound to that model.");

function Frame({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <StoryProviders>
      <div className="form-stack">{children}</div>
    </StoryProviders>
  );
}

/** The credential was rejected: retype it. */
export const KeyRefused: Story = {
  render: () => (
    <Frame>
      <AiBindRefused keyError={KEY_REFUSED} bindError={null} />
    </Frame>
  ),
};

/**
 * The key was sealed and the BINDING was refused — a different remedy, which is
 * why it is a different heading and not one generic sentence over both.
 */
export const BindRefused: Story = {
  render: () => (
    <Frame>
      <AiBindRefused keyError={null} bindError={BIND_REFUSED} />
    </Frame>
  ),
};

/** The organisation's OAuth app was not stored. */
export const AppRefused: Story = {
  render: () => (
    <Frame>
      <WriteRefused
        titleKey="oauthApp.saveFailed"
        error={refused("Those credentials were refused.")}
      />
    </Frame>
  ),
};

/** The same band in dark, where tone reaches the heading's ink. */
export const AppRefusedDark: Story = {
  globals: { theme: "dark" },
  render: () => (
    <Frame>
      <WriteRefused
        titleKey="oauthApp.saveFailed"
        error={refused("Those credentials were refused.")}
      />
    </Frame>
  ),
};

/** Nothing refused, nothing drawn. */
export const Quiet: Story = {
  render: () => (
    <Frame>
      <AiBindRefused keyError={null} bindError={null} />
      <WriteRefused titleKey="oauthApp.saveFailed" error={null} />
    </Frame>
  ),
};
