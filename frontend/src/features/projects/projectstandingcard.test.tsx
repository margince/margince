/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { StoryProviders } from "../../testing/story-utils";
import {
  aiJudgedProject360,
  deliveringProject360,
} from "./projectstanding.fixtures";
import { ProjectStanding } from "./projectstandingcard";

afterEach(cleanup);

function standing(view: ReturnType<typeof deliveringProject360>) {
  render(
    <StoryProviders>
      <ProjectStanding
        view={view}
        onMove={() => undefined}
        onOpenDocuments={() => undefined}
      />
    </StoryProviders>,
  );
}

describe("a health reading the health job wrote", () => {
  it("carries the agent tag, its reason and an undo", () => {
    standing(aiJudgedProject360());
    expect(screen.getByText("Automated by Project health check")).toBeTruthy();
    expect(
      screen.getByText(
        "Nothing has been filed on this project for at least 30 days.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Undo the AI reading" }),
    ).toBeTruthy();
  });

  it("is not claimed for a user's reading", () => {
    standing(deliveringProject360());
    expect(screen.queryByText("Automated by Project health check")).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Undo the AI reading" }),
    ).toBeNull();
  });
});

describe("a health reading a colleague recorded", () => {
  const reading = (assessedAt: string) =>
    deliveringProject360({
      health: {
        current: {
          id: "h-1",
          project_id: "pr-1",
          state: "at_risk",
          note: "The supplier slipped two weeks.",
          assessed_at: assessedAt,
          source: "human",
          source_author: "Lena Fischer",
          created_at: assessedAt,
          superseded: false,
        },
      },
    });

  it("keeps who assessed it and when beside the note, and claims no CRM source", () => {
    standing(reading("2026-08-12T09:00:00Z"));
    expect(screen.getByText("Assessed 12 Aug by Lena Fischer.")).toBeTruthy();
    expect(screen.getByText("The supplier slipped two weeks.")).toBeTruthy();
    expect(screen.queryByText("From CRM records")).toBeNull();
  });

  it("names the year of a reading made in another year", () => {
    standing(reading("2025-12-02T09:00:00Z"));
    expect(
      screen.getByText("Assessed 2 Dec 2025 by Lena Fischer."),
    ).toBeTruthy();
  });
});

describe("the project's phase ladder", () => {
  it("writes each rung's days and a dash for a rung never reached", () => {
    render(
      <StoryProviders>
        <ProjectStanding
          view={deliveringProject360()}
          onMove={() => undefined}
          onOpenDocuments={() => undefined}
        />
      </StoryProviders>,
    );
    const ladder = within(screen.getByRole("group", { name: "Phase" }));
    expect(ladder.getByText("31 days")).toBeTruthy();
    expect(ladder.getByText("134 days · now")).toBeTruthy();
    // Pursuing was skipped; Closed is a way out and carries no line.
    expect(ladder.getAllByText("—")).toHaveLength(1);
  });
});
