// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { grants, type RbacAction, type RbacObject } from "../lib/accessgrants";
import {
  type CoreFeature,
  READ_REQUIREMENTS,
  recordEnabled,
  screenEnabled,
  unreadableCoreFeatures,
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
    for (const item of NAV)
      expect(
        FEATURES.some((feature) => !screenEnabled(item.screen, [feature])),
      ).toBe(true);
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
  it("keeps unknown record kinds independent of object prototypes", () => {
    expect(recordEnabled("constructor", FEATURES)).toBe(true);
    expect(recordEnabled("__proto__", FEATURES)).toBe(true);
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

describe("destinations the reader may not read", () => {
  const COMPOSITE = ["home", "filters"];
  const named = (screen: string) =>
    READ_REQUIREMENTS.some(([withheld]) =>
      NAV.some(
        (item) =>
          item.screen === screen && !screenEnabled(item.screen, [withheld]),
      ),
    );

  it("names what each rail row needs, or says it gathers several reads", () => {
    const unnamed = NAV.map((item) => item.screen).filter(
      (screen) => !named(screen),
    );
    expect(unnamed).toEqual(COMPOSITE);
  });

  it("takes a destination off the rail when its read is not granted", () => {
    const snapshot = meFixture({
      roles: ["support"],
      allow: { contact: ["read"], company: ["read"], lead: ["read"] },
    });
    const hidden = unreadableCoreFeatures(snapshot);
    const [primary] = railTrail({ screen: "home" }, undefined, hidden);
    const rows = primary.groups.flatMap((group) =>
      group.items.map((item) => item.id),
    );
    for (const gone of ["deals", "projects", "analytics", "progress", "goals"])
      expect(rows).not.toContain(gone);
    expect(rows).toEqual(expect.arrayContaining(["home", "contacts", "leads"]));
  });

  it("offers Insights on either of its two reads, and Progress and Goals on their own", () => {
    const rows = (allow: Parameters<typeof meFixture>[0]) => {
      const [primary] = railTrail(
        { screen: "home" },
        undefined,
        unreadableCoreFeatures(meFixture(allow)),
      );
      return primary.groups.flatMap((group) =>
        group.items.map((item) => item.id),
      );
    };
    expect(rows({ allow: { forecast: ["read"] } })).toContain("analytics");
    expect(rows({ allow: { deal: ["read"] } })).toContain("analytics");
    expect(rows({ allow: { sales_target: ["read"] } })).toContain("goals");
    expect(rows({ allow: { sales_target: ["read"] } })).not.toContain(
      "progress",
    );
    expect(rows({ allow: { report_definition: ["read"] } })).toContain(
      "progress",
    );
  });

  it("offers every destination to a reader who reads everything they need", () => {
    const every = Object.fromEntries(
      READ_REQUIREMENTS.flatMap(([, objects]) =>
        objects.map((object) => [object, ["read"]]),
      ),
    );
    expect(unreadableCoreFeatures(meFixture({ allow: every }))).toEqual([]);
  });

  it("claims nothing before the snapshot has answered", () => {
    expect(unreadableCoreFeatures(undefined)).toEqual([]);
  });
});
