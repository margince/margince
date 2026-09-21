import { describe, expect, it } from "vitest";
import type { TimelineEntry } from "../design-system/composed";
import { groupChronology } from "./timelinegroups";

function mail(
  id: string,
  title: string,
  atIso: string,
  extra: Partial<TimelineEntry> = {},
): TimelineEntry {
  return {
    id,
    kind: "email",
    title,
    // A message that HAS a subject renders it as the title, so the fixture
    // carries both. `extra` can drop it back to null for the subjectless rows,
    // whose title is the body or the kind.
    subject: title,
    atIso,
    provenance: { kind: "human", self: false },
    ...extra,
  };
}

describe("grouping the account's chronology", () => {
  it("folds one conversation into one event, newest first", () => {
    const groups = groupChronology([
      mail("c", "Re: Pricing", "2026-07-03T10:00:00Z", { threadKey: "t-1" }),
      mail("b", "Re: Pricing", "2026-07-02T10:00:00Z", { threadKey: "t-1" }),
      mail("a", "Pricing", "2026-07-01T10:00:00Z", { threadKey: "t-1" }),
    ]);
    expect(groups).toHaveLength(1);
    expect(groups[0].kind).toBe("thread");
    // The group takes the position and the id of its NEWEST member, so the
    // reader's sense of what happened last survives the grouping.
    expect(groups[0].id).toBe("c");
    expect(groups[0].entries.map((e) => e.id)).toEqual(["c", "b", "a"]);
  });

  it("keeps two conversations apart even when their subjects match", () => {
    const groups = groupChronology([
      mail("x", "Re: Update", "2026-07-03T10:00:00Z", { threadKey: "t-1" }),
      mail("y", "Re: Update", "2026-07-03T11:00:00Z", { threadKey: "t-2" }),
    ]);
    // Subject grouping would have merged these. The provider's id does not.
    expect(groups).toHaveLength(2);
  });

  it("keeps one conversation together when its subject was renamed", () => {
    const groups = groupChronology([
      mail("b", "Re: now about hosting", "2026-07-03T10:00:00Z", {
        threadKey: "t-1",
      }),
      mail("a", "Pricing", "2026-07-01T10:00:00Z", { threadKey: "t-1" }),
    ]);
    expect(groups).toHaveLength(1);
  });

  it("folds a bulk send addressed to several contacts into one event", () => {
    const groups = groupChronology([
      mail("1", "Update zu Margince", "2026-07-17T09:00:00Z"),
      mail("2", "Update zu Margince", "2026-07-17T09:00:01Z"),
      mail("3", "Update zu Margince", "2026-07-17T09:00:02Z"),
    ]);
    expect(groups).toHaveLength(1);
    expect(groups[0].kind).toBe("bulk");
    expect(groups[0].entries).toHaveLength(3);
  });

  it("trusts the sender's own bulk attestation over the copy count", () => {
    const groups = groupChronology([
      mail("1", "Newsletter", "2026-07-17T09:00:00Z", { bulkAttested: true }),
    ]);
    expect(groups[0].kind).toBe("bulk");
  });

  it("folds an attested send whose copies each root their own thread", () => {
    // Measured on a real mailshot: three copies, each attested by the sender's
    // own List-Unsubscribe, and each carrying a thread_key EQUAL TO ITS OWN
    // MESSAGE ID. A first message has no In-Reply-To to root on, so the
    // provider's conversation id is the message itself — every copy of a send
    // is a thread of one, and the keys are unique by construction.
    //
    // Before the attestation check, the thread branch took every one of these
    // and drew three cards that each read "1 message". A send of fifty drew
    // fifty.
    const groups = groupChronology([
      mail("m1", "Auf geht's zur digiWiesn, Joshua", "2026-09-16T08:12:29Z", {
        bulkAttested: true,
        threadKey: "m1",
      }),
      mail("m2", "Auf geht's zur digiWiesn, Lars", "2026-09-16T08:12:28Z", {
        bulkAttested: true,
        threadKey: "m2",
      }),
      mail(
        "m3",
        "Auf geht's zur digiWiesn, Charlotte",
        "2026-09-16T08:10:09Z",
        {
          bulkAttested: true,
          threadKey: "m3",
        },
      ),
    ]);
    expect(groups).toHaveLength(1);
    expect(groups[0].kind).toBe("bulk");
    expect(groups[0].entries).toHaveLength(3);
  });

  it("keeps a real conversation grouped even when a member is attested", () => {
    // Attestation only diverts a message from ITS OWN thread key. Two messages
    // sharing one key are a conversation, and a reply that happens to carry the
    // flag must not tear the thread apart.
    const groups = groupChronology([
      mail("b", "Re: Pricing", "2026-07-02T10:00:00Z", { threadKey: "t-9" }),
      mail("a", "Pricing", "2026-07-01T10:00:00Z", { threadKey: "t-9" }),
    ]);
    expect(groups).toHaveLength(1);
    expect(groups[0].kind).toBe("thread");
  });

  it("does not let the salutation strip merge two different sends", () => {
    // The strip is the risky direction: returning too much would fold two
    // unrelated mailshots into one card and hide a message. These two share no
    // subject once the salutation is off, so they stay apart.
    const groups = groupChronology([
      mail("a", "Invoice overdue, Joshua", "2026-09-16T08:00:00Z", {
        bulkAttested: true,
        threadKey: "a",
      }),
      mail("b", "Welcome aboard, Joshua", "2026-09-16T08:00:01Z", {
        bulkAttested: true,
        threadKey: "b",
      }),
    ]);
    expect(groups).toHaveLength(2);
  });

  it("keeps a subject whose tail is not a salutation", () => {
    // A trailing clause is not a name. Stripping it would key two genuinely
    // different sends the same way, so anything longer than a name, or carrying
    // punctuation of its own, is left whole.
    const groups = groupChronology([
      mail("a", "Q3 results, revenue and outlook", "2026-09-16T08:00:00Z", {
        bulkAttested: true,
        threadKey: "a",
      }),
      mail("b", "Q3 results, costs and headcount", "2026-09-16T08:00:01Z", {
        bulkAttested: true,
        threadKey: "b",
      }),
    ]);
    expect(groups).toHaveLength(2);
  });

  it("leaves an UNATTESTED personalized run as separate rows", () => {
    // Without the sender's own attestation this is just three subjects that
    // look alike, and folding them would be the subject matching this file
    // refuses everywhere else.
    //
    // THREE copies, not two, and the count is what makes this test hold the
    // attestation rule: two would stay apart anyway on BULK_COPIES, so the
    // assertion would pass with the attestation check deleted and prove
    // nothing. At three, only the missing attestation keeps them apart.
    const groups = groupChronology([
      mail("a", "Auf geht's zur digiWiesn, Joshua", "2026-09-16T08:00:00Z", {
        threadKey: "a",
      }),
      mail("b", "Auf geht's zur digiWiesn, Lars", "2026-09-16T08:00:01Z", {
        threadKey: "b",
      }),
      mail("c", "Auf geht's zur digiWiesn, Charlotte", "2026-09-16T08:00:02Z", {
        threadKey: "c",
      }),
    ]);
    expect(groups).toHaveLength(3);
  });

  it("keeps a threadless personalized run apart without attestation", () => {
    // No thread key, so these reach the bulk key directly rather than returning
    // at the thread branch. Three copies would fold on BULK_COPIES if the
    // salutation were stripped — and nothing here attested a send, so they must
    // stay three rows. This is the case that holds the attestation condition on
    // the strip itself.
    const groups = groupChronology([
      mail("a", "Auf geht's zur digiWiesn, Joshua", "2026-09-16T08:00:00Z"),
      mail("b", "Auf geht's zur digiWiesn, Lars", "2026-09-16T08:00:01Z"),
      mail("c", "Auf geht's zur digiWiesn, Charlotte", "2026-09-16T08:00:02Z"),
    ]);
    expect(groups).toHaveLength(3);
  });

  it("leaves two same-subject messages as two rows", () => {
    // Two is a coincidence, not a send. Folding them would hide a message
    // inside a summary nobody opened.
    const groups = groupChronology([
      mail("1", "Question", "2026-07-17T09:00:00Z"),
      mail("2", "Question", "2026-07-17T18:00:00Z"),
    ]);
    expect(groups).toHaveLength(2);
    expect(groups.every((g) => g.kind === "single")).toBe(true);
  });

  it("does not fold the same subject sent on different days", () => {
    const groups = groupChronology([
      mail("1", "Weekly", "2026-07-17T09:00:00Z"),
      mail("2", "Weekly", "2026-07-18T09:00:00Z"),
      mail("3", "Weekly", "2026-07-19T09:00:00Z"),
    ]);
    expect(groups).toHaveLength(3);
  });

  it("never groups record changes", () => {
    const change = (id: string, atIso: string): TimelineEntry => ({
      id,
      kind: "change",
      title: "stage",
      atIso,
      provenance: { kind: "human", self: false },
    });
    const groups = groupChronology([
      change("c1", "2026-07-17T09:00:00Z"),
      change("c2", "2026-07-17T09:00:01Z"),
    ]);
    // Two edits are two facts.
    expect(groups).toHaveLength(2);
  });

  it("marks only the oldest group as possibly continuing past the page", () => {
    const groups = groupChronology(
      [
        mail("b", "Re: Pricing", "2026-07-03T10:00:00Z", { threadKey: "t-1" }),
        mail("a", "Intro", "2026-07-01T10:00:00Z", { threadKey: "t-2" }),
      ],
      true,
    );
    expect(groups[0].partial).toBe(false);
    // Only the oldest can have members beyond the edge of what the page holds.
    expect(groups[1].partial).toBe(true);
  });

  it("marks the group holding the oldest entry, not the last group on screen", () => {
    // The thread ranks FIRST because a group takes the position of its newest
    // member, and it also holds the page's oldest message. Marking the last
    // group would offer to continue the single row — which is complete — and
    // stay silent about the conversation that is actually cut.
    const groups = groupChronology(
      [
        mail("t-new", "Re: Pricing", "2026-07-05T10:00:00Z", {
          threadKey: "t-1",
        }),
        mail("lone", "Intro", "2026-07-03T10:00:00Z"),
        mail("t-old", "Pricing", "2026-07-01T10:00:00Z", { threadKey: "t-1" }),
      ],
      true,
    );
    expect(groups[0].kind).toBe("thread");
    expect(groups[0].partial).toBe(true);
    expect(groups[1].partial).toBe(false);
  });

  it("does not let one attested message fold an unrelated same-subject reply", () => {
    // List-Unsubscribe is carried by ONE message. A reply that merely shares
    // the subject and the day is not part of the send the sender attested to,
    // and folding it would hide it inside a summary.
    const groups = groupChronology([
      mail("blast", "Produktupdate", "2026-07-17T09:00:00Z", {
        bulkAttested: true,
      }),
      mail("reply", "Re: Produktupdate", "2026-07-17T14:00:00Z"),
    ]);
    expect(groups).toHaveLength(2);
    expect(groups[0].kind).toBe("bulk");
    expect(groups[1].kind).toBe("single");
    expect(groups[1].entries.map((e) => e.id)).toEqual(["reply"]);
  });

  it("never bulk-folds subjectless messages, whatever their rendered title", () => {
    // A subjectless row renders its BODY as the title, and a wordless one
    // renders its kind. Keyed on the title, three of those on one day would
    // fold into a bulk send that was never sent.
    const groups = groupChronology([
      mail("1", "email", "2026-07-17T09:00:00Z", { subject: null }),
      mail("2", "email", "2026-07-17T09:00:01Z", { subject: null }),
      mail("3", "email", "2026-07-17T09:00:02Z", { subject: null }),
    ]);
    expect(groups).toHaveLength(3);
    expect(groups.every((g) => g.kind === "single")).toBe(true);
  });
});
