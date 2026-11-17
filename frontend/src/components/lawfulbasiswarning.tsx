// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button } from "../design-system/button";
import { Callout } from "../design-system/callout";
import { ConfirmModal } from "../design-system/confirmmodal";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { problemMessageOf, throwProblem } from "../lib/problem";

// The warning before a send to a recipient whose country has no
// legitimate-interest basis. It applies when the record holds neither their
// consent nor an agreement with them. It does not block: the sender
// confirms it, and the server records that confirmation under their name.
// The sender may instead ask the contact for consent, which sends the consent
// mail and not the message.

type Preview = components["schemas"]["SendAuthorizationPreview"];

/** The addresses the preview marks as having no lawful basis on record. */
export function lawfulBasisMissing(preview: Preview | undefined): string[] {
  return (preview?.recipients ?? [])
    .filter((recipient) => recipient.lawful_basis_missing === true)
    .map((recipient) => recipient.address);
}

type Subject = components["schemas"]["CommunicationEvidenceSubject"];

/**
 * The records the server confirms against: each marked recipient's own
 * record, as the preview names it. A pinned lead stays the lead.
 */
function confirmableSubjects(preview: Preview | undefined): Subject[] {
  return (preview?.recipients ?? []).flatMap((recipient) =>
    recipient.lawful_basis_missing === true && recipient.evidence_subject
      ? [recipient.evidence_subject]
      : [],
  );
}

type ConsentRequest = Readonly<{
  contactId: string;
  purposeId: string;
  address: string;
}>;

/**
 * The consent requests the warning can send: each marked contact for whom the
 * preview names the purpose a request would ask about.
 */
function askableRequests(preview: Preview | undefined): ConsentRequest[] {
  return (preview?.recipients ?? []).flatMap((recipient) => {
    const subject = recipient.evidence_subject;
    return recipient.lawful_basis_missing === true &&
      recipient.consent_purpose_id &&
      subject?.entity_type === "contact"
      ? [
          {
            contactId: subject.entity_id,
            purposeId: recipient.consent_purpose_id,
            address: recipient.address,
          },
        ]
      : [];
  });
}

/** The standing warning, drawn wherever the preview is. */
export function LawfulBasisCallout({
  preview,
}: Readonly<{ preview: Preview | undefined }>) {
  const t = useT();
  const missing = lawfulBasisMissing(preview);
  if (missing.length === 0) {
    return null;
  }
  return (
    <Callout
      tone="danger"
      kind="standing"
      live="status"
      title={t("lawfulBasis.title")}
    >
      <p className="t-body">{t("lawfulBasis.body")}</p>
      <p className="t-caption">{missing.join(", ")}</p>
    </Callout>
  );
}

/** The requests one press of "Ask for consent" could not send, and why. */
type AskOutcome = Readonly<{
  failed: ConsentRequest[];
  cause: unknown;
}>;

/**
 * Mails each contact the consent request for its purpose. A contact asked once
 * is not asked again while the dialog is open, so a retry after a partial
 * failure asks only the contacts that failed.
 */
function useAskForConsent(requests: readonly ConsentRequest[]) {
  const t = useT();
  const [asked, setAsked] = useState<readonly ConsentRequest[]>([]);
  const mutation = useMutation({
    mutationFn: async (): Promise<AskOutcome> => {
      const failed: ConsentRequest[] = [];
      let cause: unknown = null;
      for (const request of requests.filter((r) => !wasAsked(asked, r))) {
        try {
          const answered = await api.POST(
            "/contacts/{id}/consent/double-opt-in",
            {
              params: { path: { id: request.contactId } },
              body: { purpose_id: request.purposeId },
            },
          );
          if (!answered.response.ok) {
            throwProblem(
              answered.error ?? { title: t("lawfulBasis.askFailed") },
            );
          }
          setAsked((before) => [...before, request]);
        } catch (error) {
          failed.push(request);
          cause ??= error;
        }
      }
      return { failed, cause };
    },
  });
  const reset = () => {
    mutation.reset();
    setAsked([]);
  };
  return { mutation, asked, reset };
}

/** Whether a request for this contact and purpose already went out. */
function wasAsked(
  asked: readonly ConsentRequest[],
  request: ConsentRequest,
): boolean {
  return asked.some(
    (a) =>
      a.contactId === request.contactId && a.purposeId === request.purposeId,
  );
}

/** The line the dialog shows after a press that did not ask everyone. */
function askFailure(
  t: ReturnType<typeof useT>,
  asked: readonly ConsentRequest[],
  outcome: AskOutcome | undefined,
): string | null {
  if (!outcome || outcome.failed.length === 0) {
    return null;
  }
  if (asked.length === 0) {
    return problemMessageOf(outcome.cause, t, t("lawfulBasis.askFailed"));
  }
  return t("lawfulBasis.askPartial", {
    asked: asked.map((r) => r.address).join(", "),
    failed: outcome.failed.map((r) => r.address).join(", "),
  });
}

/**
 * Holds a send until the user confirms the warning for every recipient it
 * applies to. `gate(proceed)` runs `proceed` at once when nothing is missing,
 * else after the confirmation is recorded. Asking for consent instead closes
 * the dialog without sending. `dialog` is drawn by the caller.
 */
export function useLawfulBasisGate(preview: Preview | undefined) {
  const t = useT();
  const toast = useToast();
  const missing = lawfulBasisMissing(preview);
  const subjects = confirmableSubjects(preview);
  const requests = askableRequests(preview);
  const [pending, setPending] = useState<(() => void) | null>(null);
  const { mutation: ask, asked, reset: resetAsk } = useAskForConsent(requests);
  const record = useMutation({
    mutationFn: async () => {
      const answered = await api.POST("/lawful-basis-confirmations", {
        body: { subjects },
      });
      if (!answered.response.ok) {
        throwProblem(answered.error ?? { title: t("lawfulBasis.failed") });
      }
      return answered.data;
    },
  });
  const close = () => {
    setPending(null);
    record.reset();
    resetAsk();
  };
  const askMessage = askFailure(t, asked, ask.data);
  const gate = (proceed: () => void) => {
    if (missing.length === 0) {
      proceed();
      return;
    }
    setPending(() => proceed);
  };
  const dialog = (
    <ConfirmModal
      open={pending !== null}
      onClose={close}
      title={t("lawfulBasis.title")}
      confirmLabel={t("lawfulBasis.confirm")}
      confirmVariant="danger"
      pending={record.isPending}
      confirmDisabled={ask.isPending}
      error={
        record.error ? problemMessageOf(record.error, t) : askMessage
      }
      actionsLead={
        requests.length > 0 ? (
          <Button
            pending={ask.isPending}
            onClick={() => {
              ask.mutate(undefined, {
                onSuccess: (outcome) => {
                  if (outcome.failed.length > 0) {
                    return;
                  }
                  close();
                  toast.show(
                    t("lawfulBasis.asked", {
                      address: requests.map((r) => r.address).join(", "),
                    }),
                  );
                },
              });
            }}
          >
            {t("lawfulBasis.askConsent")}
          </Button>
        ) : undefined
      }
      onConfirm={() => {
        record.mutate(undefined, {
          onSuccess: () => {
            const proceed = pending;
            close();
            proceed?.();
          },
        });
      }}
    >
      <Callout tone="danger" kind="standing" title={t("lawfulBasis.body")}>
        <p className="t-caption">{missing.join(", ")}</p>
      </Callout>
    </ConfirmModal>
  );
  return { missing, gate, dialog };
}
