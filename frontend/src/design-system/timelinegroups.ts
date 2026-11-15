import type { TimelineEntry, TimelineGroup } from "./timelineentry";

export type { TimelineGroup };

/**
 * Grouping the account's chronology into the events a reader recognises.
 *
 * A company timeline rendered one row per message shows the same exchange
 * several times: a product update sent to three contacts is three rows, and a
 * five-message thread is five. The reader is left reconstructing conversations
 * from fragments, which is the job the page exists to do for them.
 *
 * Three groupings, and no more:
 *
 *  - a THREAD is the provider's own conversation id. Not a subject match —
 *    grouping by subject merges two unrelated "Re: Update" exchanges and splits
 *    one that was renamed mid-conversation.
 *  - a BULK SEND is one message the sender addressed to several contacts at once.
 *    It has no thread of its own, so it is recognised by shape: same subject,
 *    same day, and either the sender's own List-Unsubscribe attestation or
 *    enough copies that no other reading is plausible.
 *
 *  - a run of NEWSLETTERS is inbound mail the sender attested as bulk, one
 *    message after another with nothing else between them, whatever their
 *    subjects and days. A team's own mailing is something the team did, so an
 *    outbound one keeps the per-send rule above.
 *
 * Everything else stays a single row. A grouping that guessed would hide a
 * message inside a summary nobody opened, which is worse than the repetition
 * it replaced.
 */

/** How many same-subject, same-day copies imply one send rather than a coincidence. */
const BULK_COPIES = 3;

/** normalizeSubject strips the reply and forward prefixes a thread accretes. */
function normalizeSubject(title: string): string {
  return title
    .replace(/^((re|aw|fwd|fw|wg)\s*:\s*)+/i, "")
    .trim()
    .toLowerCase();
}

// inboundBulk is a message the sender attested as bulk mail and the team
// received: a newsletter, or another mailing the account sends us.
function inboundBulk(entry: TimelineEntry): boolean {
  return Boolean(entry.bulkAttested) && entry.direction === "inbound";
}

function dayOf(atIso: string): string {
  return atIso.slice(0, 10);
}

/**
 * groupChronology folds a newest-first entry list into groups, preserving that
 * order. A group takes the position of its newest member, so the reader's
 * sense of "what happened last" survives the grouping. A thread holds only
 * messages with nothing between them. Where another row falls between two of
 * its messages, it splits there, and each part links to the next.
 *
 * `hasMore` says the underlying page was cut. It marks only the OLDEST group,
 * because that is the only one whose members could continue past the edge.
 */
/**
 * bulkMembership decides, before anything is grouped, which same-subject runs
 * are one send. Deciding it afterwards would leave a demoted candidate holding
 * both its messages in one row — hiding one, which is the failure this grouping
 * exists to prevent.
 */
function bulkMembership(entries: readonly TimelineEntry[]): Set<string> {
  const copies = new Map<string, number>();
  for (const entry of entries) {
    const key = groupKeyOf(entry);
    if (key?.kind !== "bulk" || inboundBulk(entry)) {
      continue;
    }
    copies.set(key.value, (copies.get(key.value) ?? 0) + 1);
  }
  const bulk = new Set<string>();
  for (const [value, count] of copies) {
    // Two same-subject messages are a coincidence, not a send. The count is
    // the only rule that can speak for a whole subject+day run: attestation
    // is carried by ONE message and is applied per entry in groupChronology,
    // because a reply that merely shares the subject and the day is not part
    // of the send the sender attested to.
    if (count >= BULK_COPIES) {
      bulk.add(value);
    }
  }
  return bulk;
}

// foldKeyOf is the key this entry folds under, or nothing for a single row. A
// bulk key folds it when the run is too large to read any other way. It also
// folds a message that carries its sender's own List-Unsubscribe attestation.
// One attested message does not speak for every same-subject message that day.
function foldKeyOf(
  entry: TimelineEntry,
  bulk: ReadonlySet<string>,
): { kind: "thread" | "bulk"; value: string } | undefined {
  const key = groupKeyOf(entry);
  if (key?.kind === "thread") {
    return key;
  }
  const bulkable =
    key?.kind === "bulk" &&
    (bulk.has(key.value) || Boolean(entry.bulkAttested));
  return bulkable ? key : undefined;
}

// joinNewsletterRun adds a received mailing to the run the list is in, or
// starts a run where the entry before it was something else.
function joinNewsletterRun(
  run: TimelineGroup | undefined,
  entry: TimelineEntry,
  groups: TimelineGroup[],
): TimelineGroup {
  if (run) {
    run.entries.push(entry);
    return run;
  }
  const started: TimelineGroup = {
    id: entry.id,
    kind: "newsletters",
    entries: [entry],
    partial: false,
  };
  groups.push(started);
  return started;
}

function joinsPreviousPart(
  kind: "thread" | "bulk",
  existing: TimelineGroup | undefined,
  previous: TimelineEntry | undefined,
  wholeThreads: boolean,
): boolean {
  return (
    existing !== undefined &&
    (wholeThreads || kind !== "thread" || existing.entries.at(-1) === previous)
  );
}

