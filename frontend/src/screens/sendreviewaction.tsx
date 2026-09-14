// The button a refused rep presses to ask somebody who may decide.
//
// The refusal itself is unchanged: the engine said no, and it still says no.
// What this adds is the move the server says this rep may make — asking
// somebody with the authority to override it, which is what most reps' next
// step actually is, because directing a send is a grant most seats do not
// hold.
//
// ONE BUTTON AND ONE OUTCOME. Pressing it stages an approval card and says so.
// It does not send, it does not promise an answer, and it does not close the
// composer: the rep has handed the question on, and the message stays where
// they left it.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { Button } from "../design-system/atoms";
import { useT } from "../i18n";
import { throwProblem } from "./common";
import type { SendReview } from "./sendreview";

// requestDecision asks the installation to decide a refused send.
//
// The variable carries the review, never a closure over render state: a
// mutation that read the id from the component's scope would ask about
// whichever review was current when it resolved, and a rep who pressed send
// twice would route the wrong one.
type DecisionRequest = Readonly<{ reviewId: string; note?: string }>;

async function requestDecision({
  reviewId,
  note,
}: DecisionRequest): Promise<string> {
  const { data, error, response } = await api.POST(
    "/communication-reviews/{id}/request-decision",
    { params: { path: { id: reviewId } }, body: note ? { note } : {} },
  );
  // Success is a real 2xx WITH a body, never merely the absence of an error:
  // openapi-fetch reports a falsy error and undefined data for a bodiless
  // non-2xx, which would otherwise read as a card that was never staged.
  if (!response.ok || !data) {
    throwProblem(error || { title: "The request could not be made." });
  }
  return data.approval_id;
}

// SendReviewActions draws what the server says this rep may do.
//
// NOTHING WHEN THERE IS NOTHING TO OFFER. An agent, or a refusal with no
// message to decide about, gets an empty list — and an empty list draws no
// buttons rather than a disabled one. A control nobody can use is a question
// the reader has to answer before they can ignore it.
export function SendReviewActions({
  review,
}: Readonly<{ review: SendReview }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const ask = useMutation({
    mutationFn: requestDecision,
    // The review's state moves when it is routed, so anything showing it is
    // now stale. Invalidating by prefix rather than by id: a surface that
    // listed reviews is as wrong as one showing this one.
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["communication-review"] }),
  });

  // THE DIRECT SEND IS THE OTHER ANSWER to the same question, and it is not
  // this component's to draw: it needs the review itself — the held message,
  // the served warning — which a refusal does not carry. SendRefusal fetches
  // that and opens the modal.
  if (!review.actions.includes("request_decision")) {
    return null;
  }
  if (ask.isSuccess) {
    return (
      <p className="t-body" role="status">
        {t("compose.reviewRequested")}
      </p>
    );
  }
  return (
    <div className="compose-review-actions">
      <Button
        variant="ghost"
        onClick={() => ask.mutate({ reviewId: review.reviewId })}
        disabled={ask.isPending}
      >
        {ask.isPending
          ? t("compose.reviewRequesting")
          : t("compose.reviewRequest")}
      </Button>
      {ask.isError && (
        <p
          className="t-body"
          style={{ color: "var(--dangerText)" }}
          role="alert"
        >
          {t("compose.reviewRequestFailed")}
        </p>
      )}
    </div>
  );
}

// ReviewReference names the work the refusal left behind.
//
// IT IS THE FALLBACK EVERY OTHER BRANCH LEANS ON. When the ask fails, the copy
// tells the rep to open the review — and that instruction is a lie unless the
// reference is on screen. When the server offers an action this build cannot
// draw, dropping it is only safe because the reference survives. Both promises
// are kept here or not at all.
export function ReviewReference({ review }: Readonly<{ review: SendReview }>) {
  const t = useT();
  return (
    <span className="t-caption">
      {t("compose.reviewReference")}: {review.reviewId}
    </span>
  );
}
