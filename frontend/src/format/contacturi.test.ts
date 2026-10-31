// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { mailbox, messengerUris, primaryPhone, telUri } from "./contacturi";

type ContactPhone = components["schemas"]["ContactPhone"];

describe("mailbox", () => {
  it("admits an ordinary address, trimmed", () => {
    expect(mailbox(" dana@brandt.example ")).toBe("dana@brandt.example");
  });

  it("keeps the plus a mailbox may carry", () => {
    expect(mailbox("dana+crm@brandt.example")).toBe("dana+crm@brandt.example");
  });

  it("refuses an address that would carry a header or a second recipient", () => {
    // Each of these reaches whatever consumes the address as a second field or
    // a broken scheme, so none is admitted.
    for (const bad of [
      "dana@brandt.example?subject=hi",
      "dana@brandt.example&cc=x@y.example",
      "dana@brandt.example,x@y.example",
      "dana;x@brandt.example",
      "mailto:dana@brandt.example",
      "dana%0abcc:x@y.example@brandt.example",
      "dana brandt@example.com",
      "dana@brandt",
      "not an address",
      "",
    ]) {
      expect(mailbox(bad), bad).toBeNull();
    }
  });
});

describe("messengerUris", () => {
  it("opens the chat in each messenger with the number's digits", () => {
    expect(messengerUris("+49 30 1234567")).toEqual({
      whatsapp: "https://wa.me/49301234567",
      telegram: "https://t.me/+49301234567",
      zalo: "https://zalo.me/49301234567",
    });
  });

  it("answers no national number, which telUri still dials", () => {
    expect(messengerUris("030 1234567")).toBeNull();
    expect(telUri("030 1234567")).toBe("tel:0301234567");
  });

  it("answers only the shape the server stores as E.164", () => {
    for (const bad of [
      "+000 1234 5678",
      "+123",
      "+1234567",
      "+1234567890123456",
    ]) {
      expect(messengerUris(bad), bad).toBeNull();
    }
    expect(messengerUris("+12345678")?.whatsapp).toBe("https://wa.me/12345678");
    expect(messengerUris("+123456789012345")?.whatsapp).toBe(
      "https://wa.me/123456789012345",
    );
  });

  it("reads a number the way the server stores it", () => {
    // ParsePhone takes 00 for + and drops a bracketed trunk 0, which a caller
    // from abroad never dials. A bare 0 after the code stays, as in Italy.
    expect(messengerUris("0049 30 1234567")?.whatsapp).toBe(
      "https://wa.me/49301234567",
    );
    expect(messengerUris("+84 (0) 912 345 678")?.whatsapp).toBe(
      "https://wa.me/84912345678",
    );
    expect(messengerUris("+39 06 1234567")?.whatsapp).toBe(
      "https://wa.me/39061234567",
    );
    expect(messengerUris("+49 30\n1234567")).toBeNull();
  });

  it("lets nothing but the digits reach the address", () => {
    for (const bad of [
      "+49 30 1234567?text=hi",
      "+49 30 1234567/x",
      "+49 30 1234567%0a",
      "+49 30 1234567#x",
      "+49 30 12345 ext 7",
    ]) {
      expect(messengerUris(bad), bad).toBeNull();
    }
  });
});

describe("telUri", () => {
  it("dials a number written the way contacts write them", () => {
    expect(telUri("+33 6 12 44 08 91")).toBe("tel:+33612440891");
    expect(telUri("(030) 123-45.67")).toBe("tel:0301234567");
  });

  it("refuses a value that is not a number", () => {
    for (const bad of ["ask reception", "12", "+", "555-CALL", ""]) {
      expect(telUri(bad), bad).toBeNull();
    }
  });
});

describe("primaryPhone", () => {
  const row = (
    id: string,
    phone: string,
    isPrimary: boolean,
  ): ContactPhone => ({
    id,
    phone,
    phone_type: "work",
    is_primary: isPrimary,
    position: 0,
    source: "manual",
    captured_by: "human:u-1",
  });

  it("takes the primary number over the first", () => {
    const phones = [
      row("a", "+49 30 1111", false),
      row("b", "+49 30 2222", true),
    ];
    expect(primaryPhone(phones)?.phone).toBe("+49 30 2222");
  });

  it("skips a primary row with no number and falls back to the first live one", () => {
    const phones = [
      row("a", "", true),
      row("b", "+49 30 1111", false),
      row("c", "+49 30 2222", false),
    ];
    expect(primaryPhone(phones)?.phone).toBe("+49 30 1111");
  });

  it("finds nothing in no rows", () => {
    expect(primaryPhone(undefined)).toBeUndefined();
    expect(primaryPhone([])).toBeUndefined();
  });
});
