import type { components } from "../api/schema";

// Which of a contact's addresses we write to.
//
// A person carries a LIST — several addresses, each with a type, a position and
// a primary flag, some of them retired. "The contact's email" is therefore a
// decision rather than a field, and it is one the server already made: this
// mirrors persondraft.primaryEmail, which is what the drafter addresses a
// message to. Two rules would mean the address a composer prefills and the
// address a draft is written to could differ, on a record where the reader can
// see both.
//
// Held by TestBothSidesPickTheSameAddress
// (backend/gates/frontendprimaryemail_test.go).

type PersonEmail = components["schemas"]["PersonEmail"];

/**
 * primaryEmail is the address to write to: the one marked primary, and
 * otherwise the first live one on the record.
 *
 * An unmarked address is still reachable — a contact with exactly one address
 * and no flag set is the ordinary case, and refusing to write to them would
 * read the flag as permission when it only RANKS.
 *
 * An archived address is skipped either way. Somebody retired it, and offering
 * it back is the one wrong answer here: mail to a retired address either
 * bounces or reaches a person who asked us to stop using it.
 *
 * Undefined when the record carries no live address at all, which a caller
 * shows as an empty field rather than as a guess.
 */
export function primaryEmail(
  emails: readonly PersonEmail[] | undefined,
): string | undefined {
  if (!emails) {
    return undefined;
  }
  let first: string | undefined;
  for (const email of emails) {
    if (email.archived_at) {
      continue;
    }
    if (email.is_primary) {
      return email.email;
    }
    first ??= email.email;
  }
  return first;
}
