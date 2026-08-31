// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { GoogleAppCard } from "./google-app";

// The card has THREE states, and the middle one is the reason it was rewritten:
// an app the deployment supplies is not a stored app and is not the absence of
// one, and reporting it as absent told operators Gmail could not be connected
// on installations where it could.
const meta: Meta<typeof GoogleAppCard> = {
  title: "Settings/Admin settings/General/Google app",
  component: GoogleAppCard,
};
export default meta;

type Story = StoryObj<typeof GoogleAppCard>;

const CLIENT_ID = "111-abc.apps.googleusercontent.com";
const SIGN_IN_URI = "https://api.acme.test/v1/auth/oidc/google/callback";
const CONNECT_URI = "https://api.acme.test/v1/connectors/google/callback";

/** An app saved through this surface, which wins over the deployment's. */
export const Stored: Story = {
  parameters: {
    googleApp: {
      configured: true,
      client_id: CLIENT_ID,
      source: "stored",
      redirect_uris: [
        { purpose: "sign_in", url: SIGN_IN_URI },
        { purpose: "mailbox_connect", url: CONNECT_URI },
      ],
    },
  },
};

/** The deployment's own credentials, used because nothing is stored. */
export const FromEnvironment: Story = {
  parameters: {
    googleApp: {
      configured: true,
      client_id: CLIENT_ID,
      source: "environment",
      redirect_uris: [{ purpose: "mailbox_connect", url: CONNECT_URI }],
    },
  },
};

/** Neither source can supply one, so neither flow can run. */
export const Absent: Story = {
  parameters: {
    googleApp: {
      configured: false,
      client_id: "",
      source: "none",
      redirect_uris: [],
    },
  },
};
