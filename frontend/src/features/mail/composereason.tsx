// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import type { components } from "../../api/schema";
import { Field } from "../../design-system/atoms";
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
 * A rep's own reason to write to a contact the engine found nothing for,
 * picked where the refusal is read. The pick is the record: the vouch is
 * written at once, under the rep's name, with the picked label as its reason.
 * It is the standing vouch `allowContact` writes, not consent, and covers only
 * the category the engine resolved. The engine is then asked again.
 */
export function ContactReasonPick({
  target,
}: Readonly<{ target: ReasonTarget }>) {
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
  return (
    <div className="compose-reason">
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
      {vouch.isError && (
        <ErrorLine>{problemMessageOf(vouch.error, t)}</ErrorLine>
      )}
    </div>
  );
}
