// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Describing an initiative or a custom report in plain words, and getting
// its setup back as a proposal the reader applies or dismisses. Nothing is
// saved by the proposal; the drawer's own Save is still the reader's press.

import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { WrittenBy } from "../../components/citations.list";
import { AiPending } from "../../design-system/aipending";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ErrorLine } from "../../design-system/errorline";
import { QuietVerbs } from "../../design-system/verbvolume";
import { countryOptions } from "../../format/assignedcountries";
import { industryWords } from "../../format/industry";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { problemCodeOf, unwrap } from "../../lib/problem";
import { blockLabel } from "./reporting.model";

export type SetupProposal = components["schemas"]["ReportSetupProposal"];
export type SetupSite = components["schemas"]["ReportSetupRequest"]["site"];
type Source = components["schemas"]["ReportSetupSource"];
type Period = NonNullable<
  components["schemas"]["ReportSetupSelection"]["period"]
>;

export function useSetupProposal(
  onProposal: (proposal: SetupProposal) => void,
) {
  return useMutation({
    mutationFn: async (ask: { site: SetupSite; description: string }) =>
      unwrap(await api.POST("/analytics/setup-proposal", { body: ask })),
    onSuccess: onProposal,
  });
}

/**
 * The ask under a description box, and its answer. The answer stays on
 * screen until the reader applies or dismisses it. So the words it was read
 * from and the phrases it could not use are read side by side.
 */
export function SetupProposalStep({
  site,
  description,
  onApply,
}: Readonly<{
  site: SetupSite;
  description: string;
  onApply: (proposal: SetupProposal) => void;
}>) {
  const t = useT();
  const [proposal, setProposal] = useState<SetupProposal | null>(null);
  const propose = useSetupProposal(setProposal);
  const words = description.trim();
  const ask = () => {
    if (words === "" || propose.isPending) return;
    setProposal(null);
    propose.mutate({ site, description: words });
  };
  const noModel =
    propose.isError && problemCodeOf(propose.error) === "ai_not_configured";
  if (noModel) {
    return <ErrorLine>{t("reporting.setup.noModel")}</ErrorLine>;
  }
  return (
    // A step inside a form whose footer holds the one fill, so it is quiet.
    <QuietVerbs>
      <div className="form-stack">
        <div className="form-actions">
          <Button
            variant="ai"
            onClick={ask}
            pending={propose.isPending}
            busyLabel={t("reporting.setup.busy")}
            disabled={words === ""}
          >
            {t(
              site === "initiative_setup"
                ? "reporting.setup.suggestInitiative"
                : "reporting.setup.suggestReport",
            )}
          </Button>
        </div>
        {propose.isPending && (
          <AiPending label={t("reporting.setup.busy")} lines={3} />
        )}
        {propose.isError && (
          <ErrorLine
            error={propose.error}
            actions={
              <Button variant="ghost" onClick={ask}>
                {t("reporting.setup.retry")}
              </Button>
            }
          />
        )}
        {proposal && (
          <ProposalCard
            proposal={proposal}
            onApply={() => {
              onApply(proposal);
              setProposal(null);
            }}
            onDismiss={() => setProposal(null)}
          />
        )}
      </div>
    </QuietVerbs>
  );
}

function ProposalCard({
  proposal,
  onApply,
  onDismiss,
}: Readonly<{
  proposal: SetupProposal;
  onApply: () => void;
  onDismiss: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const resolved = proposal.sources.length > 0;
  return (
    <Callout
      tone="ai"
      live="status"
      title={t(
        resolved
          ? "reporting.setup.proposedTitle"
          : "reporting.setup.nothingTitle",
      )}
      actions={
        <>
          {resolved && (
            <Button variant="primary" onClick={onApply}>
              {t("reporting.setup.apply")}
            </Button>
          )}
          <Button variant="ghost" onClick={onDismiss}>
            {t("reporting.setup.dismiss")}
          </Button>
        </>
      }
    >
      {resolved && (
        <ul className="assessment-lines">
          {proposal.sources.map((source) => (
            <li key={`${source.field}:${source.value}`}>
              {t("reporting.setup.source", {
                field: fieldName(source.field, t),
                value: valueName(source, proposal, t, locale),
                phrase: source.phrase,
              })}
            </li>
          ))}
        </ul>
      )}
      {proposal.clarifications.length > 0 && (
        <>
          <p className="t-caption">{t("reporting.setup.openTitle")}</p>
          <ul className="assessment-lines">
            {proposal.clarifications.map((line) => (
              <li key={line}>{line}</li>
            ))}
          </ul>
        </>
      )}
      {proposal.unsupported.length > 0 && (
        <>
          <p className="t-caption">{t("reporting.setup.unusedTitle")}</p>
          <ul className="assessment-lines">
            {proposal.unsupported.map((item) => (
              <li key={`${item.phrase}:${item.reason}`}>
                {t("reporting.setup.unusedItem", {
                  phrase: item.phrase,
                  reason: item.reason,
                })}
              </li>
            ))}
          </ul>
        </>
      )}
      <WrittenBy by={proposal.generated_by} />
    </Callout>
  );
}

function fieldName(field: Source["field"], t: Translator): string {
  switch (field) {
    case "name":
      return t("reporting.setup.fieldName");
    case "product":
      return t("reporting.product");
    case "industry":
      return t("reporting.industry");
    case "country":
      return t("reporting.setup.fieldCountry");
    case "period":
      return t("reporting.period");
    case "metric":
      return t("reporting.setup.fieldMetric");
    case "chart":
      return t("reporting.setup.fieldChart");
    case "objective":
      return t("reporting.objective");
    case "target_value":
      return t("reporting.targetValue");
    case "deadline":
      return t("reporting.deadline");
  }
}

/** The value as the reader sees it: the screen's own words for a code. */
function valueName(
  source: Source,
  proposal: SetupProposal,
  t: Translator,
  locale: Locale,
): string {
  switch (source.field) {
    case "industry":
      return industryWords(t, source.value);
    case "country":
      return (
        countryOptions(locale).find((option) => option.value === source.value)
          ?.label ?? source.value
      );
    case "period":
      return periodName(source.value, t);
    case "chart": {
      const kind = proposal.selection.blocks?.find(
        (block) => block === source.value,
      );
      return kind ? blockLabel(kind, t) : source.value;
    }
    default:
      return source.label ?? source.value;
  }
}

const PERIODS: readonly Period[] = [
  "this_week",
  "last_week",
  "this_month",
  "last_month",
  "this_quarter",
];

function periodName(value: string, t: Translator): string {
  const period = PERIODS.find((known) => known === value);
  return period ? t(`reporting.${period}`) : value;
}
