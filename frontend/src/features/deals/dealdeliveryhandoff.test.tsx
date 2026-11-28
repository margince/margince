// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  refuseUnrouted,
  StoryProviders,
} from "../../testing/story-utils";
import { DealDeliveryHandoff } from "./dealdeliveryhandoff";

const deal = "018f3a1b-0000-7000-8000-00000000a001";
const project = "018f3a1b-0000-7000-8000-00000000a002";
const channel: components["schemas"]["ChannelLink"] = {
  id: "018f3a1b-0000-7000-8000-00000000a003",
  provider: "slack",
  channel_name: "Sales room",
  channel_slug: "C_SALES",
  channel_url: "https://fixture.slack.com/archives/C_SALES",
  status: "active",
  verbosity: "important",
  message_detail: "full",
  created_at: "2026-06-01T00:00:00Z",
};
const delivery = {
  ...channel,
  id: "018f3a1b-0000-7000-8000-00000000a004",
  channel_name: "Delivery room",
  channel_slug: "C_DELIVERY",
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function mount(
  projectLinked: boolean,
  previewAvailable = true,
  options: Partial<components["schemas"]["ChannelLink"]> = {},
) {
  const dealRoom = { ...channel, ...options };
  const projectRoom = { ...delivery, ...options };
  const chosen = projectLinked ? projectRoom : dealRoom;
  const preview = vi.fn(() =>
    previewAvailable
      ? jsonResponse({
          link_id: chosen.id,
          version: 7,
          provider: "slack",
          workspace: "Account team",
          channel_name: chosen.channel_name,
          audience: [{ id: "U_DELIVERY", name: "Delivery owner" }],
          overview_text: "Current delivery scope",
          update_text: "",
          pending_updates: 0,
          excluded_updates: 0,
          verbosity: "important",
          message_detail: "full",
        })
      : jsonResponse(
          {
            title: "Source access unavailable",
            code: "messenger_consent_required",
          },
          409,
        ),
  );
  installFetchStub(
    {
      "GET /me": meRoute({}),
      [`GET /channel-links/deal/${deal}`]: () =>
        jsonResponse({ state: "linked", link: dealRoom }),
      [`GET /channel-links/project/${project}`]: () =>
        jsonResponse(
          projectLinked
            ? { state: "linked", link: projectRoom }
            : { state: "none" },
        ),
      [`GET /channel-links/${projectLinked ? `project/${project}` : `deal/${deal}`}/delivery-preview`]:
        preview,
      [`GET /projects/${project}/handoff`]: () =>
        jsonResponse({
          project_id: project,
          name: "Pilot delivery",
          phase: "planning",
          as_of: "2026-06-01T00:00:00Z",
          owner_name: "Delivery owner",
          target_end_date: "2026-07-01",
          deals: [],
          stakeholders: [],
          sections_omitted: ["stakeholders"],
          gaps: [],
          open_commitments: [
            {
              task_id: "task-one",
              subject: "Obtain Legal approval before starting",
              state: "open",
              source: "task",
              due_at: "2026-06-28",
              due_has_time: false,
              about: [],
            },
          ],
        }),
      [`GET /deals/${deal}/commitments`]: () =>
        jsonResponse({ data: [], complete: true, has_more: false }),
    },
    refuseUnrouted,
  );
  const confirm = vi.fn();
  render(
    <StoryProviders>
      <DealDeliveryHandoff
        dealId={deal}
        projectId={project}
        busy={false}
        error={null}
        onClose={() => {}}
        onConfirm={confirm}
      />
    </StoryProviders>,
  );
  return { confirm, preview };
}

it("preserves an existing project room and reviews its audience, promises and exclusions", async () => {
  const { confirm } = mount(true);
  await screen.findByText(
    "The project already has Delivery room. That channel will be preserved.",
  );
  await screen.findByText(/Obtain Legal approval before starting/);
  expect(screen.getByText(en["channelHandoff.omitted"])).toBeVisible();
  const button = screen.getByRole("button", { name: en["deal.startDelivery"] });
  await waitFor(() => expect(button).toBeEnabled());
  expect(confirm).not.toHaveBeenCalled();
  fireEvent.click(button);
  expect(confirm).toHaveBeenCalledOnce();
});

it("explains which deal room will continue when the project has no channel", async () => {
  const { preview } = mount(false);
  await screen.findByText(
    "The project has no channel. Continuing will carry Sales room into the project.",
  );
  await waitFor(() => expect(preview).toHaveBeenCalledOnce());
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: en["deal.startDelivery"] }),
    ).toBeEnabled(),
  );
});

it("does not confirm handoff with an unverified delivery audience", async () => {
  const { confirm, preview } = mount(true, false);
  await waitFor(() => expect(preview).toHaveBeenCalledOnce());
  expect(
    screen.getByRole("button", { name: en["deal.startDelivery"] }),
  ).toBeDisabled();
  expect(confirm).not.toHaveBeenCalled();
});

it.each([
  { send_crm_updates: false },
  { provider: "teams", can_send_crm_updates: false, send_crm_updates: false },
] satisfies Partial<components["schemas"]["ChannelLink"]>[])(
  "starts delivery without participant consent when the channel cannot post: %j",
  async (options) => {
    const { confirm, preview } = mount(true, false, options);
    await screen.findByText(/Obtain Legal approval before starting/);
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: en["deal.startDelivery"] }),
      ).toBeEnabled(),
    );
    expect(screen.getByText(en["channelHandoff.noOutgoing"])).toBeVisible();
    expect(preview).not.toHaveBeenCalled();
    fireEvent.click(
      screen.getByRole("button", { name: en["deal.startDelivery"] }),
    );
    expect(confirm).toHaveBeenCalledOnce();
  },
);
