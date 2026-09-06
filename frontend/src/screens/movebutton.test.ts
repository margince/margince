// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Whether a decided step has what it needs to be drawn.
//
// Asked BEFORE the slot is, which is the whole reason this is a function and
// not a null return inside the button: a caller that mounts MoveButton
// unconditionally still draws the surrounding action region, and an empty one
// reads as a control that failed to load. MoveButton keeps its own null returns
// for the narrowing each case needs; this decides whether there is a verb at
// all.

import { describe, expect, it } from "vitest";
import { hasMoveControl } from "./movebutton";

describe("whether a move can be drawn", () => {
  it("needs the operand its own verb takes", () => {
    const ACTIVITY = "01a05500-0000-7000-8000-0000000000e1";
    for (const [move, drawable, because] of [
      [
        { action: "create_task", arguments: { subject: "x" } },
        true,
        "the body the server prepared is present",
      ],
      [
        { action: "create_task" },
        false,
        "no body: a click would post {} and only be refused",
      ],
      [
        { action: "open_task", arguments: { activity_id: ACTIVITY } },
        true,
        "names the task to open",
      ],
      [
        { action: "open_task", arguments: {} },
        false,
        "names no task, so the button would 404",
      ],
      [
        { action: "open_meeting_brief", arguments: { activity_id: ACTIVITY } },
        true,
        "names the meeting",
      ],
      [{ action: "open_meeting_brief" }, false, "names no meeting"],
      [
        { action: "draft_email", arguments: { activity_id: ACTIVITY } },
        false,
        "writing to the buyer is the composer's job, not this button's",
      ],
      [{ action: "none" }, false, "the producer's own word for nothing to do"],
      [
        { action: "a_verb_from_a_newer_server" },
        false,
        "an unknown verb draws nothing rather than an unpressable control",
      ],
    ] as const) {
      expect(hasMoveControl(move), `${move.action}: ${because}`).toBe(drawable);
    }
  });

  it("refuses an activity id that is not one", () => {
    // The arguments object is typed open on the wire, so the id can arrive as
    // anything. A non-string reaching the button would render a control that
    // opens nothing.
    for (const raw of [42, null, "", { id: "x" }, []]) {
      expect(
        hasMoveControl({
          action: "open_task",
          arguments: { activity_id: raw },
        }),
        `activity_id ${JSON.stringify(raw)} must not be drawable`,
      ).toBe(false);
    }
  });
});
