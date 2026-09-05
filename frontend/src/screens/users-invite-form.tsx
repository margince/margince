// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { UserPlus } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { isOption } from "../app/options";
import { Button, Checkbox, Field, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Select, type SelectOption } from "../design-system/select";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, throwProblem } from "./common";
import { RosterPartialNote, useRoster, useRosterPartial } from "./entityref";
import { AccessPreviewPanel } from "./users-access";
import "./users-admin.css";

// The invite form: an address, a name, a role and the teams the member lands
// in, committed together. Two surfaces ask it — the roster's dialog in
// Settings → People and the setup journey's team step — and it is one form so
// the two cannot come to invite differently. What happens after the write
// (closing a dialog, minting a set-password link, moving the journey on) is
// the caller's, through `onInvited`.

export type Role = components["schemas"]["ChangeUserRoleRequest"]["role"];

// Wire keys, not product names: `manager` shows as Team Lead, `rep` as User
// (ADR-0110). The catalog carries the display names.
export const ROLES: readonly Role[] = [
  "admin",
  "management",
  "manager",
  "rep",
  "read_only",
  "ops",
];

// The catalog key each wire key reads under. `role.*` is the ONE role catalog —
// this screen used to carry a second, `users.role.*`, whose English was
// identical and therefore drifted silently the moment either was edited. The
// map is written out rather than templated because one key is not its wire key:
// `read_only` reads under `role.readOnly`, and a template would compile to a key
// the catalog does not answer.
const ROLE_LABEL_KEY = {
  admin: "role.admin",
  management: "role.management",
  manager: "role.manager",
  rep: "role.rep",
  read_only: "role.readOnly",
  ops: "role.ops",
} as const satisfies Record<Role, MessageKey>;

// roleLabel names a held role key. The catalog covers the six system roles;
// a workspace-defined key has no translation, so it reads as itself rather
// than as a missing-translation marker — the admin still learns what is held.
export const roleLabel = (t: ReturnType<typeof useT>) => (key: string) =>
  isOption(key, ROLES) ? t(ROLE_LABEL_KEY[key]) : key;

// The six system roles as pickable options — shared by the invite form and
// every roster row so the two lists cannot drift apart.
export const roleOptions = (t: ReturnType<typeof useT>): SelectOption[] =>
  ROLES.map((role) => ({ value: role, label: t(ROLE_LABEL_KEY[role]) }));

export type InvitedMember = Readonly<{ id: string; name: string }>;

/**
 * The name a member is created under when the form asks for none: the part
 * of the address before the @, with the dots and underscores people write
 * between their names read as spaces and each word capitalised. It is a
 * starting point the admin can change in Settings → People, and it is what
 * the roster shows until they do — so it has to read as a name, not as an
 * address fragment.
 */
