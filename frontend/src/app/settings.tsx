// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { lazy } from "react";

const SettingsFeature = lazy(() =>
  import("../features/settings/settings").then((module) => ({
    default: module.SettingsScreen,
  })),
);

import type { Route } from "../lib/router";
import { reviewActions } from "./reviewactions";

export function SettingsScreen({ route }: Readonly<{ route: Route }>) {
  return <SettingsFeature route={route} reviewActions={reviewActions} />;
}
