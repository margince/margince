// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { MAX_PASSWORD, MIN_PASSWORD, passwordFault } from "./passwordrule";

describe("passwordFault", () => {
  it("leaves an empty field unanswered", () => {
    expect(passwordFault("")).toBeNull();
  });

  it("names the floor, the ceiling, blank and repeated passwords", () => {
    expect(passwordFault("short")).toBe("password.tooShort");
    expect(passwordFault("🔑🔑🔑🔑")).toBe("password.tooShort");
    expect(passwordFault("ab".repeat(MAX_PASSWORD))).toBe("password.tooLong");
    expect(passwordFault(" ".repeat(MIN_PASSWORD))).toBe("password.blank");
    expect(passwordFault("a".repeat(MIN_PASSWORD))).toBe("password.repeated");
    expect(passwordFault("🔑".repeat(MIN_PASSWORD))).toBe("password.repeated");
  });

  it("accepts a password at both bounds", () => {
    expect(passwordFault("ab".repeat(MIN_PASSWORD / 2))).toBeNull();
    expect(passwordFault("ab".repeat(MAX_PASSWORD / 2))).toBeNull();
    expect(passwordFault("correct horse battery")).toBeNull();
  });
});

describe("the server's corpus", () => {
  const corpus: { note: string; password: string; fault: string }[] =
    JSON.parse(
      readFileSync(
        new URL(
          "../../../../backend/internal/modules/identity/testdata/passwordrule.json",
          import.meta.url,
        ),
        "utf8",
      ),
    );
  const spelled = (fault: string | null) =>
    fault === "password.tooShort" || fault === "password.tooLong"
      ? "length"
      : (fault?.replace("password.", "") ?? "ok");

  it("reads the full corpus", () => {
    expect(corpus.length).toBeGreaterThanOrEqual(30);
  });

  it.each(corpus)("refuses $note as the server does", (entry) => {
    expect(spelled(passwordFault(entry.password))).toBe(entry.fault);
  });
});
