// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { UserPlus } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useHoldsAdminRole } from "../app/capability";
import { Button, Checkbox, Field, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Heading } from "../design-system/heading";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import { RosterPartialNote, useRoster, useRosterPartial } from "./entityref";
import { roleOptions, useAssignableRoles } from "./roles.queries";
import { AccessPreviewPanel } from "./users-access";
import "./users-admin.css";

// The invite form: an address, a name, a role and the teams the member lands
// in, committed together. Two surfaces ask it — the roster's dialog in
// Settings → Contacts and the setup journey's team step — and it is one form so
// the two cannot come to invite differently. What happens after the write
// (closing a dialog, minting a set-password link, moving the journey on) is
// the caller's, through `onInvited`.

export type Role = components["schemas"]["ChangeUserRoleRequest"]["role"];

export type InvitedMember = Readonly<{ id: string; name: string }>;

/**
 * The name a member is created under when the form asks for none: the part
 * of the address before the @, with the dots and underscores contacts write
 * between their names read as spaces and each word capitalised. It is a
 * starting point the admin can change in Settings → Contacts, and it is what
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
   * one address is enough to get a first contact in, and the name is derived
   * from it (`nameFromEmail`) until somebody changes it.
   */
  askName?: boolean;
  onInvited: (member: InvitedMember) => void;
}>) {
  const t = useT();
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [greeting, setGreeting] = useState("");
  const assignable = useAssignableRoles(true);
  const offered = assignable.data ?? [];
  // The ordinary seat most colleagues hold, as the starting choice. A member
  // administrator whose own access does not cover it is not offered it, and
  // picks a role they are offered before the form sends anything.
  const [role, setRole] = useState<Role>("rep");
  const offersRole = offered.some((one) => one.key === role);
  const [teamIds, setTeamIds] = useState<string[]>([]);
  // Only an admin puts a member on a team (identity/teams.go), so only an
  // admin is asked which teams the new member joins.
  const placesOnTeams = useHoldsAdminRole();
  const [error, setError] = useState<string | null>(null);
  const teams = useRoster("team", placesOnTeams);
  const teamsPartial = useRosterPartial("team", placesOnTeams);

  // The role and the team set ride as the mutation's variable rather than
  // through the closure: react-query re-arms a mutation's options in a passive effect,
  // so a click in that window would otherwise invite with the PREVIOUS
  // selection — granting or omitting authority the admin did not choose.
  const displayName = askName ? name.trim() : nameFromEmail(email);
  // Asked only beside the name: the setup journey derives the name from the
  // address, and the member's first sign-in can fill this instead.
  const greetingName = askName ? greeting.trim() : "";
  const invite = useMutation({
    mutationFn: async (
      choice: Readonly<{ role: Role; teams: string[] }>,
    ): Promise<string> => {
      const { data, error: err } = await api.POST("/users", {
        body: {
          email: email.trim(),
          display_name: displayName,
          ...(greetingName === "" ? {} : { greeting_name: greetingName }),
          role: choice.role,
          team_ids: choice.teams,
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
      setGreeting("");
      setRole("rep");
      setTeamIds([]);
      setError(null);
      onInvited({ id: newUserId, name: invitedName });
    },
    onError: (e: Error) => setError(problemMessageOf(e, t)),
  });

  const canInvite =
    email.trim().length > 0 &&
    displayName.length > 0 &&
    offersRole &&
    !invite.isPending;

  return (
    // A real <form>, so Enter submits it — and the house dialog stack, so the
    // fields sit on the same rhythm as every other settings dialog.
    <form
      className="form-stack"
      onSubmit={(e) => {
        e.preventDefault();
        if (canInvite) {
          invite.mutate({ role, teams: placesOnTeams ? teamIds : [] });
        }
      }}
    >
      {titleId !== undefined && (
        <>
          <Heading size="large" className="t-h3" id={titleId}>
            {t("users.inviteTitle")}
          </Heading>
          <p>{t("users.inviteSub")}</p>
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
      {askName && (
        <Field label={t("users.greetingLabel")} hint={t("users.greetingHint")}>
          {(control) => (
            <TextInput
              {...control}
              value={greeting}
              onChange={(e) => setGreeting(e.target.value)}
            />
          )}
        </Field>
      )}
      <Field label={t("users.roleLabel")}>
        {(control) => (
          <Select
            {...control}
            value={offersRole ? role : ""}
            placeholder={t("users.setRole")}
            disabled={assignable.isPending}
            onChange={setRole}
            options={roleOptions(t, offered)}
          />
        )}
      </Field>
      {placesOnTeams && (
        <>
          {/* The teams the member joins on arrival. A team-scoped role with no
              team edits only its own records, and the preview below says so
              before the invite goes out. */}
          <fieldset className="users-invite-teams">
            <legend className="t-name">{t("users.teamsLabel")}</legend>
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
                an admin invite contacts into no team at all on the strength of
                pages nothing read. */}
            {teams.data?.length === 0 && !teamsPartial && (
              <p>{t("users.noTeamsYet")}</p>
            )}
            <RosterPartialNote partial={teamsPartial} />
          </fieldset>
        </>
      )}
      {offersRole && <AccessPreviewPanel role={role} teamIds={teamIds} />}
      {/* ABOVE the submit row, where the sibling dialogs in this family put a
          refusal: under the button it reads as a footnote to the form rather
          than as the answer to the press. */}
      {error && (
        <Callout tone="danger" kind="outcome" title={t("users.inviteFailed")}>
          {error}
        </Callout>
      )}
      {/* `.form-actions` rather than a bare button: `.form-stack` stretches
          its children, and a submit that fills the dialog reads as a banner
          rather than as the move the form is for. */}
      <div className="form-actions">
        <Button variant="primary" type="submit" disabled={!canInvite}>
          <UserPlus aria-hidden /> {t("users.invite")}
        </Button>
      </div>
    </form>
  );
}
