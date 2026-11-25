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

// The component registries an installation composes, annotated once.
//
// The generated file is emitted untyped: it is written to two paths at
// different depths and must be byte-identical, so it carries no type import.
// The annotation lands here, at the registry's one import site, so a unit
// export that is not a component fails it. It is a module of its own because
// loading the registry loads every unit, and a unit imports the app surface
// (surface/index.ts). Only the screens that mount a unit import this module.

export const composedScreens: ExtensionScreenRegistry = extensionScreens;

export const composedPanels: ExtensionPanelRegistry = extensionPanels;

export const composedApps: ExtensionAppRegistry = extensionApps;
