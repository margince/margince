/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { SETTINGS_PAGES } from "../../lib/settingscatalog";
import { meFixture } from "../../testing/mefixture";
import {
  SETTINGS_HOME_ID,
  SETTINGS_SCREEN,
  useSettingsSection,
} from "./settingsnav";
import { PAGE_ICONS } from "./settingsnavicons";

afterEach(cleanup);

// The overview's glyph as the rail draws it, read off the level itself.
function overviewGlyph(): unknown {
  const client = new QueryClient();
  client.setQueryData(["me"], meFixture({ roles: ["rep"] }));
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client }, children);
  const { result } = renderHook(
    () => useSettingsSection({ screen: SETTINGS_SCREEN }),
    { wrapper },
  );
  const home = result.current.groups
    .flatMap((group) => group.items)
    .find((entry) => entry.id === SETTINGS_HOME_ID);
  return home?.icon;
}

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

  it("keeps the overview's glyph off every page", () => {
    const glyph = overviewGlyph();
    expect(glyph).toBeDefined();
    expect(
      SETTINGS_PAGES.filter((page) => PAGE_ICONS[page.id] === glyph),
    ).toEqual([]);
  });
});
