// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch, requireVersion } from "../../api/version";
import { useAssignableUserOptions } from "../../components/assigneepicker";
import {
  EntityRef,
  RosterFailedLine,
  useRosterHint,
  useRosterPartial,
} from "../../components/entityref";
import { TaskAssignee } from "../../components/taskassignee";
import { refetchAfterAssignment } from "../../components/taskwritefollowup";
import { Button, Field } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { RecordFormDialog } from "../../design-system/recordformdialog";
import { Select } from "../../design-system/select";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";
import type { WorklistItem } from "./worklist.queries";

type Request = components["schemas"]["WebsiteInquiryAssignmentRequest"];
type Assignment = components["schemas"]["WebsiteInquiryAssignment"];

export function InquiryAssignmentDialog({
  item,
  mode,
  onClose,
}: Readonly<{
  item: WorklistItem;
  mode: Request["mode"];
  onClose: () => void;
}>) {
  const t = useT();
  const client = useQueryClient();
  const toast = useToast();
  const viewer = useMe();
  const me = viewer.data?.user.id;
  const [selected, setSelected] = useState("");
  const recipient = mode === "take" ? me : selected;
  const rosterEnabled = mode === "assign_and_follow_up";
  const { options, roster } = useAssignableUserOptions(rosterEnabled);
  const rosterHint = useRosterHint(
    roster,
    useRosterPartial("user", rosterEnabled),
  );
  const preview = useQuery({
    queryKey: [
      "website-inquiry-assignment",
      item.id,
      item.version,
      mode,
      recipient,
    ],
    enabled: Boolean(recipient),
    gcTime: 0,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
    retry: false,
    queryFn: async () => {
      if (!recipient) throw new Error(t("inquiry.assignment.chooseColleague"));
      return unwrap(
        await api.POST("/activities/{id}/inquiry-assignment/preview", {
          params: { path: { id: item.id } },
          body: { mode, recipient_id: recipient },
        }),
      );
    },
  });
  const save = useMutation({
    mutationFn: async (
      input: Readonly<{ id: string; version: number; choice: Request }>,
    ) =>
      unwrap(
        await api.POST("/activities/{id}/inquiry-assignment", {
          params: { path: { id: input.id }, ...ifMatch(input.version) },
          body: input.choice,
        }),
      ),
    onSuccess: (result, input) =>
      refetchAfterAssignment(client, input.id, result.task),
  });
  const candidate = preview.data;
  const reviewed =
    candidate?.record_type === "lead" || candidate?.record_type === "contact"
      ? candidate
      : undefined;
  const title = t(
    mode === "take" ? "inquiry.assignment.take" : "inquiry.assignment.assign",
  );
  const confirm = () => {
    if (!reviewed || !recipient) return;
    save
      .mutateAsync({
        id: item.id,
        version: requireVersion(reviewed.task.version),
        choice: {
          mode,
          recipient_id: recipient,
          record_id: reviewed.record_id,
          record_type: reviewed.record_type,
          record_version: requireVersion(reviewed.record_version),
        },
      })
      .then(
        () => {
          onClose();
          toast.show(t("inquiry.assignment.done"));
        },
        () =>
          toast.show(t("worklist.manager.reassignFailed"), { tone: "danger" }),
      );
  };
  return (
    <RecordFormDialog
      open
      title={title}
      onClose={onClose}
      fieldCount={rosterEnabled ? 1 : 0}
      closeDisabled={save.isPending}
      form={
        <div className="form-stack">
          {rosterEnabled && (
            <Field
              label={t("inquiry.assignment.chooseColleague")}
              hint={rosterHint}
            >
              {(control) => (
                <Select
                  {...control}
                  options={options}
                  value={selected}
                  onChange={setSelected}
                />
              )}
            </Field>
          )}
          {rosterEnabled && <RosterFailedLine kind="user" roster={roster} />}
          {mode === "take" && viewer.isPending && <p>{t("common.loading")}</p>}
          {mode === "take" && viewer.isError && (
            <ErrorLine error={viewer.error} />
          )}
          {recipient && preview.isPending && <p>{t("common.loading")}</p>}
          {preview.isError && (
            <>
              <ErrorLine error={preview.error} />
              <Button onClick={() => preview.refetch()}>
                {t("common.retry")}
              </Button>
            </>
          )}
          {candidate && !reviewed && (
            <ErrorLine>{t("inquiry.assignment.unsupported")}</ErrorLine>
          )}
          {reviewed && <AssignmentReview reviewed={reviewed} />}
          {save.isError && (
            <>
              <ErrorLine error={save.error} />
              <Button
                onClick={() =>
                  preview.refetch().then((result) => {
                    if (result.isSuccess) save.reset();
                  })
                }
              >
                {t("inquiry.assignment.reviewAgain")}
              </Button>
            </>
          )}
        </div>
      }
      actions={
        <>
          <Button variant="ghost" onClick={onClose} disabled={save.isPending}>
            {t("worklist.manager.cancel")}
          </Button>
          <Button
            variant="primary"
            pending={save.isPending}
            disabled={
              !reviewed || preview.isFetching || !recipient || save.isError
            }
            onClick={confirm}
          >
            {title}
          </Button>
        </>
      }
    />
  );
}

function AssignmentReview({ reviewed }: Readonly<{ reviewed: Assignment }>) {
  const t = useT();
  return (
    <div className="form-stack">
      <p>{reviewed.task.subject}</p>
      <p>
        {t("inquiry.assignment.record")}:{" "}
        <EntityRef kind={reviewed.record_type} id={reviewed.record_id} />
      </p>
      <p>
        {t("inquiry.currentRecordOwner")}:{" "}
        {reviewed.record_owner_id ? (
          <EntityRef kind="user" id={reviewed.record_owner_id} />
        ) : (
          t("log.unassigned")
        )}
      </p>
      <p>
        {t("inquiry.assignment.taskHolder")}:{" "}
        <TaskAssignee id={reviewed.task.assignee_id} />
      </p>
      <p>
        {t("inquiry.assignment.newHolder")}:{" "}
        <EntityRef kind="user" id={reviewed.recipient_id} />
      </p>
      <p>
        {t(
          reviewed.changes_record_owner
            ? "inquiry.assignment.movesLead"
            : "inquiry.assignment.taskOnly",
        )}
      </p>
    </div>
  );
}

export function InquiryTakeButton({ item }: Readonly<{ item: WorklistItem }>) {
  const t = useT();
  const [reviewing, setReviewing] = useState(false);
  return (
    <>
      <Button onClick={() => setReviewing(true)}>
        {t("inquiry.assignment.take")}
      </Button>
      {reviewing && (
        <InquiryAssignmentDialog
          item={item}
          mode="take"
          onClose={() => setReviewing(false)}
        />
      )}
    </>
  );
}
