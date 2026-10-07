import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Disclosure } from "../design-system/atoms";
import { useLocale, useT } from "../i18n";
import { QueryGate, throwProblem } from "./common";
import { reportingAmount } from "./reporting.model";

type Target = components["schemas"]["ReportingTarget"];

export function TargetHistory({ target }: Readonly<{ target: Target }>) {
  const t = useT();
  const { locale } = useLocale();
  const history = useQuery({
    queryKey: ["reporting-target", target?.id],
    enabled: !!target,
    queryFn: async () => {
      const { data, error } = await api.GET("/analytics/targets/{id}", {
        params: { path: { id: target?.id ?? "" } },
      });
      if (error) throwProblem(error);
      return data;
    },
  });
  return (
    <Disclosure summary={t("reporting.targetHistory")}>
      <QueryGate query={history} pendingLabel={t("reporting.targetHistory")}>
        {(result) => (
          <ol>
            {result.history
              ?.map((definition, index) => ({
                definition,
                revision: index + 1,
              }))
              .map(({ definition: revision, revision: number }) => (
                <li key={number}>
                  {reportingAmount(
                    revision.value,
                    target.unit,
                    target.unit === "count" ? "" : target.unit,
                    locale,
                  )}{" "}
                  · {revision.reason}
                  {revision.retired ? ` · ${t("reporting.retired")}` : ""}
                </li>
              ))}
          </ol>
        )}
      </QueryGate>
    </Disclosure>
  );
}
