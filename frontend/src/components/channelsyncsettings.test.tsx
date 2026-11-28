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
import { ChannelSyncSettings } from "./channelsyncsettings";

const record = "018f3a1b-0000-7000-8000-00000000a001";
const link = "018f3a1b-0000-7000-8000-00000000a002";
const channel: components["schemas"]["ChannelLink"] = {
  id: link,
  provider: "slack",
  channel_slug: "C_ONE",
  channel_name: "Pilot",
  channel_url: "https://fixture.slack.com/archives/C_ONE",
  verbosity: "important",
  message_detail: "full",
  status: "active",
  send_crm_updates: false,
  created_at: "2026-06-01T00:00:00Z",
};
const preview: components["schemas"]["ChannelDeliveryPreview"] = {
  link_id: link,
  version: 7,
  provider: "slack",
  workspace: "Sales team",
  channel_name: "Pilot",
  audience: [{ id: "U_REP", name: "Rita Rep" }],
  overview_text: "*Pilot*\nOwner <@rep@example.test|Rita Rep>",
  update_text: "Legal has not approved the agreement.",
  pending_updates: 1,
  excluded_updates: 0,
  verbosity: "important",
  message_detail: "full",
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function mount(answer = preview) {
  const patch = vi.fn((_body: unknown) =>
    jsonResponse({
      state: "linked",
      link: { ...channel, send_crm_updates: true },
    }),
  );
  installFetchStub(
    {
      [`GET /channel-links/deal/${record}/delivery-preview`]: () =>
        jsonResponse(answer),
      [`PATCH /channel-links/deal/${record}`]: patch,
    },
    refuseUnrouted,
  );
  render(
    <StoryProviders>
      <ChannelSyncSettings
        channel={channel}
        recordType="deal"
        recordId={record}
        onClose={() => {}}
        onSaved={() => {}}
      >
        {null}
      </ChannelSyncSettings>
    </StoryProviders>,
  );
  return patch;
}

it("requires review of the actual audience and content before enabling outgoing sync", async () => {
  const patch = mount();
  fireEvent.click(
    screen.getByRole("switch", { name: en["channelLink.postDirection"] }),
  );
  const dialog = await screen.findByRole("dialog", {
    name: en["channelPreview.title"],
  });
  await within(dialog).findByText("Legal has not approved the agreement.");
  expect(within(dialog).getByText(/Sales team/)).toBeVisible();
  expect(within(dialog).getAllByText("Rita Rep").length).toBeGreaterThan(0);
  expect(dialog).not.toHaveTextContent("<@rep@example.test");
  expect(patch).not.toHaveBeenCalled();
  fireEvent.click(
    within(dialog).getByRole("button", { name: en["channelPreview.enable"] }),
  );
  await waitFor(() =>
    expect(patch.mock.calls[0]?.[0]).toEqual({
      send_crm_updates: true,
      expected_link_id: link,
      expected_version: 7,
    }),
  );
});

it("refuses an audience preview for a replaced channel association", async () => {
  const patch = mount({ ...preview, link_id: record });
  fireEvent.click(
    screen.getByRole("switch", { name: en["channelLink.postDirection"] }),
  );
  const dialog = await screen.findByRole("dialog", {
    name: en["channelPreview.title"],
  });
  await within(dialog).findByText(en["channelDelivery.linkChanged"]);
  expect(
    within(dialog).getByRole("button", { name: en["channelPreview.enable"] }),
  ).toBeDisabled();
  expect(patch).not.toHaveBeenCalled();
});
