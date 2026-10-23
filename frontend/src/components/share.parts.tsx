import { User as UserIcon, Users as UsersIcon } from "lucide-react";
import { useMemo } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, type useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { EntityKind } from "../lib/entity";
import { unwrap } from "../lib/problem";
import { RosterPartialNote, useRoster, useRosterPartial } from "./entityref";

export type RecordGrant = components["schemas"]["RecordGrant"];

export type CreateRecordGrantRequest =
  components["schemas"]["CreateRecordGrantRequest"];

export type Access = CreateRecordGrantRequest["access"];

// The share screen serves the record kinds the app has a page for, which is
// every kind the grant contract admits. A grant on a record the reader cannot
// open would be a door to nowhere.
export type RecordType = Extract<
  CreateRecordGrantRequest["record_type"],
  EntityKind
>;

export type User = components["schemas"]["User"];

export type Team = components["schemas"]["Team"];

export type RosterSubject = {
  id: string;
  name: string;
  note: string;
  kind: "user" | "team";
};

/**
 * What a submit does to a grant the subject already holds.
 *
 * `POST /record-grants` is idempotent on
 * `(record_type, record_id, subject_type, subject_id)`. A re-assert restates
 * access, expires_at and reason (crm.yaml, createRecordGrant), so the screen
 * offers an already-granted subject. It owes the reader which of four things
 * the press means. `lower` alone takes authority away, and is asked about first.
 */
type ReassertKind = "raise" | "lower" | "amend" | "unchanged";

export type DraftFields = Readonly<{
  subject: RosterSubject;
  access: Access;
  expiresAt?: string;
  reason?: string;
}>;

/**
 * A submit against a subject who already holds a grant. Here `heldAccess` is
 * required, not optional on one shared shape. The downgrade confirm names the
 * level being taken away. A missing field would put a made-up level in the one
 * sentence where the reader decides something.
 */
export type ReassertDraft = DraftFields &
  Readonly<{ change: ReassertKind; heldAccess: Access }>;

/**
 * One submit, in full: the subject, the three fields a re-assert restates,
 * and what this press does to whatever is already there.
 *
 * Built in the render whose button the reader pressed and carried into the
 * mutation as its variable, so nothing here is older than that control. The
 * downgrade confirm submits from a later render and still sends the reader's
 * choices.
 */
export type GrantDraft =
  | (DraftFields & Readonly<{ change: "new" }>)
  | ReassertDraft;

/**
 * The identity of a grant subject, for keying one against another.
 *
 * A subject is a user or a team, and the two are separate id spaces: a team
 * may carry the same uuid as a user. So the type is part of the identity.
 * Keyed by id alone, one subject would wear the other's level on its picker
 * row. A first grant would then read as a restatement of a grant nobody holds.
 */
export function subjectKey(
  subjectType: RecordGrant["subject_type"],
  subjectId: string,
): string {
  return `${subjectType}:${subjectId}`;
}

export function reassertKind(
  held: RecordGrant,
  next: DraftFields,
): ReassertKind {
  if (held.access !== next.access) {
    return next.access === "write" ? "raise" : "lower";
  }
  // Same level, so what the subject can do stays the same. The re-assert still
  // rewrites the other two fields, so a reset expiry or a new reason is not a
  // no-op. Only all three matching is.
  const sameExpiry = (held.expires_at ?? null) === (next.expiresAt ?? null);
  const sameReason = (held.reason ?? "") === (next.reason ?? "");
  return sameExpiry && sameReason ? "unchanged" : "amend";
}

// day-count → i18n key, matching the mockup's expiry select (0/1/7/30).
export const EXPIRY_OPTIONS: { days: number; key: MessageKey }[] = [
  { days: 0, key: "share.expiry.none" },
  { days: 1, key: "share.expiry.day" },
  { days: 7, key: "share.expiry.week" },
  { days: 30, key: "share.expiry.month" },
];

export function expiresAtFor(days: number): string | undefined {
  if (days <= 0) {
    return undefined;
  }
  return new Date(Date.now() + days * 24 * 60 * 60 * 1000).toISOString();
}

// One word for a level, wherever the screen names one: the picker's own
// control, the who-has-access list, and the sentence a downgrade asks. A
// level worded one way in the list and another in the dialog reads as two
// different permissions.
export function accessLabel(level: Access, t: ReturnType<typeof useT>): string {
  return t(level === "write" ? "share.access.write" : "share.access.read");
}

export async function fetchGrants(
  recordType: RecordType,
  recordId: string,
): Promise<RecordGrant[]> {
  const data = unwrap(
    await api.GET("/record-grants", {
      params: {
        query: {
          record_type: recordType,
          record_id: recordId,
          limit: 100,
        },
      },
    }),
  );
  return data.data;
}

// A person-vs-team cue for every subject this screen names. The picker rows and
// the who-has-access list otherwise show a bare name with no cue to its kind.
// A Lucide glyph carries it (one silhouette for a person, a group for a team),
// labelled for assistive tech because the glyphs alone are not.
export function SubjectKindIcon({
  kind,
  t,
}: Readonly<{ kind: "user" | "team"; t: ReturnType<typeof useT> }>) {
  const label = t(kind === "team" ? "share.kindTeam" : "share.kindPerson");
  return kind === "team" ? (
    <UsersIcon className="share-kind-icon" aria-label={label} />
  ) : (
    <UserIcon className="share-kind-icon" aria-label={label} />
  );
}

