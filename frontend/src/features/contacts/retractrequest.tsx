// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { useT } from "../../i18n";
import { problemMessageOf, unwrap } from "../../lib/problem";

type QualifyingEvent = components["schemas"]["ConsentQualifyingEvent"];

/**
 * Takes back an exchange a colleague recorded by hand: the contact's request
 * to hear from us, or a card handed over in person. Only such an exchange
 * carries an id, so a derived one (an inbound mail, a meeting) offers nothing.
 *
 * It asks first. A withdrawn exchange cannot be brought back: recording it
 * again returns it withdrawn.
 */
export function RetractRecordedRequest({
  contactId,
  event,
  mayWrite,
}: Readonly<{
  contactId: string;
  event: QualifyingEvent | undefined;
  mayWrite: boolean;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [asking, setAsking] = useState(false);
  const retract = useMutation({
    mutationFn: async (eventId: string) => {
      unwrap(
        await api.POST(
          "/contacts/{id}/consent/qualifying-events/{event_id}/retract",
          { params: { path: { id: contactId, event_id: eventId } } },
        ),
      );
    },
    onSuccess: () => {
      setAsking(false);
      for (const key of ["contactConsentGuard", "contact-consent"]) {
        queryClient.invalidateQueries({ queryKey: [key, contactId] });
      }
    },
  });
  const eventId = event?.id;
  if (!mayWrite || !eventId) {
    return null;
  }
  return (
    <>
      <Button variant="link" onClick={() => setAsking(true)}>
        {t("consent.retractRequest")}
      </Button>
      <ConfirmModal
        open={asking}
        onClose={() => {
          setAsking(false);
          retract.reset();
        }}
        title={t("consent.retractRequestTitle")}
        confirmLabel={t("consent.retractRequest")}
        confirmVariant="danger"
        onConfirm={() => retract.mutate(eventId)}
        pending={retract.isPending}
        error={retract.isError ? problemMessageOf(retract.error, t) : null}
      >
        <p className="t-body">{t("consent.retractRequestBody")}</p>
      </ConfirmModal>
    </>
  );
}
