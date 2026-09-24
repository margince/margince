import { expect, it } from "vitest";
import { translate } from "../i18n";
import { taskRow } from "./brief.fixtures";
import { whenText } from "./worklist.copy";
import { noticeDetail, readerTask } from "./worklist.reader";

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
    "You need to prepare two options.",
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
    ["vi", "Giai đoạn không rõ"],
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
