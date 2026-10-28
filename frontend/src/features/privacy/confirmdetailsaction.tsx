import { useState } from "react";
import { Button } from "../../design-system/atoms";
import { PanelBody } from "../../design-system/panel";
import { formatDateTime } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { useMe } from "../../lib/session";
import {
  ControllerMailDialog,
  type ControllerMailIssued,
} from "./controllermail";

// Issuing the link queues a message; it does not prove delivery.
function sentenceFor(
  issued: ControllerMailIssued,
  t: ReturnType<typeof useT>,
): string {
  const address = issued.delivered_to;
  return issued.queued
    ? t("consent.askQueued", { address })
    : t("consent.askNotDelivered", { address });
}

/**
 * ConfirmDetailsAction mails the contact a link to see what is held about them,
 * correct it, and answer on marketing, once the rep has seen the mail.
 *
 * The address is never chosen here. The server derives it from the contact's
 * own live primary email. That is what lets a grant made through the link
 * stand on its own: the answer came from the subject's mailbox. This surface
 * offers the act and reports where it went, and can aim it nowhere.
 */
export function ConfirmDetailsAction({
  contactId,
  mayWrite,
  recipient,
  unavailableReason,
}: Readonly<{
  contactId: string;
  mayWrite: boolean;
  recipient?: string | null;
  unavailableReason?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  // Asked here rather than by each caller, because a caller that forgot would
  // offer an action the server refuses.
  //
  // Three answers, not two. A server that carries no answer leaves this
  // undefined, which bars the action the same way a false does. It then says
  // so, rather than naming a fact nobody has read.
  const sendable = useMe().data?.confirm_requests_sendable;
  const [previewing, setPreviewing] = useState(false);
  // Kept with the contact it was about: React reuses this component across a
  // navigation between two cached contacts, and the address must not follow.
  const [sent, setSent] = useState<{
    contactId: string;
    issued: ControllerMailIssued;
  } | null>(null);
  const issued = sent?.contactId === contactId ? sent.issued : null;

  if (!mayWrite) {
    return null;
  }
  const reason =
    sendable === undefined
      ? t("consent.askSendableUnknown")
      : sendable
        ? unavailableReason
        : t("consent.askNoSystemMail");
  return (
    <PanelBody className="consent-confirm-ask">
      <Button
        reason={reason}
        aria-haspopup="dialog"
        data-testid="confirm-details-ask"
        onClick={() => setPreviewing(true)}
      >
        {t("consent.askToConfirm")}
      </Button>
      {recipient && (
        <p className="t-caption">
          {t("consent.confirmRecipient", { address: recipient })}
        </p>
      )}
      <p>{t("consent.askToConfirmWhat")}</p>
      {issued && (
        <p className="t-caption" data-testid="confirm-details-sent">
          {sentenceFor(issued, t)} {t("consent.askExpires")}:{" "}
          {formatDateTime(issued.expires_at, locale, zone)}
        </p>
      )}
      <ControllerMailDialog
        contactId={contactId}
        kind="record_confirmation"
        open={previewing}
        onClose={() => setPreviewing(false)}
        onSent={(next) => setSent({ contactId, issued: next })}
      />
    </PanelBody>
  );
}
