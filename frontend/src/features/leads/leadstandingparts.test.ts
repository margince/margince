import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { UTC_ZONE } from "../../format/timezone";
import type { Translator } from "../../i18n";
import {
  ladderDates,
  leadDims,
  leadFigures,
  leadNext,
  leadScoreReading,
  nextBookedMeeting,
  type ReadingContext,
} from "./leadstandingparts";

type Lead = components["schemas"]["Lead"];
type Activity = components["schemas"]["Activity"];

// Each reading names the fact it is read from, so the fake translator echoes
// the key and its parameters rather than catalog copy.
const t: Translator = (key: string, params?: Record<string, string>) =>
  params ? `${key}:${JSON.stringify(params)}` : key;

const ctx: ReadingContext = {
  t,
  locale: "en",
  zone: UTC_ZONE,
  asOf: "2026-08-13T10:00:00Z",
};

function lead(extra: Partial<Lead> = {}): Lead {
  return {
    id: "l1",
    full_name: "Jonas Weber",
    status: "engaged",
    score: 72,
    source: "webform",
    captured_by: "human:u1",
    created_at: "2026-08-02T08:00:00Z",
    updated_at: "2026-08-09T08:00:00Z",
    ...extra,
  };
}

function activity(extra: Partial<Activity>): Activity {
  return {
    id: "a1",
    kind: "email",
    occurred_at: "2026-08-09T09:00:00Z",
    is_done: true,
    source: "manual",
    captured_by: "human:u1",
    created_at: "2026-08-09T09:00:00Z",
    updated_at: "2026-08-09T09:00:00Z",
    ...extra,
  };
}

describe("ladderDates", () => {
  it("dates each reached rung by its newest move in, and the current one as now", () => {
    const dates = ladderDates(
      lead(),
      [
        { new_value: "contacted", changed_at: "2026-08-04T10:00:00Z" },
        { new_value: "engaged", changed_at: "2026-08-09T10:00:00Z" },
        { new_value: "contacted", changed_at: "2026-08-03T10:00:00Z" },
      ],
      ctx,
    );
    expect(dates).toEqual({
      new: "2 Aug",
      contacted: "4 Aug",
      engaged: 'lead.ladder.enteredNow:{"at":"9 Aug"}',
    });
  });

  // No recorded move means no date: a rung is never dated by a guess.
  it("leaves a rung with no recorded move undated", () => {
    expect(ladderDates(lead({ status: "contacted" }), [], ctx)).toEqual({
      new: "2 Aug",
    });
  });

  it("dates a system-set Engaged by the signal it moved on", () => {
    const dates = ladderDates(
      lead({
        status_set_by: "system",
        qualification_evidence: {
          trigger: "inbound_reply",
          occurred_at: "2026-08-09T09:00:00Z",
        },
      }),
      [],
      ctx,
    );
    expect(dates.engaged).toBe('lead.ladder.enteredNow:{"at":"9 Aug"}');
  });

  it("dates every open rung of a closed lead, with none of them now", () => {
    const dates = ladderDates(
      lead({ status: "promoted" }),
      [
        { new_value: "contacted", changed_at: "2026-08-04T10:00:00Z" },
        { new_value: "engaged", changed_at: "2026-08-09T10:00:00Z" },
      ],
      ctx,
    );
    expect(dates).toEqual({
      new: "2 Aug",
      contacted: "4 Aug",
      engaged: "9 Aug",
    });
  });
});

const factors = [
  { factor: "meeting_booked", points: 24.6, base_points: 25 },
  { factor: "reply", points: 20 },
  { factor: "decision_maker_title", points: 15 },
  { factor: "manual:budget_hint", points: 5, signal_kind: "fact" as const },
];

describe("leadDims", () => {
  it("sums the score's own factors into Fit and Engagement", () => {
    const [fit, engagement] = leadDims(factors, ctx);
    expect(fit).toMatchObject({ value: "+15", tone: "success" });
    expect(engagement).toMatchObject({ value: "+45", tone: "success" });
  });

  // Unread is not "none": a breakdown not in hand says so instead of zero.
  it("says Not read while the breakdown is out, and None counted when it is empty", () => {
    const unread = leadDims(undefined, ctx);
    expect(unread[0]).toMatchObject({
      value: "lead.dims.notRead",
      tone: "neutral",
    });
    const empty = leadDims([], ctx);
    expect(empty[1]).toMatchObject({
      value: "lead.dims.noneCounted",
      tone: "neutral",
    });
  });
});

describe("leadScoreReading", () => {
  it.each([
    [72, "success"],
    [45, "warning"],
    [12, "neutral"],
  ] as const)("tones a score of %i as %s", (value, tone) => {
    expect(leadScoreReading(lead({ score: value }), [], ctx)).toMatchObject({
      score: value,
      tone,
    });
  });

  it("opens every signal with the points it put on the score", () => {
    const { peek } = leadScoreReading(lead(), factors, ctx);
    expect(peek.items.map((item) => item.right)).toEqual([
      "+25",
      "+20",
      "+15",
      "+5",
    ]);
    expect(peek.foot).toBeUndefined();
  });

  it("says a score set by hand is a user's", () => {
    const { peek } = leadScoreReading(
      lead({ score_override_reason: "Met them at a fair" }),
      factors,
      ctx,
    );
    expect(peek.foot).toBe(
      'lead.score.setByHand:{"reason":"Met them at a fair"}',
    );
  });
});

