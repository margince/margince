import { expect, it } from "vitest";
import { translate } from "../../i18n";
import { readerTask, sentenceTitled, whenText } from "../../lib/worklistcopy";
import { taskRow } from "../../testing/brief.fixtures";
import { noticeDetail, sharedResponsibility } from "./worklist.reader";

const t = (
  key: Parameters<typeof translate>[1],
  values?: Record<string, string>,
) => translate("en", key, values);
const viewer = { id: "reader", display_name: "Dana Weiss" };
const task = {
  ...taskRow("one", "Dana Weiss will prepare two options."),
  owner: { kind: "user" as const, id: viewer.id },
};

it("addresses the assigned reader while preserving the stored promise", () => {
  expect(readerTask(task, viewer, t).title).toBe(
    "Your task: prepare two options.",
  );
  expect(task.title).toBe("Dana Weiss will prepare two options.");
});
it("does not confuse names, reassignment, or contact ownership with a promise", () => {
  expect(
    readerTask(
      { ...task, owner: { kind: "user", id: "another-dana" } },
      viewer,
      t,
    ),
  ).toEqual({ ...task, owner: { kind: "user", id: "another-dana" } });
  expect(
    readerTask({ ...task, source: "conversation_claim" }, viewer, t).title,
  ).toBe(task.title);
  expect(
    readerTask({ ...task, title: "Alex will prepare two options." }, viewer, t)
      .title,
  ).toBe("Alex will prepare two options.");
});
it("attributes the original change, including self and legacy unknowns", () => {
  const notice = {
    ...task,
    source: "notice" as const,
    kind: "automation",
    detail: "Qualified → Won",
    notice_origin: {
      event_id: "event",
      actor_type: "human",
      actor_id: "human:reader",
      actor_name: "Dana Weiss",
      occurred_at: "2026-09-13T10:00:00Z",
    },
  };
  expect(noticeDetail(notice, viewer, t)).toBe(
    "Qualified → Won · Changed by: You",
  );
  expect(noticeDetail(notice, undefined, t)).toContain("Dana Weiss");
  expect(
    noticeDetail({ ...notice, notice_origin: undefined }, viewer, t),
  ).toContain("Change author unknown.");
});

it("renders the recorded stages in each language without inventing a missing stage", () => {
  const notice = {
    ...task,
    source: "notice" as const,
    detail: "Outdated message",
    notice_origin: {
      event_id: "event",
      actor_type: "human",
      actor_id: "human:reader",
      occurred_at: "2026-09-07T10:00:00Z",
      stage_change: { from_name: "Qualified", to_name: "Won" },
    },
  };
  expect(noticeDetail(notice, viewer, t)).toBe(
    "Qualified → Won · Changed by: You",
  );
  for (const [language, missing] of [
    ["en", "Unknown stage"],
    ["de", "Unbekannte Phase"],
    ["vi", "Không rõ giai đoạn"],
  ] as const) {
    const translated = noticeDetail(
      {
        ...notice,
        notice_origin: {
          ...notice.notice_origin,
          stage_change: { from_name: "Qualified" },
        },
      },
      viewer,
      (key, values) => translate(language, key, values),
    );
    expect(translated).toContain(`Qualified → ${missing}`);
    expect(translated).not.toContain("Outdated message");
  }
});

it("dates the original change, not its later delivery", () => {
  expect(
    whenText(
      {
        ...task,
        source: "notice",
        occurred_at: "2026-09-14T10:00:00Z",
        notice_origin: {
          event_id: "event",
          actor_type: "human",
          actor_id: "reader",
          occurred_at: "2026-09-07T10:00:00Z",
        },
      },
      t,
      "en",
      "Europe/Berlin",
      "UTC",
      new Date("2026-09-14T10:00:00Z"),
    ),
  ).toBe("07/09/2026, 12:00");
});

it("asks a domain question about its company in the reader's language", () => {
  const question = {
    ...task,
    source: "domain_question" as const,
    domain_reason_code: "forwarded_identity",
    detail: "Stored fallback prose",
  };
  for (const language of ["en", "de", "vi"] as const) {
    expect(
      noticeDetail(question, viewer, (key, values) =>
        translate(language, key, values),
      ),
    ).toBe(
      `${translate(language, "worklist.domainQuestion.headingDomain", { domain: question.id })} ${translate(language, "worklist.domainQuestion.reason.forwarded")}`,
    );
  }
});

