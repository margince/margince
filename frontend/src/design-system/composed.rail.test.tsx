// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { RecordView } from "./composed";

afterEach(cleanup);

// The rail says who wrote before a reader reads a word: a change the agent
// made carries the indigo mark, a change a person made the hollow one, and a
// thing that was said the solid one. Held on the class the sheet colours by.
describe("the timeline's rail marks who wrote", () => {
  it("draws an agent's change indigo, a person's hollow, and an exchange solid", () => {
    const { container } = render(
      <RecordView
        name="Anna Weber"
        zone="Europe/Berlin"
        timeline={[
          {
            id: "agent",
            kind: "change",
            title: "Owner set to Carol",
            atIso: "2026-06-12T09:00:00Z",
            provenance: { kind: "agent", agent: "capture" },
          },
          {
            id: "human",
            kind: "change",
            title: "Lifecycle set to Customer",
            atIso: "2026-06-13T09:00:00Z",
            provenance: { kind: "human", self: true },
          },
          {
            id: "said",
            kind: "email",
            title: "Re: fleet retrofit offer",
            atIso: "2026-06-14T09:00:00Z",
            provenance: { kind: "human", self: true },
          },
        ]}
      />,
    );
    const marks = Array.from(container.querySelectorAll(".tl-dot")).map(
      (dot) => dot.className,
    );
    expect(marks).toEqual([
      "tl-dot tl-dot-agent",
      "tl-dot tl-dot-quiet",
      "tl-dot",
    ]);
  });
});
