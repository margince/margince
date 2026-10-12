// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import { TaskVerb } from "./bulktask";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// "Create task" in the bulk bar: the button, and the form it opens to say what
// the task is before the preview files it under every selected record.
const meta: Meta = {
  title: "Records/Bulk change/Create task",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function Verb({ disabled = false }: Readonly<{ disabled?: boolean }>) {
  installFetchStub({
    "GET /me": meRoute({}),
    "GET /users": () =>
      jsonResponse({
        data: [
          {
            id: "u-mila",
            email: "mila@acme.test",
            display_name: "Mila Brandt",
          },
        ],
        page: { next_cursor: null, has_more: false },
      }),
  });
  return (
    <StoryProviders>
      <TaskVerb disabled={disabled} onReady={() => {}} />
    </StoryProviders>
  );
}

export const Ready: Story = { render: () => <Verb /> };

export const Busy: Story = { render: () => <Verb disabled /> };

export const Open: Story = {
  render: () => <Verb />,
  play: async ({ canvasElement }) => {
    await userEvent.click(
      within(canvasElement).getByRole("button", { name: "Create task" }),
    );
    await screen.findByRole("dialog", {
      name: "A task for each selected record",
    });
  },
};
