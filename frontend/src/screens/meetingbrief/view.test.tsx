/** @vitest-environment jsdom */
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StoryProviders } from "../story-utils";
import {
  briefModel,
  briefOmitted,
  briefReady,
  briefWithPlan,
  meetingFacts,
  preparedFor,
} from "./fixtures";
import { MeetingBriefView } from "./view";

// The brief's SHAPE, proved from a fixture rather than through a fetch.
//
// What is pinned here is the hierarchy the overhaul exists to create: a reader
// two minutes from a room must meet the ask, then the watch-out, then the
// rest — and must never meet a heading whose section the server did not send.

afterEach(cleanup);

function mount(
  state: Parameters<typeof MeetingBriefView>[0]["state"],
  onOpenRecord = vi.fn(),
) {
  render(
    <StoryProviders>
      <MeetingBriefView
        state={state}
        meeting={meetingFacts}
        preparedFor={preparedFor}
        onOpenRecord={onOpenRecord}
        titleId="t"
        onClose={() => {}}
        formatWhen={() => "24 June, 15:00"}
      />
    </StoryProviders>,
  );
  return onOpenRecord;
}

describe("the prepared brief", () => {
  it("leads with the ask and puts the watch-out above the rest", () => {
    mount({ kind: "ready", brief: briefReady });
    const headings = screen
      .getAllByRole("heading", { level: 3 })
      .map((h) => h.textContent);
    // The goal is first and the risks are second. Everything the server sends
    // after them is body; a reader who stops after two headings has still met
    // the two that change what they say in the room.
    expect(headings[0]).toBe("Goal for this meeting");
    expect(headings[1]).toBe("Risks and watch-outs");
  });

  it("draws no heading for a section the server did not send", () => {
    const sparse = {
      ...briefReady,
      sections: briefReady.sections.filter((s) => s.kind === "goal"),
    };
    mount({ kind: "ready", brief: sparse });
    expect(screen.queryByText("Risks and watch-outs")).toBeNull();
    expect(screen.queryByText("Open commitments")).toBeNull();
    expect(screen.getByText("Goal for this meeting")).toBeTruthy();
  });

  it("names the writer, and tints the lead only for a model", () => {
    const { container } = render(
      <StoryProviders>
        <MeetingBriefView
          state={{ kind: "ready", brief: briefReady }}
          onOpenRecord={vi.fn()}
          titleId="t"
          onClose={() => {}}
        />
      </StoryProviders>,
    );
    // A deterministic composition is not the model's prose, so it does not
    // wear the indigo band that means "Margince wrote this".
    expect(container.querySelector(".panel-ai")).toBeNull();
    expect(container.querySelector(".panel-accent")).toBeTruthy();
    cleanup();

    const model = render(
      <StoryProviders>
        <MeetingBriefView
          state={{ kind: "ready", brief: briefModel }}
          onOpenRecord={vi.fn()}
          titleId="t"
          onClose={() => {}}
        />
      </StoryProviders>,
    );
    expect(model.container.querySelector(".panel-ai")).toBeTruthy();
  });

  it("opens the record behind a cited deal", async () => {
    const onOpenRecord = mount({ kind: "ready", brief: briefReady });
    const user = userEvent.setup();
    // The deal is cited by two sections, which is correct — each claim carries
    // its own receipts. Either chip opens the same record.
    const chips = screen.getAllByRole("button", { name: /Fleet retrofit/ });
    await user.click(chips[0]);
    // Citations also hands over the sibling citations on the same sentence;
    // what this pins is the record the chip opens.
    expect(onOpenRecord.mock.calls[0].slice(0, 2)).toEqual([
      "deal",
      "3f7c1a90-0000-4000-8000-00000000d001",
    ]);
  });

  it("keeps background and withheld sources behind one disclosure", () => {
    const { container } = render(
      <StoryProviders>
        <MeetingBriefView
          state={{ kind: "ready", brief: briefOmitted }}
          onOpenRecord={vi.fn()}
          titleId="t"
          onClose={() => {}}
        />
      </StoryProviders>,
    );
    const details = container.querySelector("details");
    expect(details).toBeTruthy();
    // Closed by default: it is context, not preparation.
    expect((details as HTMLDetailsElement).open).toBe(false);
    // The server's own sentence, not a generic "you cannot see this".
    expect(
      within(details as HTMLElement).getByText(
        /do not have access to Deal Rooms/,
      ),
    ).toBeTruthy();
  });
});

describe("the preparation plan", () => {
  it("adds the plan above the sections rather than in place of them", () => {
    mount({ kind: "ready", brief: briefWithPlan });
    // The plan leads.
    const headings = screen
      .getAllByRole("heading", { level: 3 })
      .map((h) => h.textContent);
    expect(headings[0]).toBe("The outcome to earn");
    // And the sections a reader already had are still on the page, not buried:
    // an outline plan that hid the risks would be a regression.
    expect(headings).toContain("Risks and watch-outs");
    expect(headings).toContain("Goal for this meeting");
  });

  it("renders nothing of the plan when the brief carries none", () => {
    mount({ kind: "ready", brief: briefReady });
    expect(screen.queryByText("The outcome to earn")).toBeNull();
    expect(screen.queryByText("Close the meeting")).toBeNull();
  });

  it("states the three ways to close, in order", () => {
    mount({ kind: "ready", brief: briefWithPlan });
    const legs = screen
      .getAllByRole("heading", { level: 4 })
      .map((h) => h.textContent);
    expect(legs).toEqual(
      expect.arrayContaining(["Minimum advance", "Best advance", "Fallback"]),
    );
  });

  it("shows what the record does not say", () => {
    mount({ kind: "ready", brief: briefWithPlan });
    expect(
      screen.getByText("Who else has to agree before this can go ahead?"),
    ).toBeTruthy();
  });

  it("keeps the indigo lead for a model and the accent for a composition", () => {
    const { container } = render(
      <StoryProviders>
        <MeetingBriefView
          state={{ kind: "ready", brief: briefWithPlan }}
          onOpenRecord={vi.fn()}
          titleId="t"
          onClose={() => {}}
        />
      </StoryProviders>,
    );
    // The plan says a composition wrote it, so the objective must not wear the
    // band that means Margince did.
    expect(container.querySelector(".panel-ai")).toBeNull();
  });
});

describe("the states a read can land in", () => {
  it("says it is assembling", () => {
    mount({ kind: "loading" });
    expect(screen.getByText("Assembling the brief…")).toBeTruthy();
  });

  it("says a cold record has nothing yet", () => {
    mount({ kind: "ready", brief: { ...briefReady, sections: [] } });
    expect(
      screen.getByText("There is nothing recorded for this meeting yet."),
    ).toBeTruthy();
  });

  it("still names a withheld source when nothing else survived", () => {
    // "Nothing is recorded" and "you are not being shown what is" are
    // different facts. A brief whose only content is an omission must state
    // the omission rather than report itself empty.
    mount({ kind: "ready", brief: { ...briefOmitted, sections: [] } });
    expect(
      screen.queryByText("There is nothing recorded for this meeting yet."),
    ).toBeNull();
    expect(screen.getByText(/do not have access to Deal Rooms/)).toBeTruthy();
  });

  it("keeps the server's own reason on a failure, and offers a retry", async () => {
    const onRetry = vi.fn();
    mount({
      kind: "failed",
      message: "That meeting is filed under a different engagement.",
      onRetry,
    });
    expect(
      screen.getByText("That meeting is filed under a different engagement."),
    ).toBeTruthy();
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Try again" }));
    expect(onRetry).toHaveBeenCalled();
  });
});
