/** @vitest-environment happy-dom */
import { isValidElement } from "react";
import { expect, it } from "vitest";
import { KNOWN_SOURCES, rowHref } from "../../lib/worklistcopy";
import { row } from "../../testing/worklist.testkit";
import { reviewView } from "./worklist.destinations";
import { rowActivityId, rowSourceUnavailable } from "./worklist.noticesource";
import type { WorklistItem } from "./worklist.queries";
import { rowAnswer } from "./worklist.row.answer";

type ContextCase = {
  item: WorklistItem;
  via: "route" | "activity" | "inline" | "group";
};
function record(
  source: WorklistItem["source"],
  type: NonNullable<WorklistItem["subject"]>["type"],
): ContextCase {
  return {
    item: row({
      source,
      actions: ["open"],
      subject: { type, id: "record-one", label: "Named record" },
    }),
    via: "route",
  };
}
function activity(source: WorklistItem["source"]): ContextCase {
  return {
    item: row({ source, id: "activity-one", actions: ["open"] }),
    via: "activity",
  };
}
function queue(source: WorklistItem["source"]): ContextCase {
  return { item: row({ source }), via: "route" };
}

const CONTEXT = {
  approval: {
    item: row({ source: "approval", actions: ["decide"] }),
    via: "inline",
  },
  dedupe_candidate: {
    item: row({
      source: "dedupe_candidate",
      actions: ["merge"],
      pair: {
        evidence: [],
        left: { id: "one", label: "First contact" },
        right: { id: "two", label: "Second contact" },
      },
    }),
    via: "inline",
  },
  contact_case: {
    item: row({
      source: "contact_case",
      actions: ["merge"],
      contact_case: {
        anchor_id: "one",
        revision: "r1",
        members: [
          { id: "one", label: "First contact" },
          { id: "two", label: "Second contact" },
        ],
        owners: ["Ana"],
        can_merge: true,
        merge_mode: "ordinary",
      },
    }),
    via: "inline",
  },
  deal_suggestion: record("deal_suggestion", "company"),
  tag_suggestion: record("tag_suggestion", "contact"),
  task: activity("task"),
  brief_item: record("brief_item", "deal"),
  conversation_claim: {
    item: row({
      source: "conversation_claim",
      actions: ["open"],
      subject: { type: "contact", id: "buyer", label: "Mira" },
      source_record: {
        type: "activity",
        id: "promise-source",
        label: "Pricing call",
      },
    }),
    via: "activity",
  },
  customer_waiting: record("customer_waiting", "contact"),
  lead_response: record("lead_response", "lead"),
  deal_at_risk: record("deal_at_risk", "deal"),
  meeting: activity("meeting"),
  relationship_decay: record("relationship_decay", "contact"),
  failed_approval: record("failed_approval", "contact"),
  dsr: queue("dsr"),
  notice_case: record("notice_case", "contact"),
  capture_health: queue("capture_health"),
  domain_question: {
    item: row({
      source: "domain_question",
      id: "example.test",
      actions: ["keep", "discard"],
    }),
    via: "inline",
  },
  ai_work_health: queue("ai_work_health"),
  bounce: record("bounce", "contact"),
  undelivered: record("undelivered", "contact"),
  automation_run: queue("automation_run"),
  notice: {
    item: row({
      source: "notice",
      actions: ["open"],
      subject: { type: "activity", id: "source", label: "Review call" },
    }),
    via: "activity",
  },
  introduction_request: record("introduction_request", "contact"),
  meeting_outcome: activity("meeting_outcome"),
  weekly_commitment: {
    item: row({ source: "weekly_commitment" }),
    via: "inline",
  },
  awaiting_reply: record("awaiting_reply", "contact"),
  meeting_follow_up: record("meeting_follow_up", "contact"),
  batch: {
    item: row({
      source: "batch",
      category: "decisions",
      batch: { key: "uncertain_contact", count: 2 },
    }),
    via: "group",
  },
} satisfies Record<WorklistItem["source"], ContextCase>;

it("covers the generated source vocabulary without shortening it", () => {
  expect(Object.keys(CONTEXT).sort()).toEqual(
    Object.keys(KNOWN_SOURCES).sort(),
  );
});
it.each(Object.entries(CONTEXT))(
  "gives %s a working context destination",
  (source, { item, via }) => {
    expect(item.source).toBe(source);
    expect(rowSourceUnavailable(item)).toBe(false);
    if (via === "route") {
      expect(rowHref(item)).toMatch(/^#\//);
    } else if (via === "activity") {
      expect(rowActivityId(item)).toBeTruthy();
    } else if (via === "group") {
      expect(reviewView(item)).toBe("decisions");
    } else {
      const answer = rowAnswer(item, {
        working: false,
        offered: () => false,
        answer: () => {
          throw new Error("A context read must not write");
        },
      });
      expect(
        [answer.primary, answer.equals, answer.below].some(isValidElement),
      ).toBe(true);
    }
  },
);
it.each(Object.keys(CONTEXT))(
  "reads a typed %s activity reference even without a subject line",
  (source) => {
    const fixture = Object.values(CONTEXT).find(
      ({ item }) => item.source === source,
    );
    if (!fixture) throw new Error("Missing census fixture");
    const item = {
      ...fixture.item,
      source_record: { type: "activity", id: "unnamed-source" },
    } satisfies WorklistItem;
    if (["task", "meeting", "meeting_outcome"].includes(source)) {
      expect(rowActivityId(item)).toBe(item.id);
    } else {
      expect(rowActivityId(item)).toBe("unnamed-source");
      expect(rowSourceUnavailable(item)).toBe(false);
    }
  },
);
