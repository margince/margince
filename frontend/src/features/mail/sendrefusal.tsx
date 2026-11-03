// What a rep is told when a send is refused, and what they may do about it.

import { AppLink } from "../../design-system/applink";
import { useT } from "../../i18n";
import type { Refusal } from "./composesend";
import { DirectSendAction } from "./directsendaction";
import type { SendReview } from "./sendreview";
import { ReviewReference, SendReviewActions } from "./sendreviewaction";

// The moves the server offers after a refused Send, drawn inside the one send
// status. The words come from the status; these are what the rep can do.
export function SendRefusalRemedies({
  contactId,
  review,
}: Readonly<{
  contactId?: string;
  review: SendReview | null;
}>) {
  const t = useT();
  return (
    <>
      {/* Keyed by the review, so a second refusal about a DIFFERENT message
          gets a fresh control rather than the first one's success state. */}
      {review && <SendReviewActions key={review.reviewId} review={review} />}
      {/* The other answer to the same question, for a rep who holds the
          authority. The server offers one or the other, never both. */}
      {review && (
        <DirectSendAction key={`direct-${review.reviewId}`} review={review} />
      )}
      {/* The reference, whether or not there is an action to offer. A server
          naming an action this build cannot draw leaves the rep with this. */}
      {review && review.actions.length === 0 && (
        <ReviewReference review={review} />
      )}
      {contactId && (
        <AppLink href={`#/contacts/${contactId}`} className="link-button">
          {t("compose.refusalOpenContact")}
        </AppLink>
      )}
    </>
  );
}

// The refusals that are not about the recipients. A mailbox connected before
// this product could send holds a read-only grant and the provider will not
// widen one in place, so reconnecting is the whole fix. And a message carrying
// an unsubscribe link carries ONE recipient's consent credential, so it may
// only ever have one addressee.
export function SendRefusal({ refusal }: Readonly<{ refusal: Refusal }>) {
  const t = useT();
  if (refusal === "mailbox") {
    return (
      <div className="compose-refusal" role="alert">
        <p className="t-body">{t("compose.mailboxNotSendCapable")}</p>
        <AppLink href="#/settings/connections" className="link-button">
          {t("compose.mailboxNotSendCapableGoto")}
        </AppLink>
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
