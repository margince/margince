// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  isNoticeOverdue,
  isNoticeResolved,
  mayAssign,
  mayExcuse,
  NOTICE_STATES,
  noticeStateTone,
  UNRESOLVED_NOTICE_STATES,
} from "./noticecases.logic";

describe("which duties are still owed", () => {
  it("keeps a claimed duty on the queue", () => {
    // Somebody taking a case has not discharged it. A queue that dropped a
    // claimed duty would leave it owed, worked and invisible, and the only
    // seat still seeing it would be the one that took it.
    expect(UNRESOLVED_NOTICE_STATES).toContain("assigned");
    expect(isNoticeResolved("assigned")).toBe(false);
  });

  it("keeps a duty whose disclosure bounced on the queue", () => {
    // The subject was not told, so the duty is owed exactly as it was before
    // the message went out. A case resting outside the queue there would read
    // as handled while being worse than an open one, so nobody would look at
    // it again.
    expect(UNRESOLVED_NOTICE_STATES).toContain("delivery_failed");
    expect(isNoticeResolved("delivery_failed")).toBe(false);
  });

  it("keeps a blocked duty on the queue", () => {
    // Blocked says we cannot discharge it yet, not that we stopped owing it.
    expect(UNRESOLVED_NOTICE_STATES).toContain("blocked");
  });

  it("takes every ended duty off it", () => {
    for (const state of [
      "completed",
      "not_required",
      "provided_elsewhere",
      "exempt_with_reason",
    ] as const) {
      expect(isNoticeResolved(state)).toBe(true);
      expect(UNRESOLVED_NOTICE_STATES).not.toContain(state);
    }
  });

  it("derives the owed set from the vocabulary rather than a second list", () => {
    // The count is asserted, not the membership: a ninth state added to
    // NOTICE_STATES without somebody deciding whether a case sitting in it is
    // still owed fails here, which is the drift a hand-written second list
    // hides. The backend's own census works the same way.
    expect(NOTICE_STATES).toHaveLength(9);
    expect(UNRESOLVED_NOTICE_STATES).toHaveLength(5);
  });
});

describe("when a duty reads as overdue", () => {
  const past = "2026-01-01T00:00:00Z";
  const now = Date.parse("2026-06-01T00:00:00Z");

  it("marks a duty whose deadline has passed", () => {
    expect(isNoticeOverdue(past, "open", now)).toBe(true);
  });

  it("leaves a duty whose deadline is ahead", () => {
    expect(isNoticeOverdue("2026-12-01T00:00:00Z", "open", now)).toBe(false);
  });

  it("never marks a duty that has already ended", () => {
    // The deadline a closed case met, or missed, stopped mattering when it
    // closed. Marking it red would ask an officer to act on something already
    // answered.
    for (const state of [
      "completed",
      "exempt_with_reason",
      "provided_elsewhere",
      "not_required",
    ] as const) {
      expect(isNoticeOverdue(past, state, now)).toBe(false);
    }
  });

  it("still marks a claimed duty whose deadline has passed", () => {
    // Somebody having taken it does not stop the clock, and this is the case
    // an officer most needs to see.
    expect(isNoticeOverdue(past, "assigned", now)).toBe(true);
  });
});

describe("what a reader may do to a duty", () => {
  it("offers both actions on a live duty", () => {
    for (const state of ["open", "assigned", "queued", "blocked"] as const) {
      expect(mayAssign(state)).toBe(true);
      expect(mayExcuse(state)).toBe(true);
    }
  });

  it("offers neither on one that has ended", () => {
    // The server refuses both, so offering them would be a button that fails.
    // Re-excusing a discharged duty is the sharper case: the later note would
    // overwrite a real disclosure and then read as the reason it happened.
    for (const state of ["completed", "exempt_with_reason"] as const) {
      expect(mayAssign(state)).toBe(false);
      expect(mayExcuse(state)).toBe(false);
    }
  });
});

describe("how a state reads", () => {
  it("asks the reader to look at an obstacle or a closure somebody chose", () => {
    expect(noticeStateTone("blocked")).toBe("warn");
    expect(noticeStateTone("exempt_with_reason")).toBe("warn");
    expect(noticeStateTone("provided_elsewhere")).toBe("warn");
  });

  it("stays quiet on the ordinary outcome", () => {
    // A duty discharged by a delivered disclosure is what this queue exists to
    // produce. Colouring it would make the ordinary case shout.
    expect(noticeStateTone("completed")).toBeUndefined();
    expect(noticeStateTone("open")).toBeUndefined();
  });
});
