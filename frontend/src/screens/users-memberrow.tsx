// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Avatar, Badge, Button, OverflowMenu } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { CellStack } from "../design-system/cellstack";
import { ConfirmModal } from "../design-system/confirmmodal";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import type { AssignableRole } from "./roles.queries";
import type { Role } from "./users-invite-form";
import { PasswordLinkModal, usePasswordLink } from "./users-password-link";
import { offersRoleChange, roleReading } from "./users-rolecell";
import { ChangeRoleDialog } from "./users-rolechange";

type User = components["schemas"]["User"];

type MemberAction = NonNullable<User["allowed_actions"]>[number];

// Whether the server offers this reader `action` on `member`. The roster
// computes it with the checks the verb itself runs (the grant, the seat, the
// ceiling over the member, their status, the last admin, the deployment's
// link posture), so a row offers what the write accepts. An absent list is a
// roster that computed none, and offers nothing.
function offers(member: User, action: MemberAction): boolean {
  return member.allowed_actions?.includes(action) ?? false;
}

// The verbs a member's row offers, behind the one control a row spends on them,
// at the same x on every row. Nothing to offer draws nothing: a trigger over an
// empty panel is a promise the row cannot keep.
function MemberVerbs({
  member,
  pending,
  canChangeRole,
  canMintLink,
  canDeactivate,
  canReactivate,
  onChangeRole,
  onMintLink,
  onDeactivate,
  onReactivate,
}: Readonly<{
  member: User;
  pending: boolean;
  canChangeRole: boolean;
  canMintLink: boolean;
  canDeactivate: boolean;
  canReactivate: boolean;
  onChangeRole: () => void;
  onMintLink: () => void;
  onDeactivate: () => void;
  onReactivate: () => void;
}>) {
  const t = useT();
  if (!(canChangeRole || canMintLink || canDeactivate || canReactivate)) {
    return null;
  }
  return (
    <OverflowMenu label={t("users.rowActions", { name: member.display_name })}>
      {canChangeRole && (
        <Button disabled={pending} onClick={onChangeRole}>
          {t("users.roster.changeRole")}
        </Button>
      )}
      {canMintLink && (
        <Button disabled={pending} onClick={onMintLink}>
          {t("users.link.action")}
        </Button>
      )}
      {canDeactivate && (
        <Button variant="danger" disabled={pending} onClick={onDeactivate}>
          {t("users.deactivate")}
        </Button>
      )}
      {canReactivate && (
        <Button disabled={pending} onClick={onReactivate}>
          {t("users.reactivate")}
        </Button>
      )}
    </OverflowMenu>
  );
}

// statusTone maps a member's status onto the badge vocabulary.
//
// Invited is deliberately NEUTRAL rather than a warning. An invitation in
// flight is the ordinary first state of every seat, and painting a whole
// column of new colleagues amber tells an admin something is wrong when
// nothing is. Warn is kept for the states somebody has to act on.
function statusTone(
  status: string,
): "success" | "warning" | "danger" | undefined {
  switch (status) {
    case "active":
      return "success";
    case "invited":
      return undefined;
    default:
      return "warning";
  }
}