describe("leadFigures", () => {
  it("reads first response, last touch, the next meeting and the company", () => {
    const figures = leadFigures(
      lead({
        first_response_at: "2026-08-02T10:00:00Z",
        last_activity_at: "2026-08-09T09:00:00Z",
        company_name: "Weber Logistik",
      }),
      [
        activity({
          id: "m1",
          kind: "meeting",
          meeting_status: "booked",
          subject: "Depot demo",
          occurred_at: "2026-08-15T09:00:00Z",
        }),
      ],
      ctx,
    );
    expect(figures.map((figure) => [figure.key, figure.value])).toEqual([
      ["first-response", 'lead.figure.respondedIn:{"span":"2h"}'],
      ["last-touch", "9 Aug"],
      ["meeting", "15 Aug"],
      ["company", "Weber Logistik"],
    ]);
    expect(figures[2].detail).toBe("Depot demo");
  });

  it("reads last touch off what has happened, never the meeting still ahead", () => {
    const figures = leadFigures(
      lead({ last_activity_at: "2026-08-15T09:00:00Z" }),
      [
        activity({
          id: "m1",
          kind: "meeting",
          meeting_status: "booked",
          occurred_at: "2026-08-15T09:00:00Z",
        }),
        activity({ id: "e1", occurred_at: "2026-08-09T09:00:00Z" }),
      ],
      ctx,
    );
    expect(figures[1]).toMatchObject({ key: "last-touch", value: "9 Aug" });
  });

  it("trusts the whole history over a stored last activity a task set", () => {
    const figures = leadFigures(
      lead({ last_activity_at: "2026-08-10T09:00:00Z" }),
      [
        activity({
          kind: "task",
          is_done: false,
          occurred_at: "2026-08-10T09:00:00Z",
        }),
      ],
      ctx,
      true,
    );
    expect(figures[1].value).toBe("standing.noneYet");
  });

  it("ignores a stored last activity that lies ahead of the reading", () => {
    const figures = leadFigures(
      lead({ last_activity_at: "2026-08-15T09:00:00Z" }),
      [],
      ctx,
    );
    expect(figures[1].value).toBe("standing.noneYet");
  });

  // Every figure is a word when the lead lacks it, never an empty cell or 0.
  it("says what a new lead does not have yet", () => {
    const figures = leadFigures(lead({ status: "new" }), [], ctx);
    expect(figures.map((figure) => figure.value)).toEqual([
      "standing.noneYet",
      "standing.noneYet",
      "lead.figure.noneBooked",
      "lead.readings.noCompany",
    ]);
  });

  it("reads an overdue first response off the server's clock", () => {
    const [first] = leadFigures(
      lead({
        status: "new",
        sla_state: "breached",
        sla_deadline_at: "2026-08-02T12:00:00Z",
      }),
      [],
      ctx,
    );
    expect(first).toMatchObject({ value: "lead.sla.breached", tone: "danger" });
  });
});

describe("nextBookedMeeting", () => {
  it("takes the soonest booked meeting ahead, never one held or called off", () => {
    const rows = [
      activity({
        id: "late",
        kind: "meeting",
        meeting_status: "booked",
        occurred_at: "2026-08-20T09:00:00Z",
      }),
      activity({
        id: "soon",
        kind: "meeting",
        meeting_status: "booked",
        occurred_at: "2026-08-15T09:00:00Z",
      }),
      activity({
        id: "gone",
        kind: "meeting",
        meeting_status: "canceled",
        occurred_at: "2026-08-14T09:00:00Z",
      }),
      activity({
        id: "past",
        kind: "meeting",
        meeting_status: "booked",
        occurred_at: "2026-08-10T09:00:00Z",
      }),
    ];
    expect(nextBookedMeeting(rows, ctx.asOf)?.id).toBe("soon");
  });
});

describe("leadNext", () => {
  it("asks to qualify an engaged lead with evidence and an address", () => {
    const next = leadNext(
      lead({
        email: "jonas@weber.example",
        open_task_count: 1,
        qualification_evidence: {
          trigger: "meeting_booked",
          occurred_at: "2026-08-09T09:00:00Z",
        },
      }),
      ctx,
    );
    // The factor's own name: an echoed key reads as no label, so
    // `scoreFactorLabel` falls back to the name the server wrote.
    expect(next).toEqual({
      title: "lead.next.ready",
      late: { label: "meeting_booked", tone: "success" },
      sub: "lead.next.readyWhy",
      qualify: true,
      more: 1,
    });
  });

  it("falls back to the next open task, late when its due moment has passed", () => {
    const next = leadNext(
      lead({
        status: "contacted",
        next_task_subject: "Send the price sheet",
        next_task_due_at: "2026-08-10T09:00:00Z",
        open_task_count: 3,
      }),
      ctx,
    );
    expect(next).toMatchObject({
      title: "Send the price sheet",
      late: { label: "co.next.overdue", tone: "danger" },
      qualify: false,
      more: 2,
    });
  });

  it("asks nothing of a closed lead", () => {
    expect(
      leadNext(lead({ archived_at: "2026-08-12T09:00:00Z" }), ctx),
    ).toEqual({ qualify: false, more: 0 });
  });
});
