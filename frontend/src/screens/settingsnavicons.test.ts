// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { House } from "lucide-react";
import { describe, expect, it } from "vitest";
import { SETTINGS_PAGES } from "./settingscatalog";
import { PAGE_ICONS } from "./settingsnavicons";

describe("the settings page glyphs", () => {
  it("gives every page a glyph no other page carries", () => {
    const owners = new Map<unknown, string[]>();
    for (const page of SETTINGS_PAGES) {
      const glyph = PAGE_ICONS[page.id];
      owners.set(glyph, [...(owners.get(glyph) ?? []), page.id]);
    }
    const shared = [...owners.values()].filter((ids) => ids.length > 1);
    expect(shared).toEqual([]);
  });

  it("keeps the overview's house glyph off every page", () => {
    expect(
      SETTINGS_PAGES.filter((page) => PAGE_ICONS[page.id] === House),
    ).toEqual([]);
  });
});
