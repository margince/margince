import { lazy } from "react";
import { reviewActions } from "./reviewactions";

const AnalyticsFeature = lazy(() =>
  import("../features/analytics/analytics").then((module) => ({
    default: module.AnalyticsScreen,
  })),
);

export function AnalyticsScreen() {
  return <AnalyticsFeature reviewActions={reviewActions} />;
}
