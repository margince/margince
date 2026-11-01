import { expect, it } from "vitest";
import { translate } from "../../i18n";
import { whenText } from "../../lib/worklistcopy";
import { taskRow } from "../../testing/brief.fixtures";
import {
  noticeDetail,
  readerTask,
  sharedResponsibility,
} from "./worklist.reader";

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

it("renders domain maintenance reasons in the reader's language", () => {
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
    ).toBe(translate(language, "blockedDomains.source.forwardedIdentity"));
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
  const label = translate("en", "blockedDomains.source.vendorVerdict");
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
