/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ReactNode, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { weekdayInitial } from "../format/format";
import { setDateTimePreferences } from "../format/preferences";
import { type Locale, LocaleProvider } from "../i18n";
import { Button } from "./button";
import { DateInput, type DateInputProps, type ISODate } from "./dateinput";
import { Field } from "./field";
import { Heading } from "./heading";
import { Modal } from "./modal";

afterEach(() => {
  cleanup();
  setDateTimePreferences({ dateFormat: "locale", timeFormat: "locale" });
});

const TODAY = "2026-05-20";

function Due(
  props: Readonly<
    Omit<DateInputProps, "value" | "onChange"> & {
      locale?: Locale;
      initial?: ISODate | "";
      hint?: string;
      onReport?: (day: string) => void;
    }
  >,
) {
  const { locale = "en", initial = "", hint, onReport, ...rest } = props;
  const [day, setDay] = useState<ISODate | "">(initial);
  return (
    <LocaleProvider initial={locale}>
      <Field label="Due" hint={hint}>
        {(control) => (
          <DateInput
            {...control}
            {...rest}
            today={TODAY}
            value={day}
            onChange={(event) => {
              setDay(event.target.value);
              onReport?.(event.target.value);
            }}
          />
        )}
      </Field>
    </LocaleProvider>
  );
}

const box = () =>
  screen.getByRole<HTMLInputElement>("textbox", { name: "Due" });
const CHOOSE = "Choose date: Due";

describe("the face", () => {
  it("writes the day the Vietnamese way and opens a Vietnamese month that starts on Monday", async () => {
    const user = userEvent.setup();
    render(<Due locale="vi" initial="2026-05-06" />);

    expect(box().value).toBe("06/05/2026");
    expect(box().placeholder).toBe("dd/mm/yyyy");

    await user.click(screen.getByRole("button", { name: "Chọn ngày: Due" }));

    const dialog = await screen.findByRole("dialog", {
      name: "Chọn ngày: Due",
    });
    expect(within(dialog).getByText("Tháng 5 năm 2026")).toBeTruthy();
    const legend = dialog.querySelector(
      '.calendar-week[aria-hidden="true"] > span',
    );
    expect(legend?.textContent).toBe(
      weekdayInitial(new Date(2026, 4, 4), "vi"),
    );
    expect(
      within(dialog).getByRole("button", { name: "Hôm nay" }),
    ).toBeTruthy();
    expect(within(dialog).getByRole("button", { name: "Xóa" })).toBeTruthy();
  });

  it("follows the installation's month-first format over the UI language", () => {
    setDateTimePreferences({ dateFormat: "mdy", timeFormat: "locale" });
    render(<Due locale="de" initial="2026-05-06" />);

    expect(box().value).toBe("05/06/2026");
  });

  it("submits the ISO day under the caller's name", () => {
    const { container } = render(<Due initial="2026-05-06" name="due" />);

    const sent = container.querySelector<HTMLInputElement>('input[name="due"]');
    expect(sent?.value).toBe("2026-05-06");
    expect(box().name).toBe("");
  });
});

