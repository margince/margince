// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { SignOffPreview } from "./composesignoff";
import { jsonResponse, StoryProviders, stubWithSession } from "./story-utils";

// The block under the composer's body: what the send appends, as the server
// answers it.

const meta: Meta = {
  title: "Patterns/Compose mail/Sign-off",
};
export default meta;

type Story = StoryObj;

const BODY = "Shall we say Tuesday at 10?";

// A sender who wrote a signature: that signature, nothing else.
export const Signature: Story = {
  render: () => {
    stubWithSession(
      {
        "POST /emails:sign-off": () =>
          jsonResponse({
            text: "Marek Janetzke\nGradion · +49 40 123456",
            kind: "signature",
          }),
      },
      {},
    );
    return (
      <StoryProviders>
        <SignOffPreview body={BODY} subject="Tuesday" />
      </StoryProviders>
    );
  },
};

// A sender with no signature: the closing the send writes instead, and the way
// to Settings to write one.
export const NoSignature: Story = {
  render: () => {
    stubWithSession(
      {
        "POST /emails:sign-off": () =>
          jsonResponse({
            text: "Best regards,\nMarek Janetzke",
            kind: "closing",
          }),
      },
      {},
    );
    return (
      <StoryProviders>
        <SignOffPreview body={BODY} subject="Tuesday" />
      </StoryProviders>
    );
  },
};
