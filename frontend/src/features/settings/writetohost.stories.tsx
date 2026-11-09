// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { Button } from "../../design-system/atoms";
import { useWriteTo } from "../../lib/writeto";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
  visibleDialog,
} from "../../testing/story-utils";
import { WriteToHost } from "./writetohost";

function WriteAddress() {
  const write = useWriteTo();
  return (
    <Button
      disabled={!write}
      onClick={() =>
        write?.({
          entityType: "contact",
          entityId: "p-1",
          address: "buyer@demo.test",
        })
      }
    >
      Write to buyer
    </Button>
  );
}
const meta: Meta = { title: "Shell/Write to a record" };
export default meta;
type Story = StoryObj;
function host(connected: boolean) {
  installFetchStub({
    "GET /me": meRoute({ contact: ["read"] }),
    "GET /connectors": () =>
      jsonResponse({
        data: connected ? [{ provider: "gmail", status: "connected" }] : [],
        providers: [],
      }),
  });
  return (
    <StoryProviders>
      <WriteToHost>
        <WriteAddress />
      </WriteToHost>
    </StoryProviders>
  );
}
export const Connected: Story = {
  render: () => host(true),
  play: async ({ canvasElement }) => {
    const button = await within(canvasElement).findByRole("button", {
      name: "Write to buyer",
    });
    await waitFor(() => expect(button).toBeEnabled());
    await userEvent.click(button);
    await visibleDialog();
  },
};
export const NoMailbox: Story = {
  render: () => host(false),
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByRole("button", {
        name: "Write to buyer",
      }),
    ).toBeDisabled();
  },
};
export const ConnectedDark: Story = {
  ...Connected,
  globals: { theme: "dark" },
};
