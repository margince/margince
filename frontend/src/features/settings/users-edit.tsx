// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import { useRoster, useRosterPartial } from "../../components/roster";
import { useUnsavedGuard } from "../../components/unsaved";
import { Button, Field, Modal, TextInput } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { QueryStates } from "../../design-system/querystates";
import { useFieldFaults } from "../../design-system/savepage";
import { Select } from "../../design-system/select";
import { useToast } from "../../design-system/toast";
import { useLocale, useT } from "../../i18n";
import { useHoldsAdminRole } from "../../lib/capability";
import { LEGAL_ENTITIES_KEY } from "../../lib/legalentities.queries";
import { isVersionSkewOf, unwrap } from "../../lib/problem";
import { roleOptions, useAssignableRoles } from "../../lib/roles.queries";
import { TEXT_LIMITS } from "../../lib/textlimits_gen";
import { MemberEntityField, useMemberEntityDraft } from "./users-entitycell";
import { memberChecks, memberRefusal } from "./users-invite-form";
import {
  memberMutationKey,
  offers,
  type User,
  useMemberRefresh,
} from "./users-members";
import { useCaptureStopWarning } from "./users-rolecell";
import { MemberTeamsField } from "./users-teams";

// Ids are compared as ids, in code-point order, so the set check does not depend on a locale.
const byId = (a: string, b: string) => Number(a > b) - Number(a < b);

export function EditMember({
  member,
  onClose,
}: Readonly<{ member: User; onClose: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const id = useId();
  const formId = useId();
  const [faults, faultsRoot] = useFieldFaults();
  const labels = {
    display_name: t("users.nameLabel"),
    email: t("users.emailLabel"),
    role: t("users.roleLabel"),
  };
  const refresh = useMemberRefresh();
  const toast = useToast();
  const [baseline, setBaseline] = useState(member);
  const [name, setName] = useState(member.display_name);
  const [email, setEmail] = useState(member.email);
  const [role, setRole] = useState(memberRole(member));
  const [teamIds, setTeamIds] = useState(memberTeams(member));
  const roles = useAssignableRoles(true);
  const stopsCapture = useCaptureStopWarning(baseline, roles.data ?? []);
  const managesTeams = useHoldsAdminRole();
  const teams = useRoster("team", true);
  const partial = useRosterPartial("team", true);
  const entity = useMemberEntityDraft(member);
  const queryClient = useQueryClient();
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
        entity: string | null;
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
            ...(choice.entity === null
              ? {}
              : { legal_entity_id: choice.entity }),
            expected_updated_at: choice.updatedAt,
          },
        }),
      );
    },
    onSuccess: async () => {
      // A move changes which entity lists the member.
      await queryClient.invalidateQueries({ queryKey: LEGAL_ENTITIES_KEY });
      await refresh();
      toast.show(t("users.saved"));
      onClose();
    },
    onError: (error) => faults.show(memberRefusal(error, t, labels).byField),
  });
  const refused = memberRefusal(save.error, t, labels);
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
    [...teamIds].sort(byId).join(";") !==
    [...(baseline.team_ids ?? [])].sort(byId).join(";");
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
      teamsChanged ||
      entity.change !== null,
  );
  return (
    <Modal open onClose={onClose} intent="form" labelledBy={id}>
      <Heading id={id} size="large" className="modal-title">
        {t("users.editTitle", { name: member.display_name })}
      </Heading>
      <form
        id={formId}
        ref={faultsRoot}
        className="form-stack"
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          if (!ready || save.isPending) return;
          const typed = { email, display_name: name };
          if (faults.show(memberChecks(typed, labels, locale))) {
            save.reset();
            return;
          }
          save.mutate({
            name,
            email,
            role: roleChanged ? role : "",
            teams: teamsChanged ? teamIds : undefined,
            updatedAt: baseline.updated_at ?? "",
            memberId: member.id,
            entity: entity.change,
          });
        }}
      >
        {baseline.status === "invited" && <p>{t("users.invitedSetup")}</p>}
        {baseline.status === "deactivated" && <p>{t("users.editInactive")}</p>}
        <Field
          label={labels.display_name}
          required
          error={faults.of("display_name")}
        >
          {(control) => (
            <TextInput
              {...control}
              value={name}
              maxLength={TEXT_LIMITS.label}
              onChange={(event) => {
                setName(event.target.value);
                faults.edited("display_name");
              }}
            />
          )}
        </Field>
        <Field label={labels.email} required error={faults.of("email")}>
          {(control) => (
            <TextInput
              {...control}
              type="email"
              disabled={!baseline.email_editable}
              value={email}
              onChange={(event) => {
                setEmail(event.target.value);
                faults.edited("email");
              }}
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
        <Field
          label={labels.role}
          hint={stopsCapture(role, memberRole(baseline))}
          hintLive
          error={faults.of("role")}
        >
          {(control) => (
            <Select
              {...control}
              value={role}
              onChange={(next) => {
                setRole(next);
                faults.edited("role");
              }}
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
        <MemberEntityField draft={entity} />
        <MergedAccounts member={baseline} />
        {refused.rest !== null && <ErrorLine>{refused.rest}</ErrorLine>}
        {isVersionSkewOf(save.error) && (
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
          disabled={!ready || save.isPending || reload.isPending}
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
