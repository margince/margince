// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Who is named on a `selected` audience.
//
// The third audience the API has always taken and the dialog never offered:
// `AUDIENCE_CHOICES` listed workspace and participants because offering
// `selected` without a way to name anybody would be a choice the reader cannot
// complete. This is that way.
//
// The roster comes from `useRoster`, the same hook the share picker and the
// filter reference already read — one fetch and one cache entry, whether the
// names are being picked here or resolved for a chip somewhere else.

import type { components } from "../api/schema";
import { useT } from "../i18n";
import { type RosterKind, useRoster } from "./entityref";
import "./audiencemembers.css";

type AudienceMember = components["schemas"]["AudienceMember"];
type User = components["schemas"]["User"];
type Team = components["schemas"]["Team"];

/** One pickable seat or team, flattened out of the two rosters. */
type Candidate = {
  kind: RosterKind;
  id: string;
  name: string;
  note?: string;
};

export function memberKey(member: AudienceMember): string {
  return `${member.subject_type}:${member.subject_id}`;
}

/**
 * The people and teams a message can be limited to.
 *
 * Agent seats are excluded for the reason the share picker excludes them: a
 * message is limited to people and teams, never to an agent. The rule is spelled
 * here rather than borrowed, because share.tsx's copy answers a different
 * question — who a RECORD is shared with — and one changing is no reason for
 * the other to.
 */
export function useAudienceCandidates(enabled: boolean): Candidate[] {
  const users = useRoster("user", enabled);
  const teams = useRoster("team", enabled);
  const seats = ((users.data ?? []) as User[])
    .filter((seat) => !seat.is_agent)
    .map(
      (seat): Candidate => ({
        kind: "user",
        id: seat.id,
        name: seat.display_name,
        note: seat.email,
      }),
    );
  const groups = ((teams.data ?? []) as Team[]).map(
    (team): Candidate => ({ kind: "team", id: team.id, name: team.name }),
  );
  return [...seats, ...groups];
}

/**
 * The member picker, as a checklist.
 *
 * A checklist rather than a search-and-add: an audience is small and a reader
 * changing one is usually removing somebody, which a list they can see makes
 * one press and a search box makes a guessing game.
 *
 * The set it submits is the FULL replacement set, which is what the write
 * takes — so what the reader sees ticked is exactly what the message will be
 * limited to, with no merge happening behind them.
 */
export function AudienceMembers({
  candidates,
  chosen,
  onChange,
}: Readonly<{
  candidates: Candidate[];
  chosen: readonly AudienceMember[];
  onChange: (next: AudienceMember[]) => void;
}>) {
  const t = useT();
  const picked = new Set(chosen.map(memberKey));
  const toggle = (candidate: Candidate) => {
    const member: AudienceMember = {
      subject_type: candidate.kind,
      subject_id: candidate.id,
    };
    onChange(
      picked.has(memberKey(member))
        ? chosen.filter((each) => memberKey(each) !== memberKey(member))
        : [...chosen, member],
    );
  };
  if (candidates.length === 0) {
    // The list has not answered yet, or answered with nobody. Either way the
    // reader is told rather than shown an empty box that reads as an
    // organization with no people in it.
    return <p className="t-caption">{t("compose.audienceMembersLoading")}</p>;
  }
  return (
    <fieldset className="compose-audience-members">
      <legend className="t-label">{t("compose.audienceMembersLegend")}</legend>
      {candidates.map((candidate) => {
        const key = `${candidate.kind}:${candidate.id}`;
        return (
          <label key={key} className="compose-audience-member">
            <input
              type="checkbox"
              checked={picked.has(key)}
              onChange={() => toggle(candidate)}
            />
            <span>{candidate.name}</span>
            {candidate.note && (
              <span className="t-caption">{candidate.note}</span>
            )}
          </label>
        );
      })}
    </fieldset>
  );
}