describe("typing", () => {
  it("reports a German day.month.year once the reader leaves the box, written in the pattern", async () => {
    const user = userEvent.setup();
    const report = vi.fn();
    render(<Due locale="de" onReport={report} />);

    await user.type(box(), "6.5.2026");
    expect(report).not.toHaveBeenCalled();
    expect(box().checkValidity()).toBe(true);
    await user.tab();

    expect(report).toHaveBeenCalledTimes(1);
    expect(report).toHaveBeenCalledWith("2026-05-06");
    expect(box().value).toBe("06.05.2026");
  });

  it("reads month first under the mdy preference, on Enter", async () => {
    setDateTimePreferences({ dateFormat: "mdy", timeFormat: "locale" });
    const user = userEvent.setup();
    const report = vi.fn();
    render(<Due onReport={report} />);

    await user.type(box(), "5/6/2026{Enter}");

    expect(report).toHaveBeenCalledWith("2026-05-06");
  });

  it("does not settle on the Enter that confirms an input method candidate", () => {
    const report = vi.fn();
    render(<Due onReport={report} />);
    fireEvent.change(box(), { target: { value: "06.05.2026" } });

    fireEvent.keyDown(box(), { key: "Enter", isComposing: true });
    fireEvent.keyDown(box(), { key: "Enter", keyCode: 229 });
    expect(report).not.toHaveBeenCalled();

    fireEvent.keyDown(box(), { key: "Enter" });
    expect(report).toHaveBeenCalledWith("2026-05-06");
  });

  it("keeps a day it cannot read, marks it invalid, and leaves the caller no day", async () => {
    const user = userEvent.setup();
    const report = vi.fn();
    render(<Due initial="2026-05-06" onReport={report} />);

    await user.tripleClick(box());
    await user.keyboard("31/02/2026");
    await user.tab();

    expect(report).toHaveBeenLastCalledWith("");
    expect(report).not.toHaveBeenCalledWith("2026-05-06");
    expect(box().value).toBe("31/02/2026");
    expect(box().getAttribute("aria-invalid")).toBe("true");
    expect(box().validationMessage).toBe("Enter a date as DD/MM/YYYY.");
  });

  it("is invalid from the key press that makes the text unreadable, with nothing reported", async () => {
    const user = userEvent.setup();
    const report = vi.fn();
    render(<Due initial="2026-05-06" onReport={report} />);

    await user.click(box());
    await user.keyboard("{End}{Backspace}");

    expect(box().checkValidity()).toBe(false);
    expect(box().validationMessage).toBe("Enter a date as DD/MM/YYYY.");
    expect(report).not.toHaveBeenCalled();
  });

  it("reads a year below 1000 as no day", async () => {
    const user = userEvent.setup();
    render(<Due />);

    await user.type(box(), "09/09/0099{Enter}");

    expect(box().getAttribute("aria-invalid")).toBe("true");
    expect(box().validationMessage).toBe("Enter a date as DD/MM/YYYY.");
  });

  it("reports an emptied box as no date", async () => {
    const user = userEvent.setup();
    const report = vi.fn();
    render(<Due initial="2026-05-06" onReport={report} />);

    await user.clear(box());
    await user.tab();

    expect(report).toHaveBeenCalledWith("");
  });

  it("refuses a typed day outside min and max", async () => {
    const user = userEvent.setup();
    const report = vi.fn();
    render(<Due min="2026-05-10" max="2026-05-30" onReport={report} />);

    await user.type(box(), "09/05/2026");
    await user.tab();

    expect(report).not.toHaveBeenCalled();
    expect(box().getAttribute("aria-invalid")).toBe("true");
    expect(box().validationMessage).toBe("Earliest date: 10/05/2026");
  });
});

// As callers that act on a day write it: an emptied box asks for nothing.
function KeepsItsDay(props: Readonly<{ onReport: (day: string) => void }>) {
  const [day, setDay] = useState<ISODate | "">("2026-05-06");
  return (
    <LocaleProvider initial="en">
      <Field label="Due">
        {(control) => (
          <DateInput
            {...control}
            today={TODAY}
            value={day}
            onChange={(event) => {
              props.onReport(event.target.value);
              if (event.target.value) {
                setDay(event.target.value);
              }
            }}
          />
        )}
      </Field>
    </LocaleProvider>
  );
}

describe("a caller that refuses an empty box", () => {
  it("lets the reader empty the box and type a new day", async () => {
    const user = userEvent.setup();
    const report = vi.fn();
    render(<KeepsItsDay onReport={report} />);

    await user.click(box());
    await user.keyboard("{End}{Backspace>10/}");
    expect(box().value).toBe("");
    await user.keyboard("07/05/2026");
    await user.tab();

    expect(report).toHaveBeenCalledTimes(1);
    expect(report).toHaveBeenCalledWith("2026-05-07");
    expect(box().value).toBe("07/05/2026");
  });

  it("shows its day again when the box is left empty", async () => {
    const user = userEvent.setup();
    render(<KeepsItsDay onReport={() => {}} />);

    await user.clear(box());
    await user.tab();

    expect(box().value).toBe("06/05/2026");
  });

  it("keeps a refused day on show, with its reason, while the caller keeps its own", async () => {
    const user = userEvent.setup();
    render(<KeepsItsDay onReport={() => {}} />);

    await user.tripleClick(box());
    await user.keyboard("31/02/2026");
    await user.tab();

    expect(box().value).toBe("31/02/2026");
    expect(screen.getByRole("alert").textContent).toBe(
      "Enter a date as DD/MM/YYYY.",
    );
  });
});