export function nameFromEmail(email: string): string {
  const local = email.trim().split("@")[0] ?? "";
  return local
    .split(/[._-]+/)
    .filter((part) => part !== "")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

export function InviteUserForm({
  titleId,
  askName = true,
  onInvited,
}: Readonly<{
  /**
   * The heading's id, for a dialog to name itself by. Absent, the form draws
   * no heading of its own: a surface whose title already says "invite" does
   * not want it said twice.
   */
  titleId?: string;
  /**
   * Whether the form asks for the member's name. The setup journey does not:
   * one address is enough to get a first person in, and the name is derived
   * from it (`nameFromEmail`) until somebody changes it.
   */
  askName?: boolean;
  onInvited: (member: InvitedMember) => void;
}>) {
  const t = useT();
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [role, setRole] = useState<Role>("rep");
  const [teamIds, setTeamIds] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);
  const teams = useRoster("team", true);
  const teamsPartial = useRosterPartial("team", true);

  // The team set rides as the mutation's variable rather than through the
  // closure: react-query re-arms a mutation's options in a passive effect,
  // so a click in that window would otherwise invite with the PREVIOUS
  // selection — granting or omitting authority the admin did not choose.
  const displayName = askName ? name.trim() : nameFromEmail(email);
  const invite = useMutation({
    mutationFn: async (teams: string[]): Promise<string> => {
      const { data, error: err } = await api.POST("/users", {
        body: {
          email: email.trim(),
          display_name: displayName,
          role,
          team_ids: teams,
        },
      });
      if (err) {
        throwProblem(err);
      }
      return data.id;
    },
    onSuccess: (newUserId) => {
      const invitedName = displayName;
      setEmail("");
      setName("");
      setRole("rep");
      setTeamIds([]);
      setError(null);
      onInvited({ id: newUserId, name: invitedName });
    },
    onError: (e: Error) => setError(problemMessageOf(e, t)),
  });

  const canInvite =
    email.trim().length > 0 && displayName.length > 0 && !invite.isPending;

  return (
    // A real <form>, so Enter submits it — and the house dialog stack, so the
    // fields sit on the same rhythm as every other settings dialog.
    <form
      className="form-stack"
      onSubmit={(e) => {
        e.preventDefault();
        if (canInvite) {
          invite.mutate(teamIds);
        }
      }}
    >
      {titleId !== undefined && (
        <>
          <h2 className="t-h3 modal-title" id={titleId}>
            {t("users.inviteTitle")}
          </h2>
          <p className="t-caption">{t("users.inviteSub")}</p>
        </>
      )}
      <Field label={t("users.emailLabel")} required>
        {(control) => (
          <TextInput
            {...control}
            placeholder={t("users.emailPlaceholder")}
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
        )}
      </Field>
      {askName && (
        <Field label={t("users.nameLabel")} required>
          {(control) => (
            <TextInput
              {...control}
              placeholder={t("users.namePlaceholder")}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          )}
        </Field>
      )}
      <Field label={t("users.roleLabel")}>
        {(control) => (
          <Select
            {...control}
            value={role}
            onChange={(value) => {
              if (isOption(value, ROLES)) setRole(value);
            }}
            options={roleOptions(t)}
          />
        )}
      </Field>
      {/* The teams the member joins on arrival. A team-scoped role with no
          team edits only its own records, and the preview below says so
          before the invite goes out. */}
      <fieldset className="users-invite-teams">
        <legend className="t-caption">{t("users.teamsLabel")}</legend>
        {(teams.data ?? []).flatMap((entry) =>
          "name" in entry ? (
            <Checkbox
              key={entry.id}
              className="t-body"
              label={entry.name}
              checked={teamIds.includes(entry.id)}
              onChange={(event) =>
                setTeamIds((current) =>
                  event.target.checked
                    ? [...current, entry.id]
                    : current.filter((id) => id !== entry.id),
                )
              }
            />
          ) : (
            []
          ),
        )}
        {/* "No teams yet" is a claim about the workspace, so only a roster
            read to its end may make it: a walk that stopped early would have
            an admin invite people into no team at all on the strength of
            pages nothing read. */}
        {teams.data?.length === 0 && !teamsPartial && (
          <p className="t-caption">{t("users.noTeamsYet")}</p>
        )}
        <RosterPartialNote partial={teamsPartial} />
      </fieldset>
      <AccessPreviewPanel role={role} teamIds={teamIds} />
      {/* `.form-actions` rather than a bare button: `.form-stack` stretches
          its children, and a submit that fills the dialog reads as a banner
          rather than as the move the form is for. */}
      <div className="form-actions">
        <Button variant="primary" small type="submit" disabled={!canInvite}>
          <UserPlus aria-hidden /> {t("users.invite")}
        </Button>
      </div>
      {/* A refused invite is the surface saying something is wrong, which is
          what Callout's `danger` tone is. `alert` because the reader pressed
          the button and has to act on the answer. */}
      {error && (
        <Callout tone="danger" live="alert">
          {error}
        </Callout>
      )}
    </form>
  );
}
