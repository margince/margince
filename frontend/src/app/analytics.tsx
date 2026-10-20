import { AnalyticsScreen as AnalyticsFeature } from "../features/analytics/analytics";
import { ReviewActionsProvider } from "./reviewactions";

export function AnalyticsScreen() {
  return (
    <ReviewActionsProvider>
      <AnalyticsFeature />
    </ReviewActionsProvider>
  );
}
