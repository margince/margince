// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { translate, type useT } from "../i18n";
import type { Transport } from "./contacttransports";
import { transportForActivity, transportsFor } from "./contacttransports";

type Contact360 = components["schemas"]["Contact360"];

// Two channels and an address, so "which one" is a real question rather than
// one the fixture answers by having a single option.
const TRANSPORTS: Transport[] = [
  { id: "email", label: "Email" },
  { id: "whatsapp", label: "WhatsApp", anchorId: "wa-newest" },
  { id: "telegram", label: "Telegram", anchorId: "tg-newest" },
];

function pageWith(
  activities: Array<{ id: string; kind: string; channel_provider?: string }>,
): Contact360 {
  return {
    activities: { data: activities, page: {} },
  } as unknown as Contact360;
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
  // ranked. Answering with the contact's lead transport and saying nothing is
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

// WHICH WAYS this contact can be written to at all — the answer the composer's
// dial is built from, and the record verb's label with it.
//
// These read the rules directly rather than through a rendered composer, which
// is where they used to live. Every one of them is a statement about
// reachability and anchors; asserting them through a drawer meant mounting the
// whole composer, its queries and its consent gate to find out whether a list
// had two entries in it.

// Naming a provider is the reader's own language, so the composer passes one in.
const nameProvider = (provider: string) =>
  provider === "dispact" ? "Dispact" : provider;

// The REAL catalog, not a stub answering one key by name. A stub keeps passing
// after the key it names is renamed or retired — which is exactly what happened
// to the key this used to hold — and it proves nothing about the word a reader
// sees. No cast: `useT` returns a plain translator, so a function of the same
// shape simply is one.
const say: ReturnType<typeof useT> = (key, params) =>
  translate("en", key, params);

function contact(
  options: Readonly<{
    email?: boolean;
    reachable?: { provider: string; reachable: boolean }[];
    activities?: {
      id: string;
      kind: string;
      channel_provider?: string;
      occurred_at: string;
    }[];
  }>,
): Contact360 {
  return {
    contact: {
      emails: options.email
        ? [{ email: "dana@brandt.example", is_primary: true }]
        : [],
      reachability: options.reachable ?? [],
    },
    activities: { data: options.activities ?? [] },
  } as unknown as Contact360;
}

const aMessage = (provider: string, id: string, at: string) => ({
  id,
  kind: "message",
  channel_provider: provider,
  occurred_at: at,
});

describe("transportsFor", () => {
  it("offers mail alone for a contact with an address and no channel", () => {
    const got = transportsFor(contact({ email: true }), nameProvider, say);
    expect(got.map((t) => t.id)).toEqual(["email"]);
  });

  // Mail leads because it is the one transport that can OPEN a conversation:
  // `POST /emails` names its addressee, and send-message resolves one from the
  // conversation it answers.
  it("leads with mail when both are available", () => {
    const got = transportsFor(
      contact({
        email: true,
        reachable: [{ provider: "dispact", reachable: true }],
        activities: [aMessage("dispact", "a-1", "2026-08-15T08:00:00Z")],
      }),
      nameProvider,
      say,
    );
    expect(got.map((t) => t.id)).toEqual(["email", "dispact"]);
    expect(got[1].anchorId).toBe("a-1");
  });

  // Reachability and an anchor answer different questions and BOTH are
  // required: a live identity with nothing to continue would be a choice that
  // fails at the send, since no endpoint opens a channel conversation.
  it("withholds a channel the contact is reachable on but has no conversation on", () => {
    const got = transportsFor(
      contact({
        email: true,
        reachable: [{ provider: "dispact", reachable: true }],
      }),
      nameProvider,
      say,
    );
    expect(got.map((t) => t.id)).toEqual(["email"]);
  });

  // The mirror: a conversation exists, but the identity is blocked or archived.
  it("withholds a channel whose identity is no longer reachable", () => {
    const got = transportsFor(
      contact({
        email: true,
        reachable: [{ provider: "dispact", reachable: false }],
        activities: [aMessage("dispact", "a-1", "2026-08-15T08:00:00Z")],
      }),
      nameProvider,
      say,
    );
    expect(got.map((t) => t.id)).toEqual(["email"]);
  });

  // The contact this exists for: no address anywhere, one chat conversation.
  // Offering mail here would offer a mailbox nobody has.
  it("offers the channel alone for a contact with no address", () => {
    const got = transportsFor(
      contact({
        reachable: [{ provider: "dispact", reachable: true }],
        activities: [aMessage("dispact", "a-1", "2026-08-15T08:00:00Z")],
      }),
      nameProvider,
      say,
    );
    expect(got.map((t) => t.id)).toEqual(["dispact"]);
  });

  // One entry per provider, anchored on its NEWEST conversation — which is what
  // a rep means when they pick that transport from the list.
  it("anchors a provider on its most recent conversation, once", () => {
    const got = transportsFor(
      contact({
        reachable: [{ provider: "dispact", reachable: true }],
        activities: [
          aMessage("dispact", "older", "2026-08-01T08:00:00Z"),
          aMessage("dispact", "newest", "2026-08-15T08:00:00Z"),
        ],
      }),
      nameProvider,
      say,
    );
    expect(got).toHaveLength(1);
    expect(got[0].anchorId).toBe("newest");
  });
});
