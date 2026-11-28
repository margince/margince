// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueryClient } from "@tanstack/react-query";
import { Send } from "lucide-react";
import { useState } from "react";
import type { components } from "../../api/schema";
import { Button, Textarea } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { formatTimeOfDay } from "../../format/format";
import { useNow } from "../../format/now";
import { type Translator, useLocale, useT } from "../../i18n";
import { useCanMutate } from "../../lib/capability";
import { isImeCommit } from "../../lib/imecommit";
import { problemMessageOf } from "../../lib/problem";
import { useMe } from "../../lib/session";
import { useViewerZone } from "../../lib/viewerzone";
import {
  keepMessengerDraft,
  type MessengerDraft,
  readMessengerDraft,
} from "./messenger.draft";
import {
  type MessengerContext,
  type MessengerScope,
  useMessengerReceipt,
  useMessengerSend,
} from "./messenger.queries";

type Attempt = components["schemas"]["MessengerAttempt"];

export function MessengerComposer({
  scope,
  context,
  root,
  draftKey,
  onSent,
}: Readonly<{
  scope: MessengerScope;
  context: MessengerContext;
  root: string;
  draftKey: string;
  onSent: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const canMutate = useCanMutate();
  const me = useMe();
  const client = useQueryClient();
  const [draft, Draft] = useState(() => readMessengerDraft(draftKey));
  const now = useNow(draft.retryAt ? 1000 : 0);
  const cooling = Boolean(draft.retryAt && Date.parse(draft.retryAt) > now);
  const [editing, Editing] = useState(false);
  const [result, Result] = useState<Attempt | null>(null);
  const review = unresolvedDelivery(draft, context.unresolved_attempt, result);
  const locked = review.request !== null;
  const send = useMessengerSend();
  const receipt = useMessengerReceipt();
  const busy = send.isPending || receipt.isPending;
  const update = (next: MessengerDraft) => {
    keepMessengerDraft(draftKey, next);
    Draft(next);
  };
  const accept = (attempt: Attempt) => {
    Result(attempt);
    if (attempt.state === "accepted") {
      Draft(readMessengerDraft(draftKey));
      onSent();
      return;
    }
    if (attempt.state === "failed") {
      Draft(readMessengerDraft(draftKey));
      return;
    }
  };
  const submit = () => {
    if (!canMutate || cooling || busy || locked || !draft.text.trim()) return;
    const request = crypto.randomUUID();
    update({ ...draft, request });
    send.mutate(
      {
        scope,
        draftKey,
        body: {
          request_id: request,
          acknowledged_request_id: draft.acknowledged,
          grant_version: context.grant_version,
          conversation_id: context.conversation.id,
          root_id: root || undefined,
          text: draft.text,
          audience_key: context.audience_key,
        },
      },
      {
        onSuccess: accept,
        onError: () => {
          Draft(readMessengerDraft(draftKey));
          void client.invalidateQueries({
            queryKey: ["messenger-context", scope, context.conversation.id],
          });
        },
      },
    );
  };
  const error = send.error || receipt.error;
  return (
    <div className="messenger-compose">
      <ConfirmModal
        open={editing}
        onClose={() => Editing(false)}
        title={t("messenger.reviewDelivery")}
        confirmLabel={t("messenger.checkedConversation")}
        onConfirm={() => {
          update({
            ...draft,
            request: null,
            acknowledged: review.request ?? undefined,
          });
          Result(null);
          send.reset();
          receipt.reset();
          Editing(false);
        }}
      >
        <p>{t("messenger.newAttemptWarning")}</p>
      </ConfirmModal>
      <Textarea
        aria-label={t("messenger.write", { name: context.conversation.name })}
        value={draft.text}
        rows={3}
        disabled={busy || locked}
        onChange={(event) =>
          update({ ...draft, text: event.target.value, request: null })
        }
        onKeyDown={(event) => {
          const touch = window.matchMedia("(pointer:coarse)").matches;
          if (
            event.key === "Enter" &&
            !event.shiftKey &&
            !isImeCommit(event) &&
            !touch
          ) {
            event.preventDefault();
            submit();
          }
        }}
      />
      {!me.isPending && !canMutate && <p>{t("settings.seatRefused")}</p>}
      <small>{t("messenger.sendingAs", { name: context.sender.name })}</small>
      {error && (
        <Callout tone="warning" title={t("messenger.sendProblem")}>
          {problemMessageOf(error, t)}
        </Callout>
      )}
      {result?.state === "failed" && (
        <p role="status">{t(deliveryFailure(result.failure_code))}</p>
      )}
      {cooling && draft.retryAt && (
        <p role="status">
          {t("messenger.retryAfter", {
            at: formatTimeOfDay(draft.retryAt, locale, zone),
          })}
        </p>
      )}
      {review.request && !busy && (
        <div role="status">
          <p>{t("messenger.checkDelivery")}</p>
          <Button
            onClick={() => {
              if (!draft.request) {
                void client.invalidateQueries({
                  queryKey: [
                    "messenger-context",
                    scope,
                    context.conversation.id,
                  ],
                });
                return;
              }
              if (draft.request)
                receipt.mutate(
                  { scope, request: draft.request, draftKey },
                  { onSuccess: accept },
                );
            }}
          >
            {t("messenger.checkReceipt")}
          </Button>
          <Button
            disabled={review.pending}
            reason={review.pending ? t("messenger.deliveryPending") : undefined}
            onClick={() => Editing(true)}
          >
            {t("messenger.editDraft")}
          </Button>
        </div>
      )}
      <div className="messenger-send-actions">
        {result?.state === "accepted" && (
          <small role="status">{t("messenger.accepted")}</small>
        )}
        <Button
          variant="primary"
          pending={send.isPending}
          disabled={
            !canMutate || cooling || busy || locked || !draft.text.trim()
          }
          reason={sendRefusal(t, {
            canMutate,
            cooling,
            locked,
            empty: !draft.text.trim(),
          })}
          onClick={submit}
        >
          <Send aria-hidden="true" />
          {t("messenger.send")}
        </Button>
      </div>
    </div>
  );
}

function deliveryFailure(code: string | undefined) {
  switch (code) {
    case "reconnect":
      return "messenger.failureReconnect";
    case "unavailable":
      return "messenger.failureUnavailable";
    case "review_access":
      return "messenger.failureAccess";
    case "review_message":
      return "messenger.failureReview";
    case "unsupported":
      return "messenger.failureUnsupported";
    case "authorization_changed":
      return "messenger.failureAuthorization";
    case "rate_limited":
      return "messenger.failureRate";
    default:
      return "messenger.failed";
  }
}

function sendRefusal(
  t: Translator,
  state: Readonly<{
    canMutate: boolean;
    cooling: boolean;
    locked: boolean;
    empty: boolean;
  }>,
) {
  if (!state.canMutate) return t("settings.seatRefused");
  if (state.cooling) return t("messenger.failureRate");
  if (state.locked) return t("messenger.checkDelivery");
  if (state.empty) return t("messenger.messageRequired");
  return undefined;
}

function unresolvedDelivery(
  draft: MessengerDraft,
  remote: Attempt | undefined,
  result: Attempt | null,
) {
  const resolved = result?.state === "accepted" || result?.state === "failed";
  const outstanding =
    remote?.request_id !== draft.acknowledged &&
    !(resolved && result?.request_id === remote?.request_id)
      ? remote
      : undefined;
  const request = draft.request ?? outstanding?.request_id ?? null;
  const state =
    result?.request_id === request ? result?.state : outstanding?.state;
  return { request, pending: state === "pending" };
}
