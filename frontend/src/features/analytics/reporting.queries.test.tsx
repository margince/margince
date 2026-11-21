/** @vitest-environment happy-dom */
import { QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../../i18n";
import { reportingStoryCatalog } from "../../testing/reporting.fixtures";
import { storyQueryClient } from "../../testing/story-utils";
import {
  MetricLabels,
  reportingCatalogKey,
  useMetricNames,
} from "./reporting.queries";

afterEach(cleanup);

// A catalog that offers open pipeline and withholds sales won.
function namesUnder(
  labels: Parameters<typeof MetricLabels.Provider>[0]["value"],
) {
  const client = storyQueryClient();
  client.setQueryDefaults(reportingCatalogKey, {
    staleTime: Number.POSITIVE_INFINITY,
  });
  client.setQueryData(reportingCatalogKey, {
    ...reportingStoryCatalog,
    metrics: reportingStoryCatalog.metrics.filter(
      (metric) => metric.id !== "bookings_won",
    ),
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <MetricLabels.Provider value={labels}>{children}</MetricLabels.Provider>
      </LocaleProvider>
    </QueryClientProvider>
  );
  return renderHook(() => useMetricNames(), { wrapper }).result.current;
}

describe("useMetricNames", () => {
  it("names an offered metric by the view's label over the catalog's", () => {
    const name = namesUnder({
      open_pipeline: "Open pipeline closing this quarter",
    });
    expect(name("open_pipeline")).toBe("Open pipeline closing this quarter");
    expect(name("stage_age")).toBe("Time in current stage");
  });

  it("names a metric the catalog withholds as unavailable, whatever the view calls it", () => {
    const name = namesUnder({ bookings_won: "Sales won this quarter" });
    expect(name("bookings_won")).toBe("Metric no longer available");
  });
});
