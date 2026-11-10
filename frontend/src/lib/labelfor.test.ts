// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { MessageKey } from "../i18n/en";
import { labelFor, UNKNOWN_VALUE_LABEL, valueFor } from "./labelfor";

type Phase = "open" | "closed";

const TONE: Readonly<Record<Phase, "success" | "default">> = {
  open: "success",
  closed: "default",
};

const LABEL: Readonly<Record<Phase, MessageKey>> = {
  open: "lead.statusNew",
  closed: "lead.statusDisqualified",
};

// A value a newer server sends: the closed union cannot name it.
const later = (value: string) => value as Phase;

describe("valueFor", () => {
  it("answers what the map holds for a value it lists", () => {
    expect(valueFor(TONE, "open", "default")).toBe("success");
  });

  it("answers the fallback for a value the map does not list", () => {
    expect(valueFor(TONE, later("archived"), "default")).toBe("default");
  });

  it("answers undefined for a caller that has its own answer", () => {
    expect(valueFor(LABEL, later("archived"), undefined)).toBe(undefined);
  });

  // An object's own prototype is not a value the map lists.
  it("answers the fallback for a name every object inherits", () => {
    expect(valueFor(TONE, later("toString"), "default")).toBe("default");
  });
});

describe("labelFor", () => {
  it("answers the neutral label for a value the map does not list", () => {
    expect(labelFor(LABEL, "closed")).toBe("lead.statusDisqualified");
    expect(labelFor(LABEL, later("archived"))).toBe(UNKNOWN_VALUE_LABEL);
  });
});
