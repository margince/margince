// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { ActionRow } from "../../design-system/actionrow";
import { Badge, Button } from "../../design-system/atoms";
import {
  CopyableText,
  useClipboardCopy,
} from "../../design-system/clipboardcopy";
import { Disclosure } from "../../design-system/disclosure";
import { type Fact, FactList } from "../../design-system/factlist";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody } from "../../design-system/panel";
import { formatDateTime, formatNumber } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { type Translator, useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { healthScope, useHealthScope } from "../../lib/healthscope";
import { navigate } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";
import { jobKindNameKey } from "./jobkinds";
import {
  healthCodes,
  healthMeasures,
  healthRecovery,
  healthResponsible,
  healthSourceLabels,
} from "./systemhealth.copy";
import { findingEffectKey, findingNextStepKey } from "./systemhealth.effect";
import { readHealthReport } from "./systemhealth.report";
import { healthSectionForSource } from "./systemhealthnavigation";
import { healthTone } from "./systemhealthstatus";

type Finding = components["schemas"]["HealthFinding"];

export function FindingPanel({
  finding,
  related,
  detail = false,
}: Readonly<{ finding: Finding; related?: Finding[]; detail?: boolean }>) {
  const t = useT();
  const findings = related ?? [finding];
  const leading =
    findings.find((entry) => healthTone(entry) === "danger") ?? finding;
  return (
    <Panel
      title={
        related
          ? t(healthSourceLabels[finding.source] ?? "health.unknown")
          : findingTitle(finding, t)
      }
      titleAction={<FindingStatus finding={leading} />}
    >
      {findings.map((entry, index) => (
        <PanelBody key={entry.id}>
          {related && (
            <ActionRow>
              <Heading as="h3" size="small">
                {findingTitle(entry, t)}
              </Heading>
              <FindingStatus finding={entry} />
            </ActionRow>
          )}
          <FindingScope finding={entry} />
          {!findings
            .slice(0, index)
            .some(
              (previous) =>
                findingEffectKey(previous) === findingEffectKey(entry),
            ) && (
            <p>
              <strong>{t("health.effectUntilFixed")}</strong>{" "}
              {t(findingEffectKey(entry))}
            </p>
          )}
          {detail ? (
            <FindingFacts finding={entry} />
          ) : (
            <Disclosure summary={t("health.evidenceDetails")}>
              <FindingFacts finding={entry} />
            </Disclosure>
          )}
        </PanelBody>
      ))}
      <PanelBody>
        <p>{t(findingNextStepKey(finding))}</p>
        <FindingActions finding={finding} detail={detail} grouped={!!related} />
      </PanelBody>
    </Panel>
  );
}

function FindingScope({ finding }: Readonly<{ finding: Finding }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <p className="t-caption">
      {finding.count !== undefined && finding.unit
        ? plural(healthMeasures[finding.unit], finding.count, {
            count: formatNumber(finding.count, locale),
          })
        : t("health.scopeUnknown")}{" "}
      · {t(healthResponsible[finding.responsible])}
    </p>
  );
}

function FindingStatus({ finding }: Readonly<{ finding: Finding }>) {
  const t = useT();
  const tone = healthTone(finding);
  return (
    <Badge tone={tone}>
      {t(
        finding.code === "queue_unverified"
          ? "health.failedRunsReview"
          : tone === "danger"
            ? "health.needsFix"
            : "health.needsReview",
      )}
    </Badge>
  );
}

function FindingActions({
  finding,
  detail,
  grouped,
}: Readonly<{ finding: Finding; detail: boolean; grouped: boolean }>) {
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
  const destination = finding.action.destination;
  const params = new Map([
    ["section", healthSectionForSource(finding.source) ?? ""],
    ["finding", grouped ? "" : finding.id],
    ["state", finding.code === "queue_unverified" ? "discarded" : ""],
    [
      "kind",
      finding.job_kinds?.length === 1 ? (finding.job_kinds[0] ?? "") : "",
    ],
  ]);
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
        params,
      );
  };
  return (
    <>
      <ActionRow>
        {!detail && (
          <Button onClick={go} disabled={!finding.action.available}>
            {t(destinationLabels[destination] ?? findingActionLabel(finding))}
          </Button>
        )}
        <Button pending={copy.pending} onClick={copy.copy}>
          {copy.label}
        </Button>
      </ActionRow>
      {copy.notice}
      {copy.notice && copy.preparedText && (
        <CopyableText text={copy.preparedText} label={t("health.copy")} />
      )}
    </>
  );
}

function findingActionLabel(finding: Finding): MessageKey {
  if (finding.source === "jobs")
    return finding.code === "queue_unverified"
      ? "health.inspectRuns"
      : "health.inspectWork";
  if (finding.source === "capture") return "health.inspectMail";
  if (finding.source === "updates") return "health.inspectUpdates";
  return "health.inspect";
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
  if (finding.code === "queue_overdue" && kindKey)
    return t("health.jobDelayed", { kind: t(kindKey) });
  return finding.code === "queue_unverified" && kindKey
    ? t("health.jobStopped", { kind: t(kindKey) })
    : t(healthCodes[finding.code] ?? "health.unknown");
}

function FindingFacts({ finding }: Readonly<{ finding: Finding }>) {
  const t = useT();
  const { locale } = useLocale();
  const date = (at?: string) =>
    at ? formatDateTime(at, locale, viewerZone()) : t("health.unknown");
  const facts: Fact[] = [];
  if (finding.window_start && finding.window_end)
    facts.push({
      key: "window",
      term: t("health.window"),
      value: `${date(finding.window_start)} – ${date(finding.window_end)}`,
    });
  for (const [key, label, value] of [
    ["observed", "health.observed", finding.observed_at],
    ["first", "health.firstObserved", finding.first_observed_at],
    ["next", "health.nextAttempt", finding.next_attempt_at],
  ] satisfies [string, MessageKey, string | undefined][]) {
    if (value) facts.push({ key, term: t(label), value: date(value) });
  }
  if (healthRecovery[finding.recovery])
    facts.push({
      key: "recovery",
      term: t("health.recovery"),
      value: t(healthRecovery[finding.recovery] ?? "health.unknown"),
    });
  return <FactList facts={facts} />;
}
