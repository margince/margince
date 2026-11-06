// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { UserMinus } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { RosterPartialNote } from "../../components/roster";
import {
  Avatar,
  Button,
  Field,
  Modal,
  SearchField,
} from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ComboBox } from "../../design-system/combobox";
import { Heading } from "../../design-system/heading";
import { IconAction } from "../../design-system/iconaction";
import { NameDialog, nameRefusal } from "../../design-system/namedialog";
import { useToast } from "../../design-system/toast";
import { forReader } from "../../format/collate";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { Locale } from "../../i18n/locale";
import { ANALYTICS_CONTEXT_KEY } from "../../lib/analyticskeys";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { matchingMembers } from "./users-members";
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
  // Where focus goes once a write has landed and the list reflects it. That is
  // the row now in a removed row's place, or the title (-1) when its control
  // is gone. Pressed holds the press's place until the write succeeds. A
  // search typed meanwhile moves no focus.
  const pressed = useRef<number | null>(null);
  // A ref, not state. The roster refetch after the write is the render that
  // moves focus, so holding the target needs no render of its own.
  const refocus = useRef<{
    at: number;
    userId: string;
    member: boolean;
  } | null>(null);
  const [draft, setDraft] = useState("");
  // A team of a hundred or more is found in by name, not by scrolling.
  const [search, setSearch] = useState("");
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
    onSuccess: (_answer, written) => {
      const at = pressed.current;
      pressed.current = null;
      if (at !== null) {
        refocus.current = {
          at,
          userId: written.userId,
          member: written.member,
        };
      }
      qc.invalidateQueries({ queryKey: ["users"] });
      qc.invalidateQueries({ queryKey: ["teams"] });
      qc.invalidateQueries({ queryKey: ANALYTICS_CONTEXT_KEY });
    },
    onError: () => {
      pressed.current = null;
    },
  });
  const everyMember = membersOf(users, team.id, locale);
  // The rows shown, and the list the refocus below walks: a removal while a
  // search stands lands on the next row the reader can see.
  const members = matchingMembers(everyMember, search);
  // SetTeamMember refuses an agent seat and a seat that is not active.
  const candidates = users
    .filter(
      (user) =>
        !(user.team_ids ?? []).includes(team.id) &&
        !user.is_agent &&
        user.status === "active",
    )
    .sort((a, b) => forReader(a.display_name, b.display_name, locale));

  // Runs once the roster shows the landed write: the removed member is off the
  // team, or the added one is on it. Not on a search, which changes the rows
  // shown but writes nothing.
  useEffect(() => {
    const target = refocus.current;
    if (
      target === null ||
      everyMember.some((member) => member.id === target.userId) !==
        target.member
    ) {
      return;
    }
    refocus.current = null;
    const verbs = list.current?.querySelectorAll("button") ?? [];
    const next =
      target.at < 0 ? undefined : verbs[Math.min(target.at, verbs.length - 1)];
    (next ?? document.getElementById(titleId))?.focus();
  }, [everyMember, titleId]);

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
    pressed.current = candidates.length === 1 ? -1 : null;
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
        {everyMember.length > 0 && (
          <Field
            label={t("teams.searchMembers")}
            hintLive
            hint={
              search
                ? t("users.resultCount", {
                    matching: formatNumber(members.length, locale),
                    // The team's own count, which no roster page cuts short.
                    total: formatNumber(
                      Math.max(team.member_count ?? 0, everyMember.length),
                      locale,
                    ),
                  })
                : undefined
            }
          >
            {(control) => (
              <SearchField
                {...control}
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder={t("users.searchPlaceholder")}
              />
            )}
          </Field>
        )}
        {everyMember.length === 0 ? (
          <p>{t("users.teamNoMembers")}</p>
        ) : members.length === 0 ? (
          <p>{t("users.noSearchResults")}</p>
        ) : (
          <ul
            ref={list}
            className="users-team-roster"
            aria-label={t("users.teamMembersLabel")}
          >
            {members.map((member, index) => (
              <li key={member.id} className="users-team-member">
                <Avatar
                  name={member.display_name}
                  identity={member.id}
                  photo={member.avatar_url}
                />
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
                      pressed.current = index;
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
      qc.invalidateQueries({ queryKey: ANALYTICS_CONTEXT_KEY });
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

/** Creates a team and refreshes every list that names teams. Shared by the
 *  Teams page and the member dialogs' team picker. */
export function useCreateTeam(onCreated?: () => void) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (name: string) =>
      unwrap(await api.POST("/teams", { body: { name } })),
    onSuccess: async () => {
      onCreated?.();
      qc.invalidateQueries({ queryKey: ANALYTICS_CONTEXT_KEY });
      await qc.invalidateQueries({ queryKey: ["teams"] });
    },
  });
}

// Creating a team is the page's verb, not a row's: it sits in the page head.
export function NewTeamAction() {
  const t = useT();
  const [open, setOpen] = useState(false);
  const create = useCreateTeam(() => setOpen(false));
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
