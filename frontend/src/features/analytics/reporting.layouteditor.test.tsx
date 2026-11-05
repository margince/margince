/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, it } from "vitest";
import { StoryProviders } from "../../testing/story-utils";
import type { ReportingLayout } from "./reporting.layout";
import { LayoutEditor } from "./reporting.layouteditor";
import { reportingCatalogClient } from "./reporting.story-fixtures";

afterEach(cleanup);

const twoSections: ReportingLayout = [
  {
    title: "Pipeline",
    cards: [
      { id: "a", kind: "stage_distribution", width: "half" },
      {
        id: "b",
        kind: "bookings_trend",
        metric: "bookings_won",
        width: "half",
      },
    ],
  },
  {
    title: "Targets",
    cards: [{ id: "c", kind: "target_progress", width: "full" }],
  },
];

let latest: ReportingLayout = twoSections;

function Harness({
  start = twoSections,
}: Readonly<{ start?: ReportingLayout }>) {
  const [layout, setLayout] = useState(start);
  const [client] = useState(reportingCatalogClient);
  return (
    <StoryProviders client={client}>
      <LayoutEditor
        layout={layout}
        choices={[]}
        onChange={(next) => {
          latest = next;
          setLayout(next);
        }}
      />
    </StoryProviders>
  );
}

const order = () =>
  latest.map((section) => section.cards.map((card) => card.id));

it("moves a card within its section by the arrow keys on its handle", async () => {
  const user = userEvent.setup();
  render(<Harness />);
  screen.getByRole("button", { name: /^Move .*, 1 of 2$/ }).focus();
  await user.keyboard("{ArrowDown}");
  expect(order()).toEqual([["b", "a"], ["c"]]);
});

it("moves a section down, and offers no move past either end", async () => {
  const user = userEvent.setup();
  render(<Harness />);
  expect(
    screen.getByRole("button", { name: "Move up: Section 1" }),
  ).toBeDisabled();
  expect(
    screen.getByRole("button", { name: "Move down: Section 2" }),
  ).toBeDisabled();
  await user.click(
    screen.getByRole("button", { name: "Move down: Section 1" }),
  );
  expect(latest.map((section) => section.title)).toEqual([
    "Targets",
    "Pipeline",
  ]);
});

it("keeps focus with a section it moves, so pressing again moves it further", async () => {
  const user = userEvent.setup();
  const three: ReportingLayout = [
    ...twoSections,
    {
      title: "Outcomes",
      cards: [{ id: "d", kind: "bookings_trend", width: "full" }],
    },
  ];
  render(<Harness start={three} />);
  screen.getByRole("button", { name: "Move down: Section 1" }).focus();
  await user.keyboard("{Enter}");
  await user.keyboard("{Enter}");
  expect(latest.map((section) => section.title)).toEqual([
    "Targets",
    "Outcomes",
    "Pipeline",
  ]);
});
