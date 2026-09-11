// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The confirm link's OTHER answer: one named subscription, and nothing else.
//
// A consent link's mail said "confirm this subscription". Answering it with the
// record card would hand whoever holds the link the contact's name, employer,
// address, phone and the whole provenance trail — wider than the mail described
// and wider than the link's own write side allows. So this page shows the
// purpose and the contact's current answer to it, and reads no record field at
// all.
//
// It is a separate component rather than a branch inside the record body
// because the two share no field. The record body reads `provenance`,
// `full_name` and four correctable fields; a subscription payload carries three
// strings. Sharing one component meant every one of those reads was a guess
// about which body had arrived, and `card.provenance.length` on a subscription
// answer is the crash this file removes.

import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Card } from "../design-system/atoms";
import { useT } from "../i18n";
import { throwProblem } from "./common";
import {
  explainPublicError,
  LinkInvalidError,
  RateLimitedError,
} from "./preferences";

type SubscriptionPage = components["schemas"]["SubscriptionConfirmationPage"];

// SubscriptionConfirm owns the consent link end to end: its own submit, its own
// wording, its own confirmed and refused states.
//
// It used to share the record page's mutation, which is what produced two of
// the three defects review found here — the submit posted the record page's
// marketing sentence as the proof of a subscription, and a refusal rendered
// nowhere because this branch returns before the record page's error line.
// Sharing a submit between two pages that ask different questions was the
// mistake; each now answers for itself.
export function SubscriptionConfirm({
  token,
  card,
}: Readonly<{ token: string; card: SubscriptionPage }>) {
  const t = useT();
  const [done, setDone] = useState(false);

  // The sentence THIS page shows, which is the one the server stores verbatim
  // as consent evidence. Read where it is rendered, not re-derived at submit
  // time, because the two drifting is how proof stops matching the promise.
  const wording = t("confirm.subscription.ask", {
    purpose: card.purpose_label,
  });

  const submit = useMutation({
    mutationFn: async ({ policyText }: { policyText: string }) => {
      const { error, response } = await api.POST("/public/confirm/{token}", {
        params: { path: { token } },
        body: {
          corrections: [],
          request_erasure: false,
          marketing_choice: "granted" as const,
          marketing_wording: policyText,
        },
      });
      // GATED ON THE STATUS, NOT ON `error`. openapi-fetch returns
      // {error: undefined} for a non-2xx with an empty body — a proxy 502, a
      // 500 that wrote no problem document — so asking only whether `error` is
      // truthy reports those as success, and this page's success state tells
      // somebody their consent was recorded when nothing was written.
      if (!response.ok) {
        if (response.status === 404) {
          throw new LinkInvalidError();
        }
        if (response.status === 429) {
          throw new RateLimitedError();
        }
        throwProblem(error);
      }
    },
    onSuccess: () => setDone(true),
  });

  return (
    <SubscriptionConfirmBody
      card={done ? { ...card, state: "granted" } : card}
      submitting={submit.isPending}
      error={submit.error ? explainPublicError(submit.error, t) : undefined}
      onConfirm={() => submit.mutate({ policyText: wording })}
    />
  );
}

export function SubscriptionConfirmBody({
  card,
  onConfirm,
  submitting,
  error,
}: Readonly<{
  card: SubscriptionPage;
  onConfirm: () => void;
  submitting: boolean;
  /** A refused submit, already translated. Rendered beside the button: this
   *  page returns before the record page's own error line, so a rejection
   *  passed nowhere is a rejection nobody sees. */
  error?: string;
}>) {
  const t = useT();

  // ALREADY ANSWERED is its own state, not a disabled button.
  //
  // Somebody who confirmed and then followed the same link again — a second
  // click, a mail client prefetching, a forwarded message — should be told
  // their answer is recorded rather than shown a form that looks broken.
  if (card.state === "granted") {
    return (
      <div className="pref-page">
        <Card>
          <h1 className="t-h2">{t("confirm.subscription.alreadyTitle")}</h1>
          <p className="t-body">
            {t("confirm.subscription.alreadyBody", {
              purpose: card.purpose_label,
            })}
          </p>
        </Card>
      </div>
    );
  }

  return (
    <div className="pref-page">
      <h1 className="t-h2">{t("confirm.subscription.title")}</h1>
      <Card>
        <h2 className="t-h3">{card.purpose_label}</h2>
        <p className="t-body">
          {t("confirm.subscription.ask", { purpose: card.purpose_label })}
        </p>
        {/* One affirmative act, never a pre-selected one: a page that arrives
            with the answer already given is not the contact's own act. */}
        <Button onClick={onConfirm} disabled={submitting}>
          {t("confirm.subscription.confirm")}
        </Button>
        {error && (
          <p className="t-caption confirm-error" role="status">
            {error}
          </p>
        )}
      </Card>
    </div>
  );
}
