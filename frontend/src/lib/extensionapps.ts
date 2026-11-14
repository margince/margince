// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type ExtensionAppDescriptor,
  type ExtensionDescriptor,
  extensions,
} from "@composition/extensions";
import { extensionApps } from "@composition/screens";
import type { ComponentType } from "react";
import { type AccessSnapshot, grants } from "./accessgrants";
import { isExtensionRbacObject } from "./extensions";
import type { Route } from "./router";
import { useMe } from "./session";

// A unit's app: its own screen as a destination in the home side drawer, at
// `#/app/<unit>`. It is a second door to the unit, apart from the sub-setting
// at `#/ext/<unit>`. A unit declares it in Go (extension.App); the registry
// below is what the generator derived from that declaration.

/** The route segment an app lives under: `#/app/<name>`. */
export const EXTENSION_APP_SCREEN = "app";

/**
 * The drawer headings an app may sit under. Mirrors extension.AppGroups() in
 * backend/pkg/extension/app.go; backend/gates/frontendentryslots_test.go fails
 * when the two lists differ.
 */
export const EXTENSION_APP_GROUPS = [
  "records",
  "work",
  "intelligence",
] as const;

export type ExtensionAppGroup = (typeof EXTENSION_APP_GROUPS)[number];

/** An app with the unit that declared it, which is where it leads. */
export type UnitApp = ExtensionAppDescriptor & { readonly unit: string };

/**
 * The components the units' frontend packages export as their app, by unit.
 * Annotated here, at the registry's one import site, so a unit's export that
 * is not a component fails this assignment.
 */
export type ExtensionAppRegistry = Readonly<
  Record<string, ComponentType | undefined>
>;

export const composedApps: ExtensionAppRegistry = extensionApps;

/** Every composed unit's app, in registry order. */
export function declaredApps(
  registry: readonly ExtensionDescriptor[] = extensions,
): readonly UnitApp[] {
  return registry.flatMap((unit) =>
    unit.app ? [{ ...unit.app, unit: unit.name }] : [],
  );
}

/** The app unit `name` declares, or null when it is unknown or declares none. */
export function findApp(
  name: string | undefined,
  registry: readonly ExtensionDescriptor[] = extensions,
): UnitApp | null {
  return declaredApps(registry).find((app) => app.unit === name) ?? null;
}

/**
 * Whether this reader may open the app. One predicate for the drawer entry and
 * the route, so a role that is not shown an entry is also refused the address.
 */
export function mayOpenApp(snapshot: AccessSnapshot, app: UnitApp): boolean {
  return (
    isExtensionRbacObject(app.object) && grants(snapshot, app.object, "read")
  );
}

/** The apps this reader may open. */
export function appsFor(
  snapshot: AccessSnapshot,
  registry: readonly ExtensionDescriptor[] = extensions,
): readonly UnitApp[] {
  return declaredApps(registry).filter((app) => mayOpenApp(snapshot, app));
}

/**
 * The app a route opens for this reader, or null. The page chrome names the
 * page from it, so a refused reader is not told the app's name either.
 */
export function useOpenableApp(route: Route): UnitApp | null {
  const me = useMe();
  const app = route.screen === EXTENSION_APP_SCREEN ? findApp(route.id) : null;
  return app && mayOpenApp(me.data, app) ? app : null;
}

/** `#/app/<unit>`. */
export function appRoute(unit: string): Route {
  return { screen: EXTENSION_APP_SCREEN, id: unit };
}
