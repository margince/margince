// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "./users-admin.css";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useCallback, useState } from "react";
import { api } from "../../api/client";
import { Button, Field } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { QueryStates } from "../../design-system/querystates";
import { RecordPicker } from "../../design-system/recordpicker";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import {
  memberRetirementBlocker,
  memberRetirementConsequence,
} from "../../lib/memberretirement";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";
import {
  matchingMembers,
  memberAnchorId,
  memberMutationKey,
  type User,
  useMemberRefresh,
} from "./users-members";

export function RetireMember({
  member,
  members,
  merge,
  onClose,
}: Readonly<{
  member: User;
  members: readonly User[];
  merge: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const toast = useToast();
  const refresh = useMemberRefresh();
  const [target, setTarget] = useState("");
  const preview = useQuery({
    queryKey: ["member-retirement-preview", member.id, target, merge],
    queryFn: async () =>
      unwrap(
        await api.GET("/users/{id}/retirement-preview", {
          params: {
            path: { id: member.id },
            query: {
              mode: merge ? "merge" : "remove",
              ...(target ? { to_user_id: target } : {}),
            },
          },
        }),
      ),
  });
  const retire = useMutation({
    mutationKey: memberMutationKey(member.id, merge ? "merge" : "remove"),
    mutationFn: async (choice: Readonly<{ target: string; token: string }>) => {
      if (merge) {
        unwrap(
          await api.POST("/users/{id}/merge", {
            params: { path: { id: member.id } },
            body: { to_user_id: choice.target, preview_token: choice.token },
          }),
        );
      } else {
        unwrap(
          await api.POST("/users/{id}/remove", {
            params: { path: { id: member.id } },
            body: {
              ...(choice.target ? { to_user_id: choice.target } : {}),
              preview_token: choice.token,
            },
          }),
        );
      }
    },
    onSuccess: async () => {
      await refresh();
      toast.show(t(merge ? "users.merged" : "users.removed"));
      onClose();
    },
    onError: async () => {
      await preview.refetch();
    },
  });
  const owned = preview.data?.owned;
  const counts = owned
    ? [
        [t("users.work.contacts"), owned.contact],
        [t("users.work.companies"), owned.company],
        [t("users.work.leads"), owned.lead],
        [t("users.work.deals"), owned.deal],
        [t("users.work.projects"), owned.project],
        [t("users.work.tasks"), owned.task],
        [t("users.work.alerts"), preview.data?.signal_count ?? 0],
      ]
    : [];
  const ready =
    preview.isSuccess &&
    !preview.isFetching &&
    preview.data.blockers.length === 0 &&
    (!merge || target !== "");
  return (
    <ConfirmModal
      open
      onClose={onClose}
      title={t(merge ? "users.mergeTitle" : "users.removeTitle", {
        name: member.display_name,
      })}
      confirmLabel={t(merge ? "users.merge" : "users.remove")}
      confirmVariant="danger"
      pending={retire.isPending}
      confirmDisabled={!ready}
      error={retire.error ? problemMessageOf(retire.error, t) : null}
      onConfirm={() => {
        if (ready) retire.mutate({ target, token: preview.data.token });
      }}
      returnFocusTo={() =>
        document.getElementById(memberAnchorId(member.id)) ??
        document.querySelector<HTMLInputElement>('input[type="search"]')
      }
    >
      <p>{t(merge ? "users.mergeBody" : "users.removeBody")}</p>
      <RetirementTarget
        memberId={member.id}
        members={members}
        merge={merge}
        target={target}
        onChange={setTarget}
        disabled={retire.isPending}
      />
      <QueryStates query={preview} pendingLabel={t("users.workToTransfer")}>
        {null}
      </QueryStates>
      {owned && (
        <div>
          <p>{t("users.workToTransfer")}</p>
          <ul>
            {counts.map(([label, count]) => (
              <li key={label}>
                {label}: {count}
              </li>
            ))}
          </ul>
        </div>
      )}
      {preview.data?.blockers.length ? (
        <ul>
          {preview.data.blockers.map((blocker) => (
            <li key={blocker}>{memberRetirementBlocker(blocker, t)}</li>
          ))}
        </ul>
      ) : null}
      {merge && <p>{t("users.visibleWorkNote")}</p>}
      <p className="users-retirement-address">
        {t("users.mergeSource")}: {preview.data?.source_email ?? member.email}
      </p>
      {preview.data?.survivor_email && (
        <p className="users-retirement-address">
          {t("users.survivingMember")}: {preview.data.survivor_email}
        </p>
      )}
      {preview.data?.consequences.map((code) => (
        <p key={code}>{memberRetirementConsequence(code, t)}</p>
      ))}
    </ConfirmModal>
  );
}

function RetirementTarget({
  memberId,
  members,
  merge,
  target,
  onChange,
  disabled,
}: Readonly<{
  memberId: string;
  members: readonly User[];
  merge: boolean;
  target: string;
  onChange: (id: string) => void;
  disabled: boolean;
}>) {
  const t = useT();
  const me = useMe();
  const selected = members.find((one) => one.id === target);
  const searchMembers = useCallback(
    async (query: string) =>
      matchingMembers(members, query)
        .filter(
          (one) =>
            one.id !== memberId &&
            (!merge || one.id !== me.data?.user.id) &&
            !one.is_agent &&
            one.status !== "suspended" &&
            (merge || one.status === "active"),
        )
        .map((one) => ({
          id: one.id,
          name: one.display_name,
          detail: one.email,
        })),
    [members, memberId, me.data?.user.id, merge],
  );
  return (
    <Field
      label={t(merge ? "users.survivingMember" : "users.handover.action")}
      hint={
        !target
          ? t(merge ? "users.chooseMember" : "users.noHandover")
          : undefined
      }
      hintLive={!merge}
    >
      {(control) => (
        <>
          <RecordPicker
            {...control}
            searchTargets={searchMembers}
            selected={
              selected
                ? {
                    id: selected.id,
                    name: selected.display_name,
                    detail: selected.email,
                  }
                : null
            }
            onPick={(candidate) => onChange(candidate.id)}
            disabled={disabled}
          />
          {!merge && target && (
            <Button
              variant="link"
              disabled={disabled}
              onClick={() => {
                onChange("");
                document.getElementById(control.id)?.focus();
              }}
            >
              {t("field.unset")}
            </Button>
          )}
        </>
      )}
    </Field>
  );
}