// One member: the table row, and a second row under it for a refused write, in
// a <tbody> of their own so the refusal stays with the member it is about.
export function MemberRow({
  member,
  roles,
  isMe,
  teamNames,
  canChangeRole,
}: Readonly<{
  member: User;
  roles: readonly AssignableRole[];
  isMe: boolean;
  // Null where the reader gets no team or role data and the columns are not drawn.
  teamNames: ReadonlyMap<string, string> | null;
  canChangeRole: boolean;
}>) {
  const t = useT();
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [confirmOff, setConfirmOff] = useState(false);
  const [changingRole, setChangingRole] = useState(false);
  const [linkOpen, setLinkOpen] = useState(false);
  // Where the dialogs hand focus back. The menu item a dialog was opened from
  // is gone by then (Deactivate becomes Reactivate), but the member's own cell
  // stays, because the roster is read with include_inactive.
  const anchor = useRef<HTMLDivElement | null>(null);
  const passwordLink = usePasswordLink();
  const openLink = () => {
    setLinkOpen(true);
    void passwordLink.mint(member.id);
  };
  // Returns the refetch so each onSuccess can hand it back to react-query,
  // which then keeps the mutation pending until the new roster lands. Without
  // that the mutation settles first and the row renders the pre-change roster
  // it still has cached: the member's OLD role, briefly, right after a
  // successful change.
  const refresh = () => {
    setError(null);
    return Promise.all([
      qc.invalidateQueries({ queryKey: ["users-admin"] }),
      // The seat COUNT moves with the roster: deactivating a member frees a
      // seat and reactivating one spends it, and Settings → Seats reads that
      // number from its own endpoint with a five-minute staleTime.
      qc.invalidateQueries({ queryKey: ["installation-seat-usage"] }),
      qc.invalidateQueries({ queryKey: ["installation-license"] }),
    ]);
  };
  const onError = (e: Error) => setError(problemMessageOf(e, t));
  const toast = useToast();

  const setRole = useMutation({
    mutationFn: async (role: Role) => {
      const { error: err } = await api.PATCH("/users/{id}/role", {
        params: { path: { id: member.id } },
        body: { role },
      });
      if (err) {
        throwProblem(err);
      }
    },
    onSuccess: async () => {
      await refresh();
      setChangingRole(false);
      // No Undo: reversing it means knowing which role they held before, and
      // the roster this row redraws from has already been replaced. The dialog
      // says so before the change instead.
      toast.show(t("users.roleSaved", { name: member.email }));
    },
    onError,
  });

  const deactivate = useMutation({
    mutationFn: async () => {
      const { error: err } = await api.POST("/users/{id}/deactivate", {
        params: { path: { id: member.id } },
      });
      if (err) {
        throwProblem(err);
      }
    },
    onSuccess: async () => {
      // The refreshed roster FIRST, then the dialog: closing it hands focus back
      // to the row, and a row still showing "Active" would announce the state
      // this confirm just ended.
      await refresh();
      setConfirmOff(false);
      // A true inverse, and both halves are already on this row: `reactivate`
      // restores exactly what `deactivate` took away, with nothing to re-supply.
      toast.show(t("users.deactivated", { name: member.email }), {
        action: { label: t("common.undo"), onAct: () => reactivate.mutate() },
      });
    },
    onError,
  });

  const reactivate = useMutation({
    mutationFn: async () => {
      const { error: err } = await api.POST("/users/{id}/reactivate", {
        params: { path: { id: member.id } },
      });
      if (err) {
        throwProblem(err);
      }
    },
    onSuccess: async () => {
      await refresh();
      toast.show(t("users.reactivated", { name: member.email }));
    },
    onError,
  });

  const pending =
    setRole.isPending || deactivate.isPending || reactivate.isPending;
  const offersChange = offersRoleChange(
    member,
    roles,
    offers(member, "change_role"),
  );
  const reading = roleReading(member, roles, canChangeRole && !offersChange, t);
  const teamIds = member.team_ids ?? [];

  return (
    <tbody data-testid={`member-${member.id}`}>
      <tr>
        <td>
          <div className="users-roster-member" ref={anchor} tabIndex={-1}>
            <Avatar name={member.display_name} identity={member.id} />
            <CellStack>
              <span className="t-name">
                {member.display_name}
                {isMe && (
                  <span className="t-caption">
                    {` ${t("users.roster.you")}`}
                  </span>
                )}
              </span>
              <span className="t-caption">{member.email}</span>
            </CellStack>
          </div>
        </td>
        {teamNames && (
          <>
            <td className="users-roster-role">
              <CellStack>
                <span>{reading.role}</span>
                {reading.caption && (
                  <span className="t-caption">{reading.caption}</span>
                )}
              </CellStack>
            </td>
            <td className="users-roster-teamcell">
              {teamIds.length === 0 ? (
                <span className="t-caption">{t("users.roster.noTeam")}</span>
              ) : (
                <div className="users-roster-teams">
                  {/* A team the walk has not named (yet) draws nothing rather
                      than a glyph a screen reader would read out. */}
                  {teamIds.map((id) => {
                    const name = teamNames.get(id);
                    return name === undefined ? null : (
                      <Badge key={id}>{name}</Badge>
                    );
                  })}
                </div>
              )}
            </td>
          </>
        )}
        <td className="users-roster-statuscell">
          <div className="users-roster-status">
            <Badge tone={statusTone(member.status)}>
              {t(`users.status.${member.status}`)}
            </Badge>
            {/* The workspace's agent identity sits in this roster because it
                OWNS records, so the row says what it is rather than passing
                for a colleague. */}
            {member.is_agent && <Badge tone="ai">{t("users.agentSeat")}</Badge>}
          </div>
        </td>
        <td className="users-roster-actions">
          <MemberVerbs
            member={member}
            pending={pending}
            canChangeRole={offersChange}
            canMintLink={offers(member, "issue_password_link")}
            canDeactivate={offers(member, "deactivate")}
            canReactivate={offers(member, "reactivate")}
            onChangeRole={() => {
              setRole.reset();
              setChangingRole(true);
            }}
            onMintLink={openLink}
            onDeactivate={() => setConfirmOff(true)}
            onReactivate={() => reactivate.mutate()}
          />
        </td>
      </tr>
      {error && (
        <tr className="users-roster-error-row">
          <td colSpan={teamNames ? 5 : 3} className="users-roster-error">
            <Callout tone="danger" kind="outcome" title={t("users.notSaved")}>
              {error}
            </Callout>
          </td>
        </tr>
      )}
      {changingRole && (
        <ChangeRoleDialog
          member={member}
          roles={roles}
          open={changingRole}
          pending={setRole.isPending}
          error={setRole.error ? problemMessageOf(setRole.error, t) : null}
          onClose={() => setChangingRole(false)}
          onConfirm={(role) => setRole.mutate(role)}
          returnFocusTo={() => anchor.current}
        />
      )}
      <ConfirmModal
        open={confirmOff}
        onClose={() => setConfirmOff(false)}
        title={t("users.deactivateConfirmTitle", { name: member.display_name })}
        confirmLabel={t("users.deactivate")}
        confirmVariant="danger"
        pending={deactivate.isPending}
        error={deactivate.error ? problemMessageOf(deactivate.error, t) : null}
        onConfirm={() => deactivate.mutate()}
        returnFocusTo={() => anchor.current}
      >
        {/* Deactivating the agent seat is a posture an operator is entitled to
            take, so it stays offered, and its body says what does NOT stop:
            scheduled extension jobs keep running. */}
        <p>
          {t(
            member.is_agent
              ? "users.deactivateAgentConfirmBody"
              : "users.deactivateConfirmBody",
          )}
        </p>
      </ConfirmModal>
      {linkOpen && (
        <PasswordLinkModal
          memberName={member.display_name}
          link={passwordLink.state.link}
          pending={passwordLink.state.pending}
          error={passwordLink.state.error}
          onRetry={openLink}
          onClose={() => {
            // Drop the credential with the dialog, never merely hide it.
            passwordLink.clear();
            setLinkOpen(false);
          }}
        />
      )}
    </tbody>
  );
}
