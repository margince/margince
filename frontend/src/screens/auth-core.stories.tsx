// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { IdentityRegion } from "./auth-core";
import "./auth.css";
import { StoryProviders } from "./story-utils";

/**
 * The identity region on its own, so its motion can be reviewed without a form
 * beside it.
 *
 * It has ONE axis left: the sign-in phase, which the Core beside the copy
 * answers. The region used to carry the installation's AI posture as well —
 * configured, unconfigured, a probe that never answered — and those states went
 * with the runtime line itself, along with the disclosure kicker, the send
 * promise and the handover.
 *
 * The decorator gives it the surface's grid rather than a centring box: the
 * region is a grid child, and reviewing it outside that context would show a
 * layout the product never renders.
 */
const meta = {
  title: "Signed out/Identity region",
  component: IdentityRegion,
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <div className="auth-surface">
          <Story />
        </div>
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof IdentityRegion>;
export default meta;

type Story = StoryObj<typeof meta>;

/** Waiting on a person: nothing is running, and the Core does not pretend to
 *  be listening for them. */
export const Idle: Story = { args: { phase: "idle" } };

/** The credential is in flight. */
export const SigningIn: Story = { args: { phase: "signing-in" } };

/** The installation cannot answer at all. */
export const Unavailable: Story = { args: { phase: "unavailable" } };
