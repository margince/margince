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
 * A reason the message already names is one press, not a second pick.
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
