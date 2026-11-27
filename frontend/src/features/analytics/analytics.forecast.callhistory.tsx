// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { EntityRef } from "../../components/entityref";
import { EmptyState } from "../../design-system/atoms";
import { Disclosure } from "../../design-system/disclosure";
import { QueryGate } from "../../design-system/querystates";
import { formatAnalyticsMoney, formatDateTime } from "../../format/format";
import { type Locale, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import type { ForecastPeriod, ForecastReadings } from "./forecast.queries";

export function ForecastCallHistory({
  readings,
  period,
  locale,
}: Readonly<{
  readings: ForecastReadings;
  period: ForecastPeriod;
  locale: Locale;
}>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const allowed =
    !readings.call_withheld && readings.scope_kind !== "managed_teams";
  const query = useQuery({
    queryKey: [
      "forecast",
      "calls",
      period,
      readings.period_start,
      readings.scope_kind,
      readings.scope_id,
    ],
    queryFn: async () => {
      if (readings.scope_kind === "managed_teams") {
        throw new Error("the forecast population has no single call history");
      }
      return unwrap(
        await api.GET("/forecast/calls", {
          params: {
            query: {
              period,
              as_of: readings.period_start,
              scope_kind: readings.scope_kind,
              scope_id: readings.scope_id,
            },
          },
        }),
      );
    },
    enabled: open && allowed,
  });
  if (!allowed) return null;
  return (
    <Disclosure
      summary={t("forecast.callHistory")}
      open={open}
      onToggle={setOpen}
    >
      {open && (
        <QueryGate query={query} pendingLabel={t("forecast.callHistory")}>
          {(data) =>
            data.calls.length === 0 ? (
              <EmptyState>{t("forecast.noCallHistory")}</EmptyState>
            ) : (
              <ol>
                {data.calls.map((call) => (
                  <li key={call.id}>
                    <p className="t-caption">
                      {formatDateTime(
                        call.created_at,
                        locale,
                        readings.timezone,
                      )}
                      {" · "}
                      {formatAnalyticsMoney(
                        call.amount_minor,
                        call.currency,
                        locale,
                      )}
                      {" · "}
                      {t("forecast.callAuthor")}{" "}
                      <EntityRef kind="user" id={call.author_id} />
                    </p>
                    {call.note && <p>{call.note}</p>}
                  </li>
                ))}
              </ol>
            )
          }
        </QueryGate>
      )}
    </Disclosure>
  );
}
