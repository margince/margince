// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { Button } from "../design-system/atoms";
import { useOpenSource } from "../lib/opensource";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
  visibleDialog,
} from "../testing/story-utils";
import { SourceHost } from "./sourcehost";

function OpenMeeting() {
  const open = useOpenSource();
  return (
    <Button onClick={() => open?.("meeting-1")}>Read meeting source</Button>
  );
}
const meta: Meta = { title: "Shell/Source host" };
export default meta;
type Story = StoryObj;
export const Meeting: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"] }),
      "GET /activities/meeting-1": () =>
        jsonResponse({
          id: "meeting-1",
          kind: "meeting",
          subject: "Pricing discussion",
          body: "Pat: I will send revised pricing.",
          occurred_at: "2024-10-18T10:00:00Z",
          content_state: "available",
          links: [],
        }),
    });
    return (
      <StoryProviders>
        <SourceHost>
          <OpenMeeting />
        </SourceHost>
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Read meeting source",
      }),
    );
    await visibleDialog();
    await expect(
      within(document.body).findByText("Pricing discussion"),
    ).resolves.toBeVisible();
  },
};
export const MeetingDark: Story = { ...Meeting, globals: { theme: "dark" } };
