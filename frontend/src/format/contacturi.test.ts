// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { mailbox, telUri } from "./contacturi";

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
