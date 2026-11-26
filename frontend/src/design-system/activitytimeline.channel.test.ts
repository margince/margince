// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { translate } from "../i18n";
import { activityTimeline } from "./activitytimeline";

type Activity = components["schemas"]["Activity"];

const t = ((
  key: Parameters<typeof translate>[1],
  values?: Record<string, string>,
) => translate("en", key, values)) as never;

const directory: Record<string, string> = {
  whatsapp: "WhatsApp",
  "ext:chatunit:chatunit": "Chat Unit",
};
const channelName = (key: string) => directory[key] ?? key;

function activity(
  id: string,
  kind: string,
  extra: Partial<Activity> = {},
): Activity {
  return {
    id,
    kind,
    occurred_at: "2026-11-20T10:00:00Z",
    occurred_precision: "instant",
    links: [],
    source: "manual",
    is_done: false,
    offers_email: false,
    created_at: "2026-11-20T10:00:00Z",
    updated_at: "2026-11-20T10:00:00Z",
    captured_by: "human:u-1",
    ...extra,
  } as Activity;
}

const channels = (activities: Activity[], named = true) =>
  activityTimeline(activities, undefined, undefined, {
    t,
    locale: "en",
    channelName: named ? channelName : undefined,
  }).map((entry) => entry.channel);

// Each message says which channel carried it, so a conversation that spans
// mail and chats reads without opening a message.
describe("the channel a message came by", () => {
  it("names a mail as email, a core chat by its provider and a unit's by its source", () => {
    expect(
      channels([
        activity("m", "email"),
        activity("w", "message", { channel_provider: "whatsapp" }),
        activity("u", "message", {
          channel_provider: "chatunit_chat",
          source: "ext:chatunit:chatunit",
        }),
      ]),
    ).toEqual(["Email", "WhatsApp", "Chat Unit"]);
  });

  it("names nothing that is not a message, nor a message with no channel", () => {
    expect(channels([activity("n", "note"), activity("c", "message")])).toEqual(
      [undefined, undefined],
    );
  });

  it("names nothing when the caller holds no channel names", () => {
    expect(
      channels(
        [activity("w", "message", { channel_provider: "whatsapp" })],
        false,
      ),
    ).toEqual([undefined]);
  });
});
