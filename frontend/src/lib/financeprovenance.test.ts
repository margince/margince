// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { translate } from "../i18n";
import { financeSourceName } from "./financeprovenance";

const t = (key: Parameters<typeof translate>[1]) => translate("en", key);

describe("financeSourceName", () => {
  it("prefers the declared name, then the unit's description", () => {
    expect(
      financeSourceName(t, { provider_name: "Ledger", description: "Reads." }),
    ).toBe("Ledger");
    expect(financeSourceName(t, { description: "Reads." })).toBe("Reads.");
  });

  it("calls a source that declares neither Unnamed source", () => {
    expect(financeSourceName(t, {})).toBe("Unnamed source");
  });
});
