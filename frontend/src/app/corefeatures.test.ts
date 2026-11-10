// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { grants } from "../lib/accessgrants";
import {
  type CoreFeature,
  recordEnabled,
  screenEnabled,
} from "../lib/corefeatures";
import { meFixture } from "../testing/mefixture";
import { firstAvailableRoute, NAV, railTrail } from "./nav";

const FEATURES: readonly CoreFeature[] = [
  "home",
  "contacts",
  "companies",
  "leads",
  "deals",
  "projects",
  "filters",
  "analytics",
];

describe("disabled main-menu features", () => {
  it("covers every core menu destination", () => {
    expect(NAV.map((item) => item.screen).sort()).toEqual([...FEATURES].sort());
  });
  it.each(FEATURES)(
    "removes %s from the real rail while keeping the other destinations",
    (feature) => {
      const [primary] = railTrail({ screen: "home" }, undefined, [feature]);
      const rows = primary.groups.flatMap((group) =>
        group.items.map((item) => item.id),
      );
      expect(rows).not.toContain(feature);
      for (const other of FEATURES.filter((entry) => entry !== feature))
        expect(rows).toContain(other);
    },
  );
  it("keeps Settings reachable when every core feature is off", () => {
    expect(firstAvailableRoute(FEATURES)).toEqual({ screen: "settings" });
    expect(firstAvailableRoute(["home"])).toEqual({
      screen: NAV.find((item) => item.screen !== "home")?.screen,
    });
  });
  it("blocks dependent pages and references through the same decision", () => {
    expect(screenEnabled("offers", ["deals"])).toBe(false);
    expect(screenEnabled("partners", ["companies"])).toBe(false);
    expect(screenEnabled("lists", ["filters"])).toBe(false);
    expect(recordEnabled("deal", ["deals"])).toBe(false);
    expect(recordEnabled("company", ["deals"])).toBe(true);
    expect(recordEnabled("deal", [])).toBe(true);
  });
  it("hides controls without changing the role snapshot", () => {
    const snapshot = meFixture({
      allow: { deal: ["read", "create"], contact: ["read"] },
      settingsAvailability: { disabled_features: ["deals"] },
    });
    expect(snapshot.authorization?.objects?.deal?.create).toBe(true);
    expect(grants(snapshot, "deal", "create")).toBe(false);
    expect(grants(snapshot, "contact", "read")).toBe(true);
    const enabled = {
      ...snapshot,
      settings_availability: {
        ...snapshot.settings_availability,
        disabled_features: [],
      },
    };
    expect(grants(enabled, "deal", "create")).toBe(true);
  });
});
