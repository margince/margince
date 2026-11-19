import { useCallback } from "react";
import { api } from "../../api/client";
import { useIdempotencyKey } from "../../api/idempotencykey";
import type { components } from "../../api/schema";
import { unwrap } from "../../lib/problem";

// The one way a report is created. The key outlives a failed attempt, so a
// retry replays the first result when its answer was lost. A confirmed save
// renews the key for the next report.
export function useCreateReport() {
  const idempotency = useIdempotencyKey();
  return useCallback(
    async (body: components["schemas"]["ReportingReportInput"]) => {
      const report = unwrap(
        await api.POST("/analytics/reports", {
          params: { header: { "Idempotency-Key": idempotency.key } },
          body,
        }),
      );
      idempotency.renew();
      return report;
    },
    [idempotency],
  );
}
