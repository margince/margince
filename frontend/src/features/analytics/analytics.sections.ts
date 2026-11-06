// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { lazy } from "react";

export const ForecastView = lazy(() =>
  import("./analytics.forecast").then((module) => ({
    default: module.ForecastView,
  })),
);
export const SharedForecastView = lazy(() =>
  import("./analytics.forecast").then((module) => ({
    default: module.SharedForecastView,
  })),
);
export const FramedSection = lazy(() =>
  import("./analytics.framed").then((module) => ({
    default: module.FramedSection,
  })),
);
export const QuestionsView = lazy(() =>
  import("./analytics.questions").then((module) => ({
    default: module.QuestionsView,
  })),
);
export const ReportingDefinitions = lazy(() =>
  import("./reporting.definitions").then((module) => ({
    default: module.ReportingDefinitions,
  })),
);
export const ReportingLibrary = lazy(() =>
  import("./reporting.library").then((module) => ({
    default: module.ReportingLibrary,
  })),
);
export const ReportingTargets = lazy(() =>
  import("./reporting.targets").then((module) => ({
    default: module.ReportingTargets,
  })),
);
export const ReportBoard = lazy(() =>
  import("./analytics.reportboard").then((module) => ({
    default: module.ReportBoard,
  })),
);