it("shows the site triage's own sentence under a vendor question's reason", () => {
  const sentence = "Classified as a mail or hosting vendor: sells hosting.";
  const vendor = {
    ...task,
    source: "domain_question" as const,
    domain_reason_code: "vendor_verdict",
    domain_note: sentence,
    detail: "Stored fallback prose",
  };
  const label = `${translate("en", "worklist.domainQuestion.headingDomain", { domain: vendor.id })} ${translate("en", "worklist.domainQuestion.reason.vendor")}`;
  expect(
    noticeDetail(vendor, viewer, (key, values) => translate("en", key, values)),
  ).toBe(`${label} ${sentence}`);
  expect(
    noticeDetail({ ...vendor, domain_note: undefined }, viewer, (key, values) =>
      translate("en", key, values),
    ),
  ).toBe(label);
});

it("names shared recipients without claiming nobody owns their reply", () => {
  const shared = {
    ...task,
    owner: {
      kind: "shared" as const,
      users: [
        { id: "alice", label: "Alice" },
        { id: "bob", label: "Bob" },
      ],
    },
  };
  expect(sharedResponsibility(shared, "en", t)).toBe(
    "Shared responsibility: Alice and Bob",
  );
  expect(
    sharedResponsibility(
      {
        ...shared,
        owner: {
          kind: "shared",
          users: [{ id: "alice", label: "Alice" }, { id: "bob" }],
        },
      },
      "en",
      t,
    ),
  ).toBe("2 responsible recipients");
  expect(
    sharedResponsibility({ ...shared, owner: { kind: "shared" } }, "en", t),
  ).toBeNull();
  expect(sharedResponsibility(task, "en", t)).toBeNull();
});

it("gives an approval row its proposed values as the supporting line", () => {
  const approval = {
    ...taskRow("approval", "Update record deal: name=Acme, Inc."),
    source: "approval" as const,
    detail: undefined,
    summary_parts: {
      sentence: "Update record deal",
      fields: [{ field: "name", value: "Acme, Inc." }],
      more_fields: 1,
    },
  };
  expect(noticeDetail(approval, viewer, t)).toBe(
    "Name: Acme, Inc. · 1 more not shown",
  );
  expect(
    noticeDetail({ ...approval, summary_parts: undefined }, viewer, t),
  ).toBe(undefined);
});

it("titles an approval row by its sentence, falling back to the whole title", () => {
  const approval = {
    ...taskRow("approval", "Update record deal: name=Acme, Inc."),
    source: "approval" as const,
    summary_parts: {
      sentence: "Update record deal",
      fields: [],
      more_fields: 0,
    },
  };
  expect(sentenceTitled(approval).title).toBe("Update record deal");
  const blank = {
    ...approval,
    summary_parts: { ...approval.summary_parts, sentence: "" },
  };
  expect(sentenceTitled(blank).title).toBe(
    "Update record deal: name=Acme, Inc.",
  );
  expect(sentenceTitled(task)).toBe(task);
});

// A task's row says its agreed time when it has one, and leaves the end of a
// day with no time to the "due today" reason. The server's flag decides, never
// what the clock reads.
it("reads a task's due moment by its time-of-day flag", () => {
  const now = new Date("2026-09-14T08:00:00Z");
  const when = (dueAt: string, due_has_time: boolean) =>
    whenText(
      { ...task, due_at: dueAt, due_has_time },
      t,
      "en",
      "UTC",
      "UTC",
      now,
    );
  expect(when("2026-09-14T23:59:59Z", false)).toBeNull();
  expect(when("2026-09-14T15:00:00Z", false)).toBeNull();
  expect(when("2026-09-16T15:00:00Z", false)).toBe(
    t("worklist.when.due", { when: "16/09/2026" }),
  );
  expect(when("2026-09-14T15:00:00Z", true)).toBe(
    t("worklist.when.due", { when: "15:00" }),
  );
  expect(when("2026-09-16T15:00:00Z", true)).toBe(
    t("worklist.when.due", { when: "16/09/2026, 15:00" }),
  );
});
