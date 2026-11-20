// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { translate } from "../i18n";
import { en } from "../i18n/en";
import {
  imapErrorMessage,
  imapRefusal,
  inRange,
  PORT_RANGE,
} from "./imapconnect";
import { ProblemError } from "./problem";

const t = (key: Parameters<typeof translate>[1]) => translate("en", key);

const loginRejected = new ProblemError({
  status: 422,
  code: "imap_login_rejected",
  detail: "The mailbox rejected these credentials.",
});

const portRefused = new ProblemError({
  status: 422,
  code: "validation_error",
  detail: "`imap.port` must be a number from 1 to 65535, not 0",
  details: {
    errors: [
      {
        field: "imap.port",
        code: "out_of_range",
        message: "`imap.port` must be a number from 1 to 65535, not 0",
        params: { min: "1", max: "65535" },
      },
    ],
  },
});

describe("a port", () => {
  it("is a whole number from 1 to 65535", () => {
    expect(inRange(993, PORT_RANGE)).toBe(true);
    expect(inRange(0, PORT_RANGE)).toBe(false);
    expect(inRange(65536, PORT_RANGE)).toBe(false);
    expect(inRange(99.5, PORT_RANGE)).toBe(false);
    expect(inRange(Number("abc"), PORT_RANGE)).toBe(false);
  });
});

describe("a refused IMAP connect", () => {
  it("says a refused login in the catalog's words, on either form", () => {
    expect(imapErrorMessage(loginRejected, t)).toBe(
      en["connectors.imapLoginRejected"],
    );
    expect(imapRefusal(loginRejected, t, { "imap.port": "Port" })).toEqual({
      byField: new Map(),
      rest: en["connectors.imapLoginRejected"],
    });
  });

  it("puts a field the form labels on that field, with nothing left at the foot", () => {
    const refused = imapRefusal(portRefused, t, { "imap.port": "Port" });
    expect(refused.byField.get("imap.port")).toBe(
      "Port must be from 1 to 65,535.",
    );
    expect(refused.rest).toBeNull();
  });
});
