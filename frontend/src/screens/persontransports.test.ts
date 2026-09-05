// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import type { Transport } from "./persontransports";
import { transportForActivity } from "./persontransports";

type Person360 = components["schemas"]["Person360"];

// Two channels and an address, so "which one" is a real question rather than
// one the fixture answers by having a single option.
const TRANSPORTS: Transport[] = [
  { id: "email", label: "Email" },
  { id: "whatsapp", label: "WhatsApp", anchorId: "wa-newest" },
  { id: "telegram", label: "Telegram", anchorId: "tg-newest" },
];

function pageWith(
  activities: Array<{ id: string; kind: string; channel_provider?: string }>,
): Person360 {
  return {
    activities: { data: activities, page: {} },
  } as unknown as Person360;
}

const PAGE = pageWith([
  { id: "wa-older", kind: "message", channel_provider: "whatsapp" },
  { id: "wa-newest", kind: "message", channel_provider: "whatsapp" },
  { id: "tg-newest", kind: "message", channel_provider: "telegram" },
  { id: "mail-1", kind: "email" },
]);

describe("transportForActivity", () => {
  // Nothing named is not the same as something named and missing, and the pair
  // exists so a caller can tell them apart.
  it("claims nothing when no conversation was named", () => {
    expect(transportForActivity(TRANSPORTS, PAGE, undefined)).toEqual({
      chosen: undefined,
      stale: false,
    });
  });

  it("answers a message on the channel it is on", () => {
    const got = transportForActivity(TRANSPORTS, PAGE, "tg-newest");
    expect(got.stale).toBe(false);
    expect(got.chosen?.id).toBe("telegram");
  });

  // THE NAMED MESSAGE, not the provider's newest. The transport list offers the
  // latest conversation per provider because that is what a rep means when they
  // pick from the list; a caller who named one means that one.
  it("anchors on the conversation named rather than the provider's newest", () => {
    const got = transportForActivity(TRANSPORTS, PAGE, "wa-older");
    expect(got.chosen?.id).toBe("whatsapp");
    expect(got.chosen?.anchorId).toBe("wa-older");
  });

  // Anything that is not a channel message is answered by mail, which is the
  // one transport that can open a conversation rather than continue one.
  it("answers a mail with mail", () => {
    const got = transportForActivity(TRANSPORTS, PAGE, "mail-1");
    expect(got.chosen?.id).toBe("email");
    expect(got.stale).toBe(false);
  });

  // The case the pair exists for: a channel disconnected since the row was
  // ranked. Answering with the person's lead transport and saying nothing is
  // the reader writing into a conversation they did not choose.
  it("reports a named channel this contact no longer has as stale", () => {
    const withoutTelegram = TRANSPORTS.filter((t) => t.id !== "telegram");
    const got = transportForActivity(withoutTelegram, PAGE, "tg-newest");
    expect(got.chosen).toBeUndefined();
    expect(got.stale).toBe(true);
  });

  // And a message that has fallen off the page's own window entirely — the
  // row named it, the record no longer carries it.
  it("reports a message the record does not carry as stale", () => {
    const got = transportForActivity(TRANSPORTS, PAGE, "gone-1");
    expect(got.chosen).toBeUndefined();
    expect(got.stale).toBe(true);
  });
});
