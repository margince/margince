// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import { problemMessageOf, throwProblem } from "../../lib/problem";

type CommunicationContext = components["schemas"]["CommunicationContext"];

/** What a vouch is about: the contact, and the kind of message it covers. */
export type ReasonTarget = Readonly<{
  contactId: string;
  address: string;
  category: CommunicationContext;
}>;

type Vouch = ReasonTarget & Readonly<{ reason: string }>;

/**
 * A rep's own reason to write to a contact the engine found nothing for. It is
 * recorded where the refusal is read, rather than on the contact page.
 *
 * It is the standing vouch `allowContact` writes: not consent and not a lawful
 * basis, and only for the one category the engine resolved this message to.
 * The engine's answer is then asked again, so the composer reads the server's
 * verdict on the vouch rather than assuming it.
 */
export function ContactReasonForm({
  target,
  onRecorded,
}: Readonly<{ target: ReasonTarget; onRecorded: () => void }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [reason, setReason] = useState("");
  const vouch = useMutation({
    mutationFn: async (asked: Vouch) => {
      const { error, response } = await api.POST(
        "/contacts/{id}/consent/allow",
        {
          params: { path: { id: asked.contactId } },
          body: { category: asked.category, reason: asked.reason },
        },
      );
      if (!response.ok) {
        throwProblem(error || { title: t("compose.actionFailed") });
      }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["send-permission"] });
      onRecorded();
    },
  });
  const written = reason.trim();
  return (
    <form
      className="compose-reason"
      onSubmit={(event) => {
        event.preventDefault();
        if (written) vouch.mutate({ ...target, reason: written });
      }}
    >
      <Field
        label={t("compose.reasonLabel", { address: target.address })}
        hint={t("compose.reasonHint")}
      >
        {(control) => (
          <TextInput
            {...control}
            value={reason}
            maxLength={500}
            onChange={(event) => setReason(event.target.value)}
          />
        )}
      </Field>
      <Button
        type="submit"
        pending={vouch.isPending}
        disabled={!written}
        busyLabel={t("compose.reasonRecording")}
      >
        {t("compose.reasonRecord")}
      </Button>
      {vouch.isError && (
        <ErrorLine>{problemMessageOf(vouch.error, t)}</ErrorLine>
      )}
    </form>
  );
}