// The shape the app's own forms take: state in the screen, posted on submit.
function PostsOnSubmit(
  props: Readonly<{ noValidate?: boolean; onPost: (day: string) => void }>,
) {
  const [day, setDay] = useState<ISODate | "">("2026-05-06");
  return (
    <LocaleProvider initial="en">
      <form
        noValidate={props.noValidate}
        onSubmit={(event) => {
          event.preventDefault();
          props.onPost(day);
        }}
      >
        <Field label="Due">
          {(control) => (
            <DateInput
              {...control}
              today={TODAY}
              value={day}
              onChange={(event) => setDay(event.target.value)}
            />
          )}
        </Field>
        <Button type="submit">Save</Button>
      </form>
    </LocaleProvider>
  );
}

describe("a form around a day it cannot read", () => {
  it("is invalid by the time the caller's own blur reads it", async () => {
    const user = userEvent.setup();
    const seen = vi.fn();
    render(
      <Due
        initial="2026-05-06"
        onBlur={(event) => seen(event.currentTarget.checkValidity())}
      />,
    );

    await user.tripleClick(box());
    await user.keyboard("31/02/2026");
    await user.tab();

    expect(seen).toHaveBeenCalledWith(false);
  });

  it("is refused by the browser's own check", async () => {
    const user = userEvent.setup();
    const post = vi.fn();
    render(<PostsOnSubmit onPost={post} />);

    await user.tripleClick(box());
    await user.keyboard("31/02/2026");
    await user.click(screen.getByRole("button", { name: "Save" }));

    expect(post).not.toHaveBeenCalled();
  });

  it("posts no day, never the earlier one, when the form skips that check", async () => {
    const user = userEvent.setup();
    const post = vi.fn();
    render(<PostsOnSubmit noValidate onPost={post} />);

    await user.tripleClick(box());
    await user.keyboard("31/02/2026{Enter}");

    expect(post).toHaveBeenCalledWith("");
  });
});

// The day between two other fields, so Enter in one of them submits the form.
function DayAmongFields(
  props: Readonly<{
    titleRequired?: boolean;
    onPost: () => void;
    onInvalid: (cancelled: boolean) => void;
  }>,
) {
  const [day, setDay] = useState<ISODate | "">("2026-05-06");
  return (
    <LocaleProvider initial="en">
      <form
        onSubmit={(event) => {
          event.preventDefault();
          props.onPost();
        }}
      >
        <label>
          Title
          <input required={props.titleRequired} />
        </label>
        <Field label="Due">
          {(control) => (
            <DateInput
              {...control}
              today={TODAY}
              value={day}
              onChange={(event) => setDay(event.target.value)}
              onInvalid={(event) => props.onInvalid(event.defaultPrevented)}
            />
          )}
        </Field>
        <label>
          Note
          <input />
        </label>
        <Button type="submit">Save</Button>
      </form>
    </LocaleProvider>
  );
}

// happy-dom moves no focus on a refused submit. These read what the box leaves
// to the browser, which focuses only an uncancelled invalid event.
describe("a submit the browser refuses over a day it cannot read", () => {
  it("leaves the day to the browser when it is the form's first refusal", async () => {
    const user = userEvent.setup();
    const post = vi.fn();
    const invalid = vi.fn();
    render(<DayAmongFields onPost={post} onInvalid={invalid} />);

    await user.tripleClick(box());
    await user.keyboard("31/02/2026");
    await user.type(screen.getByRole("textbox", { name: "Note" }), "x{Enter}");

    expect(post).not.toHaveBeenCalled();
    expect(invalid.mock.calls).toEqual([[false]]);
    expect(screen.getAllByRole("alert")).toHaveLength(1);
  });

  it("cancels its bubble when an earlier field is the form's first refusal", async () => {
    const user = userEvent.setup();
    const post = vi.fn();
    const invalid = vi.fn();
    render(<DayAmongFields titleRequired onPost={post} onInvalid={invalid} />);

    await user.tripleClick(box());
    await user.keyboard("31/02/2026");
    await user.type(screen.getByRole("textbox", { name: "Note" }), "x{Enter}");

    expect(post).not.toHaveBeenCalled();
    expect(invalid.mock.calls).toEqual([[true]]);
    expect(screen.getAllByRole("alert")).toHaveLength(1);
  });
});

