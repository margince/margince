import type { RosterSubject } from "../../components/share.parts";
import { holdsEveryWord, searchWords } from "../../format/collate";

// The owner already sees and edits the record, so a share to them adds
// nothing and would list them twice.
export function shareCandidates(
  roster: readonly RosterSubject[],
  term: string,
  ownerId: string | undefined,
): RosterSubject[] {
  const words = searchWords(term);
  return roster.filter(
    (subject) =>
      !(subject.kind === "user" && subject.id === ownerId) &&
      (words.length === 0 ||
        holdsEveryWord(words, [subject.name, subject.note])),
  );
}
