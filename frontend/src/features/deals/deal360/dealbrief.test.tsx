/** @vitest-environment happy-dom */
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";
import type { RailEdit } from "../../../components/railfield";
import { translate } from "../../../i18n";
import { StoryProviders } from "../../../testing/story-utils";
import { DealBrief } from "./dealbrief";

afterEach(cleanup);

function briefEdit(description: string | null) {
  const save = vi.fn(async () => undefined);
  const edit: RailEdit = {
    kind: "deal",
    record: { id: "d1", version: 1, description },
    fields: [{ key: "description", label: "deal.brief", type: "textarea" }],
    canEdit: true,
    save,
  };
  return { edit, save };
}

it.each([
  [null, "deal.briefAdd"],
  ["They close the month by hand.", "deal.briefEdit"],
] as const)(
  "opens the brief's own editor from the card's one link (brief %s)",
  async (brief, verb) => {
    const user = userEvent.setup();
    const { edit, save } = briefEdit(brief);
    render(
      <StoryProviders>
        <DealBrief brief={brief} edit={edit} />
      </StoryProviders>,
    );
    await user.click(
      screen.getByRole("button", { name: translate("en", verb) }),
    );
    const dialog = await screen.findByRole("dialog", {
      name: translate("en", "deal.brief"),
    });
    const box = within(dialog).getByRole("textbox", {
      name: translate("en", "deal.brief"),
    });
    await user.clear(box);
    await user.type(box, "Payroll by hand");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(save).toHaveBeenCalledWith(
        { description: "Payroll by hand" },
        {},
        expect.anything(),
      ),
    );
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
