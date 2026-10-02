// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { NumberSetting } from "./numbersetting";
import { SettingList, SettingRow } from "./settingrow";

afterEach(cleanup);

const REFUSAL = "Enter a whole number of pages from 1 to 200.";

function renderPages(onCommit: (next: number) => void) {
  render(
    <SettingList>
      <SettingRow
        label="Pages per read"
        description="Most pages one read fetches."
        control={(control) => (
          <NumberSetting
            control={control}
            testId="pages"
            value={60}
            min={1}
            max={200}
            refusal={REFUSAL}
            onCommit={onCommit}
          />
        )}
      />
    </SettingList>,
  );
  return screen.getByRole("textbox", { name: "Pages per read" });
}

describe("NumberSetting", () => {
  it("commits a whole number in range on Enter", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    const box = renderPages(onCommit);
    await user.clear(box);
    await user.type(box, "120{Enter}");
    expect(onCommit).toHaveBeenCalledWith(120);
    expect(box).not.toHaveAttribute("aria-invalid");
  });

  // The reader corrects what they typed instead of retyping it, and hears the
  // rule beside the refusal rather than in place of it.
  it("keeps an out-of-range value in the box with the refusal under it", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    const box = renderPages(onCommit);
    await user.clear(box);
    await user.type(box, "201");
    await user.tab();
    expect(onCommit).not.toHaveBeenCalled();
    expect(box).toHaveValue("201");
    expect(box).toHaveAttribute("aria-invalid", "true");
    expect(box).toHaveAccessibleDescription(
      `Most pages one read fetches. ${REFUSAL}`,
    );
  });

  it.each(["", "1.5", "lots"])(
    "refuses %j as not a whole number",
    async (typed) => {
      const user = userEvent.setup();
      const onCommit = vi.fn();
      const box = renderPages(onCommit);
      await user.clear(box);
      if (typed !== "") {
        await user.type(box, typed);
      }
      await user.tab();
      expect(onCommit).not.toHaveBeenCalled();
      expect(screen.getByText(REFUSAL)).toBeInTheDocument();
    },
  );

  it("treats typing the stored value back as no edit", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    const box = renderPages(onCommit);
    await user.clear(box);
    await user.type(box, "60{Enter}");
    expect(onCommit).not.toHaveBeenCalled();
    expect(screen.queryByText(REFUSAL)).toBeNull();
  });
});
