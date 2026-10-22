// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useRecordOwnerOptions } from "../../components/assigneepicker";
import {
  useRoster,
  useRosterPartial,
  useRosterPartialHint,
} from "../../components/entityref";
import { Field } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { ErrorLine } from "../../design-system/errorline";
import { Select } from "../../design-system/select";
import { useToast } from "../../design-system/toast";
import { formatNumber, INTL_LOCALE } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { problemMessageOf, unwrap } from "../../lib/problem";
import type { User } from "./users-members";

// Deactivating a member moves none of the records they own, so their work is
// handed to a colleague here: before they leave, or afterwards.

type HandoverCounts = components["schemas"]["HandoverCounts"];

const COUNTED = [
  ["deal", "users.handover.deals"],
  ["lead", "users.handover.leads"],
  ["contact", "users.handover.contacts"],
  ["company", "users.handover.companies"],
  ["project", "users.handover.projects"],
] as const;

/** The records in a count as one phrase ("16 deals and 4 leads"); "" for none. */
function useRecordsPhrase() {
  const plural = usePlural();
  const { locale } = useLocale();
  return (counts: HandoverCounts) => {
    const parts = COUNTED.filter(([kind]) => counts[kind] > 0).map(
      ([kind, base]) =>
        plural(base, counts[kind], {
          count: formatNumber(counts[kind], locale),
        }),
    );
    return new Intl.ListFormat(INTL_LOCALE[locale], {
      style: "long",
      type: "conjunction",
    }).format(parts);
  };
}

function useMemberHandover(memberId: string, enabled: boolean) {
  return useQuery({
    queryKey: ["users", memberId, "handover"],
    enabled,
    queryFn: async () =>
      unwrap(
        await api.GET("/users/{id}/handover", {
          params: { path: { id: memberId } },
        }),
      ).owned,
  });
}

/** What the member still owns, said where they are about to be deactivated. */
export function HandoverSummary({
  member,
  enabled,
}: Readonly<{ member: User; enabled: boolean }>) {
  const t = useT();
  const phrase = useRecordsPhrase();
  const owned = useMemberHandover(member.id, enabled);
  if (owned.isError) {
    return <ErrorLine error={owned.error} />;
  }
  const records = owned.data ? phrase(owned.data) : "";
  if (!records) {
    return null;
  }
  return (
    <p>
      {t("users.handover.owns", { name: member.display_name, records })}{" "}
      {t("users.handover.deactivateNote")}
    </p>
  );
}

/** Hand every live record the member owns to one colleague. */
export function HandoverModal({
  member,
  open,
  onClose,
  returnFocusTo,
}: Readonly<{
  member: User;
  open: boolean;
  onClose: () => void;
  returnFocusTo: () => HTMLElement | null;
}>) {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();
  const phrase = useRecordsPhrase();
  const [receiver, setReceiver] = useState("");
  // One key per opening, so a retried Hand over replays the first answer.
  const [idempotencyKey] = useState(() => crypto.randomUUID());
  const owned = useMemberHandover(member.id, open);
  const options = useRecordOwnerOptions(open).filter(
    (option) => option.value !== member.id,
  );
  // A roster that failed or stopped short is said so: a picker missing
  // colleagues looks just like a team without them.
  const roster = useRoster("user", open);
  const partialHint = useRosterPartialHint(useRosterPartial("user", open));
  const receiverName =
    options.find((option) => option.value === receiver)?.label ?? receiver;

  const handOver = useMutation({
    mutationFn: async (to: string) =>
      unwrap(
        await api.POST("/users/{id}/handover", {
          params: {
            path: { id: member.id },
            header: { "Idempotency-Key": idempotencyKey },
          },
          body: { to_user_id: to },
        }),
      ),
    onSuccess: async (result) => {
      // Records of every kind changed owner, so every list and page reads anew.
      await queryClient.invalidateQueries();
      onClose();
      const left = phrase(result.left);
      if (left) {
        toast.show(
          t("users.handover.left", {
            name: member.display_name,
            records: left,
          }),
          { tone: "danger", sticky: true },
        );
        return;
      }
      const moved = phrase(result.moved);
      toast.show(
        moved
          ? t("users.handover.done", { records: moved, name: receiverName })
          : t("users.handover.nothingMoved"),
      );
    },
  });

  const records = owned.data ? phrase(owned.data) : "";
  return (
    <ConfirmModal
      open={open}
      onClose={onClose}
      title={t("users.handover.title", { name: member.display_name })}
      confirmLabel={t("users.handover.confirm")}
      confirmReason={receiver ? undefined : t("users.handover.choose")}
      pending={handOver.isPending}
      error={handOver.error ? problemMessageOf(handOver.error, t) : null}
      onConfirm={() => handOver.mutate(receiver)}
      returnFocusTo={returnFocusTo}
    >
      <p>{t("users.handover.body", { name: member.display_name })}</p>
      {owned.data && (
        <p>
          {records
            ? t("users.handover.owns", { name: member.display_name, records })
            : t("users.handover.ownsNothing", { name: member.display_name })}
        </p>
      )}
      {owned.isError && <ErrorLine error={owned.error} />}
      <Field
        label={t("users.handover.receiver")}
        hint={roster.isError ? t("owner.rosterFailed") : partialHint}
      >
        {(control) => (
          <Select
            {...control}
            options={options}
            value={receiver}
            placeholder={t("users.handover.choose")}
            onChange={setReceiver}
          />
        )}
      </Field>
    </ConfirmModal>
  );
}
