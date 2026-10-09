// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { UserMinus } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import {
  Avatar,
  Button,
  Field,
  Modal,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ComboBox } from "../design-system/combobox";
import { Heading } from "../design-system/heading";
import { IconAction } from "../design-system/iconaction";
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
  const [draft, setDraft] = useState(team.name);
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
  const name = draft.trim();
  return (
    <>
      <Button
        onClick={() => {
          setDraft(team.name);
          rename.reset();
          setOpen(true);
        }}
      >
        {t("users.teamRename")}
      </Button>
      <TeamNameDialog
        open={open}
        onClose={() => setOpen(false)}
        title={t("users.teamRenameTitle")}
        submitLabel={t("users.teamRenameSave")}
        refusedTitle={t("users.notRenamed")}
        draft={draft}
        onDraft={setDraft}
        ready={name !== "" && name !== team.name && !rename.isPending}
        error={rename.isError ? rename.error : null}
        pending={rename.isPending}
        onSubmit={() => rename.mutate({ id: team.id, name })}
      />
    </>
  );
}

// The one dialog a team's name is typed into, new or renamed. It closes only on
// the write that landed, so a refusal keeps the name where it was typed.
function TeamNameDialog({
  open,
  onClose,
  title,
  submitLabel,
  refusedTitle,
  draft,
  onDraft,
  ready,
  error,
  pending,
  onSubmit,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  title: string;
  submitLabel: string;
  refusedTitle: string;
  draft: string;
  onDraft: (name: string) => void;
  ready: boolean;
  error: Error | null;
  pending: boolean;
  onSubmit: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  const formId = useId();
  return (
    <Modal open={open} onClose={onClose} labelledBy={titleId} intent="form">
      <Heading size="large" className="t-h3 modal-title" id={titleId}>
        {title}
      </Heading>
      <form
        id={formId}
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          if (ready) onSubmit();
        }}
      >
        <Field label={t("users.teamNameLabel")} required>
          {(control) => (
            <TextInput
              {...control}
              value={draft}
              placeholder={t("users.newTeamPlaceholder")}
              disabled={pending}
              onChange={(event) => onDraft(event.target.value)}
            />
          )}
        </Field>
        {error && (
          <Callout tone="danger" kind="outcome" title={refusedTitle}>
            {problemMessageOf(error, t)}
          </Callout>
        )}
      </form>
      <div className="actions">
        <Button type="submit" form={formId} variant="primary" disabled={!ready}>
          {submitLabel}
        </Button>
      </div>
    </Modal>
  );
}

// Creating a team is the card's verb, not a row's: it sits in the title band.
export function NewTeamAction() {
  const t = useT();
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState("");
  const create = useMutation({
    mutationFn: async (name: string) => {
      return unwrap(await api.POST("/teams", { body: { name } }));
    },
    onSuccess: () => {
      setDraft("");
      setOpen(false);
      qc.invalidateQueries({ queryKey: ["teams"] });
    },
  });
  const name = draft.trim();
  return (
    <>
      {/* Named for what it opens; the dialog's submit reads "Create team". */}
      <Button onClick={() => setOpen(true)}>{t("users.newTeamOpen")}</Button>
      <TeamNameDialog
        open={open}
        onClose={() => setOpen(false)}
        title={t("users.newTeamLabel")}
        submitLabel={t("users.createTeam")}
        refusedTitle={t("users.notCreated")}
        draft={draft}
        onDraft={setDraft}
        ready={name !== "" && !create.isPending}
        error={create.isError ? create.error : null}
        pending={create.isPending}
        onSubmit={() => create.mutate(name)}
      />
    </>
  );
}
