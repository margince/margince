// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useUrlParams } from "../app/urlstate";
import type { ListChip } from "../design-system/listsurface.dials";
import { forReader } from "../format/collate";
import { useLocale, useT } from "../i18n";
import { useAssignableUserOptions } from "./assigneepicker";
import { useMe } from "./common";
import { rosterReading, useRoster, useRosterPartial } from "./entityref";
import { useMemberNames } from "./membernames";

// The owner and team dials every owner-scoped list offers: leads, contacts,
// companies, deals and projects. A record is owned by one colleague, so the
// Owner dial lists colleagues. A team owns nothing, so "rows owned by a member of
// this team" is a dial of its own. The server refuses owner_id, owner_team_id
// and unassigned together, so each dial clears the other's parameters.

const OWNER_PARAMS = ["owner_id", "unassigned"] as const;
const TEAM_PARAMS = ["owner_team_id"] as const;

type Option = { value: string; label: string };
type Translate = ReturnType<typeof useT>;

/**
 * The Owner and Team dials, in that order.
 *
 * Owner is built only once /me has answered. An option whose value is still
 * "" reads as "clear this filter" to the table, so a half-built dial would
 * narrow nothing while looking armed. Team is left out while the workspace
 * has no team and none is applied.
 */
export function useOwnerChips(): readonly ListChip[] {
  const t = useT();
  const owner = useOwnerDial(t);
  const team = useTeamDial(t);
  return [...(owner ? [owner] : []), ...(team ? [team] : [])];
}

/** The value of one filter parameter in the address, under any list scope. */
function useApplied(param: string): string | undefined {
  const [params] = useUrlParams();
  const key = [...params.keys()].find(
    (candidate) =>
      (candidate === param || candidate.endsWith(`.${param}`)) &&
      params.get(candidate),
  );
  return key ? (params.get(key) ?? undefined) : undefined;
}

/**
 * You first, then every active colleague by name, then the unowned queue.
 *
 * An applied owner the roster does not list still gets an option, named by id
 * lookup. A saved view can restore a deactivated seat. Without its option the
 * list would stay narrowed with no dial to clear it.
 */
function useOwnerDial(t: Translate): ListChip | undefined {
  const { locale } = useLocale();
  const viewerId = useMe().data?.user.id;
  const colleagues = useAssignableUserOptions(Boolean(viewerId));
  const applied = useApplied("owner_id");
  const listed = colleagues.some((option) => option.value === applied);
  const stray = applied && applied !== viewerId && !listed ? applied : "";
  const strayName = useMemberNames(stray ? [stray] : []).get(stray);
  if (!viewerId) {
    return undefined;
  }
  const others: Option[] = colleagues
    .filter((option) => option.value !== viewerId)
    .map((option) => ({
      value: `owner_id:${option.value}`,
      label: option.label,
    }));
  if (stray) {
    others.push({
      value: `owner_id:${stray}`,
      label: strayName ?? t("common.loading"),
    });
  }
  others.sort((a, b) => forReader(a.label, b.label, locale));
  return {
    key: "owner",
    label: t("list.owner"),
    allLabel: t("list.filterOwnerAll"),
    filterable: true,
    excludes: TEAM_PARAMS,
    options: [
      { value: `owner_id:${viewerId}`, label: t("list.filterOwnerMe") },
      ...others,
      { value: "unassigned:true", label: t("list.filterOwnerUnassigned") },
    ],
  };
}

/**
 * Every team in the workspace. Listing teams the viewer is not on is safe:
 * owner_team_id only narrows the rows the viewer may already read.
 */
function useTeamDial(t: Translate): ListChip | undefined {
  const { locale } = useLocale();
  const viewerId = useMe().data?.user.id;
  const roster = useRoster("team", Boolean(viewerId));
  const partial = useRosterPartial("team", Boolean(viewerId));
  const applied = useApplied("owner_team_id");
  const options = teamOptions(roster, partial, applied, t);
  if (options.length === 0) {
    return undefined;
  }
  options.sort((a, b) => forReader(a.label, b.label, locale));
  return {
    key: "owner_team",
    label: t("list.team"),
    allLabel: t("list.filterTeamAll"),
    filterable: true,
    excludes: OWNER_PARAMS,
    options,
  };
}

/**
 * One option per team the roster lists, plus the applied team when the roster
 * does not name it. That option says the name is loading, did not load, or
 * (after a finished walk, since `/teams` leaves archived teams out) that the
 * team is unavailable. The request stays narrowed by it, so it must show.
 */
function teamOptions(
  roster: ReturnType<typeof useRoster>,
  partial: boolean,
  applied: string | undefined,
  t: Translate,
): Option[] {
  const options = (roster.data ?? []).map((entry) => ({
    value: `owner_team_id:${entry.id}`,
    // The team roster: every entry carries a name. The user roster is the
    // other kind this hook serves, and it is never asked for here.
    label: "display_name" in entry ? entry.display_name : entry.name,
  }));
  if (!applied || options.some((o) => o.value === `owner_team_id:${applied}`)) {
    return options;
  }
  const labels = {
    pending: t("common.loading"),
    failed: t("ref.nameLoadFailed"),
    unnamed: t("list.teamUnavailable"),
  };
  const reading = rosterReading(roster, partial);
  return [
    ...options,
    { value: `owner_team_id:${applied}`, label: labels[reading] },
  ];
}
