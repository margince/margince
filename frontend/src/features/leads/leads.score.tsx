import { type ReactNode, useEffect, useState } from "react";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { Badge, Button, Field, TextInput } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import {
  OverviewCard,
  OverviewList,
  OverviewRow,
  OverviewText,
} from "../../design-system/overviewcard";
import { formatNumber, formatSignedNumber } from "../../format/format";
import { type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { originLabelFor, useLeadOrigins } from "../../lib/leadorigins";
import { scoreFactorLabel } from "./leadpresentation";
import type { Lead, LeadWriter } from "./leads";
import { FactorSources } from "./leads.score.sources";
import { useLeadScore } from "./leadscore.queries";
import { isEngagementFactor, isFitFactor } from "./leadstandingparts";

type ScoreFactor = components["schemas"]["LeadScoreFactor"];

const SIGNAL_KIND: Record<
  NonNullable<ScoreFactor["signal_kind"]>,
  MessageKey
> = {
  fact: "lead.factorKind.fact",
  assumption: "lead.factorKind.assumption",
  judgement: "lead.factorKind.judgement",
};

// The server's score grants points to a title naming a decision maker, by this
// pattern. It is mirrored here only to say why a title earned nothing; it never
// adds to the score, which the server computes.
const DECISION_MAKER_TITLE =
  /(chief|vp|head|director|founder|owner|c[a-z]o)\b/i;

// What a lead is missing, in the model's own terms. It shows when no retained
// breakdown exists yet.
//
// A zero score and an unscored lead show the same number and mean opposite
// things: "assessed, and it earns nothing" versus "not assessed". A rep reads
// both as a bad prospect, and only one is (ADR-0108). These reasons are always
// derivable, so the page states them rather than our own storage history.
function ScoreShortfall({ lead }: Readonly<{ lead: Lead }>) {
  const t = useT();
  const origins = useLeadOrigins();
  const missing: string[] = [];
  if (!lead.title) {
    missing.push(t("lead.shortfall.noTitle"));
  } else if (!DECISION_MAKER_TITLE.test(lead.title)) {
    missing.push(t("lead.shortfall.titleNotSenior", { title: lead.title }));
  }
  const origin = origins.data?.find((o) => o.key === lead.origin);
  if (!lead.origin) {
    // Split as the title pair above is: interpolating an absent value
    // would print "Came in as undefined" at a rep.
    missing.push(t("lead.shortfall.noOrigin"));
  } else if (origin?.intent === "low") {
    missing.push(
      t("lead.shortfall.originPenalised", {
        origin: originLabelFor(lead, origins.data) ?? lead.origin,
      }),
    );
  } else if (origin?.intent !== "high") {
    missing.push(
      t("lead.shortfall.originNoIntent", {
        origin: originLabelFor(lead, origins.data) ?? lead.origin,
      }),
    );
  }
  // This is not a claim that no reply or meeting exists. Engagement lives in
  // linked activities the client never reads. A decayed reply can round to
  // nothing while still being a reply, so "no reply yet" is a claim this page
  // cannot support. What it can say is what would move the score.
  missing.push(t("lead.shortfall.engagementMoves"));

  return (
    <div className="lead-stack-tight">
      <span>{t("lead.shortfall.lead")}</span>
      <ul className="lead-plainlist">
        {missing.map((reason) => (
          <li key={reason}>{reason}</li>
        ))}
      </ul>
    </div>
  );
}

// What kind of point a factor is, under its name. It is who the lead is, what they
// did (and in how many activities), or whose judgement a hand-set signal is.
function factorKind(factor: ScoreFactor, t: Translator): ReactNode {
  if (factor.signal_kind) {
    return (
      <>
        {t(SIGNAL_KIND[factor.signal_kind])}
        {factor.set_by && (
          <>
            {" · "}
            <EntityRef kind="user" id={factor.set_by} asText />
          </>
        )}
      </>
    );
  }
  if (isFitFactor(factor.factor)) {
    return t("lead.dims.fit");
  }
  if (!isEngagementFactor(factor.factor)) {
    return null;
  }
  if ((factor.source_activity_ids?.length ?? 0) === 0) {
    return t("lead.dims.engagement");
  }
  return (
    <>
      {t("lead.dims.engagement")}
      {" · "}
      <FactorSources factor={factor} />
    </>
  );
}

function FactorRow({ factor }: Readonly<{ factor: ScoreFactor }>) {
  const t = useT();
  const { locale } = useLocale();
  const points = Math.round(factor.points);
  return (
    <OverviewRow
      title={scoreFactorLabel(factor.factor, t)}
      sub={factorKind(factor, t)}
      right={
        <Badge tone={points < 0 ? "warning" : "success"}>
          {formatSignedNumber(points, locale)}
        </Badge>
      }
    />
  );
}

type ScoreReading = Readonly<{ body: ReactNode; foot?: string }>;

// The factors that add up to the score, one row each, and the one line under
// them. The server refuses the breakdown for an archived lead, so there the
// card keeps the figure alone rather than drawing a failure.
function useScoreReading(lead: Lead): ScoreReading {
  const t = useT();
  const { locale } = useLocale();
  const explain = useLeadScore(lead);
  const total = t("lead.scoreTotal", {
    score: formatNumber(lead.score, locale),
  });
  if (lead.archived_at) {
    return { body: <OverviewText>{total}</OverviewText> };
  }
  if (explain.isPending) {
    return { body: <OverviewText>{t("lead.scoreLoading")}</OverviewText> };
  }
  if (explain.isError) {
    return { body: <ErrorLine error={explain.error} /> };
  }
  const current = explain.data?.current;
  if (!explain.data?.explained || !current) {
    return {
      body:
        lead.score === 0 ? (
          <ScoreShortfall lead={lead} />
        ) : (
          <OverviewText>{t("lead.scoreNotStoredYet")}</OverviewText>
        ),
    };
  }
  const factors = current.factors ?? [];
  // The factors add up to the model's score, so the foot names that one.
  const sum = t("lead.scoreTotal", {
    score: formatNumber(current.score_computed, locale),
  });
  // Behavioural points decay; the server sends `base_points` only for them.
  const decays = factors.some((factor) => factor.base_points != null);
  return {
    body:
      factors.length === 0 ? (
        <OverviewText>{t("lead.scoreNoFactors")}</OverviewText>
      ) : (
        <OverviewList>
          {factors.map((factor) => (
            <FactorRow key={factor.factor} factor={factor} />
          ))}
        </OverviewList>
      ),
    foot: decays ? `${sum} · ${t("lead.scoreHalves")}` : sum,
  };
}

// The Commercial Judgement override: a score and the reason for it, saved on
// the same PATCH /leads/{id} every other edit here sends with If-Match.
function OverrideForm({
  writer,
  terminalReasonId,
  onCancel,
}: Readonly<{
  writer: LeadWriter;
  terminalReasonId: string;
  onCancel: () => void;
}>) {
  const t = useT();
  const [scoreValue, setScoreValue] = useState("");
  const [reasonValue, setReasonValue] = useState("");
  const parsedScore = Number(scoreValue);
  const scoreInvalid =
    scoreValue.trim() === "" ||
    !Number.isInteger(parsedScore) ||
    parsedScore < 0 ||
    parsedScore > 100;
  return (
    <div className="lead-stack lead-override">
      <Field label={t("lead.overrideScoreValue")}>
        {(control) => (
          <TextInput
            {...control}
            type="number"
            min={0}
            max={100}
            value={scoreValue}
            onChange={(event) => setScoreValue(event.target.value)}
          />
        )}
      </Field>
      <Field label={t("lead.overrideReason")}>
        {(control) => (
          <TextInput
            {...control}
            value={reasonValue}
            onChange={(event) => setReasonValue(event.target.value)}
          />
        )}
      </Field>
      {/* ds:ignore a lead line is a labelled row; this one happens to hold only verbs */}
      <div className="lead-line">
        <Button
          variant="primary"
          disabled={
            reasonValue.trim() === "" || scoreInvalid || writer.patch.isPending
          }
          reasonId={writer.readOnly ? terminalReasonId : undefined}
          onClick={() =>
            writer.save({
              score: parsedScore,
              score_override_reason: reasonValue.trim(),
            })
          }
        >
          {t("lead.saveOverride")}
        </Button>
        <Button onClick={onCancel}>{t("create.cancel")}</Button>
      </div>
    </div>
  );
}

// A score a human set: whose reason, what the model said, and the way back.
function OverriddenNote({
  lead,
  writer,
  terminalReasonId,
}: Readonly<{ lead: Lead; writer: LeadWriter; terminalReasonId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <div className="lead-stack">
      <p>
        {t("lead.scoreOverridden", {
          reason: lead.score_override_reason ?? "",
        })}
      </p>
      {lead.score_computed != null && (
        <p className="t-caption">
          {t("lead.machineScore", {
            score: formatNumber(lead.score_computed, locale),
          })}
        </p>
      )}
      <Button
        disabled={writer.patch.isPending || writer.readOnly}
        reasonId={writer.readOnly ? terminalReasonId : undefined}
        onClick={() => writer.save({ score: null })}
      >
        {t("lead.clearOverride")}
      </Button>
    </div>
  );
}

/**
 * The score card: each factor with the points it gave, one line on the
 * total, and the override behind the title's link. A human's score says so
 * above the factors, which then explain the model's number.
 */
export function LeadScoreCard({
  lead,
  writer,
  terminalReasonId,
}: Readonly<{
  lead: Lead;
  writer: LeadWriter;
  terminalReasonId: string;
}>) {
  const t = useT();
  const [overriding, setOverriding] = useState(false);
  const reading = useScoreReading(lead);
  const saved =
    writer.patch.isSuccess && "score" in (writer.patch.variables?.body ?? {});
  useEffect(() => {
    if (saved) {
      setOverriding(false);
    }
  }, [saved]);
  const offersOverride =
    !lead.archived_at && !lead.score_override_reason && !overriding;
  return (
    <OverviewCard
      title={t("lead.card.score")}
      action={
        offersOverride ? (
          <Button
            variant="link"
            reasonId={writer.readOnly ? terminalReasonId : undefined}
            onClick={() => setOverriding(true)}
          >
            {t("lead.overrideScore")}
          </Button>
        ) : undefined
      }
      foot={reading.foot}
    >
      {overriding && (
        <OverrideForm
          writer={writer}
          terminalReasonId={terminalReasonId}
          onCancel={() => setOverriding(false)}
        />
      )}
      {lead.score_override_reason && (
        <OverriddenNote
          lead={lead}
          writer={writer}
          terminalReasonId={terminalReasonId}
        />
      )}
      {reading.body}
    </OverviewCard>
  );
}
