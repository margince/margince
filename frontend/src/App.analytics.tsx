import { reviewActions } from "./App.reviewactions";
import { AnalyticsScreen as AnalyticsFeature } from "./features/analytics/analytics";

export function AnalyticsScreen() {
  return <AnalyticsFeature reviewActions={reviewActions} />;
}
