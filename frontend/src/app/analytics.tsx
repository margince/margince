import { AnalyticsScreen as AnalyticsFeature } from "../features/analytics/analytics";
import { reviewActions } from "./reviewactions";

export function AnalyticsScreen() {
  return <AnalyticsFeature reviewActions={reviewActions} />;
}
