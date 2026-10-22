// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider, translate } from "../i18n";
import { NumberSetting, NumberSettingRow } from "./numbersetting";
import { PageSaveBar, SavePageProvider } from "./savepage";
import { SettingList, SettingRow } from "./settingrow";

afterEach(cleanup);

const REFUSAL = "Enter a whole number of pages from 1 to 200.";

function pages(value: number, onCommit: (next: number) => unknown) {
  return (
    <SettingList>
      <SettingRow
        label="Pages per read"
        description="Most pages one read fetches."
        control={(control) => (
          <NumberSetting
            control={control}
            testId="pages"
            value={value}
            min={1}
            max={200}
            refusal={REFUSAL}
            onCommit={onCommit}
          />
        )}
      />
    </SettingList>
  );
}

function renderPages(onCommit: (next: number) => void) {
  render(pages(60, onCommit));
  return screen.getByRole("textbox", { name: "Pages per read" });
}

describe("NumberSetting", () => {
  it("says why the reader may not change it, and refuses the edit on that alone", () => {
    render(
      <SettingList>
        <SettingRow
          label="Pages per read"
          description="Most pages one read fetches."
          control={(control) => (
            <NumberSetting
              control={control}
              value={60}
              min={1}
              max={200}
              refusal={REFUSAL}
              reason="Only an administrator can change this."
              onCommit={vi.fn()}
            />
          )}
        />
      </SettingList>,
    );
    const box = screen.getByRole("textbox", { name: "Pages per read" });
    expect(box).toBeDisabled();
    expect(box).toHaveAccessibleDescription(
      "Most pages one read fetches. Only an administrator can change this.",
    );
  });

  it("commits a whole number in range on Enter", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    const box = renderPages(onCommit);
    await user.clear(box);
    await user.type(box, "120{Enter}");
    expect(onCommit).toHaveBeenCalledWith(120);
    expect(box).not.toHaveAttribute("aria-invalid");
  });

  it("commits nothing on an input method's Enter", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    const box = renderPages(onCommit);
    await user.clear(box);
    await user.type(box, "120");

    fireEvent.keyDown(box, { key: "Enter", keyCode: 229 });

    expect(onCommit).not.toHaveBeenCalled();
  });

  it("names the caller's element after the row's description, and only once given", () => {
    const row = (describedBy?: string) => (
      <SettingList>
        <NumberSettingRow
          label="Pages per read"
          description="Most pages one read fetches."
          value={60}
          min={1}
          max={200}
          refusal={REFUSAL}
          describedBy={describedBy}
          onCommit={vi.fn()}
        />
      </SettingList>
    );
    const { rerender } = render(
      <>
        {row()}
        <p id="card-refusal">The save was refused.</p>
      </>,
    );
    const box = screen.getByRole("textbox", { name: "Pages per read" });
    expect(box).toHaveAccessibleDescription("Most pages one read fetches.");
    rerender(
      <>
        {row("card-refusal")}
        <p id="card-refusal">The save was refused.</p>
      </>,
    );
    expect(box).toHaveAccessibleDescription(
      "Most pages one read fetches. The save was refused.",
    );
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

  it.each(["", "1.5", "lots", "1e2", "0x10", "-5"])(
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

  it.each([
    ["1", 1],
    ["200", 200],
  ])("commits the bound %s itself", async (typed, want) => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    const box = renderPages(onCommit);
    await user.clear(box);
    await user.type(box, `${typed}{Enter}`);
    expect(onCommit).toHaveBeenCalledWith(want);
  });

  it("refuses a value under the lower bound", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    const box = renderPages(onCommit);
    await user.clear(box);
    await user.type(box, "0{Enter}");
    expect(onCommit).not.toHaveBeenCalled();
    expect(screen.getByText(REFUSAL)).toBeInTheDocument();
  });

  // A sweep whose 0 means off: 0 and lowestOn..max are the choices, and the
  // gap between them is refused like any other value out of range.
  it("refuses the gap between off and the lowest value that switches it on", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    render(
      <SettingList>
        <SettingRow
          label="Sweep"
          description="Seconds, or 0 for off."
          control={(control) => (
            <NumberSetting
              control={control}
              value={3600}
              min={0}
              max={604_800}
              lowestOn={300}
              refusal={REFUSAL}
              onCommit={onCommit}
            />
          )}
        />
      </SettingList>,
    );
    const box = screen.getByRole("textbox", { name: "Sweep" });
    await user.clear(box);
    await user.type(box, "299{Enter}");
    expect(onCommit).not.toHaveBeenCalled();
    await user.clear(box);
    await user.type(box, "0{Enter}");
    expect(onCommit).toHaveBeenCalledWith(0);
    await user.clear(box);
    await user.type(box, "300{Enter}");
    expect(onCommit).toHaveBeenCalledWith(300);
  });

  // The blur after an Enter lands while the first save is still in flight, so
  // the stored value has not moved yet and only the draft can tell it was sent.
  it("sends one save for an Enter followed by leaving the box", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    const box = renderPages(onCommit);
    await user.clear(box);
    await user.type(box, "120{Enter}");
    await user.tab();
    expect(onCommit).toHaveBeenCalledTimes(1);
  });

  // A reading under a draft the field then refuses would vouch for the draft.
  it.each(["1e2", "0x10", "-5", "0", "201", "12.5"])(
    "reads no words under %j, which the field refuses",
    async (typed) => {
      const user = userEvent.setup();
      render(
        <SettingList>
          <SettingRow
            label="Pages per read"
            description="Most pages one read fetches."
            control={(control) => (
              <NumberSetting
                control={control}
                value={60}
                min={1}
                max={200}
                refusal={REFUSAL}
                reading={(count) => `${count} pages in words`}
                onCommit={vi.fn()}
              />
            )}
          />
        </SettingList>,
      );
      const box = screen.getByRole("textbox", { name: "Pages per read" });
      expect(screen.getByText("60 pages in words")).toBeInTheDocument();
      await user.clear(box);
      await user.type(box, typed);
      expect(screen.queryByText(/pages in words/)).toBeNull();
      await user.clear(box);
      await user.type(box, "120");
      expect(screen.getByText("120 pages in words")).toBeInTheDocument();
    },
  );

  it("shows a newer stored value instead of an older draft", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    const { rerender } = render(pages(60, onCommit));
    const box = screen.getByRole("textbox", { name: "Pages per read" });
    await user.clear(box);
    await user.type(box, "120");
    rerender(pages(90, onCommit));
    expect(box).toHaveValue("90");
    await user.tab();
    expect(onCommit).not.toHaveBeenCalled();
  });
});

describe("NumberSetting on a page with a save bar", () => {
  function onPage(onCommit: (next: number) => Promise<void>) {
    render(
      <LocaleProvider initial="en">
        <SavePageProvider>
          {pages(60, onCommit)}
          <PageSaveBar />
        </SavePageProvider>
      </LocaleProvider>,
    );
    return screen.getByRole("textbox", { name: "Pages per read" });
  }
  const save = () =>
    screen.getByRole("button", { name: translate("en", "saveBar.save") });

  it("holds the typed number as a draft until the bar saves it", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn(async () => undefined);
    const box = onPage(onCommit);
    await user.clear(box);
    await user.type(box, "120{Enter}");
    await user.tab();
    expect(onCommit).not.toHaveBeenCalled();

    await user.click(save());
    expect(onCommit).toHaveBeenCalledExactlyOnceWith(120);
  });

  it("keeps a number out of range away from the bar's save", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn(async () => undefined);
    const box = onPage(onCommit);
    await user.clear(box);
    await user.type(box, "500");
    await user.tab();
    await user.click(save());

    expect(onCommit).not.toHaveBeenCalled();
    expect(screen.getByText(REFUSAL)).toBeInTheDocument();
  });
});
