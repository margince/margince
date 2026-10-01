// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Who may find a list: asked the same way wherever a list is made or changed,
// and said back the same way wherever one is shown. The server's rule is
// auth.listSharingPredicate; this file only puts it into the reader's words.

import { Field } from "../design-system/atoms";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useMe } from "./common";
import { rosterMissLabel } from "./entityref";
import type { List } from "./lists.queries";
import { useRosterNames, useRosterPartial } from "./roster";

const SHARINGS = ["private", "team", "workspace"] as const;

const SHARING_LABEL: Record<List["sharing"], MessageKey> = {
  private: "lists.sharing.private",
  team: "lists.sharing.team",
  workspace: "lists.sharing.workspace",
};

/** A list's audience as the form holds it; `teamId` null is the owner's teams. */
export type ListAudience = Readonly<{
  sharing: List["sharing"];
  teamId: string | null;
}>;

/** What a new list starts with: the server's own default, now shown. */
export const DEFAULT_AUDIENCE: ListAudience = { sharing: "team", teamId: null };

// A Select value cannot be null, so the owner's-teams choice needs a spelling
// no team id can collide with.
const OWNER_TEAMS = "owner-teams";

/** A team's name off the roster, or the honest reason there is none yet. */
function useTeamName(): (teamId: string) => string {
  const t = useT();
  const teams = useRosterNames("team", true);
  const partial = useRosterPartial("team", true);
  return (teamId) => {
    const found = teams.data?.find((team) => team.id === teamId);
    if (found && "name" in found) {
      return found.name;
    }
    return rosterMissLabel(teams, partial, t, t("lists.audience.unknownTeam"));
  };
}

/**
 * "Who can find it", and which team when the answer is a team.
 *
 * `ownerIsReader` decides how the owner's-teams choice is worded: a list's
 * owner reads it as their own teams, a steward editing someone else's list
 * does not.
 */
export function ListAudienceFields({
  value,
  onChange,
  ownerIsReader,
}: Readonly<{
  value: ListAudience;
  onChange: (next: ListAudience) => void;
  ownerIsReader: boolean;
}>) {
  const t = useT();
  const me = useMe();
  const teamName = useTeamName();
  const mine = me.data?.teams ?? [];
  // A list already shared with a team the reader is not in keeps that team on
  // offer, so opening the form never silently proposes a different one.
  const teamIds =
    value.teamId !== null && !mine.includes(value.teamId)
      ? [...mine, value.teamId]
      : mine;
  return (
    <>
      <Field label={t("lists.sharingLabel")} hint={t("lists.sharingHint")}>
        {(control) => (
          <Select
            {...control}
            value={value.sharing}
            onChange={(next) =>
              onChange({ ...value, sharing: next as List["sharing"] })
            }
            options={SHARINGS.map((sharing) => ({
              value: sharing,
              label: t(SHARING_LABEL[sharing]),
            }))}
          />
        )}
      </Field>
      {value.sharing === "team" && (
        <Field label={t("lists.teamLabel")}>
          {(control) => (
            <Select
              {...control}
              value={value.teamId ?? OWNER_TEAMS}
              onChange={(next) =>
                onChange({
                  ...value,
                  teamId: next === OWNER_TEAMS ? null : next,
                })
              }
              options={[
                {
                  value: OWNER_TEAMS,
                  label: t(
                    ownerIsReader
                      ? "lists.team.allMine"
                      : "lists.team.allOwners",
                  ),
                },
                ...teamIds.map((teamId) => ({
                  value: teamId,
                  label: teamName(teamId),
                })),
              ]}
            />
          )}
        </Field>
      )}
    </>
  );
}

/**
 * Who can find a list, named: a team by its name, the owner's teams as the
 * reader's own when they are the owner.
 *
 * Returned as a function so a table reads the roster and the session once for
 * all of its rows.
 */
export function useListAudienceLabel(): (
  list: Pick<List, "sharing" | "team_id" | "owner_id">,
) => string {
  const t = useT();
  const me = useMe();
  const teamName = useTeamName();
  return (list) => {
    if (list.sharing !== "team") {
      return t(SHARING_LABEL[list.sharing]);
    }
    if (list.team_id) {
      return teamName(list.team_id);
    }
    // A team list that names no team and has lost its owner is findable by
    // every seat, so nobody is left unable to take it over.
    if (!list.owner_id) {
      return t("lists.sharing.workspace");
    }
    return list.owner_id === me.data?.user.id
      ? t("lists.audience.yourTeams")
      : t("lists.audience.ownerTeams");
  };
}
