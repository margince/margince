import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, StatCard } from "../design-system/atoms";
import { FactList } from "../design-system/factlist";
import { ReadingsGrid } from "../design-system/readingsgrid";
import { formatDateTime, formatDecimal, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../i18n";
import { throwProblem } from "./common";
import { leadScoreKey } from "./leadkeys";
import {
  leadStatusLabel,
  scoreFactorLabel,
  terminalBadge,
} from "./leadpresentation";
import { firstResponseClock } from "./leadstanding";

// The lead's four readings, in the cards every record page draws them in: how
// it scores and what made the score; whether anybody has answered, and how
// late; where it stands on the ladder; and which company it came from.
//
// Four, and these four. The first response is the one that was missing: the
// server runs a clock on every inbound lead and the page stated its verdict
// only as a line under the ladder, where a reader scanning the band for the
// lead that needs them never met it. Every slot states its absence in words —
// a lead with no company has none, and an empty slot would say the page failed
// to load one.

type Lead = components["schemas"]["Lead"];

export function LeadReadings({ lead }: Readonly<{ lead: Lead }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <ReadingsGrid label={t("lead.readings.title")} testId="lead-readings">
      <ScoreCard lead={lead} locale={locale} t={t} />
      <FirstResponseCard lead={lead} locale={locale} t={t} />
      <StatCard label={t("lead.status")} value={statusReading(lead, t)} />
      <StatCard
        label={t("create.companyName")}
        value={lead.company_name ?? t("lead.detailsUnset")}
      />
    </ReadingsGrid>
  );
}

/** The status as the readings state it: the terminal wording when it has one. */
export function statusReading(lead: Lead, t: Translator): string {
  const terminal = terminalBadge(lead.status);
  const label = terminal?.label ?? leadStatusLabel(lead.status);
  return label ? t(label) : lead.status;
}

// The score, with the factors that add up to it behind the figure. The bar is
// the score out of a hundred — the one reading on this page that HAS a
// denominator. An override says so in the detail, because the figure is then a
// human's and the factors sum to the machine's.
function ScoreCard({
  lead,
  locale,
  t,
}: Readonly<{ lead: Lead; locale: Locale; t: Translator }>) {
  const explain = useQuery({
    queryKey: leadScoreKey(lead.id),
    queryFn: async () => {
      const { data, error } = await api.GET("/leads/{id}/score", {
        params: { path: { id: lead.id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
  const factors = explain.data?.current?.factors ?? [];
  // The receipt behind the figure follows the read: still working, could not
  // be read (with the one thing to do about it), nothing counted, or the
  // factors. A basis that vanished on a failed read left the score looking
  // complete, which is the one thing a receipt must not do.
  const basis = explain.isPending ? (
    <p className="t-small">{t("lead.scoreLoading")}</p>
  ) : explain.isError ? (
    <>
      <p className="t-small">{t("lead.scoreFactorsFailed")}</p>
      <Button small variant="ghost" onClick={() => explain.refetch()}>
        {t("common.retry")}
      </Button>
    </>
  ) : factors.length > 0 ? (
    <FactList
      numeric
      facts={factors.map((factor) => ({
        key: factor.factor,
        term: scoreFactorLabel(factor.factor, t),
        value: formatDecimal(factor.points, locale, 1),
      }))}
    />
  ) : (
    <p className="t-small">{t("lead.scoreNoFactors")}</p>
  );
  return (
    <StatCard
      label={t("lead.score")}
      value={formatNumber(lead.score, locale)}
      numeric
      detail={
        lead.score_override_reason
          ? t("lead.overriddenBadge")
          : lead.score_reason
            ? scoreFactorLabel(lead.score_reason, t)
            : t("lead.scoreNoSignals")
      }
      meter={{ filled: lead.score, total: 100 }}
      basisLabel={t("co.strip.basis.reading")}
      basis={basis}
    />
  );
}

// Whether anybody has answered this lead, and how that stands against the
// installation's own target. The server derives the verdict; the card says it
// in a word and puts the deadline under it. A terminal lead owes no first
// response and the server sends no clock for one, so the card reads what
// happened rather than what is due.
function FirstResponseCard({
  lead,
  locale,
  t,
}: Readonly<{ lead: Lead; locale: Locale; t: Translator }>) {
  // The reader's own zone: the deadline is measured from the moment the lead
  // arrived, and a lead carries no workspace location of its own to prefer
  // over where the reader is.
  const zone = viewerZone();
  const label = t("lead.readings.firstResponse");
  if (lead.first_response_at) {
    // Answered, and only that: the server drops the clock once a response is
    // in, so whether it was on time is not a fact this card holds.
    return (
      <StatCard
        label={label}
        value={t("lead.readings.answered")}
        detail={t("lead.sla.answeredAt", {
          at: formatDateTime(lead.first_response_at, locale, zone),
        })}
      />
    );
  }
  const clock = firstResponseClock(lead);
  if (!clock) {
    // Nobody has answered and the installation runs no clock: owed, without
    // a deadline to be late against.
    return (
      <StatCard
        label={label}
        value={t("lead.readings.owed")}
        detail={t("lead.readings.noClock")}
      />
    );
  }
  const breached = clock.state === "breached";
  const atRisk = clock.state === "at_risk";
  return (
    <StatCard
      label={label}
      value={
        breached
          ? t("lead.sla.breached")
          : atRisk
            ? t("lead.sla.atRisk")
            : t("lead.sla.withinTarget")
      }
      detail={t(breached ? "lead.sla.overdueSince" : "lead.sla.dueBy", {
        at: formatDateTime(clock.deadline, locale, zone),
      })}
      tone={breached ? "danger" : atRisk ? "warn" : undefined}
      dot={breached || atRisk}
    />
  );
}
