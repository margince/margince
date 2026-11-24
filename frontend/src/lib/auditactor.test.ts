// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { translate } from "../i18n";
import { qualifierPhrase } from "./auditactor";

const t = (key: Parameters<typeof translate>[1], p?: Record<string, string>) =>
  translate("en", key, p);

describe("qualifierPhrase", () => {
  it("names the client when the row records one", () => {
    expect(qualifierPhrase({ qualifierName: "Claude" }, t)).toBe("via Claude");
  });

  it("falls back to the generic phrase, and is empty without a machine", () => {
    expect(qualifierPhrase({ qualifierKey: "audit.viaAgent" }, t)).toBe(
      "via an agent",
    );
    expect(qualifierPhrase({}, t)).toBeUndefined();
  });
});