// The subject-picker rows, shared by the normal path and the partial-roster
// failure path (one roster query succeeded, the other did not).
//
// A subject who already holds a grant is selectable, and the row names the
// level held. A disabled row would leave revoke-and-regrant as the one way to
// change a level. Naming the level lets the reader tell an upgrade from a
// downgrade before pressing anything.
function renderSubjectList(
  candidates: RosterSubject[],
  held: ReadonlyMap<string, RecordGrant>,
  selected: RosterSubject | null,
  t: ReturnType<typeof useT>,
  onPick: (candidate: RosterSubject) => void,
) {
  return (
    <ul className="share-subject-list">
      {candidates.map((candidate) => {
        const key = subjectKey(candidate.kind, candidate.id);
        const heldGrant = held.get(key);
        return (
          <li key={key}>
            <Button
              className="share-subject-row"
              aria-pressed={
                selected !== null &&
                subjectKey(selected.kind, selected.id) === key
              }
              onClick={() => onPick(candidate)}
            >
              <span className="share-subject-name">
                <SubjectKindIcon kind={candidate.kind} t={t} />
                <span>{candidate.name}</span>
              </span>
              <span className="share-subject-held">
                {heldGrant && (
                  <Badge
                    tone={heldGrant.access === "write" ? "accent" : undefined}
                  >
                    {t(
                      heldGrant.access === "write"
                        ? "share.holdsWrite"
                        : "share.holdsRead",
                    )}
                  </Badge>
                )}
                <span>{candidate.note}</span>
              </span>
            </Button>
          </li>
        );
      })}
    </ul>
  );
}

// The whole "who can I share with" picker body: loading, error, empty, list.
// It owns nothing but the render ladder; all state (term/subject) and the
// roster queries stay in the parent.
export function RosterPicker({
  usersQuery,
  teamsQuery,
  partial,
  filteredRoster,
  held,
  subject,
  t,
  onPick,
}: Readonly<{
  usersQuery: { isPending: boolean; isError: boolean; refetch: () => unknown };
  teamsQuery: { isPending: boolean; isError: boolean; refetch: () => unknown };
  // Whether either roster stopped short of the workspace. A subject nothing
  // here read cannot be granted access. A picker that does not say so reads as
  // the complete list of who there is to share with.
  partial: boolean;
  filteredRoster: RosterSubject[];
  // The grant each subject already holds on this record, by `subjectKey`.
  held: ReadonlyMap<string, RecordGrant>;
  subject: RosterSubject | null;
  t: ReturnType<typeof useT>;
  onPick: (candidate: RosterSubject) => void;
}>) {
  // Both roster fetches collapse a failure to `[]` in the caller's `roster`, so
  // the picker never crashes. That makes a failed request look like a workspace
  // with zero shareable subjects. So gate on loading and error first: the empty
  // picker renders only once both queries succeeded with no subjects.
  if (usersQuery.isPending || teamsQuery.isPending) {
    return <p data-testid="share-roster-loading">{t("share.rosterLoading")}</p>;
  }
  if (usersQuery.isError || teamsQuery.isError) {
    return (
      <div data-testid="share-roster-error">
        <ErrorLine>
          {usersQuery.isError && teamsQuery.isError
            ? t("share.rosterErrorBoth")
            : usersQuery.isError
              ? t("share.rosterErrorUsers")
              : t("share.rosterErrorTeams")}
        </ErrorLine>
        <Button
          className="share-roster-retry"
          onClick={() => {
            if (usersQuery.isError) usersQuery.refetch();
            if (teamsQuery.isError) teamsQuery.refetch();
          }}
        >
          {t("common.retry")}
        </Button>
        {/* A partial failure still lets a subject that did load be picked.
            The error is informational, not a hard block. */}
        {filteredRoster.length > 0 &&
          renderSubjectList(filteredRoster, held, subject, t, onPick)}
      </div>
    );
  }
  if (filteredRoster.length === 0) {
    return (
      <>
        <p data-testid="share-roster-empty">{t("share.rosterEmpty")}</p>
        {/* "Nobody matches" over a roster that stopped early is the reader
            being told the subject they are looking for does not exist, on the
            strength of pages nothing read. */}
        <RosterPartialNote partial={partial} />
      </>
    );
  }
  return (
    <>
      {renderSubjectList(filteredRoster, held, subject, t, onPick)}
      <RosterPartialNote partial={partial} />
    </>
  );
}

// useShareRoster is the share picker's roster: every colleague's seat and every
// team, as one list. It reads EntityRef's cache entries, so one roster fetch
// serves the picker here and a resolved name elsewhere.
export function useShareRoster() {
  const plural = usePlural();
  const { locale } = useLocale();
  const usersQuery = useRoster("user", true);
  const teamsQuery = useRoster("team", true);
  // The list is incomplete when either walk stopped short of its end. Both
  // hooks run on every render, so the hook count never depends on how deep
  // the user roster went.
  const usersPartial = useRosterPartial("user", true);
  const teamsPartial = useRosterPartial("team", true);
  const roster: RosterSubject[] = useMemo(() => {
    // Agent seats carry is_agent so the share picker can leave them out. A
    // record is shared with colleagues and teams, never with an agent.
    const users = ((usersQuery.data ?? []) as User[])
      .filter((u) => !u.is_agent)
      .map(
        (u): RosterSubject => ({
          id: u.id,
          name: u.display_name,
          note: u.email,
          kind: "user",
        }),
      );
    const teams = ((teamsQuery.data ?? []) as Team[]).map(
      (team): RosterSubject => {
        const count = team.member_count ?? 0;
        return {
          id: team.id,
          name: team.name,
          note: plural("share.teamMembers", count, {
            count: formatNumber(count, locale),
          }),
          kind: "team",
        };
      },
    );
    return [...users, ...teams];
  }, [usersQuery.data, teamsQuery.data, plural, locale]);
  return {
    roster,
    usersQuery,
    teamsQuery,
    rosterPartial: usersPartial || teamsPartial,
  };
}
