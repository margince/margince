// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Panel, PanelBody } from "../../design-system/panel";
import {
  BulletChart,
  type ChartReading,
} from "../../design-system/report-charts";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { healthTone } from "./systemhealthstatus";

type Source = components["schemas"]["HealthSource"];

export function HealthReadings({
  sources,
  findings,
}: Readonly<{
  sources: Source[];
  findings: components["schemas"]["HealthFinding"][];
}>) {
  const t = useT();
  const { locale } = useLocale();
  const checked = sources.filter(
    (source) =>
      !["withheld", "not_configured", "not_monitored"].includes(
        source.availability,
      ),
  );
  if (checked.length === 0) return null;
  const failed = checked.filter((source) => healthTone(source) === "danger");
  const review = checked.filter(
    (source) =>
      healthTone(source) === "warning" &&
      findings.some(
        (finding) => finding.source === source.id && finding.requires_action,
      ),
  );
  const passing = checked.filter((source) => healthTone(source) === "success");
  const pending =
    checked.length - failed.length - review.length - passing.length;
  const reading = (
    key: string,
    label: string,
    value: number,
    tone?: ChartReading["tone"],
  ): ChartReading => ({
    key,
    label,
    value,
    amount: formatNumber(value, locale),
    tone,
  });
  const readings = [
    reading("failed", t("health.checksFailed"), failed.length, "danger"),
    reading("review", t("health.checksReview"), review.length, "warning"),
    reading("passing", t("health.checksPassing"), passing.length, "success"),
    ...(pending > 0
      ? [reading("pending", t("health.checksPending"), pending, "neutral")]
      : []),
  ];
  return (
    <Panel title={t("health.checkResults")}>
      <PanelBody>
        <BulletChart
          readings={readings}
          label={t("health.checkResults")}
          dataLabel={t("health.checkNumbers")}
          valueLabel={t("health.checkResults")}
          targetLabel={t("health.checkResults")}
        />
      </PanelBody>
    </Panel>
  );
}
