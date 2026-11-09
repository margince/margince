// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { lazy } from "../../i18n/lazy";

export const ForecastView = lazy(async () => ({
  default: (await import("./analytics.forecast")).ForecastView,
}));
export const SharedForecastView = lazy(async () => ({
  default: (await import("./analytics.forecast")).SharedForecastView,
}));
export const FramedSection = lazy(async () => ({
  default: (await import("./analytics.framed")).FramedSection,
}));
export const QuestionsView = lazy(async () => ({
  default: (await import("./analytics.questions")).QuestionsView,
}));
export const ReportingDefinitions = lazy(async () => ({
  default: (await import("./reporting.definitions")).ReportingDefinitions,
}));
export const ReportingLibrary = lazy(async () => ({
  default: (await import("./reporting.library")).ReportingLibrary,
}));
export const ReportingTargets = lazy(async () => ({
  default: (await import("./reporting.targets")).ReportingTargets,
}));
export const ReportBoard = lazy(async () => ({
  default: (await import("./analytics.reportboard")).ReportBoard,
}));

export const ReportingOverview = lazy(async () => ({
  default: (await import("./reporting.overview")).ReportingOverview,
}));
