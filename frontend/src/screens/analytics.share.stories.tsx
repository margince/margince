// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import { ShareViewButton } from "./analytics.share";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// The share dialog, in both states a reader meets it in. The kind picker is
// the first: two promises, told apart in words rather than by a label. The
// link reveal is the second, and it is the one worth capturing — it is shown
// once, so a regression that hid the caution would be invisible until somebody
// closed the dialog and lost their link. The link itself is a code block, the
// one place on this surface that draws in the monospace face.

const shareRoutes: RouteMap = {
  "GET /me": meRoute({}),
  "POST /forecast/shares": () =>
    jsonResponse(
      {
        id: "share-1",
        kind: "live",
        target: "forecast",
        expires_at: "2026-10-03T00:00:00Z",
        token: "shr_9f2c4a1e",
        created_at: "2026-09-03T00:00:00Z",
      },
      201,
    ),
};

function ShareButtonStory() {
  return (
    <StoryProviders>
      <ShareViewButton
        target="forecast"
        scope={{ kind: "workspace", label: "Whole workspace" }}
        snapshotId="snap-1"
      />
    </StoryProviders>
  );
}

async function openDialog(canvasElement: HTMLElement) {
  await userEvent.click(
    await within(canvasElement).findByRole("button", { name: "Share view" }),
  );
}

const meta: Meta<typeof ShareViewButton> = {
  title: "Records/Reports/Share view",
  component: ShareViewButton,
};
export default meta;

type Story = StoryObj<typeof ShareViewButton>;

export const ShareDialogKinds: Story = {
  render: () => <ShareButtonStory />,
  beforeEach: () => installFetchStub(shareRoutes),
  play: async ({ canvasElement }) => {
    await openDialog(canvasElement);
  },
};

export const ShareDialogLinkShownOnce: Story = {
  render: () => <ShareButtonStory />,
  beforeEach: () => installFetchStub(shareRoutes),
  play: async ({ canvasElement }) => {
    await openDialog(canvasElement);
    // `screen`, not `canvas`, for anything inside the dialog: Modal portals to
    // document.body, so the dialog is a sibling of the canvas rather than a
    // descendant of it, and a canvas-scoped query for its confirm waits out its
    // full budget for a button that is on screen the whole time. The TRIGGER
    // stays canvas-scoped, because that one really is in the story's own tree.
    await userEvent.click(
      await screen.findByRole("button", { name: "Create link" }),
    );
    await screen.findByTestId("forecast-share-link");
  },
};
