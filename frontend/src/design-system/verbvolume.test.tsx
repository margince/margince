/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { Button } from "./atoms";
import { QuietVerbs } from "./verbvolume";

afterEach(cleanup);

function face(name: string): string[] {
  return screen.getByRole("button", { name }).className.split(" ");
}

function marked(name: string): boolean {
  return (
    screen.getByRole("button", { name }).querySelector(".lucide-sparkles") !==
    null
  );
}

describe("a control's volume follows its region", () => {
  it("fills primary and ai outside a quiet region", () => {
    render(
      <>
        <Button variant="primary">Save</Button>
        <Button variant="ai">Draft reply</Button>
      </>,
    );
    expect(face("Save")).toContain("btn-primary");
    expect(face("Draft reply")).toContain("btn-ai");
  });

  it("draws both outlined inside one, and keeps danger red", () => {
    render(
      <QuietVerbs>
        <Button variant="primary">Mark done</Button>
        <Button variant="ai">Draft reply</Button>
        <Button variant="danger">Delete</Button>
      </QuietVerbs>,
    );
    expect(face("Mark done")).toContain("btn-ghost");
    expect(face("Draft reply")).toEqual(
      expect.arrayContaining(["btn-ghost", "btn-aiQuiet"]),
    );
    expect(face("Delete")).toContain("btn-danger");
  });

  it("keeps the fill where the region says it is the card in hand", () => {
    render(
      <QuietVerbs quiet={false}>
        <Button variant="ai">Draft reply</Button>
      </QuietVerbs>,
    );
    expect(face("Draft reply")).toContain("btn-ai");
  });
});

describe("an AI verb draws its own mark", () => {
  it("draws the ✦ on ai and aiQuiet, filled or quieted", () => {
    render(
      <>
        <Button variant="ai">Draft reply</Button>
        <Button variant="aiQuiet">Shorter</Button>
        <QuietVerbs>
          <Button variant="ai">Prepare</Button>
        </QuietVerbs>
        <Button variant="primary">Save</Button>
      </>,
    );
    expect(marked("Draft reply")).toBe(true);
    expect(marked("Shorter")).toBe(true);
    expect(marked("Prepare")).toBe(true);
    expect(marked("Save")).toBe(false);
  });
});
