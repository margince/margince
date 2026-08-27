import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Field, Textarea } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Select } from "../design-system/select";
import { leadIdentityName } from "../format/leadname";
import { useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import { leadWriteKeys } from "./leadkeys";
import { useLeadDisqualifyReasons } from "./leadsources";

type Lead = components["schemas"]["Lead"];

// Closing a lead asks why. The reason comes from the administered list
// (Settings › Data model), is required here because the list exists to be
// answered from, and lands on the lead where the page and the list's
// Disqualified view read it back.

export function DisqualifyDialog({
  lead,
  open,
  onClose,
  onDisqualified,
}: Readonly<{
  lead: Lead;
  open: boolean;
  onClose: () => void;
  onDisqualified: (closed: Lead) => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const reasons = useLeadDisqualifyReasons();
  const [reasonId, setReasonId] = useState("");
  const [note, setNote] = useState("");
  const active = (Array.isArray(reasons.data) ? reasons.data : []).filter(
    (reason) => reason.active,
  );

  // Which lead, closed for which reason, with which note — carried as the
  // mutation's variable rather than read off render state inside it. The press
  // belongs to the render that drew the confirm, so what it hands over cannot
  // be older than the form the reader filled in.
  const disqualify = useMutation({
    mutationFn: async (closure: {
      leadId: string;
      reasonId: string;
      note: string | null;
    }) => {
      const { data, error } = await api.DELETE("/leads/{id}", {
        params: { path: { id: closure.leadId } },
        body: { reason_id: closure.reasonId, note: closure.note },
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
    onSuccess: (closed, closure) => {
      for (const key of leadWriteKeys(closure.leadId)) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      onDisqualified(closed);
    },
  });

  // A close in flight is not something to walk away from: the dialog stays
  // until the server has answered.
  const close = () => {
    if (disqualify.isPending) return;
    disqualify.reset();
    onClose();
  };
  const name = leadIdentityName(lead);

  return (
    <ConfirmModal
      open={open}
      onClose={close}
      title={t("lead.disqualify.title", { name })}
      confirmLabel={t("lead.disqualify.confirm")}
      confirmVariant="danger"
      // Required, and said so: a closed lead with no reason is what the
      // administered list exists to prevent.
      confirmReason={reasonId ? undefined : t("lead.disqualify.reasonRequired")}
      onConfirm={() =>
        disqualify.mutate({
          leadId: lead.id,
          reasonId,
          note: note.trim() || null,
        })
      }
      pending={disqualify.isPending}
      error={
        disqualify.isError ? problemMessageOf(disqualify.error, t) : undefined
      }
    >
      <div className="lead-qualify">
        <Field label={t("lead.disqualify.reason")} required>
          {(control) => (
            <Select
              {...control}
              value={reasonId}
              placeholder={t("lead.disqualify.pickReason")}
              onChange={setReasonId}
              options={active.map((reason) => ({
                value: reason.id,
                label: reason.label,
              }))}
            />
          )}
        </Field>
        <Field label={t("lead.disqualify.note")}>
          {(control) => (
            <Textarea
              {...control}
              value={note}
              onChange={(event) => setNote(event.target.value)}
            />
          )}
        </Field>
      </div>
    </ConfirmModal>
  );
}
