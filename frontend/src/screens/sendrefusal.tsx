// What a rep is told when a send is refused, and what they may do about it.
//
// Lifted out of compose.tsx: the refusal now renders an ACTION beside its
// explanation, and the two belong together — a reader asking "what happens when
// the engine says no" should find the words and the button in one place rather
// than in a 2700-line composer.

import { useT } from "../i18n";
import type { Refusal } from "./compose";
import type { SendReview } from "./sendreview";
import { ReviewReference, SendReviewActions } from "./sendreviewaction";

// Each refusal states the condition and where it is resolved. The consent gate
// is the default-deny suppression (A22/ADR-0011) this surface exists to make
// visible. A mailbox connected before this product could send holds a read-only
// grant and the provider will not widen one in place, so reconnecting is the
// whole fix. And a message carrying an unsubscribe link carries ONE recipient's
// consent credential, so it may only ever have one addressee.
export function SendRefusal({
  refusal,
  personId,
  review,
}: Readonly<{
  refusal: Refusal;
  personId?: string;
  review?: SendReview | null;
}>) {
  const t = useT();
  if (refusal === "consent") {
    return (
      <div className="compose-refusal" role="alert">
        <p className="t-body">
          <strong>{t("compose.consentBlockedTitle")}</strong>
        </p>
        <p className="t-body" style={{ color: "var(--dangerText)" }}>
          {t("compose.consentBlocked")}
        </p>
        {/* WHAT THE SERVER SAYS THIS REP MAY DO, before the link to the
            contact page. That link is the older answer and a weaker one: the
            engine refused on a judgement about this message, and there is
            nothing on the contact page to change. Asking somebody who may
            override it is the move most reps actually need. */}
        {/* Keyed by the review, so a second refusal about a DIFFERENT message
            gets a fresh control rather than the first one's success state —
            which would show "Asked" for a message nobody has asked about. */}
        {review && <SendReviewActions key={review.reviewId} review={review} />}
        {/* AND THE REFERENCE, whether or not there is an action to offer. A
            server naming an action this build cannot draw leaves the rep with
            this, which is what makes dropping the action safe. */}
        {review && review.actions.length === 0 && (
          <ReviewReference review={review} />
        )}
        {personId && (
          <a href={`#/contacts/${personId}`} className="link-button">
            {t("compose.consentGoto")}
          </a>
        )}
      </div>
    );
  }
  if (refusal === "mailbox") {
    return (
      <div className="compose-refusal" role="alert">
        <p className="t-body">{t("compose.mailboxNotSendCapable")}</p>
        <a href="#/settings/connections" className="link-button">
          {t("compose.mailboxNotSendCapableGoto")}
        </a>
      </div>
    );
  }
  if (refusal === "sharedUnsubscribe") {
    return (
      <div className="compose-refusal" role="alert">
        <p className="t-body">{t("compose.sharedUnsubscribeToken")}</p>
      </div>
    );
  }
  return null;
}