// A form that saves on submit and logs each report, so the order is visible.
function SavesTheDay(
  props: Readonly<{
    initial?: ISODate | "";
    onReport: (day: string) => void;
    onSave: (day: string) => void;
  }>,
) {
  const [day, setDay] = useState<ISODate | "">(props.initial ?? "2026-05-06");
  return (
    <LocaleProvider initial="en">
      <form
        onSubmit={(event) => {
          event.preventDefault();
          props.onSave(day);
        }}
      >
        <Field label="Due">
          {(control) => (
            <DateInput
              {...control}
              today={TODAY}
              value={day}
              onChange={(event) => {
                props.onReport(event.target.value);
                setDay(event.target.value);
              }}
            />
          )}
        </Field>
        <Button type="submit">Save</Button>
      </form>
    </LocaleProvider>
  );
}

describe("a typed day reaches the caller once the reader settles it", () => {
  it("is what the form submits when the reader presses Enter", async () => {
    const user = userEvent.setup();
    const save = vi.fn();
    render(<SavesTheDay onReport={() => {}} onSave={save} />);

    await user.tripleClick(box());
    await user.keyboard("25/09/2026{Enter}");

    expect(save).toHaveBeenCalledTimes(1);
    expect(save).toHaveBeenCalledWith("2026-09-25");
  });

  it("is reported as the pointer presses Save, before the click submits it", async () => {
    const user = userEvent.setup();
    const report = vi.fn();
    const save = vi.fn();
    render(<SavesTheDay onReport={report} onSave={save} />);
    const saveButton = screen.getByRole("button", { name: "Save" });

    await user.tripleClick(box());
    await user.keyboard("25/09/2026");
    expect(report).not.toHaveBeenCalled();
    await user.pointer({ keys: "[MouseLeft>]", target: saveButton });

    expect(report).toHaveBeenCalledWith("2026-09-25");
    expect(save).not.toHaveBeenCalled();
    await user.pointer({ keys: "[/MouseLeft]", target: saveButton });
    expect(save).toHaveBeenCalledWith("2026-09-25");
  });

  it("asks a caller that saves on change for one write per settle", async () => {
    const user = userEvent.setup();
    const patch = vi.fn();
    render(
      <SavesTheDay initial="2026-09-15" onReport={patch} onSave={() => {}} />,
    );

    await user.click(box());
    await user.keyboard(
      "{Home}{ArrowRight}{ArrowRight}{Backspace}{Backspace}25",
    );
    expect(box().value).toBe("25/09/2026");
    expect(patch).not.toHaveBeenCalled();
    await user.keyboard("{Enter}");
    await user.tab();

    expect(patch).toHaveBeenCalledTimes(1);
    expect(patch).toHaveBeenCalledWith("2026-09-25");
  });

  it("is refused at submit when the reader presses Enter on text it cannot read", async () => {
    const user = userEvent.setup();
    const save = vi.fn();
    render(<SavesTheDay onReport={() => {}} onSave={save} />);

    await user.tripleClick(box());
    await user.keyboard("31/02/2026{Enter}");

    expect(save).not.toHaveBeenCalled();
    expect(box().checkValidity()).toBe(false);
  });
});

describe("a day picked after a typed one it could not read", () => {
  it("leaves the box valid by the time the caller hears the pick", async () => {
    const user = userEvent.setup();
    const heard: Array<[string, boolean]> = [];
    render(
      <Due
        initial="2026-05-06"
        onReport={(day) => heard.push([day, box().checkValidity()])}
      />,
    );

    await user.tripleClick(box());
    await user.keyboard("31/02/2026");
    await user.click(screen.getByRole("button", { name: CHOOSE }));
    await user.click(
      await screen.findByRole("button", { name: "Thursday, 7 May 2026" }),
    );

    expect(heard.at(-1)).toEqual(["2026-05-07", true]);
    expect(box().getAttribute("aria-invalid")).toBeNull();
  });
});

// The caller's value moves from outside the field: a reset, a reload.
function Resettable() {
  const [day, setDay] = useState<ISODate | "">("2026-05-06");
  return (
    <LocaleProvider initial="en">
      <Field label="Due">
        {(control) => (
          <DateInput
            {...control}
            today={TODAY}
            value={day}
            onChange={(event) => setDay(event.target.value)}
          />
        )}
      </Field>
      <Button onClick={() => setDay("2026-05-06")}>Reset</Button>
    </LocaleProvider>
  );
}

