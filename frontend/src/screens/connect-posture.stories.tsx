// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { ConnectPostureStep } from "./connect-posture";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The posture question asked while a mailbox is being connected, before its
// history has been read.
//
// Two facts decide what this step draws, and both come from outside it: the
// posture the server already wrote on the connection (the prop), and whether
// the workspace permits `shared` at all (GET /capture/settings). The refused
// `shared` option is the state worth reviewing — it is PRESENT and disabled,
// with the sentence saying an admin has to open it, because a missing option
// would tell a reader their product has two postures.

type MailPosture = NonNullable<
  components["schemas"]["CaptureConnection"]["mail_posture"]
>;

type CaptureSettings = components["schemas"]["CaptureSettings"];

const SETTINGS: CaptureSettings = {
  mail_sharing: true,
  shared_posture_allowed: false,
  auto_enrich: true,
  signature_enrich: true,
};

function step(
  args: Readonly<{ sharedAllowed: boolean; posture?: MailPosture }>,
) {
  return () => {
    installFetchStub({
      "GET /capture/settings": () =>
        jsonResponse({
          ...SETTINGS,
          shared_posture_allowed: args.sharedAllowed,
        }),
    });
    return (
      <StoryProviders>
        <ConnectPostureStep provider="gmail" posture={args.posture} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ConnectPostureStep> = {
  title: "Onboarding/Mail posture",
  component: ConnectPostureStep,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof ConnectPostureStep>;

/**
 * The connection as the server just made it: no posture read back yet, so the
 * step shows `classified` — the column default — rather than an answer nobody
 * stored, and `shared` is offered refused.
 */
export const BeforeTheBackread: Story = {
  render: step({ sharedAllowed: false }),
};

/** A mailbox the reader has put beyond the team, with its own help sentence. */
export const AlwaysHeld: Story = {
  render: step({ sharedAllowed: false, posture: "held" }),
};

/**
 * A workspace that has opted into `shared`: the option loses its refusal and
 * the admin sentence under it goes away with it.
 */
export const SharedOpenedByTheWorkspace: Story = {
  render: step({ sharedAllowed: true, posture: "shared" }),
};

// The help line and the refusal sentence both sit at caption contrast on the
// dark ground, which is where a derived ink is easiest to lose.
export const BeforeTheBackreadDark: Story = {
  globals: { theme: "dark" },
  render: step({ sharedAllowed: false }),
};
