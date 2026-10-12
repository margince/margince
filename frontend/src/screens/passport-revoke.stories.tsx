// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { Button } from "../design-system/atoms";
import { usePassportRevoke } from "./passport-revoke";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

function RevokeHarness() {
  const revoke = usePassportRevoke({
    verb: "settings.revoke",
    question: "settings.revokeConfirm",
    focusAfter: () => null,
  });
  return (
    <>
      <Button onClick={() => revoke.ask("pp-1")}>Revoke Scout</Button>
      {revoke.confirm}
    </>
  );
}

const meta: Meta<typeof RevokeHarness> = {
  title: "Settings/You/Agents/Ending a credential",
  component: RevokeHarness,
  render: () => {
    installFetchStub({
      "GET /passports": () =>
        jsonResponse({
          data: [],
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <RevokeHarness />
      </StoryProviders>
    );
  },
};
export default meta;
type Story = StoryObj<typeof RevokeHarness>;

export const Confirm: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Revoke Scout" }),
    );
    await within(canvasElement.ownerDocument.body).findByRole("dialog");
  },
};
