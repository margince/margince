// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { greetingNameOf } from "./greetingname";

describe("greetingNameOf", () => {
  it("greets the name a colleague chose", () => {
    expect(
      greetingNameOf({
        greeting_name: "Sofia",
        display_name: "Dr. Sofia Meier",
      }),
    ).toBe("Sofia");
  });

  it("falls back to the display name's first word when none was chosen", () => {
    expect(greetingNameOf({ display_name: "  Ada Lovelace " })).toBe("Ada");
    expect(
      greetingNameOf({ greeting_name: null, display_name: "Ada Lovelace" }),
    ).toBe("Ada");
  });

  // A blank greeting name is not a choice; greeting " " would greet nobody.
  it("reads a blank greeting name as none", () => {
    expect(
      greetingNameOf({ greeting_name: "   ", display_name: "Ada Lovelace" }),
    ).toBe("Ada");
  });

  it("answers null when nothing names the seat", () => {
    expect(greetingNameOf(undefined)).toBeNull();
    expect(greetingNameOf({ display_name: "  " })).toBeNull();
  });
});
