// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { grants, type RbacAction, type RbacObject } from "../lib/accessgrants";
import {
  type CoreFeature,
  recordEnabled,
  screenEnabled,
} from "../lib/corefeatures";
import { meFixture } from "../testing/mefixture";
import { firstAvailableRoute, NAV, railTrail, walkUpTarget } from "./nav";

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
    expect(NAV.map((item) => item.screen)).toEqual(
      expect.arrayContaining([...FEATURES]),
    );
    expect(NAV).toHaveLength(FEATURES.length);
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
  it("walking out of Settings cannot return to a disabled page", () => {
    expect(walkUpTarget(undefined, { screen: "home" }, ["home"])).toEqual(
      firstAvailableRoute(["home"]),
    );
    expect(walkUpTarget(undefined, { screen: "contacts" }, [])).toEqual({
      screen: "contacts",
    });
    expect(walkUpTarget(undefined, { screen: "home" }, FEATURES)).toEqual({
      screen: "settings",
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
  it.each<{ object: RbacObject; disabled: CoreFeature[] }>([
    { object: "product", disabled: ["deals"] },
    { object: "pipeline", disabled: ["deals"] },
    { object: "offer", disabled: ["deals"] },
    { object: "saved_view", disabled: ["filters"] },
    { object: "commission", disabled: ["analytics"] },
    { object: "commission", disabled: ["deals"] },
    { object: "report_definition", disabled: ["analytics"] },
  ])(
    "hides $object controls when their feature is off",
    ({ object, disabled }) => {
      const actions: readonly RbacAction[] = ["read"];
      const allow = { [object]: actions };
      const snapshot = meFixture({
        allow,
        settingsAvailability: { disabled_features: disabled },
      });
      expect(snapshot.authorization?.objects?.[object]?.read).toBe(true);
      expect(grants(snapshot, object, "read")).toBe(false);
      const enabled = meFixture({
        allow,
        settingsAvailability: { disabled_features: [] },
      });
      expect(grants(enabled, object, "read")).toBe(true);
    },
  );
  it("hides controls without changing the role snapshot", () => {
    const allow: Partial<Record<RbacObject, readonly RbacAction[]>> = {
      deal: ["read", "create"],
      contact: ["read"],
    };
    const snapshot = meFixture({
      allow,
      settingsAvailability: { disabled_features: ["deals"] },
    });
    expect(snapshot.authorization?.objects?.deal?.create).toBe(true);
    expect(grants(snapshot, "deal", "create")).toBe(false);
    expect(grants(snapshot, "contact", "read")).toBe(true);
    const enabled = meFixture({
      allow,
      settingsAvailability: { disabled_features: [] },
    });
    expect(grants(enabled, "deal", "create")).toBe(true);
  });
});
