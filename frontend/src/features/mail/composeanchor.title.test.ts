// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { type Translator, translate } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { Locale } from "../../i18n/locale";
import { windowTitle } from "./composeanchor";

const english: Locale = "en";

const t: Translator = Object.assign(
  (key: MessageKey, params?: Record<string, string>) =>
    translate(english, key, params),
  { locale: english },
);

const reply = (anchor: { subject?: string | null } | undefined) =>
  windowTitle({
    isChannelReply: false,
    wordingTitle: "Send",
    replying: true,
    anchor,
    subject: "",
    recordName: "Demo GmbH",
    t,
  });

describe("a reply's window title", () => {
  it("reads Reply while the answered message loads or cannot be read", () => {
    expect(reply(undefined)).toBe("Reply");
  });

  it("reads Re: and the subject once the message is known", () => {
    expect(reply({ subject: "Renewal terms" })).toBe("Re: Renewal terms");
  });

  it("says the known message has no subject", () => {
    expect(reply({})).toBe("Re: No subject");
  });
});
