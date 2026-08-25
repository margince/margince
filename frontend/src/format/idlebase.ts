// When a record last showed a sign of life: its newest activity when there is
// one, else the instant it was created.
//
// The server measures silence from exactly this fallback — the stalled-deal
// rule, the gone-quiet project report, an account's coverage view — and the
// deal board draws a card's age from it. Two answers to that question means a
// board that ages a deal differently from the list that filed it stalled.
//
// This module is a DECLARED MIRROR of the server's spelling, not a second
// answer: `backend/frontendidlebase_test.go` reads the expression below and
// the Go `idlebase.SQL` beside it, and fails when either side changes the
// order alone.
//
// The order is the whole rule and it is invisible. `created_at` is never
// absent, so a fallback written the other way round answers "created" for
// every record while reading, to a person, exactly like this one.

/** The two timestamps a record carries, as the wire sends them. */
export type IdleBaseRecord = {
  last_activity_at?: string | null;
  created_at: string;
};

/** The instant a record's silence is measured from. */
export function idleSince(record: IdleBaseRecord): string {
  return record.last_activity_at ?? record.created_at;
}
