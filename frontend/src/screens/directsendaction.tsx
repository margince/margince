// The button a rep who may overrule the engine presses.
//
// The refusal names the review it left behind; the modal needs the review
// itself — the message being held, and the caution the server serves for it.
// This is the step between: it fetches one and opens the other.
//
// FETCHED WHEN THEY ASK FOR IT, not when the refusal appears. Most refusals are
// read and abandoned, and a request fired on every one would be work done for
// nothing. The button opens the modal and the modal waits a moment, which is
// the honest order: the person has already decided to look.

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import { Button } from "../design-system/atoms";
import { useT } from "../i18n";
import { throwProblem } from "./common";
import { DirectSendModal } from "./directsendmodal";
import type { SendReview } from "./sendreview";

async function fetchReview(reviewId: string) {
  const { data, error, response } = await api.GET(
    "/communication-reviews/{id}",
    {
      params: { path: { id: reviewId } },
    },
  );
  // Success is a real 2xx WITH a body: openapi-fetch reports a falsy error and
  // undefined data for a bodiless non-2xx, which would otherwise open a modal
  // with nothing in it.
  if (!response.ok || !data) {
    throwProblem(error || { title: "The review could not be opened." });
  }
  return data;
}

// SETTLED_STATES are the review states with nothing left to decide. The server
// refuses a decision on any of them, so offering the form would be walking
// somebody through an acknowledgement that cannot be recorded.
const SETTLED_STATES = ["resolved", "superseded", "cancelled"];

export function DirectSendAction({
  review,
  onSent,
}: Readonly<{ review: SendReview; onSent?: (activityId: string) => void }>) {
  const t = useT();
  const [open, setOpen] = useState(false);

  const opened = useQuery({
    queryKey: ["communication-review", review.reviewId],
    queryFn: () => fetchReview(review.reviewId),
    // Only once the person has asked to see it. A refusal read and abandoned
    // should cost nothing.
    enabled: open,
    // ONE ATTEMPT, and the button is the retry. React Query's own retries
    // leave the control looking busy for seconds with nothing to press, and a
    // person who has decided to overrule the engine should be told promptly
    // that the review would not open rather than watched to wait.
    retry: false,
  });

  if (!review.actions.includes("direct_send")) {
    return null;
  }
  // A REVIEW THAT IS NO LONGER WAITING is not one to decide. It can be
  // resolved, superseded or cancelled between the refusal and the click — the
  // message sent from another tab, a colleague directing it — and opening the
  // confirm form over one would walk somebody through an acknowledgement the
  // server then refuses.
  const settled = opened.data
    ? SETTLED_STATES.includes(opened.data.state)
    : false;
  return (
    <>
      {/* PENDING, not disabled: the shared contract keeps the control
          focusable and announces aria-busy, and a fetch is exactly the wait it
          exists for. */}
      <Button
        variant="danger"
        onClick={() => {
          setOpen(true);
          // A FAILED FETCH MUST BE RETRYABLE. Setting `open` again changes
          // nothing when it is already true, so without this the button is a
          // dead end after the first failure.
          //
          // Asked of anything not currently in flight rather than of isError
          // alone: a query that has settled without data — refetched to
          // nothing, evicted from the cache — is the same dead end wearing a
          // different state.
          if (!opened.isFetching && !opened.data) {
            void opened.refetch();
          }
        }}
        pending={open && opened.isFetching}
        busyLabel={t("directSend.opening")}
      >
        {t("directSend.open")}
      </Button>
      {opened.isError && (
        <p
          className="t-body"
          style={{ color: "var(--dangerText)" }}
          role="alert"
        >
          {t("directSend.couldNotOpen")}
        </p>
      )}
      {/* MOUNTED ONLY WITH THE REVIEW IN HAND. A modal opened over a pending
          fetch would show its acknowledgement and its confirm button before the
          warning those exist to be read against had arrived. */}
      {settled && (
        <p className="t-body" role="status">
          {t("directSend.alreadySettled")}
        </p>
      )}
      {open && opened.data && !settled && (
        <DirectSendModal
          open
          onClose={() => setOpen(false)}
          review={opened.data}
          onSent={onSent}
        />
      )}
    </>
  );
}
