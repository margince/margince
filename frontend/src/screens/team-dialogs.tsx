// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { UserMinus } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Avatar, Button, Field, Modal } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ComboBox } from "../design-system/combobox";
import { Heading } from "../design-system/heading";
import { IconAction } from "../design-system/iconaction";
import { NameDialog, nameRefusal } from "../design-system/namedialog";
import { useToast } from "../design-system/toast";
import { forReader } from "../format/collate";
import { useLocale, useT } from "../i18n";
import type { Locale } from "../i18n/locale";
import { problemMessageOf, unwrap } from "./common";
import { RosterPartialNote } from "./roster";
import "./users-access.css";

export type TeamUser = components["schemas"]["User"];
type Team = components["schemas"]["Team"];

/** The users of a team, off the roster's `team_ids`, in the reader's order. */
export function membersOf(
  users: readonly TeamUser[],
  teamId: string,
  locale: Locale,
): TeamUser[] {
  return users
    .filter((user) => (user.team_ids ?? []).includes(teamId))
    .sort((a, b) => forReader(a.display_name, b.display_name, locale));
}

// Who is on one team, and the two writes that change it. The writes are the
// admin's alone, so `canEdit` false leaves the list and drops the verbs.
export function TeamMembersModal({
  team,
  users,
  usersPartial,
  canEdit,
  open,
  onClose,
}: Readonly<{
  team: Team;
  users: readonly TeamUser[];
  usersPartial: boolean;
  canEdit: boolean;
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const qc = useQueryClient();
  const titleId = useId();
  const list = useRef<HTMLUListElement | null>(null);
  // Where focus goes once the list changes under it: the row now in a removed
  // row's place. It is the title (-1) when the control it was on is gone.
  const refocusAt = useRef<number | null>(null);
  const [draft, setDraft] = useState("");
  const setMember = useMutation({
    mutationFn: async ({
      teamId,
      userId,
      member,
    }: {
      teamId: string;
      userId: string;
      member: boolean;
    }) => {
      const params = { params: { path: { id: teamId, userId } } };
      unwrap(
        member
          ? await api.PUT("/teams/{id}/members/{userId}", params)
          : await api.DELETE("/teams/{id}/members/{userId}", params),
      );
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["users"] });
      qc.invalidateQueries({ queryKey: ["teams"] });
    },
    onError: () => {
      refocusAt.current = null;
    },
  });
  const members = membersOf(users, team.id, locale);
  // SetTeamMember refuses an agent seat and a seat that is not active.
  const candidates = users
    .filter(
      (user) =>
        !(user.team_ids ?? []).includes(team.id) &&
        !user.is_agent &&
        user.status === "active",
    )
    .sort((a, b) => forReader(a.display_name, b.display_name, locale));

  useEffect(() => {
    const at = refocusAt.current;
    if (at === null) {
      return;
    }
    refocusAt.current = null;
    const verbs = list.current?.querySelectorAll("button") ?? [];
    const next = at < 0 ? undefined : verbs[Math.min(at, members.length - 1)];
    (next ?? document.getElementById(titleId))?.focus();
  }, [members.length, titleId]);

  const pick = (value: string) => {
    const user = candidates.find((candidate) => candidate.id === value);
    if (!user) {
      setDraft(value);
      return;
    }
    // The field stays enabled to keep focus, so this holds one write at a time.
    if (setMember.isPending) {
      return;
    }
    setDraft("");
    // The last one picked takes the field away with it.
    refocusAt.current = candidates.length === 1 ? -1 : null;
    setMember.mutate({ teamId: team.id, userId: user.id, member: true });
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      labelledBy={titleId}
      intent="form"
      // Not the first row's verb: Enter there would take somebody off the team.
      initialFocusTo={() => document.getElementById(titleId)}
    >
      <Heading size="large" className="modal-title" id={titleId} tabIndex={-1}>
        {team.name}
      </Heading>
      <div className="form-stack">
        {members.length === 0 ? (
          <p>{t("users.teamNoMembers")}</p>
        ) : (
          <ul
            ref={list}
            className="users-team-roster"
            aria-label={t("users.teamMembersLabel")}
          >
            {members.map((member, index) => (
              <li key={member.id} className="users-team-member">
                <Avatar name={member.display_name} identity={member.id} />
                <span className="users-team-member-name">
                  <span>{member.display_name}</span>
                  <span className="t-caption">{member.email}</span>
                </span>
                {canEdit ? (
                  <IconAction
                    icon={<UserMinus aria-hidden />}
                    label={t("users.teamRemoveMember", {
                      name: member.display_name,
                      team: team.name,
                    })}
                    pending={
                      setMember.isPending &&
                      setMember.variables?.userId === member.id
                    }
                    // The pressed verb stays enabled so it keeps focus and shows
                    // its wait; only the other rows' verbs hold.
                    disabled={
                      setMember.isPending &&
                      setMember.variables?.userId !== member.id
                    }
                    onClick={() => {
                      refocusAt.current = index;
                      setMember.mutate({
                        teamId: team.id,
                        userId: member.id,
                        member: false,
                      });
                    }}
                  />
                ) : null}
              </li>
            ))}
          </ul>
        )}
        {canEdit ? (
          candidates.length === 0 ? (
            <p className="t-caption">{t("users.teamNobodyToAdd")}</p>
          ) : (
            <Field label={t("users.teamAddMember")}>
              {(control) => (
                <ComboBox
                  {...control}
                  value={draft}
                  onChange={pick}
                  placeholder={t("users.teamAddPlaceholder")}
                  suggestions={candidates.map((user) => ({
                    value: user.id,
                    label: user.display_name,
                    hint: user.email,
                  }))}
                />
              )}
            </Field>
          )
        ) : (
          <p className="t-caption">{t("users.teamMembersReadOnly")}</p>
        )}
        {setMember.isError && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("users.teamNotChanged")}
          >
            {problemMessageOf(setMember.error, t)}
          </Callout>
        )}
        <RosterPartialNote partial={usersPartial} />
      </div>
      <div className="actions">
        <Button variant="primary" onClick={onClose}>
          {t("users.teamMembersDone")}
        </Button>
      </div>
    </Modal>
  );
}

