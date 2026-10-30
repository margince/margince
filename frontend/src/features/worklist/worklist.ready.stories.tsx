// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { en } from "../../i18n/en";
import { installFetchStub, StoryProviders } from "../../testing/story-utils";
import { row } from "./worklist.fixtures";
import type { WorklistItem } from "./worklist.queries";
import { MeetingReadyVerb, PreparationNote } from "./worklist.ready";

// The reader's own readiness for a meeting: the I'm ready button while it is
// still to confirm, the Ready mark once said, and the facts that changed since.

function meeting(
  preparation: NonNullable<WorklistItem["preparation"]>,
): WorklistItem {
  return row({
    id: "m1",
    source: "meeting",
    category: "meetings",
    level: 1,
    title: "Quarterly review with Turbinenbau",
    kind: preparation.state === "ready" ? "prepared" : "unprepared",
    due_at: "2026-09-02T13:00:00Z",
    preparation,
  });
}

function frame(item: WorklistItem) {
  return () => {
    installFetchStub({
      "PUT /worklist/ready": () => new Response(null, { status: 204 }),
    });
    return (
      <StoryProviders>
        <ToastProvider>
          <MeetingReadyVerb item={item} />
          <PreparationNote item={item} />
          <ToastRegion />
        </ToastProvider>
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof MeetingReadyVerb> = {
  title: "Records/Worklist/Meeting readiness",
  component: MeetingReadyVerb,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof MeetingReadyVerb>;

export const StillToConfirm: Story = {
  render: frame(
    meeting({
      activity_id: "m1",
      state: "needs_review",
      revision: "rev-1",
      changed: [],
    }),
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: en["worklist.ready.confirm"] }),
    );
    await within(canvasElement.ownerDocument.body).findByText(
      en["worklist.ready.done"],
    );
  },
};

export const Ready: Story = {
  render: frame(
    meeting({
      activity_id: "m1",
      state: "ready",
      revision: "rev-1",
      changed: [],
    }),
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.queryByRole("button")).toBeNull();
    await expect(canvas.getByText(en["worklist.ready.badge"])).toBeVisible();
  },
};

export const ChangedSinceReady: Story = {
  render: frame(
    meeting({
      activity_id: "m1",
      state: "needs_review",
      revision: "rev-2",
      changed: ["start_time", "agenda"],
    }),
  ),
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByText(
      "Changed since you were ready: start time, agenda",
    );
  },
};
