/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { meetingRow, readingsDay } from "../../testing/brief.fixtures";
import { jsonResponse, render, stubApi } from "../../testing/brief.testkit";
import { BriefScreen } from "./brief";
import { BriefFeed } from "./brief.feed";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

// Every number on Home says what it counts, so a reader never guesses whether
// the cards, the Worklist or the urgent work is meant.
describe("Home's counts", () => {
  it("names what the Worklist switch counts", async () => {
    const day = readingsDay({}, [
      meetingRow("m1", false),
      meetingRow("m2", true),
    ]);
    stubApi({ "GET /worklist": () => jsonResponse(day) });
    render(<BriefScreen />);
    const toggle = await screen.findByRole("button", {
      name: /Show Worklist\W+2 items in the Worklist/,
    });
    expect(toggle.querySelector(".optioncount")?.getAttribute("title")).toBe(
      "2 items in the Worklist",
    );
  });

  it("says nothing beside the head when every card of the day is drawn", () => {
    stubApi({});
    const { container } = render(
      <BriefFeed
        day={readingsDay({}, [meetingRow("m1", false)])}
        state="ready"
      />,
    );
    expect(container.querySelector(".brief-first-count")).toBeNull();
  });
});
