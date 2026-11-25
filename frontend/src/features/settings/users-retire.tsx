// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "./users-admin.css";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useCallback, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, Field } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { QueryStates } from "../../design-system/querystates";
import { RecordPicker } from "../../design-system/recordpicker";
import { useToast } from "../../design-system/toast";
import { usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
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
    // A holder confirms from their own mail, so the answer arrives unasked.
    refetchInterval: (query) => confirmationPollInterval(query.state.data),
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
  // The merge dialog names a duplicate; the removal names the member.
  const addressLabel = merge ? "users.mergeSource" : "users.retirement.address";
  const owned = preview.data?.owned;
  const counts = owned
    ? [
        [t("users.work.contacts"), owned.contact],
        [t("users.work.companies"), owned.company],
        [t("users.work.leads"), owned.lead],
        [t("users.work.deals"), owned.deal],
        [t("users.work.projects"), owned.project],
        [t("users.work.tasks"), owned.task],
        [t("users.work.lists"), owned.list ?? 0],
        [t("users.work.alerts"), preview.data?.signal_count ?? 0],
      ]
    : [];
  // A blocker is said in the callout at the top, so the hold is explained. A
  // merge with no survivor picked is one: the preview names it.
  const blocked = Boolean(preview.data?.blockers.length);
  const settled = preview.isSuccess && !preview.isFetching && !blocked;
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
      confirmDisabled={!settled}
      error={retire.error ? problemMessageOf(retire.error, t) : null}
      onConfirm={() => {
        if (settled) retire.mutate({ target, token: preview.data.token });
      }}
      returnFocusTo={() =>
        document.getElementById(memberAnchorId(member.id)) ??
        document.querySelector<HTMLInputElement>('input[type="search"]')
      }
    >
      {/* First, so the reason the confirm is held is the first thing read. */}
      <RetirementBlockers blockers={preview.data?.blockers} />
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
      {merge && (
        <MergeConfirmations
          memberId={member.id}
          target={target}
          preview={preview.data}
          refetch={() => preview.refetch()}
        />
      )}
      {merge && <MergeNotes member={member} />}
      <p className="users-retirement-address">
        {t(addressLabel)}: {preview.data?.source_email ?? member.email}
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

function MergeNotes({ member }: Readonly<{ member: User }>) {
  const t = useT();
  return (
    <>
      {member.merge_needs_confirmation && (
        <p>{t("users.mergeNeedsConfirmation")}</p>
      )}
      <p>{t("users.visibleWorkNote")}</p>
    </>
  );
}

const CONFIRMATION_POLL_MS = 10_000;

type MergePreview = components["schemas"]["MemberRetirementPreview"];

function confirmationPollInterval(
  preview: MergePreview | undefined,
): number | false {
  const waiting = preview?.confirmations?.some(
    (side) => side.state === "requested",
  );
  return waiting ? CONFIRMATION_POLL_MS : false;
}

// Where each holder stands on a merge of a used account, and the button that
// mails the links. Asking is offered only while confirmation is all that holds
// the merge, because the server refuses it for any other blocker.
function MergeConfirmations({
  memberId,
  target,
  preview,
  refetch,
}: Readonly<{
  memberId: string;
  target: string;
  preview: MergePreview | undefined;
  refetch: () => Promise<unknown>;
}>) {
  const t = useT();
  const toast = useToast();
  const ask = useMutation({
    mutationKey: memberMutationKey(memberId, "merge-request"),
    mutationFn: async (token: string) =>
      unwrap(
        await api.POST("/users/{id}/merge-requests", {
          params: { path: { id: memberId } },
          body: { to_user_id: target, preview_token: token },
        }),
      ),
    onSuccess: async () => {
      await refetch();
      toast.show(t("users.merge.asked"));
    },
  });
  if (!preview?.confirmations) return null;
  const sides = preview.confirmations;
  const open = sides.some(
    (side) => side.state === "requested" || side.state === "not_requested",
  );
  const onlyConfirmationsHold = preview.blockers.every(
    (blocker) => blocker === "confirmation_missing",
  );
  const asked = sides.some((side) => side.state === "requested");
  return (
    <div>
      <p>{t("users.merge.confirmTitle")}</p>
      <ul>
        {sides.map((side) => (
          <li key={side.user_id}>
            {side.display_name}: {t(CONFIRMATION_STATE[side.state])}
          </li>
        ))}
      </ul>
      {open && onlyConfirmationsHold && (
        <Button
          disabled={ask.isPending}
          onClick={() => ask.mutate(preview.token)}
        >
          {t(asked ? "users.merge.askAgain" : "users.merge.ask")}
        </Button>
      )}
      {ask.error && <p role="alert">{problemMessageOf(ask.error, t)}</p>}
    </div>
  );
}

const CONFIRMATION_STATE: Readonly<
  Record<components["schemas"]["MemberMergeConfirmation"]["state"], MessageKey>
> = {
  confirmed: "users.merge.state.confirmed",
  requested: "users.merge.state.requested",
  not_requested: "users.merge.state.not_requested",
  not_needed: "users.merge.state.not_needed",
};

// What holds the confirm, read before anything else in the dialog.
function RetirementBlockers({
  blockers,
}: Readonly<{ blockers: readonly string[] | undefined }>) {
  const t = useT();
  const plural = usePlural();
  if (!blockers?.length) return null;
  return (
    <Callout
      tone="warning"
      kind="standing"
      title={plural("users.retirement.blocked", blockers.length)}
    >
      <ul>
        {blockers.map((blocker) => (
          <li key={blocker}>{memberRetirementBlocker(blocker, t)}</li>
        ))}
      </ul>
    </Callout>
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
