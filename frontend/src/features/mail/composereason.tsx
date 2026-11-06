// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import type { components } from "../../api/schema";
import { Button, Field } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import { contextOptions } from "../../lib/compose-context";
import { problemMessageOf } from "../../lib/problem";
import { recordOverrule } from "./sendreviewaction";

type CommunicationContext = components["schemas"]["CommunicationContext"];

/** What a vouch is about: the contact, and the kind of message it covers. */
export type ReasonTarget = Readonly<{
  contactId: string;
  address: string;
  category: CommunicationContext;
}>;

type Vouch = ReasonTarget & Readonly<{ reason: string }>;

/**
 * A rep's own reason to write to a contact the engine found nothing for. The
 * standing vouch `allowContact` writes, under the rep's name and not consent,
 * for the category the engine resolved; the engine is then asked again.
 * A reason the message already names is one press, not a second pick. The
 * caption says it is standing, so it is not read as a fact about one message.
 */
export function ContactReasonPick({
  target,
  chosen,
}: Readonly<{ target: ReasonTarget; chosen?: CommunicationContext }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [picked, setPicked] = useState("");
  const vouch = useMutation({
    mutationFn: (asked: Vouch) =>
      recordOverrule({
        overrulable: [{ contactId: asked.contactId, category: asked.category }],
        reason: asked.reason,
        onRecorded: () => {},
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["send-permission"] });
    },
  });
  const options = contextOptions(t).filter((option) => option.value !== "");
  const chosenLabel = options.find((o) => o.value === chosen)?.label;
  return (
    <div className="compose-reason">
      <p className="t-caption">
        {t("compose.reasonStanding", { address: target.address })}
      </p>
      {chosenLabel ? (
        <Button
          onClick={() => vouch.mutate({ ...target, reason: chosenLabel })}
          pending={vouch.isPending}
          aria-label={t("compose.reasonRecordFor", {
            reason: chosenLabel,
            address: target.address,
          })}
        >
          {t("compose.reasonRecord")}
        </Button>
      ) : (
        <Field
          label={t("compose.reasonLabel", { address: target.address })}
          labelHidden
        >
          {(control) => (
            <Select
              {...control}
              options={options}
              value={picked}
              placeholder={t("compose.whyPick")}
              disabled={vouch.isPending}
              onChange={(value) => {
                const label = options.find((o) => o.value === value)?.label;
                if (!label) return;
                setPicked(value);
                vouch.mutate({ ...target, reason: label });
              }}
            />
          )}
        </Field>
      )}
      {vouch.isError && (
        <ErrorLine>{problemMessageOf(vouch.error, t)}</ErrorLine>
      )}
    </div>
  );
}

type Preview = components["schemas"]["SendAuthorizationPreview"];

/**
 * The reason the record already carries for every recipient, when the claimed
 * one refuses them all. The engine names alternatives only for a recipient it
 * refuses. A switch changes the reason for the whole message, so a send with
 * an allowed recipient is offered none. An answer still out, or one that
 * failed, offers none either.
 */
export function alternativeReasonOf(permission: {
  preview: Preview | undefined;
  asking: boolean;
  unanswered: boolean;
}): CommunicationContext | undefined {
  if (permission.asking || permission.unanswered || !permission.preview) {
    return undefined;
  }
  const { recipients } = permission.preview;
  if (!recipients.every((r) => r.would_refuse)) {
    return undefined;
  }
  const [first, ...rest] = recipients;
  return (first?.supported_alternatives ?? []).find((reason) =>
    rest.every((r) => r.supported_alternatives?.includes(reason)),
  );
}

/**
 * The one press that answers a refused reason with the one the record
 * carries. It switches the reason and the engine is asked again; it records
 * nothing and sends nothing.
 */
export function ReasonSwitch({
  reason,
  onSwitch,
}: Readonly<{
  reason: CommunicationContext;
  onSwitch: (next: CommunicationContext) => void;
}>) {
  const t = useT();
  const label =
    contextOptions(t).find((option) => option.value === reason)?.label ??
    reason;
  return (
    <div className="compose-reason">
      <p className="t-caption">{t("compose.reasonSwitchText")}</p>
      <Button onClick={() => onSwitch(reason)}>
        {t("compose.reasonSwitch", { reason: label })}
      </Button>
    </div>
  );
}
