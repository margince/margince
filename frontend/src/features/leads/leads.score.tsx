import { useEffect, useState } from "react";
import {
  Badge,
  Button,
  Disclosure,
  Field,
  TextInput,
} from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody } from "../../design-system/panel";
import { formatDecimal, formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { scoreFactorLabel, scoreTone } from "./leadpresentation";
import type { Lead, LeadWriter } from "./leads";
import { useLeadScore } from "./leadscore.queries";

// The server's score grants points to a title naming a decision maker, by this
// pattern. It is mirrored here only to say why a title earned nothing; it never
// adds to the score, which the server computes.
const DECISION_MAKER_TITLE =
  /(chief|vp|head|director|founder|owner|c[a-z]o)\b/i;

const HIGH_INTENT_SOURCES = new Set(["inbound", "webform", "referral"]);

// The server takes five points off for these sources, in leadscore.go, rather
// than granting nothing. Calling that "no buying intent on its own" would turn
// a subtraction into a neutral, a kinder claim than the model made.
const LOW_INTENT_SOURCES = new Set(["import", "crawl"]);

// What a lead is missing, in the model's own terms. It shows when no retained
// breakdown exists yet.
//
// A zero score and an unscored lead show the same number and mean opposite
// things: "assessed, and it earns nothing" versus "not assessed". A rep reads
// both as a bad prospect, and only one is (ADR-0108). These reasons are always
// derivable, so the page states them rather than our own storage history.
function ScoreShortfall({ lead }: Readonly<{ lead: Lead }>) {
  const t = useT();
  const missing: string[] = [];
  if (!lead.title) {
    missing.push(t("lead.shortfall.noTitle"));
  } else if (!DECISION_MAKER_TITLE.test(lead.title)) {
    missing.push(t("lead.shortfall.titleNotSenior", { title: lead.title }));
  }
  if (!lead.source) {
    // Split as the title pair above is: interpolating an absent value
    // would print "Came in as undefined" at a rep.
    missing.push(t("lead.shortfall.noSource"));
  } else if (LOW_INTENT_SOURCES.has(lead.source)) {
    missing.push(t("lead.shortfall.sourcePenalised", { source: lead.source }));
  } else if (!HIGH_INTENT_SOURCES.has(lead.source)) {
    missing.push(t("lead.shortfall.sourceNoIntent", { source: lead.source }));
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

function ScoreBreakdown({ lead }: Readonly<{ lead: Lead }>) {
  const t = useT();
  const { locale } = useLocale();
  const explain = useLeadScore(lead);

  if (explain.isPending) {
    return <span>{t("lead.scoreLoading")}</span>;
  }
  if (explain.isError) {
    return <ErrorLine error={explain.error} />;
  }
  const current = explain.data?.current;
  if (!explain.data?.explained || !current) {
    // No retained breakdown. For a score of zero the reasons are still
    // derivable from the lead in hand, and they are what the reader came for.
    // "This score predates the breakdown" would leave a 0 looking like a bad
    // prospect rather than an unassessed one (ADR-0108).
    //
    // A non-zero score is a different case. Something did count, and this
    // client cannot say what, so it says only that the breakdown is not stored.
    return lead.score === 0 ? (
      <ScoreShortfall lead={lead} />
    ) : (
      <span>{t("lead.scoreNotStoredYet")}</span>
    );
  }
  const factors = current.factors ?? [];
  // Under a Commercial Judgement override the displayed score is the human's,
  // and these factors sum to the machine's. So the reader is told which number
  // they are looking at.
  const overridden = current.override_reason != null;

  return (
    <div className="lead-stack-tight">
      {overridden && (
        <span>
          {t("lead.scoreFactorsExplainMachine", {
            score: formatNumber(current.score_computed, locale),
          })}
        </span>
      )}
      {factors.length === 0 ? (
        <span>{t("lead.scoreNoFactors")}</span>
      ) : (
        <ul className="lead-plainlist">
          {factors.map((factor) => (
            <li key={factor.factor} className="lead-factor">
              <span>{scoreFactorLabel(factor.factor, t)}</span>
              <span className="t-num">
                {formatDecimal(factor.points, locale, 1)}
              </span>
              {factor.base_points != null && (
                // The decay as arithmetic a reader can check: 25 halving
                // every 14 days is why this row reads 12.5 today.
                <span className="t-caption t-num">
                  {t("lead.scoreDecayed", {
                    base: formatNumber(factor.base_points, locale),
                  })}
                </span>
              )}
              {factor.source_activity_ids != null &&
                factor.source_activity_ids.length > 0 && (
                  // How many records fed the factor. The ids themselves are
                  // already filtered to what this reader may open, so the
                  // count never claims more than they can see.
                  <span className="t-caption">
                    {t("lead.scoreSources", {
                      count: formatNumber(
                        factor.source_activity_ids.length,
                        locale,
                      ),
                    })}
                  </span>
                )}
            </li>
          ))}
        </ul>
      )}
      <span className="t-caption t-num">
        {t("lead.scoreReconciles", {
          raw: formatDecimal(current.raw_sum, locale, 2),
          rounded: formatNumber(current.rounded_sum, locale),
          score: formatNumber(current.score_computed, locale),
        })}
      </span>
    </div>
  );
}

// The breakdown behind the score. The server refuses it for an archived lead,
// so there the line is left out instead of drawn as a failure.
function ScoreExplanation({ lead }: Readonly<{ lead: Lead }>) {
  const t = useT();
  if (lead.archived_at) {
    return null;
  }
  return (
    <>
      <span>{t("lead.explainScore")}</span>
      <ScoreBreakdown lead={lead} />
    </>
  );
}

// The score block: its explanation, and the Commercial Judgement override.
// Status, owner and score controls share one PATCH /leads/{id} mutation sent
// with If-Match(lead.version).
function LeadScorePanel({
  lead,
  terminalReasonId,
  overriding,
  setOverriding,
  scoreValue,
  setScoreValue,
  reasonValue,
  setReasonValue,
  writer,
}: Readonly<{
  lead: Lead;
  terminalReasonId: string;
  overriding: boolean;
  setOverriding: (next: boolean) => void;
  scoreValue: string;
  setScoreValue: (next: string) => void;
  reasonValue: string;
  setReasonValue: (next: string) => void;
  writer: LeadWriter;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const { readOnly } = writer;
  const reasonBlank = reasonValue.trim() === "";
  const scoreBlank = scoreValue.trim() === "";
  const parsedScore = Number(scoreValue);
  const scoreInvalid =
    scoreBlank ||
    !Number.isInteger(parsedScore) ||
    parsedScore < 0 ||
    parsedScore > 100;

  return (
    <div className="lead-stack">
      <ScoreExplanation lead={lead} />
      {lead.score_override_reason ? (
        <div className="lead-stack">
          <p>
            {t("lead.scoreOverridden", {
              reason: lead.score_override_reason,
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
            disabled={writer.patch.isPending || readOnly}
            reasonId={readOnly ? terminalReasonId : undefined}
            onClick={() => writer.save({ score: null })}
          >
            {t("lead.clearOverride")}
          </Button>
        </div>
      ) : overriding ? (
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
              disabled={reasonBlank || scoreInvalid || writer.patch.isPending}
              reasonId={readOnly ? terminalReasonId : undefined}
              onClick={() =>
                writer.save({
                  score: parsedScore,
                  score_override_reason: reasonValue.trim(),
                })
              }
            >
              {t("lead.saveOverride")}
            </Button>
            <Button onClick={() => setOverriding(false)}>
              {t("create.cancel")}
            </Button>
          </div>
        </div>
      ) : (
        // "Machine-computed score" was a label with no value beside it,
        // naming what the badge above already says. The override is a rare
        // action and stands alone.
        <Button
          reasonId={readOnly ? terminalReasonId : undefined}
          onClick={() => setOverriding(true)}
        >
          {t("lead.overrideScore")}
        </Button>
      )}
    </div>
  );
}

/**
 * The score as a card of the reading: it folds to one line with its top
 * factor, and opens for the breakdown and the override. The inputs a rep
 * enters by hand follow it in the overview, full width.
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
  const { locale } = useLocale();
  const [overriding, setOverriding] = useState(false);
  const [scoreValue, setScoreValue] = useState("");
  const [reasonValue, setReasonValue] = useState("");
  // This form's save, not any save. The owner picker and the ladder share the
  // mutation, so `isSuccess` alone would clear a half-typed override when an
  // owner is assigned. A refused save leaves what the reader typed in place.
  // Both of this form's writes carry `score` (a value, or null to clear), and
  // no other control on the page sends it.
  const saved =
    writer.patch.isSuccess && "score" in (writer.patch.variables?.body ?? {});
  useEffect(() => {
    if (saved) {
      setOverriding(false);
      setScoreValue("");
      setReasonValue("");
    }
  }, [saved]);
  return (
    <Panel title={t("lead.score")}>
      <PanelBody>
        <Disclosure
          summary={
            <span className="lead-score-summary">
              <Badge tone={scoreTone(lead.score)}>
                {t("lead.score")}: {formatNumber(lead.score, locale)}
              </Badge>{" "}
              <span className="t-caption">
                {lead.score_reason
                  ? scoreFactorLabel(lead.score_reason, t)
                  : t("lead.scoreNoSignals")}
              </span>
            </span>
          }
        >
          <LeadScorePanel
            lead={lead}
            terminalReasonId={terminalReasonId}
            overriding={overriding}
            setOverriding={setOverriding}
            scoreValue={scoreValue}
            setScoreValue={setScoreValue}
            reasonValue={reasonValue}
            setReasonValue={setReasonValue}
            writer={writer}
          />
        </Disclosure>
      </PanelBody>
    </Panel>
  );
}
