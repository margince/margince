// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  extensionApps,
  extensionPanels,
  extensionScreens,
} from "@composition/screens";
import type {
  ExtensionAppRegistry,
  ExtensionPanelRegistry,
  ExtensionScreenRegistry,
} from "./extensions";

// The component registries an installation composes, typed at their one
// import site, so a unit export that is not a component fails here.
//
// Loading the registry loads every unit, and a unit imports the app surface
// (surface/index.ts). So only the screens that mount a unit import this file.

export const composedScreens: ExtensionScreenRegistry = extensionScreens;

export const composedPanels: ExtensionPanelRegistry = extensionPanels;

export const composedApps: ExtensionAppRegistry = extensionApps;
