// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import { useRoster, useRosterPartial } from "../../components/roster";
import { useUnsavedGuard } from "../../components/unsaved";
import { Button, Field, Modal, TextInput } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { QueryStates } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { useToast } from "../../design-system/toast";
import { useT } from "../../i18n";
import { useHoldsAdminRole } from "../../lib/capability";
import { problemCode, unwrap } from "../../lib/problem";
import { roleOptions, useAssignableRoles } from "../../lib/roles.queries";
import {
  memberMutationKey,
  offers,
  type User,
  useMemberRefresh,
} from "./users-members";
import { MemberTeamsField } from "./users-teams";

export function EditMember({
  member,
  onClose,
}: Readonly<{ member: User; onClose: () => void }>) {
  const t = useT();
  const id = useId();
  const formId = useId();
  const refresh = useMemberRefresh();
  const toast = useToast();
  const [baseline, setBaseline] = useState(member);
  const [name, setName] = useState(member.display_name);
  const [email, setEmail] = useState(member.email);
  const [role, setRole] = useState(memberRole(member));
  const [teamIds, setTeamIds] = useState(memberTeams(member));
  const roles = useAssignableRoles(true);
  const managesTeams = useHoldsAdminRole();
  const teams = useRoster("team", true);
  const partial = useRosterPartial("team", true);
  const save = useMutation({
    mutationKey: memberMutationKey(member.id, "edit"),
    mutationFn: async (
      choice: Readonly<{
        name: string;
        email: string;
        role: string;
        teams: string[] | undefined;
        updatedAt: string;
        memberId: string;
      }>,
    ) => {
      unwrap(
        await api.PATCH("/users/{id}", {
          params: { path: { id: choice.memberId } },
          body: {
            display_name: choice.name.trim(),
            email: choice.email.trim(),
            ...(choice.role ? { role: choice.role } : {}),
            ...(choice.teams ? { team_ids: choice.teams } : {}),
            expected_updated_at: choice.updatedAt,
          },
        }),
      );
    },
    onSuccess: async () => {
      await refresh();
      toast.show(t("users.saved"));
      onClose();
    },
  });
  const reload = useMutation({
    mutationFn: async (memberId: string) =>
      unwrap(
        await api.GET("/users/{id}", { params: { path: { id: memberId } } }),
      ),
    onSuccess: (current) => {
      setBaseline(current);
      setName(current.display_name);
      setEmail(current.email);
      setRole(memberRole(current));
      setTeamIds(memberTeams(current));
      save.reset();
    },
  });
  const teamsChanged =
    [...teamIds].sort().join(";") !==
    [...(baseline.team_ids ?? [])].sort().join(";");
  const initialRole = memberRole(baseline);
  const roleChanged = role !== initialRole;
  const ready =
    !!baseline.updated_at &&
    (!roleChanged || roles.isSuccess) &&
    (!teamsChanged || (teams.isSuccess && !partial));
  useUnsavedGuard(
    name !== baseline.display_name ||
      email !== baseline.email ||
      roleChanged ||
      teamsChanged,
  );
  return (
    <Modal open onClose={onClose} intent="form" labelledBy={id}>
      <Heading id={id} size="large" className="modal-title">
        {t("users.editTitle", { name: member.display_name })}
      </Heading>
      <form
        id={formId}
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          if (ready && !save.isPending)
            save.mutate({
              name,
              email,
              role: roleChanged ? role : "",
              teams: teamsChanged ? teamIds : undefined,
              updatedAt: baseline.updated_at ?? "",
              memberId: member.id,
            });
        }}
      >
        {baseline.status === "invited" && <p>{t("users.invitedSetup")}</p>}
        {baseline.status === "deactivated" && <p>{t("users.editInactive")}</p>}
        <Field label={t("users.nameLabel")} required>
          {(control) => (
            <TextInput
              {...control}
              required
              value={name}
              maxLength={255}
              onChange={(event) => setName(event.target.value)}
            />
          )}
        </Field>
        <Field label={t("users.emailLabel")} required>
          {(control) => (
            <TextInput
              {...control}
              type="email"
              disabled={!baseline.email_editable}
              required
              value={email}
              onChange={(event) => setEmail(event.target.value)}
            />
          )}
        </Field>
        <MemberEmailNote editable={!!baseline.email_editable} />
        {email.trim() !== baseline.email && (
          <p>{t("users.emailChangeEndsSessions")}</p>
        )}
        <QueryStates query={roles} pendingLabel={t("users.roleLabel")}>
          {null}
        </QueryStates>
        <Field label={t("users.roleLabel")}>
          {(control) => (
            <Select
              {...control}
              value={role}
              onChange={setRole}
              disabled={!roles.isSuccess || !offers(member, "change_role")}
              options={roleOptions(t, roles.data ?? [])}
              placeholder={t("users.keepRoles")}
            />
          )}
        </Field>
        <MemberTeamsField
          value={teamIds}
          onChange={setTeamIds}
          canManage={managesTeams}
          joinable={inactiveJoinable(baseline)}
        />
        {!managesTeams && <p>{t("users.teamAdminOnly")}</p>}
        <MergedAccounts member={baseline} />
        {save.error && <ErrorLine error={save.error} />}
        {problemCode(save.error) === "version_skew" && (
          <Button
            type="button"
            disabled={reload.isPending}
            onClick={() => reload.mutate(member.id)}
          >
            {t("users.reloadMember")}
          </Button>
        )}
        {reload.error && <ErrorLine error={reload.error} />}
      </form>
      <div className="actions">
        <Button type="button" variant="ghost" onClick={onClose}>
          {t("users.editCancel")}
        </Button>
        <Button
          type="submit"
          form={formId}
          variant="primary"
          disabled={
            !ready ||
            save.isPending ||
            reload.isPending ||
            !name.trim() ||
            !email.trim()
          }
        >
          {t("users.editSave")}
        </Button>
      </div>
    </Modal>
  );
}

function memberRole(member: User): string {
  return member.roles?.length === 1 ? (member.roles[0] ?? "") : "";
}

// A deactivated or suspended member may leave a team but join no new one.
function inactiveJoinable(member: User) {
  const inactive =
    member.status === "deactivated" || member.status === "suspended";
  const current = member.team_ids ?? [];
  return inactive ? (teamId: string) => current.includes(teamId) : undefined;
}

function MergedAccounts({ member }: Readonly<{ member: User }>) {
  const t = useT();
  return member.merged_from?.length ? (
    <div>
      <p>{t("users.mergedAccounts")}</p>
      <ul>
        {member.merged_from.map((account) => (
          <li key={account.id}>{account.display_name}</li>
        ))}
      </ul>
    </div>
  ) : null;
}

function memberTeams(member: User): string[] {
  return member.team_ids ?? [];
}

function MemberEmailNote({ editable }: Readonly<{ editable: boolean }>) {
  const t = useT();
  return editable ? null : <p>{t("users.retirement.protected_email")}</p>;
}
