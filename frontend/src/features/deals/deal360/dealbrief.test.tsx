/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";
import { translate } from "../../../i18n";
import { StoryProviders } from "../../../testing/story-utils";
import { DealBrief } from "./dealbrief";

afterEach(cleanup);

it.each([
  [null, "deal.briefAdd"],
  ["They close the month by hand.", "deal.briefEdit"],
] as const)(
  "opens the deal's fields from the card's one link (brief %s)",
  async (brief, verb) => {
    const user = userEvent.setup();
    const onEdit = vi.fn();
    render(
      <StoryProviders>
        <DealBrief brief={brief} onEdit={onEdit} />
      </StoryProviders>,
    );
    await user.click(
      screen.getByRole("button", { name: translate("en", verb) }),
    );
    expect(onEdit).toHaveBeenCalledOnce();
  },
);

it("says the brief is empty and offers no link to a reader who may not write", () => {
  render(
    <StoryProviders>
      <DealBrief brief="   " />
    </StoryProviders>,
  );
  expect(screen.getByText(translate("en", "deal.briefEmpty"))).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: translate("en", "deal.briefAdd") }),
  ).toBeNull();
});

it("clamps a brief longer than six lines and opens it on request", async () => {
  const user = userEvent.setup();
  const brief = ["one", "two", "three", "four", "five", "six", "seven"].join(
    "\n",
  );
  render(
    <StoryProviders>
      <DealBrief brief={brief} />
    </StoryProviders>,
  );
  const more = screen.getByRole("button", {
    name: translate("en", "deal.briefMore"),
  });
  expect(document.querySelector(".d360-brief-clamped")).toBeTruthy();
  await user.click(more);
  expect(document.querySelector(".d360-brief-clamped")).toBeNull();
});
