// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { lazy } from "react";

const SettingsFeature = lazy(() =>
  import("./features/settings/settings").then((module) => ({
    default: module.SettingsScreen,
  })),
);

// Its own chunk too: the first-run form is drawn on one settings page, and
// only while the installation binds no model.
const AiSetupPanel = lazy(() =>
  import("./features/sign-in/installation-setup.ai-later").then((module) => ({
    default: module.AiSetupPanel,
  })),
);

import { reviewActions } from "./App.reviewactions";
import type { Route } from "./lib/router";

export function SettingsScreen({ route }: Readonly<{ route: Route }>) {
  return (
    <SettingsFeature
      route={route}
      reviewActions={reviewActions}
      aiSetup={<AiSetupPanel />}
    />
  );
}
