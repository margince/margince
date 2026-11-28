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
  within,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { en } from "../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  refuseUnrouted,
  StoryProviders,
} from "../testing/story-utils";
import { ChannelDeliveryRecovery } from "./channeldeliveryreview";

const record = "018f3a1b-0000-7000-8000-00000000a001";
const linkId = "018f3a1b-0000-7000-8000-00000000a002";
const receipt = "018f3a1b-0000-7000-8000-00000000a003";
const channel: components["schemas"]["ChannelLink"] = {
  id: linkId,
  provider: "slack",
  channel_slug: "C_FIXTURE",
  channel_name: "Account team",
  channel_url: "https://fixture.slack.com/archives/C_FIXTURE",
  verbosity: "important",
  status: "active",
  created_at: "2026-06-01T00:00:00Z",
};
const review: components["schemas"]["ChannelDeliveryReview"] = {
  link_id: linkId,
  overview_card_paused: false,
  deliveries: [
    {
      id: receipt,
      state: "unknown",
      created_at: "2026-06-01T00:00:00Z",
      update_count: 2,
      overview_card: false,
    },
  ],
};
function mount(answer = review, paused = false) {
  const write = vi.fn((_body: unknown) =>
    jsonResponse({ ...answer, deliveries: [] }),
  );
  const read = vi.fn(() => jsonResponse(answer));
  installFetchStub(
    {
      [`GET /channel-links/deal/${record}/delivery-review`]: read,
      [`POST /channel-links/deal/${record}/delivery-review`]: write,
    },
    refuseUnrouted,
  );
  render(
    <StoryProviders>
      <ChannelDeliveryRecovery
        channel={{ ...channel, overview_card_paused: paused }}
        recordType="deal"
        recordId={record}
      />
    </StoryProviders>,
  );
  return { read, write };
}
afterEach(cleanup);
it("reviews only explicit receipt IDs and does not perform a messaging write", async () => {
  const { read, write } = mount();
  expect(read).not.toHaveBeenCalled();
  fireEvent.click(
    screen.getByRole("button", { name: en["channelDelivery.review"] }),
  );
  const dialog = await screen.findByRole("dialog");
  await within(dialog).findByText(/CRM updates: 2/);
  expect(within(dialog).getByText(en["channelDelivery.help"])).toBeVisible();
  fireEvent.click(
    within(dialog).getByRole("button", { name: en["channelDelivery.skip"] }),
  );
  await waitFor(() => expect(write).toHaveBeenCalledTimes(1));
  expect(write.mock.calls[0]?.[0]).toEqual({
    link_id: linkId,
    action: "skip_unknown",
    acknowledged: true,
    delivery_ids: [receipt],
  });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});
it("refuses a stale linked-channel response", async () => {
  const { write } = mount({ ...review, link_id: record });
  fireEvent.click(
    screen.getByRole("button", { name: en["channelDelivery.review"] }),
  );
  const dialog = await screen.findByRole("dialog");
  expect(
    await within(dialog).findByText(en["channelDelivery.linkChanged"]),
  ).toBeVisible();
  expect(
    within(dialog).getByRole("button", { name: en["channelDelivery.skip"] }),
  ).toBeDisabled();
  expect(write).not.toHaveBeenCalled();
});
it("requires a separate deliberate restart for a paused overview card", async () => {
  const { write } = mount(
    { ...review, overview_card_paused: true, deliveries: [] },
    true,
  );
  fireEvent.click(
    screen.getByRole("button", { name: en["channelDelivery.restart"] }),
  );
  const dialog = await screen.findByRole("dialog");
  expect(
    within(dialog).getByText(en["channelDelivery.restartWarning"]),
  ).toBeVisible();
  await waitFor(() =>
    expect(
      within(dialog).getByRole("button", {
        name: en["channelDelivery.restart"],
      }),
    ).toBeEnabled(),
  );
  fireEvent.click(
    within(dialog).getByRole("button", { name: en["channelDelivery.restart"] }),
  );
  await waitFor(() => expect(write).toHaveBeenCalledTimes(1));
  expect(write.mock.calls[0]?.[0]).toEqual({
    link_id: linkId,
    action: "restart_card",
    acknowledged: true,
    delivery_ids: [],
  });
});