export function RenameTeamAction({ team }: Readonly<{ team: Team }>) {
  const t = useT();
  const toast = useToast();
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const rename = useMutation({
    mutationFn: async ({ id, name }: { id: string; name: string }) => {
      unwrap(
        await api.PATCH("/teams/{id}", {
          params: { path: { id } },
          body: { name },
        }),
      );
    },
    onSuccess: (_renamed, { name }) => {
      setOpen(false);
      qc.invalidateQueries({ queryKey: ["teams"] });
      toast.show(t("users.teamRenamed", { name }));
    },
  });
  const renameRefused = nameRefusal(rename.error, t, "users.teamDuplicate");
  return (
    <>
      <Button
        onClick={() => {
          rename.reset();
          setOpen(true);
        }}
      >
        {t("users.teamRename")}
      </Button>
      <NameDialog
        open={open}
        onClose={() => setOpen(false)}
        title={t("users.teamRenameTitle")}
        label={t("users.teamNameLabel")}
        initial={team.name}
        placeholder={t("users.newTeamPlaceholder")}
        confirmLabel={t("users.teamRenameSave")}
        pending={rename.isPending}
        problem={renameRefused.problem}
        nameProblem={renameRefused.nameProblem}
        onSave={(name) => rename.mutate({ id: team.id, name })}
      />
    </>
  );
}

// Creating a team is the card's verb, not a row's: it sits in the title band.
export function NewTeamAction() {
  const t = useT();
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const create = useMutation({
    mutationFn: async (name: string) => {
      return unwrap(await api.POST("/teams", { body: { name } }));
    },
    onSuccess: () => {
      setOpen(false);
      qc.invalidateQueries({ queryKey: ["teams"] });
    },
  });
  const createRefused = nameRefusal(create.error, t, "users.teamDuplicate");
  return (
    <>
      {/* Named for what it opens; the dialog's submit reads "Create team". */}
      <Button
        onClick={() => {
          create.reset();
          setOpen(true);
        }}
      >
        {t("users.newTeamOpen")}
      </Button>
      <NameDialog
        open={open}
        onClose={() => setOpen(false)}
        title={t("users.newTeamLabel")}
        label={t("users.teamNameLabel")}
        placeholder={t("users.newTeamPlaceholder")}
        confirmLabel={t("users.createTeam")}
        pending={create.isPending}
        problem={createRefused.problem}
        nameProblem={createRefused.nameProblem}
        onSave={(name) => create.mutate(name)}
      />
    </>
  );
}
