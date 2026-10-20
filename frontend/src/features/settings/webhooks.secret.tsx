import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import { Button, Modal } from "../../design-system/atoms";
import {
  CopyableText,
  useClipboardCopy,
} from "../../design-system/clipboardcopy";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { Heading } from "../../design-system/heading";
import { useT } from "../../i18n";
import { problemMessageOf, unwrap } from "../../lib/problem";
import type {
  WebhookSubscription,
  WebhookSubscriptionCreated,
} from "./webhooks";

// Rotate-secret mirrors RejectOfferAction in deals/offers.actions.tsx. A confirm
// guards the one irreversible side effect: the old secret stops verifying the
// moment this succeeds. The new secret goes up to the card, which shows it in
// the same SecretRevealModal a create uses.
export function RotateSecretAction({
  subscription,
  onRotated,
}: Readonly<{
  subscription: WebhookSubscription;
  onRotated: (secret: string) => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [confirming, setConfirming] = useState(false);
  const mutation = useMutation({
    mutationFn: async (): Promise<WebhookSubscriptionCreated> => {
      return unwrap(
        await api.POST("/webhook-subscriptions/{id}/rotate-secret", {
          params: { path: { id: subscription.id } },
        }),
      );
    },
    onSuccess: (created) => {
      queryClient.invalidateQueries({ queryKey: ["webhook-subscriptions"] });
      queryClient.invalidateQueries({
        queryKey: ["webhook-subscription", subscription.id],
      });
      setConfirming(false);
      onRotated(created.signing_secret);
    },
  });

  return (
    <>
      {/* There is no way back to the old secret, so the control and the
          confirm's own button both read as destructive. The confirm names the
          act, not the dialog's mechanics. */}
      <Button
        variant="danger"
        onClick={() => setConfirming(true)}
        data-testid="rotate-webhook-secret"
      >
        {t("webhooks.rotate")}
      </Button>
      <ConfirmModal
        open={confirming}
        onClose={() => setConfirming(false)}
        title={t("webhooks.rotateConfirm.title")}
        confirmLabel={t("webhooks.rotate")}
        confirmVariant="danger"
        onConfirm={() => mutation.mutate()}
        pending={mutation.isPending}
        error={mutation.isError ? problemMessageOf(mutation.error, t) : null}
      >
        <p className="t-body">{t("webhooks.rotateConfirm.body")}</p>
      </ConfirmModal>
    </>
  );
}

// The one-time secret reveal, shown after a successful create and gone when
// this modal closes. The secret lives only in the parent's local state, never in
// the react-query cache, and no list or get response can re-derive it.
export function SecretRevealModal({
  secret,
  onClose,
}: Readonly<{ secret: string; onClose: () => void }>) {
  const t = useT();
  const headingId = useId();
  const copy = useClipboardCopy(secret, {
    copy: t("webhooks.secret.copy"),
    copied: t("webhooks.secret.copied"),
    remedy: t("webhooks.secret.copyFailed"),
  });

  return (
    <Modal open onClose={onClose} labelledBy={headingId} intent="confirm">
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {t("webhooks.secret.title")}
      </Heading>
      {/* One stack owns every interval in this dialog, so the warning, the
          secret and the copy attempt do not each set a margin of their own. */}
      <div className="form-stack">
        <p>{t("webhooks.secret.warning")}</p>
        <CopyableText
          text={secret}
          label={t("webhooks.secret.title")}
          testId="webhook-signing-secret"
        />
        {copy.notice}
      </div>
      {/* Dismissing destroys the only copy of the secret, which no read can
          re-derive: Copy is primary, and Done stays available before a copy
          with a caution saying what leaving costs. */}
      {!copy.copied && (
        // ds:ignore a caution, not a refusal
        <p className="webhook-secret-caution">
          {t("webhooks.secret.leaveWarning")}
        </p>
      )}
      <div className="actions">
        <Button onClick={onClose}>{t("webhooks.secret.done")}</Button>
        <Button variant="primary" onClick={copy.copy}>
          {copy.label}
        </Button>
      </div>
    </Modal>
  );
}
