// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { expect, userEvent, within } from "storybook/test";
import { CallAction } from "../design-system/callaction";
import { en } from "../i18n/en";
import { useMe } from "../lib/session";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { CallLogHost, CallLogInline } from "./calllog.host";

const meta: Meta<typeof CallLogHost> = {
  title: "Patterns/Call log",
  component: CallLogHost,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof CallLogHost>;

const DANA = { entityType: "contact", entityId: "p-1" } as const;

function stubSeat() {
  installFetchStub({
    "GET /me": meRoute({ activity: ["create"] }, { seat: "full" }),
  });
}

// Drawn once the seat has answered, so the press never meets an unknown grant.
function OnceSeated({ children }: Readonly<{ children: ReactNode }>) {
  return useMe().data ? children : null;
}

// The press opens Log activity on Call, naming the contact it files on.
export const Host: Story = {
  render: () => {
    stubSeat();
    return (
      <StoryProviders>
        <CallLogHost>
          <OnceSeated>
            <CallAction
              appearance="button"
              number="+49 30 1234 5678"
              label={en["record.verb.call"]}
              record={DANA}
            />
          </OnceSeated>
        </CallLogHost>
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const call = await within(canvasElement).findByRole("link", {
      name: en["record.verb.call"],
    });
    await userEvent.click(call);
    await expect(
      await within(document.body).findByRole("dialog", {
        name: en["log.title"],
      }),
    ).toBeVisible();
  },
};

// Inside a dialog the log opens in place, under the surface's own verbs.
export const Inline: Story = {
  render: () => {
    stubSeat();
    return (
      <StoryProviders>
        <CallLogInline call={{ ...DANA, seq: 1 }} onClose={() => {}} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByRole("region", {
        name: en["log.title"],
      }),
    ).toBeVisible();
  },
};
