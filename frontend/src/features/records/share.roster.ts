import type { GrantDraft, RosterSubject } from "../../components/share.parts";
import { holdsEveryWord, searchWords } from "../../format/collate";

// The owner already holds the record, so a share to them adds nothing and
// would list them twice. Until the owner is known (undefined), no colleague is.
function mayShareWith(
  subject: RosterSubject,
  ownerId: string | null | undefined,
): boolean {
  return (
    subject.kind !== "user" || (ownerId !== undefined && subject.id !== ownerId)
  );
}

// The picked subject while it may still be granted: a refreshed access read
// can name the colleague already picked as the owner.
export function pickedToGrant(
  subject: RosterSubject | null,
  ownerId: string | null | undefined,
): RosterSubject | null {
  return subject && mayShareWith(subject, ownerId) ? subject : null;
}

// An open confirmation holds a pick too, and the owner can change under it.
export function draftToGrant(
  draft: GrantDraft | null,
  ownerId: string | null | undefined,
): GrantDraft | null {
  return draft && pickedToGrant(draft.subject, ownerId) ? draft : null;
}

export function shareCandidates(
  roster: readonly RosterSubject[],
  term: string,
  ownerId: string | null | undefined,
): RosterSubject[] {
  const words = searchWords(term);
  return roster.filter(
    (subject) =>
      mayShareWith(subject, ownerId) &&
      (words.length === 0 ||
        holdsEveryWord(words, [subject.name, subject.note])),
  );
}