function shareThreadEntries(groups: TimelineGroup[]): void {
  const threads = new Map<string, TimelineGroup[]>();
  for (const group of groups) {
    const key =
      group.kind === "thread" ? group.entries[0]?.threadKey : undefined;
    if (!key) continue;
    const parts = threads.get(key) ?? [];
    parts.push(group);
    threads.set(key, parts);
  }
  for (const parts of threads.values()) {
    if (parts.length < 2) continue;
    const entries = parts.flatMap((part) => part.entries);
    for (const part of parts) part.threadEntries = entries;
  }
}

export function groupChronology(
  entries: readonly TimelineEntry[],
  hasMore = false,
  // A list of conversations, not of events, keeps each thread whole.
  { wholeThreads = false }: { wholeThreads?: boolean } = {},
): TimelineGroup[] {
  const bulk = bulkMembership(entries);
  const groups: TimelineGroup[] = [];
  const byKey = new Map<string, TimelineGroup>();
  // The run of newsletters the list is in, until any other entry ends it.
  let newsletters: TimelineGroup | undefined;

  for (const [index, entry] of entries.entries()) {
    if (inboundBulk(entry)) {
      newsletters = joinNewsletterRun(newsletters, entry, groups);
      continue;
    }
    newsletters = undefined;
    const groupable = foldKeyOf(entry, bulk);
    if (!groupable) {
      groups.push({
        id: entry.id,
        kind: "single",
        entries: [entry],
        partial: false,
      });
      continue;
    }
    const existing = byKey.get(groupable.value);
    if (
      existing &&
      joinsPreviousPart(
        groupable.kind,
        existing,
        entries[index - 1],
        wholeThreads,
      )
    ) {
      existing.entries.push(entry);
      continue;
    }
    // A group takes the position and the id of its newest member, which is the
    // first one seen in a newest-first list.
    const group: TimelineGroup = {
      id: entry.id,
      kind: groupable.kind,
      entries: [entry],
      partial: false,
    };
    if (existing && groupable.kind === "thread") {
      existing.olderPart = group.id;
      group.newerPart = existing.id;
    }
    byKey.set(groupable.value, group);
    groups.push(group);
  }

  shareThreadEntries(groups);
  // Only the part touching the page edge may continue beyond the loaded rows.
  const oldest = entries.at(-1);
  if (hasMore && oldest !== undefined) {
    const cut = groups.find((group) => group.entries.includes(oldest));
    if (cut) {
      cut.partial = true;
    }
  }
  return groups;
}

/**
 * groupKeyOf decides what, if anything, this entry groups by. A change row
 * never groups: it is a field edit, and two edits are two facts.
 */
function groupKeyOf(
  entry: TimelineEntry,
): { kind: "thread" | "bulk"; value: string } | undefined {
  if (entry.kind === "change") {
    return undefined;
  }
  // A thread key groups a conversation — EXCEPT on a message the sender
  // attested as bulk, where it is not a conversation key at all.
  //
  // A first message roots its thread on itself: there is no In-Reply-To or
  // References header to root on, so the provider's conversation id is the
  // message's own id. Every copy of a mailshot is therefore a thread of one,
  // and each copy's key is unique by construction — so this branch, taken
  // first, kept a send of fifty from ever reaching the bulk rule below and
  // drew fifty cards that each said "1 message".
  //
  // Falling through on attestation alone is what keeps this narrow. The flag is
  // the sender's own List-Unsubscribe, read off the message by capture rather
  // than inferred here, so a real conversation never loses its grouping to a
  // guess about subjects.
  if (entry.threadKey && !entry.bulkAttested) {
    return { kind: "thread", value: `thread:${entry.threadKey}` };
  }
  if (entry.kind !== "email") {
    return undefined;
  }
  // The message's OWN subject, never the rendered title: `title` falls back to
  // the body, and then to the kind, so a run of subjectless emails would share
  // a key and fold into a bulk send that was never sent.
  const subject = normalizeSubject(entry.subject ?? "");
  if (!subject) {
    return undefined;
  }
  // An ATTESTED send may drop a trailing salutation from its key, because a
  // mailshot personalizes the subject per recipient: one send arrives as
  // "…zur digiWiesn, Joshua", "…, Lars", "…, Charlotte". Keyed whole, those are
  // three keys and the send draws three cards.
  //
  // Only when the sender attested it. The attestation is their own
  // List-Unsubscribe header, so the claim "this is one send" comes from the
  // sender rather than from this function noticing that two subjects look
  // alike — which is the reasoning this file refuses everywhere else, because
  // it merges two unrelated "Re: Update" exchanges.
  const key = entry.bulkAttested ? withoutSalutation(subject) : subject;
  return { kind: "bulk", value: `bulk:${key}:${dayOf(entry.atIso)}` };
}

/**
 * withoutSalutation drops a trailing ", <name>" from an attested send's
 * subject, so the copies of one mailshot share a key.
 *
 * Bounded deliberately: ONE trailing comma, and only when what follows is a
 * short run of letters with no comma of its own. A subject that ends in a list,
 * a clause, or a company name with punctuation keeps its whole subject and
 * simply groups as it did before — the failure of this returning too little is
 * a duplicate card, and of returning too much is two sends merged into one.
 */
function withoutSalutation(subject: string): string {
  return subject.replace(/,\s*\p{L}[\p{L}'-]{0,23}$/u, "").trim() || subject;
}