describe("a value the caller sets", () => {
  it("replaces a refused day the reader left", async () => {
    const user = userEvent.setup();
    render(<Resettable />);

    await user.tripleClick(box());
    await user.keyboard("31/02/2026");
    await user.tab();
    expect(box().getAttribute("aria-invalid")).toBe("true");
    await user.click(screen.getByRole("button", { name: "Reset" }));

    expect(box().value).toBe("06/05/2026");
    expect(box().getAttribute("aria-invalid")).toBeNull();
    expect(box().checkValidity()).toBe(true);
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("replaces text the reader is still typing", async () => {
    const user = userEvent.setup();
    const held = (day: ISODate | "") => (
      <LocaleProvider initial="en">
        <Field label="Due">
          {(control) => <DateInput {...control} today={TODAY} value={day} />}
        </Field>
      </LocaleProvider>
    );
    const { rerender } = render(held("2026-05-06"));

    await user.tripleClick(box());
    await user.keyboard("abc");
    rerender(held(""));

    expect(box().value).toBe("");
    expect(box().checkValidity()).toBe(true);
    expect(document.activeElement).toBe(box());
    rerender(held("2026-05-06"));
    expect(box().value).toBe("06/05/2026");
  });
});

describe("a refusal the reader can see", () => {
  it("is written under the box and describes it, beside the field's own hint", async () => {
    const user = userEvent.setup();
    render(<Due min="2026-05-10" hint="Any working day" />);

    await user.type(box(), "09/05/2026");
    expect(screen.queryByRole("alert")).toBeNull();
    await user.tab();

    const refusal = screen.getByRole("alert");
    expect(refusal.textContent).toBe("Earliest date: 10/05/2026");
    const describedBy = box().getAttribute("aria-describedby")?.split(" ");
    expect(describedBy).toContain(refusal.id);
    expect(describedBy).toContain(screen.getByText("Any working day").id);
  });
});

// The boxes a real layout gives: a field ending at the page gutter of a phone,
// and the calendar's own width. Happy-dom lays nothing out.
function laidOut(field: Readonly<{ left: number; right: number }>) {
  vi.stubGlobal("innerWidth", 390);
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
    function (this: HTMLElement) {
      return this.classList.contains("date-input")
        ? new DOMRect(field.left, 300, field.right - field.left, 40)
        : new DOMRect(0, 0, 0, 0);
    },
  );
  vi.spyOn(HTMLElement.prototype, "offsetWidth", "get").mockImplementation(
    function (this: HTMLElement) {
      return this.classList.contains("date-popup") ? 274 : 0;
    },
  );
}

describe("where the calendar opens", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("hangs off the end of a field in the right-hand column, inside the screen", async () => {
    laidOut({ left: 201, right: 374 });
    const user = userEvent.setup();
    render(<Due />);

    await user.click(screen.getByRole("button", { name: CHOOSE }));
    const dialog = await screen.findByRole("dialog", { name: CHOOSE });

    expect(dialog.style.left).toBe("100px");
  });

  it("stays on the screen when the field's end is nearer the start than its width", async () => {
    laidOut({ left: 16, right: 190 });
    const user = userEvent.setup();
    render(<Due />);

    await user.click(screen.getByRole("button", { name: CHOOSE }));
    const dialog = await screen.findByRole("dialog", { name: CHOOSE });

    expect(dialog.style.left).toBe("4px");
  });
});

