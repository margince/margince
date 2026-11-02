// @vitest-environment happy-dom

import { beforeEach, describe, expect, it, vi } from "vitest";
import { expectReadable } from "../readable.testkit";
import { briefFixture, catchUpFixture, meetingFixture } from "./fixture";

const asked: string[] = [];

vi.mock("../actions", () => ({
  askAssistant: (text: string) => {
    asked.push(text);
  },
}));

const { render } = await import("./main");

function mount(data: unknown): HTMLElement {
  const root = document.createElement("main");
  document.body.replaceChildren(root);
  render(root, data, []);
  return root;
}

const parts = (root: HTMLElement) =>
  [...root.querySelectorAll(".context-part")].map((p) => ({
    title: p.querySelector(".t-eyebrow")?.textContent,
    lines: [...p.querySelectorAll(".context-line")].map((l) => l.textContent),
  }));

beforeEach(() => {
  asked.length = 0;
});

describe("the story card before a meeting", () => {
  it("leads with what the meeting is for, then the plan in the order it is used", () => {
    const root = mount(meetingFixture.data);
    expectReadable(root);
    expect(root.querySelector("h1")?.textContent).toBe("Before the meeting");
    expect(root.querySelector(".panel-head .badge")?.textContent).toBe(
      "Prepared",
    );
    expect(root.querySelector(".intro")?.textContent).toBe(
      "Agree the rollout date for the second site.",
    );
    expect(parts(root)).toEqual([
      {
        title: "Open with",
        lines: ["Thank Ana for the pilot numbers she sent on Monday."],
      },
      {
        title: "Ask",
        lines: [
          "Who signs off the second site?Procurement changed hands in September.",
        ],
      },
      {
        title: "They may ask",
        lines: [
          "Can the price hold into next year?The discount ends on 31 December.",
        ],
      },
      {
        title: "The risk",
        lines: [
          "The budget may move to next year.Avoid: Offering a discount before they ask.",
        ],
      },
      { title: "Still open", lines: ["Send the security questionnaire back"] },
      {
        title: "Lately",
        lines: ["Call with Ana about the pilot results2026-10-27"],
      },
    ]);
  });

  it("still lists what is open when no plan was written", () => {
    const { brief: _, ...bare } = meetingFixture.data as Record<
      string,
      unknown
    >;
    const root = mount(bare);
    expectReadable(root);
    expect(root.querySelector(".panel-head .badge")).toBeNull();
    expect(parts(root).map((p) => p.title)).toEqual(["Still open", "Lately"]);
  });
});

describe("the story card for one record", () => {
  it("counts the recent touches and lists each part that has something in it", () => {
    const root = mount(catchUpFixture.data);
    expectReadable(root);
    expect(root.querySelector(".view-hero-figure")?.textContent).toBe(
      "2recent touches",
    );
    expect(root.querySelector(".view-hero-sentence")?.textContent).toBe(
      "1 thing is still to do on this company.",
    );
    expect(parts(root).map((p) => p.title)).toEqual(["Lately", "Still to do"]);
  });

  it("asks the assistant for the next step on the record", () => {
    const root = mount(catchUpFixture.data);
    root.querySelector<HTMLButtonElement>(".row-action")?.click();
    expect(asked).toEqual([expect.stringContaining("this company")]);
  });
});

describe("the story card for this morning's brief", () => {
  it("counts the deals still to look at among everything on the brief", () => {
    const root = mount(briefFixture.data);
    expectReadable(root);
    expect(root.querySelector(".view-hero-figure")?.textContent).toBe(
      "2deals to look at",
    );
    expect(root.querySelector(".view-ring")?.textContent).toBe("3on the brief");
    expect(root.querySelector(".view-hero-sentence")?.textContent).toContain(
      "Picked from 23 open deals",
    );
  });

  it("says so when the brief is empty", () => {
    const root = mount({ ...(briefFixture.data as object), items: [] });
    expect(root.querySelector(".intro")?.textContent).toBe(
      "Nothing is on the brief today.",
    );
  });
});
