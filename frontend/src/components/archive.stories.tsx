// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
  visibleDialog,
} from "../testing/story-utils";
import { ArchiveAction } from "./archive";

const meta: Meta<typeof ArchiveAction> = {
  title: "Patterns/Archive record",
  component: ArchiveAction,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => {
      installFetchStub({ "GET /me": meRoute({ contact: ["delete"] }) });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
};
export default meta;

type Story = StoryObj<typeof ArchiveAction>;

export const ConfirmOpen: Story = {
  args: {
    label: "Archive",
    archivedMessage: "Contact archived",
    confirmText: "Archive this record? There is no undo.",
    archive: () => Promise.resolve({ id: "p-1" }),
    invalidate: "contacts",
    recordKey: "contact",
    requires: { object: "contact", action: "delete" },
    onArchived: () => undefined,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const trigger = await canvas.findByTestId("archive-record");
    await waitFor(() => expect(trigger).toBeEnabled());
    await userEvent.click(trigger);
    await visibleDialog();
  },
};
