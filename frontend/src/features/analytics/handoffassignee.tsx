// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { useRecordOwnerOptions } from "../../components/assigneepicker";
import { useRosterHint, useRosterPartial } from "../../components/entityref";
import { Button, Field } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { ErrorLine } from "../../design-system/errorline";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import { problemMessageOf, unwrap } from "../../lib/problem";

export function HandoffAssigneeModal({
  id,
  onClose,
}: Readonly<{ id: string; onClose: () => void }>) {
  const t = useT();
  const client = useQueryClient();
  const [receiver, setReceiver] = useState("");
  const { options, roster } = useRecordOwnerOptions();
  const rosterHint = useRosterHint(roster, useRosterPartial("user", true));
  const current = useQuery({
    queryKey: ["handoff-assignee", id],
    staleTime: 0,
    queryFn: async ({ signal }) =>
      unwrap(
        await api.GET("/sdr-handoffs/{id}/assignee", {
          params: { path: { id } },
          signal,
        }),
      ),
  });
  const change = useMutation({
    mutationFn: async (input: { receiver: string; version: number }) =>
      unwrap(
        await api.PATCH("/sdr-handoffs/{id}/assignee", {
          params: { path: { id } },
          body: { assigned_to: input.receiver, version: input.version },
        }),
      ),
    onError: async () => {
      await client.invalidateQueries({ queryKey: ["handoff-assignee", id] });
    },
    onSuccess: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: ["sdr-summary"] }),
        client.invalidateQueries({ queryKey: ["handoff-assignee", id] }),
        client.invalidateQueries({ queryKey: ["users"] }),
      ]);
      onClose();
    },
  });
  const ready =
    current.isSuccess &&
    !current.isFetching &&
    current.data.can_reassign &&
    receiver !== "" &&
    roster.isSuccess;
  return (
    <ConfirmModal
      open
      onClose={onClose}
      title={t("analytics.handoffReassign")}
      confirmLabel={t("analytics.handoffReassign")}
      pending={change.isPending}
      confirmReason={
        ready ? undefined : t("analytics.handoffReassignUnavailable")
      }
      error={change.error ? problemMessageOf(change.error, t) : null}
      onConfirm={() => {
        if (ready && current.data)
          change.mutate({ receiver, version: current.data.version });
      }}
    >
      <p>{t("analytics.handoffReassignHint")}</p>
      {current.isError && (
        <ErrorLine
          error={current.error}
          actions={
            <Button onClick={() => void current.refetch()}>
              {t("common.retry")}
            </Button>
          }
        />
      )}
      {roster.isError && (
        <ErrorLine
          error={roster.error}
          actions={
            <Button onClick={() => void roster.refetch()}>
              {t("common.retry")}
            </Button>
          }
        />
      )}
      {current.data && !current.data.can_reassign && (
        <p>{t("analytics.handoffReassignUnavailable")}</p>
      )}
      <Field label={t("users.handover.receiver")} hint={rosterHint}>
        {(control) => (
          <Select
            {...control}
            options={options}
            value={receiver}
            onChange={setReceiver}
            placeholder={t("users.handover.choose")}
            disabled={change.isPending || !current.data?.can_reassign}
          />
        )}
      </Field>
    </ConfirmModal>
  );
}
