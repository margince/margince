// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { unwrap } from "../../lib/problem";

type Contact = components["schemas"]["Contact"];
type Reason = NonNullable<
  components["schemas"]["ContactImpersonationWarning"]["reason"]
>;

// The reason is a closed code, never text from the mail. A code this build
// does not know reads as the sentence for a warning with no stored reason.
const REASON_COPY: Readonly<Record<Reason, MessageKey>> = {
  punycode_domain: "contact.impersonation.reason.punycodeDomain",
  display_name_other_domain:
    "contact.impersonation.reason.displayNameOtherDomain",
};

function reasonKey(reason: string | undefined): MessageKey {
  const known = Object.entries(REASON_COPY).find(([code]) => code === reason);
  return known ? known[1] : "contact.impersonation.reason.unrecorded";
}

/**
 * The warning on a contact whose first mail showed a sign of impersonation,
 * with the answer that clears it. It draws from the contact the page already
 * read, so a failed read is the page's own state and nothing shows here.
 */
export function ContactImpersonationWarning({
  contact,
  refusedReasonId,
}: Readonly<{
  contact: Contact;
  // The id of the page's sentence on why this reader may not edit the contact.
  refusedReasonId?: string;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const clear = useMutation({
    // The clear names the warning this reader was shown. The server refuses
    // it when the contact now carries another one.
    mutationFn: async (shown: { id: string; detectedAt: string }) => {
      unwrap(
        await api.POST("/contacts/{id}/impersonation-warning/clear", {
          params: { path: { id: shown.id } },
          body: { detected_at: shown.detectedAt },
        }),
        t,
      );
    },
    // Read again after a refusal too: the page then shows the warning that stands.
    onSettled: (_answer, _error, shown) =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: ["contact360", shown.id] }),
        queryClient.invalidateQueries({ queryKey: ["contact", shown.id] }),
      ]),
  });
  const warning = contact.impersonation_warning;
  if (!warning) {
    return null;
  }
  return (
    <Callout
      kind="standing"
      tone="warning"
      title={t("contact.impersonation.title")}
      actions={
        <Button
          pending={clear.isPending}
          reasonId={refusedReasonId}
          onClick={() =>
            clear.mutate({ id: contact.id, detectedAt: warning.detected_at })
          }
        >
          {t("contact.impersonation.clear")}
        </Button>
      }
    >
      {t(reasonKey(warning.reason))}
      <ErrorLine error={clear.error} />
    </Callout>
  );
}
