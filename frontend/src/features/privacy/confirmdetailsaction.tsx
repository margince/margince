import { useMutation } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { PanelBody } from "../../design-system/panel";
import { formatDateTime } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";

// Issuing the link queues a message; it does not prove delivery.
function sentenceFor(
  issued: components["schemas"]["ConfirmRequestIssued"],
  t: ReturnType<typeof useT>,
): string {
  const address = issued.delivered_to;
  return issued.queued
    ? t("consent.askQueued", { address })
    : t("consent.askNotDelivered", { address });
}

/**
 * ConfirmDetailsAction mails the contact a link to see what is held about them,
 * correct it, and answer on marketing.
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
  const ask = useMutation({
    // Keyed on the contact, so a result belongs to the record it was asked
    // about. React reuses this component across a navigation between two
    // cached contacts. Without the key, the previous contact's address sat
    // under the new record's rows, naming somebody else's mailbox as this
    // contact's.
    mutationKey: ["confirm-request", contactId],
    mutationFn: async (id: string) => {
      return unwrap(
        await api.POST("/contacts/{id}/consent/confirm-request", {
          params: { path: { id } },
        }),
      );
    },
  });

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
        disabled={ask.isPending}
        reason={reason}
        data-testid="confirm-details-ask"
        onClick={() => ask.mutate(contactId)}
      >
        {t("consent.askToConfirm")}
      </Button>
      {recipient && (
        <p className="t-caption">
          {t("consent.confirmRecipient", { address: recipient })}
        </p>
      )}
      <p>{t("consent.askToConfirmWhat")}</p>
      <ErrorLine error={ask.error} />
      {ask.data && (
        <p className="t-caption" data-testid="confirm-details-sent">
          {sentenceFor(ask.data, t)} {t("consent.askExpires")}:{" "}
          {formatDateTime(ask.data.expires_at, locale, zone)}
        </p>
      )}
    </PanelBody>
  );
}
