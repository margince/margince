// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { ActionRow } from "../../design-system/actionrow";
import { Button } from "../../design-system/atoms";
import {
  CopyableText,
  useClipboardCopy,
} from "../../design-system/clipboardcopy";
import { Disclosure } from "../../design-system/disclosure";
import { FactList } from "../../design-system/factlist";
import { Panel, PanelBody } from "../../design-system/panel";
import { formatDateTime, formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { type Translator, useLocale, usePlural, useT } from "../../i18n";
import { healthScope, useHealthScope } from "../../lib/healthscope";
import { navigate } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";
import { jobKindNameKey } from "./jobkinds";
import {
  healthCodes,
  healthMeasures,
  healthRecovery,
  healthResponsible,
} from "./systemhealth.copy";
import { findingEffectKey, findingNextStepKey } from "./systemhealth.effect";
import { readHealthReport } from "./systemhealth.report";
import { healthSectionForSource } from "./systemhealthnavigation";

type Finding = components["schemas"]["HealthFinding"];

export function FindingPanel({
  finding,
  detail = false,
}: Readonly<{ finding: Finding; detail?: boolean }>) {
  const t = useT();
  const client = useQueryClient();
  const scope = useHealthScope();
  const copy = useClipboardCopy(
    {
      key: JSON.stringify([scope, finding]),
      read: async () => {
        const report = await client.fetchQuery({
          queryKey: ["system-health-report", scope, finding],
          queryFn: ({ signal }) => readHealthReport(finding, signal),
          staleTime: 0,
        });
        if (healthScope(client.getQueryData(["me"])) !== scope)
          throw new Error("Reader changed");
        return report;
      },
    },
    {
      copy: t("health.copy"),
      copied: t("health.copied"),
      remedy: t("health.copyRemedy"),
      pending: t("health.loading"),
      failureTitle: t("health.reportCopyFailed"),
    },
  );
  const title = findingTitle(finding, t);
  const destination = finding.action.destination;
  const go = () => {
    client.invalidateQueries({ queryKey: ["system-health"] });
    if (destination === "models" || destination === "connections")
      navigate(settingsHref(destination));
    else
      navigate(
        {
          ...settingsHref("system-health"),
          id2:
            destination === "configuration" ? "configuration" : "diagnostics",
        },
        new Map([
          ["section", healthSectionForSource(finding.source) ?? ""],
          ["finding", finding.id],
          ["state", finding.code === "queue_unverified" ? "discarded" : ""],
          [
            "kind",
            finding.job_kinds?.length === 1 ? (finding.job_kinds[0] ?? "") : "",
          ],
        ]),
      );
  };
  return (
    <Panel title={title}>
      <PanelBody>
        <p>
          <strong>{t("health.effectUntilFixed")}</strong>{" "}
          {t(findingEffectKey(finding))}
        </p>
        <p>{t(findingNextStepKey(finding))}</p>
        <ActionRow>
          {!detail && (
            <Button onClick={go} disabled={!finding.action.available}>
              {t(destinationLabels[destination] ?? "health.inspect")}
            </Button>
          )}
          {detail && (
            <Button pending={copy.pending} onClick={copy.copy}>
              {copy.label}
            </Button>
          )}
        </ActionRow>
        {copy.notice}
        {copy.notice && copy.preparedText && (
          <CopyableText text={copy.preparedText} label={t("health.copy")} />
        )}
        {detail ? (
          <FindingFacts finding={finding} />
        ) : (
          <Disclosure summary={t("health.evidenceDetails")}>
            <FindingFacts finding={finding} />
            <Button pending={copy.pending} onClick={copy.copy}>
              {copy.label}
            </Button>
          </Disclosure>
        )}
      </PanelBody>
    </Panel>
  );
}

const destinationLabels: Readonly<
  Record<string, "health.openModels" | "health.openConnections">
> = {
  models: "health.openModels",
  connections: "health.openConnections",
};
function findingTitle(finding: Finding, t: Translator) {
  const kindKey =
    finding.job_kinds?.length === 1
      ? jobKindNameKey(finding.job_kinds[0] ?? "")
      : undefined;
  return finding.code === "queue_unverified" && kindKey
    ? t("health.jobStopped", { kind: t(kindKey) })
    : t(healthCodes[finding.code] ?? "health.unknown");
}

function FindingFacts({ finding }: Readonly<{ finding: Finding }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const date = (at?: string) =>
    at ? formatDateTime(at, locale, viewerZone()) : t("health.unknown");
  return (
    <FactList
      facts={[
        {
          key: "impact",
          term: t("health.impact"),
          value: t(
            finding.impact === "measured"
              ? "health.measuredImpact"
              : finding.impact === "possible"
                ? "health.possibleImpact"
                : "health.unknownImpact",
          ),
        },
        {
          key: "scope",
          term: t("health.scope"),
          value:
            finding.count !== undefined && finding.unit
              ? plural(healthMeasures[finding.unit], finding.count, {
                  count: formatNumber(finding.count, locale),
                })
              : t("health.unknown"),
        },
        {
          key: "window",
          term: t("health.window"),
          value:
            finding.window_start && finding.window_end
              ? `${date(finding.window_start)} – ${date(finding.window_end)}`
              : t("health.currentRetained"),
        },
        {
          key: "observed",
          term: t("health.observed"),
          value: date(finding.observed_at),
        },
        {
          key: "first",
          term: t("health.firstObserved"),
          value: date(finding.first_observed_at),
        },
        {
          key: "next",
          term: t("health.nextAttempt"),
          value: date(finding.next_attempt_at),
        },
        {
          key: "responsible",
          term: t("health.responsible"),
          value: t(healthResponsible[finding.responsible]),
        },
        {
          key: "recovery",
          term: t("health.recovery"),
          value: t(healthRecovery[finding.recovery] ?? "health.unknown"),
        },
      ]}
    />
  );
}
