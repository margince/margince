// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../story-utils";
import { ConnectScene, type ProviderAvailability } from "./connect-scene";
import "./conversation.css";

// The connect act's work surface on its own, without the act that owns its
// state. The act's stories (`Onboarding/Conversation/Connect act`) walk the
// route and consent outcomes; this one keeps the two sections in view: the mail
// cards the step waits on, and the network section whose "recommended" badge
// sits in the default tone because LinkedIn never gates the act.

const ALL_READY: ProviderAvailability[] = [
  { provider: "gmail", reason: "ready" },
  { provider: "graph", reason: "ready" },
  { provider: "imap", reason: "ready" },
];

const meta: Meta<typeof ConnectScene> = {
  title: "Onboarding/Conversation/Connect scene",
  component: ConnectScene,
};
export default meta;
type Story = StoryObj<typeof ConnectScene>;

/** Nothing connected yet: the mail cards, and the network section beside. */
export const NothingConnected: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ channel_connection: ["read", "create"] }),
      "GET /connectors": () => jsonResponse({ data: [], providers: ALL_READY }),
    });
    return (
      <StoryProviders>
        <ConnectScene
          provider={null}
          onPick={() => {}}
          onDialogClose={() => {}}
          dialogShowsResult={false}
          dialogPanel={null}
          returnPanel={null}
          onFinish={() => {}}
          finishing={false}
          wantsOvernight
          onWantsOvernightChange={() => {}}
          overnightFailed={false}
          linkedinStatus="pending"
          onLinkedinSave={() => {}}
          onLinkedinSkip={() => {}}
          linkedinPending={false}
          linkedinError={null}
        />
      </StoryProviders>
    );
  },
};
