import { useState } from "react";
import { useCanWrite } from "../../app/capability";
import { useRecordZone } from "../../app/recordzone";
import { Badge, Button } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatDate } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import {
  type OutcomeReview,
  templateForOutcome,
  useOutcomeReviews,
  useReviewTemplates,
} from "../outcomereview.queries";
import { OutcomeReviewModal } from "./outcomereviewmodal";
import "./deal360.css";

/**
 * What a closed deal recorded about how it went.
 *
 * The panel exists only on a CLOSED deal. An open one has no outcome to review,
 * and a card inviting somebody to review a deal still in flight would be asking
 * for a verdict nobody can give.
 *
 * A review belongs to a CLOSING rather than to the deal. Reopening and closing
 * again makes a new outcome to review, and the old review stays readable
 * against the closing it was written about — marked as an earlier one, so a
 * reader never mistakes March's loss review for June's win.
 */
export function OutcomeReviewPanel({
  dealId,
  status,
  closingOccurrenceId,
}: Readonly<{
  dealId: string;
  status: string;
  // The closing this deal is on NOW. Null on a deal closed before closings were
  // recorded — those keep their reviews readable but take no new one, because
  // there is no occurrence to attach it to.
  closingOccurrenceId?: string | null;
}>) {
  const t = useT();
  // The permission the SERVER checks, asked the way the log-activity form beside
  // this panel asks it. A review is written as an activity, so `activity:create`
  // is the grant that decides it — not the deal's writability, which is neither
  // necessary nor sufficient: a reader with a writable deal and no activity
  // grant would meet a form the server refuses, and one with the grant on a
  // deal they may only read would never see the button at all.
  const canWrite = useCanWrite("activity", "create");
  const [adding, setAdding] = useState(false);
  const closed = status === "won" || status === "lost";
  const { data, isPending, isError } = useOutcomeReviews(dealId, closed);
  const { data: templates } = useReviewTemplates();
  if (!closed) {
    return null;
  }
  const reviews = data ?? [];
  const outcome = status === "won" ? "won" : "lost";
  const template = templateForOutcome(templates, outcome);
  // Reviewing needs three things to be true at once, and the button is absent
  // rather than disabled when they are not: the grant to write an activity, a
  // closing to hang the review on, and a template that asks the questions. A disabled button would
  // invite a reader to hunt for the reason.
  const canAdd = canWrite && !!closingOccurrenceId && !!template;
  const current = reviews.filter(
    (r) => r.closing_occurrence_id === closingOccurrenceId,
  );
  const earlier = reviews.filter(
    (r) => r.closing_occurrence_id !== closingOccurrenceId,
  );
  return (
    <Panel
      title={t("outcomeReview.title")}
      actions={
        canAdd ? (
          <Button variant="ghost" onClick={() => setAdding(true)}>
            {t("outcomeReview.add")}
          </Button>
        ) : undefined
      }
    >
      <PanelBody>
        {isPending || isError || reviews.length === 0 ? (
          <SurfaceState
            state={isPending ? "loading" : isError ? "failed" : "empty"}
            emptyLabel={t("outcomeReview.empty")}
            emptyDetail={t("outcomeReview.emptyDetail")}
            loadingLabel={t("outcomeReview.title")}
          >
            {null}
          </SurfaceState>
        ) : (
          <>
            {current.map((review) => (
              <ReviewCard key={review.id} review={review} />
            ))}
            {earlier.length > 0 && (
              <>
                {/* An earlier closing's review stays readable and says which
                    outcome it was about. The deal was closed differently then,
                    and a review read as if it were about today's outcome is
                    worse than one nobody can find. */}
                <p className="t-eyebrow">{t("outcomeReview.earlier")}</p>
                {earlier.map((review) => (
                  <ReviewCard key={review.id} review={review} earlier />
                ))}
              </>
            )}
          </>
        )}
      </PanelBody>
      {template && closingOccurrenceId && (
        <OutcomeReviewModal
          open={adding}
          onClose={() => setAdding(false)}
          dealId={dealId}
          closingOccurrenceId={closingOccurrenceId}
          template={template}
        />
      )}
    </Panel>
  );
}

/**
 * One review, rendered from the questions FROZEN onto it rather than from the
 * template as it reads today.
 *
 * The template is editable. Rendering today's questions beside an old review's
 * answers would put words in the author's mouth — an answer filed under a
 * question that has since been reworded would appear to answer the new wording.
 */
function ReviewCard({
  review,
  earlier,
}: Readonly<{ review: OutcomeReview; earlier?: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  return (
    <div className="outcome-review">
      <div className="outcome-review-head">
        <Badge quiet tone={review.outcome === "won" ? "success" : "danger"}>
          {t(
            review.outcome === "won"
              ? "outcomeReview.outcomeWon"
              : "outcomeReview.outcomeLost",
          )}
        </Badge>
        <span className="muted">
          {formatDate(review.created_at, locale, recordZone)}
        </span>
        {earlier && (
          <span className="muted">{t("outcomeReview.earlierMark")}</span>
        )}
      </div>
      <dl className="firmo">
        {review.questions.map((question) => (
          <div key={question.key}>
            <dt className="t-eyebrow">{question.label}</dt>
            {/* An unanswered optional question shows as a stated blank rather
                than a missing row, so the reader sees WHICH question went
                unanswered instead of a shorter list. */}
            <dd>
              {review.answers?.[question.key] || (
                <span className="muted">{t("outcomeReview.noAnswer")}</span>
              )}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  );
}