describe("the calendar", () => {
  it("opens on the chosen day, moves with the arrows and picks with Enter", async () => {
    const user = userEvent.setup();
    const report = vi.fn();
    render(<Due initial="2026-05-06" onReport={report} />);
    const opener = screen.getByRole("button", { name: CHOOSE });
    expect(opener.getAttribute("aria-haspopup")).toBe("dialog");
    expect(opener.getAttribute("aria-expanded")).toBe("false");

    await user.click(opener);
    const dialog = await screen.findByRole("dialog", { name: CHOOSE });
    expect(opener.getAttribute("aria-expanded")).toBe("true");
    expect(document.activeElement?.getAttribute("data-day")).toBe("2026-05-06");

    await user.keyboard("{ArrowRight}{Enter}");

    expect(report).toHaveBeenCalledWith("2026-05-07");
    expect(dialog.isConnected).toBe(false);
    expect(document.activeElement).toBe(opener);
    expect(box().value).toBe("07/05/2026");
  });

  it("opens on today with nothing chosen, and Escape closes it back onto the button", async () => {
    const user = userEvent.setup();
    const report = vi.fn();
    render(<Due onReport={report} />);
    const opener = screen.getByRole("button", { name: CHOOSE });

    await user.click(opener);
    await screen.findByRole("dialog", { name: CHOOSE });
    expect(document.activeElement?.getAttribute("data-day")).toBe("2026-05-20");

    await user.keyboard("{Escape}");

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(document.activeElement).toBe(opener);
    expect(report).not.toHaveBeenCalled();
  });

  it("keeps Tab inside its own controls", async () => {
    const user = userEvent.setup();
    render(<Due initial="2026-05-06" />);

    await user.click(screen.getByRole("button", { name: CHOOSE }));
    const dialog = await screen.findByRole("dialog");
    await user.tab();
    expect(document.activeElement?.textContent).toBe("Today");
    await user.tab();
    await user.tab();

    expect(document.activeElement?.getAttribute("aria-label")).toBe(
      "Previous month",
    );
    expect(dialog.contains(document.activeElement)).toBe(true);
  });

  it("refuses the days outside min and max, with the reason in their name", async () => {
    const user = userEvent.setup();
    const report = vi.fn();
    render(<Due min="2026-05-10" max="2026-05-30" onReport={report} />);

    await user.click(screen.getByRole("button", { name: CHOOSE }));
    const early = await screen.findByRole("button", {
      name: "Saturday, 9 May 2026, Earliest date: 10/05/2026",
    });
    await user.click(early);

    expect(early.getAttribute("aria-disabled")).toBe("true");
    expect(report).not.toHaveBeenCalled();
    await user.click(
      screen.getByRole("button", { name: "Sunday, 10 May 2026" }),
    );
    expect(report).toHaveBeenCalledWith("2026-05-10");
  });

  it("clears the day with Clear and picks today with Today", async () => {
    const user = userEvent.setup();
    const report = vi.fn();
    render(<Due initial="2026-05-06" onReport={report} />);
    const opener = screen.getByRole("button", { name: CHOOSE });

    await user.click(opener);
    await user.click(await screen.findByRole("button", { name: "Clear" }));
    expect(report).toHaveBeenLastCalledWith("");
    expect(box().value).toBe("");

    await user.click(opener);
    await user.click(await screen.findByRole("button", { name: "Today" }));
    expect(report).toHaveBeenLastCalledWith("2026-05-20");
  });

  it("closes alone on Escape inside a dialog, which stays open", async () => {
    const user = userEvent.setup();
    const closeDialog = vi.fn();
    render(
      <InDialog onClose={closeDialog}>
        <Due />
      </InDialog>,
    );

    await user.click(screen.getByRole("button", { name: CHOOSE }));
    await screen.findByRole("dialog", { name: CHOOSE });
    await user.keyboard("{Escape}");

    expect(screen.queryByRole("dialog", { name: CHOOSE })).toBeNull();
    expect(closeDialog).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog", { name: "Log activity" })).toBeTruthy();
  });

  it("names its button and its calendar after the field, and plainly without a label", async () => {
    const user = userEvent.setup();
    render(
      <LocaleProvider initial="en">
        <Field label="Starts" required>
          {(control) => <DateInput {...control} today={TODAY} />}
        </Field>
        <DateInput today={TODAY} />
      </LocaleProvider>,
    );

    await user.click(
      screen.getByRole("button", { name: "Choose date: Starts" }),
    );
    expect(
      await screen.findByRole("dialog", { name: "Choose date: Starts" }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Choose date" })).toBeTruthy();
  });

  it("does not open while the field is disabled or read-only", () => {
    render(
      <>
        <Due disabled />
        <Due readOnly />
      </>,
    );

    for (const opener of screen.getAllByRole("button", { name: CHOOSE })) {
      expect(opener).toHaveProperty("disabled", true);
    }
  });
});

function InDialog(
  props: Readonly<{ onClose: () => void; children: ReactNode }>,
) {
  return (
    <>
      <Button>Behind</Button>
      <Modal open onClose={props.onClose} labelledBy="log" intent="form">
        <Heading size="large" id="log">
          Log activity
        </Heading>
        {props.children}
      </Modal>
    </>
  );
}
