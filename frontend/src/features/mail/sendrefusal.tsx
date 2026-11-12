// What a rep is told when a send is refused, and what they may do about it.

import { AppLink } from "../../design-system/applink";
import { useT } from "../../i18n";
import type { Refusal } from "./composesend";
import { DirectSendAction } from "./directsendaction";
import type { SendReview } from "./sendreview";

// The move the server offers after a refused Send, drawn inside the one send
// status: this one message, sent with the rep's reason. The words come from
// the status.
export function SendRefusalRemedies({
  review,
  onDirectSent,
  directOpen,
  onDirectOpenChange,
}: Readonly<{
  review: SendReview | null;
  /** What the composer does once the exception has sent the message. */
  onDirectSent?: () => void;
  /** Whether the reason form is open; the composer's Send press opens it. */
  directOpen?: boolean;
  onDirectOpenChange?: (open: boolean) => void;
}>) {
  if (!review) return null;
  return (
    <>
      {/* Keyed by the review, so a second refusal about a different message
          gets a fresh control. */}
      <DirectSendAction
        key={`direct-${review.reviewId}`}
        review={review}
        onSent={onDirectSent}
        open={directOpen}
        onOpenChange={onDirectOpenChange}
      />
      {/* Without the send to offer, the reference is what lets a human pick
          the review up. */}
      {!review.actions.includes("direct_send") && (
        <ReviewReference review={review} />
      )}
    </>
  );
}

// The work the refusal left behind, named so a colleague can open it.
function ReviewReference({ review }: Readonly<{ review: SendReview }>) {
  const t = useT();
  return (
    <span className="t-caption">
      {t("compose.reviewReference")}: {review.reviewId}
    </span>
  );
}

// The refusals that are not about the recipients. A mailbox connected before
// this product could send holds a read-only grant. The provider will not widen
// one in place, so reconnecting is the whole fix. And a message carrying
// an unsubscribe link carries one recipient's consent credential, so it may
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
