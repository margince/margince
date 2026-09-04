/** @vitest-environment jsdom */

import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { createPortal } from "react-dom";
import { afterEach, expect, it, vi } from "vitest";
import { verticalPlacement } from "./anchored";
import {
  Checkbox,
  DataTable,
  Field,
  OverflowMenu,
  PendingBody,
  Radio,
  SegmentedControl,
  Textarea,
  TextInput,
} from "./atoms";
import { Select } from "./select";

// The dropdown these cases pair with a Field is the Select from select.tsx — a
// button and a portalled listbox, not a native `<select>`. Its own behaviour is
// specified in select.test.tsx; here it stands in for "a control a Field wires
// up", which is the only thing these cases are about.
const STAGES = [
  { value: "won", label: "Won" },
  { value: "lost", label: "Lost" },
];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The items are components with their own reads — the company's edit form
// fetches the user roster and the custom-field catalogue — so rendering them
// before the menu is ever opened made every reader of every record page pay
// for actions they did not take.
it("does not mount its items until the menu is first opened", async () => {
  let mounted = 0;
  function CostlyAction() {
    mounted += 1;
    return <button type="button">Merge</button>;
  }
  render(
    <OverflowMenu label="More actions">
      <CostlyAction />
    </OverflowMenu>,
  );
  expect(mounted).toBe(0);

  await userEvent.click(screen.getByRole("button", { name: "More actions" }));
  expect(mounted).toBeGreaterThan(0);

  // And they stay mounted once opened: an item's dialog restores focus to the
  // element that opened it, which must survive the panel being hidden again.
  await userEvent.click(screen.getByRole("button", { name: "More actions" }));
  // hidden: true — the closed panel is `hidden`, and the point is that the
  // item is still in the tree rather than visible.
  expect(
    screen.getByRole("button", { name: "Merge", hidden: true }),
  ).toBeTruthy();
});

// A card that hides its own overflow — every Panel does, so full-bleed rows
// respect its radius — used to clip this panel to the card's edge, and a menu
// on the last row lost the actions it exists to offer. The panel is drawn at
// the body and positioned against the trigger, so no ancestor can crop it.
it("draws its panel outside the container that would clip it", async () => {
  const user = userEvent.setup();
  const { container } = render(
    <OverflowMenu label="More actions">
      <button type="button">Archive</button>
    </OverflowMenu>,
  );

  await user.click(screen.getByRole("button", { name: "More actions" }));

  const items = screen.getByRole("button", { name: "Archive" }).parentElement;
  expect(items?.className).toContain("overflow-menu-items");
  expect(container.contains(items)).toBe(false);
  expect(document.body.contains(items)).toBe(true);
  // Positioned, not statically placed: a panel at the document's top-left is a
  // panel that has lost its trigger.
  expect(items?.getAttribute("style")).toContain("top:");
});

// An item that DID something is finished, and a menu still standing over the
// page after it reads as a control that never took the press. Focus goes back
// to the trigger rather than being dropped on <body> with the panel that held
// it: the reader's place is the button they opened.
it("closes and returns focus to the trigger when an item is chosen", async () => {
  const user = userEvent.setup();
  render(
    <OverflowMenu label="More actions">
      <button type="button">Archive</button>
    </OverflowMenu>,
  );
  const trigger = screen.getByRole("button", { name: "More actions" });
  await user.click(trigger);

  await user.click(screen.getByRole("button", { name: "Archive" }));

  expect(trigger.getAttribute("aria-expanded")).toBe("false");
  expect(document.activeElement).toBe(trigger);
});

// The panel is a sibling of the trigger in the DOM, so "click outside" has to
// mean outside BOTH — otherwise pressing anything in the panel reads as
// clicking away. The caller's own prose lives in here too (the one sentence
// saying why an archived record refuses these verbs), and a click landing on a
// paragraph has chosen nothing.
it("stays open when the click inside the panel chose nothing", async () => {
  const user = userEvent.setup();
  render(
    <OverflowMenu label="More actions">
      <p>Archived accounts take no writes.</p>
      <button type="button">Archive</button>
    </OverflowMenu>,
  );
  const trigger = screen.getByRole("button", { name: "More actions" });
  await user.click(trigger);

  await user.click(screen.getByText("Archived accounts take no writes."));

  expect(trigger.getAttribute("aria-expanded")).toBe("true");
});

// A toggle is not a verb. The reader opened this menu to set two switches, and
// the control that drew a region open is also the only one that closes it — so
// an item that SAYS it is a setting (aria-pressed, aria-expanded) leaves the
// panel standing while the verbs beside it do not.
it("stays open when the item chosen sets something rather than doing it", async () => {
  const user = userEvent.setup();
  function InspectorToggle() {
    const [open, setOpen] = useState(false);
    return (
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((was) => !was)}
      >
        Runs
      </button>
    );
  }
  render(
    <OverflowMenu label="More actions">
      <InspectorToggle />
    </OverflowMenu>,
  );
  const trigger = screen.getByRole("button", { name: "More actions" });
  await user.click(trigger);

  await user.click(screen.getByRole("button", { name: "Runs" }));

  expect(trigger.getAttribute("aria-expanded")).toBe("true");
});

// The one item that must NOT close the menu under itself: one whose whole job
// is to put a dialog up. A dialog restores focus, on close, to the control that
// opened it — so hiding that control first strands the reader on <body>. The
// menu reads the same `.overlay` its Escape handler reads, one commit after the
// press, which is the first moment the answer exists.
it("stays open when the item it just ran opened a dialog", async () => {
  const user = userEvent.setup();
  function DialogAction() {
    const [open, setOpen] = useState(false);
    return (
      <>
        <button type="button" onClick={() => setOpen(true)}>
          Merge with…
        </button>
        {open && createPortal(<div className="overlay" />, document.body)}
      </>
    );
  }
  render(
    <OverflowMenu label="More actions">
      <DialogAction />
    </OverflowMenu>,
  );
  const trigger = screen.getByRole("button", { name: "More actions" });
  await user.click(trigger);

  await user.click(screen.getByRole("button", { name: "Merge with…" }));

  expect(trigger.getAttribute("aria-expanded")).toBe("true");
});

// The trigger's own geometry, asserted here because this is the design system's
// own primitive: it draws the ellipsis on every record header in the product, so
// a rectangle here is a rectangle everywhere at once. `btn-icon` is what makes a
// button square and drops the width floor a WORD needs, and the trigger has no
// word — it was a bare `.btn.btn-sm` with a padding rule of its own, which is
// how a 32px square came to be drawn 30px or 36px wide depending on which of two
// equal-specificity rules the sheet order happened to put last.
it("draws its trigger as the square the icon-only button defines", () => {
  render(
    <OverflowMenu label="More actions">
      <button type="button">Merge</button>
    </OverflowMenu>,
  );

  const trigger = screen.getByRole("button", { name: "More actions" });
  expect(trigger.classList.contains("btn-icon")).toBe(true);
  expect(trigger.classList.contains("btn-sm")).toBe(true);
});

// The panel is FIXED, so the viewport is all the room there is: a menu placed
// below a trigger near the bottom edge puts its actions where no page scrolling
// reaches them. Stated over the measurements themselves, because jsdom gives
// every element a zero-sized rectangle and the rule is arithmetic.
it("opens the panel toward whichever side of the trigger has room", () => {
  const viewport = 800;
  vi.stubGlobal("innerHeight", viewport);
  // A real DOMRect, not a two-field literal cast into the shape: a box whose
  // `top` was supplied and whose height was not is a box no element has, and
  // `verticalPlacement` is free to read a field the literal never spelled.
  const near = (top: number) => new DOMRect(0, top, 0, 30);

  // Room below: the panel hangs from the trigger.
  const down = verticalPlacement(near(100), 200);
  expect(down.top).toBeGreaterThan(130);
  expect(down.maxHeight).toBeGreaterThan(200);

  // A trigger on the last row of a long page: below is 30px, so the panel
  // opens UPWARD and ends above the trigger rather than off the bottom edge.
  const up = verticalPlacement(near(viewport - 40), 200);
  expect(up.top + 200).toBeLessThanOrEqual(viewport - 40);

  // Taller than either side: it takes the roomier one and is capped to it, so
  // it scrolls inside itself instead of running past an edge.
  const squeezed = verticalPlacement(near(500), 2000);
  expect(squeezed.maxHeight).toBeLessThan(viewport);
  expect(squeezed.maxHeight).toBeGreaterThan(0);
});

// The label is half the control. Every hand-rolled site this atom replaces got
// its accessible name from a <label> wrapping the input, and a reader ticks the
// box by clicking the words — so both are asserted through the label text, not
// through the input node.
it("names a Checkbox by its label, and the label text toggles it", async () => {
  const seen: boolean[] = [];
  render(
    <Checkbox
      label="Include archived records"
      onChange={(event) => seen.push(event.target.checked)}
    />,
  );

  const box = screen.getByRole("checkbox", {
    name: "Include archived records",
  });
  await userEvent.click(screen.getByText("Include archived records"));

  expect(seen).toEqual([true]);
  expect((box as HTMLInputElement).checked).toBe(true);
});

// Radios group by `name`, which the atom must forward untouched — drop it and
// every option becomes independently selectable, which looks like working UI
// until two are on at once.
it("keeps Radios sharing a name mutually exclusive", async () => {
  render(
    <>
      <Radio name="side" label="Owner" defaultChecked />
      <Radio name="side" label="Team" />
    </>,
  );

  const owner = screen.getByRole("radio", {
    name: "Owner",
  }) as HTMLInputElement;
  const team = screen.getByRole("radio", { name: "Team" }) as HTMLInputElement;
  expect(owner.checked).toBe(true);

  await userEvent.click(screen.getByText("Team"));

  expect(team.checked).toBe(true);
  expect(owner.checked).toBe(false);
});

// Screens layer their own layout on top of the atom (`.compose-body`,
// `.share-reason`). Dropping either class silently unstyles the control, so the
// merge is asserted rather than assumed.
it("merges a caller's className with the atom's own", () => {
  render(<Textarea aria-label="Body" className="compose-body" />);

  expect([...screen.getByLabelText("Body").classList].sort()).toEqual([
    "compose-body",
    "textarea",
  ]);
});

// The label/control pairing is the whole reason Field exists, and the failure
// it prevents is silent: a mistyped id in either half leaves a control that
// looks labelled and is not. Asserting through getByLabelText proves the
// association rather than the markup.
it("pairs a Field's label with its control, and two Fields never collide", () => {
  render(
    <>
      <Field label="Deal name">
        {(control) => <TextInput {...control} defaultValue="Globex" />}
      </Field>
      <Field label="Stage">
        {(control) => (
          <Select
            {...control}
            options={STAGES}
            value="won"
            onChange={() => {}}
          />
        )}
      </Field>
    </>,
  );

  expect((screen.getByLabelText("Deal name") as HTMLInputElement).value).toBe(
    "Globex",
  );
  // Two instances of the same component must not share an id — the second
  // label would then point at the first control, and typing into one would
  // read as the other.
  expect(screen.getByLabelText("Deal name").id).not.toBe(
    screen.getByLabelText("Stage").id,
  );
});

// A hint has to describe the control without becoming part of its name: read
// as a name, the whole help text is announced on every focus.
it("describes a Field's control by its hint without naming it that", () => {
  render(
    <Field label="Reason" hint="Shown to the person you are sharing with">
      {(control) => <Textarea {...control} />}
    </Field>,
  );

  const control = screen.getByLabelText("Reason");
  const describedBy = control.getAttribute("aria-describedby");
  expect(describedBy).toBeTruthy();
  expect(document.getElementById(describedBy ?? "")?.textContent).toBe(
    "Shown to the person you are sharing with",
  );
});

// `required` is one prop, spent in two places — the visible marker and the
// control's own state. The marker is aria-hidden so the requirement is
// announced once, by the control, not twice.
it("marks a required Field once for the eye and once for the control", () => {
  render(
    <Field label="Role" required>
      {(control) => (
        <Select
          {...control}
          options={[{ value: "admin", label: "Admin" }]}
          value="admin"
          onChange={() => {}}
        />
      )}
    </Field>,
  );

  // getByRole resolves the accessible name the way an assistive technology
  // does, so the aria-hidden asterisk is excluded and the name is still "Role".
  // Queried by label TEXT it would read "Role *" — which is the visible string,
  // not the announced one.
  const control = screen.getByRole("combobox", { name: "Role" });
  // aria-required, not `required`: the trigger is a button, and a button carries
  // no constraint validation for the attribute to drive.
  expect(control.getAttribute("aria-required")).toBe("true");
  expect(screen.getByText("*").getAttribute("aria-hidden")).toBe("true");
});

// A table that scrolls sideways holds columns a pointer can drag to and a
// keyboard cannot reach at all, so the box takes a tab stop and a name — and
// takes neither while it fits, because a tab stop in front of every table in
// the product is a cost every keyboard reader pays for the few that overflow.
// jsdom lays nothing out, so the two widths the decision reads are stubbed on
// the prototype: that is the whole input to it.
function stubBoxWidths(scrollWidth: number, clientWidth: number) {
  for (const [property, value] of [
    ["scrollWidth", scrollWidth],
    ["clientWidth", clientWidth],
  ] as const) {
    vi.spyOn(HTMLDivElement.prototype, property, "get").mockReturnValue(value);
  }
}

const PRODUCT_COLUMNS = [
  { key: "name", header: "Name", render: (row: { name: string }) => row.name },
];
const PRODUCT_ROWS = [{ name: "Consulting Day" }];

it("makes a table's scroll box reachable and named once it overflows", () => {
  stubBoxWidths(930, 654);
  render(
    <DataTable
      label="Products"
      columns={PRODUCT_COLUMNS}
      rows={PRODUCT_ROWS}
      rowKey={(row) => row.name}
    />,
  );
  const box = screen.getByRole("region", { name: "Products" });
  expect(box.className).toContain("table-scroll");
  expect(box.getAttribute("tabindex")).toBe("0");
});

it("leaves a table that fits its box out of the tab order", () => {
  stubBoxWidths(654, 654);
  const { container } = render(
    <DataTable
      label="Products"
      columns={PRODUCT_COLUMNS}
      rows={PRODUCT_ROWS}
      rowKey={(row) => row.name}
    />,
  );
  expect(screen.queryByRole("region")).toBeNull();
  const box = container.querySelector(".table-scroll");
  expect(box?.getAttribute("tabindex")).toBeNull();
});

// A segmented option can carry a dot saying something waits behind it. The dot
// is decorative by contract: it draws the eye, and the surface it points at
// states the fact in words. A mark that carried the meaning alone would be
// invisible to a screen reader and to a reader who cannot see the colour.
const TABS = ["overview", "research"] as const;
const TAB_LABELS = { overview: "Overview", research: "Data & tools" };

it("marks only the options told to carry one", () => {
  const { container } = render(
    <SegmentedControl
      options={TABS}
      value="overview"
      onChange={() => undefined}
      labels={TAB_LABELS}
      marks={{ research: true }}
    />,
  );
  const marks = container.querySelectorAll(".segmented-mark");
  expect(marks.length).toBe(1);
  expect(
    screen
      .getByRole("button", { name: "Data & tools" })
      .querySelector(".segmented-mark"),
  ).not.toBeNull();
});

it("hides the mark from the accessibility tree", () => {
  render(
    <SegmentedControl
      options={TABS}
      value="overview"
      onChange={() => undefined}
      labels={TAB_LABELS}
      marks={{ research: true }}
    />,
  );
  // The attribute itself, not the accessible name it protects: the mark is an
  // empty span, so a name assertion passes whether or not it is hidden and
  // proves nothing about the contract this test is named for.
  const mark = screen
    .getByRole("button", { name: "Data & tools" })
    .querySelector(".segmented-mark");
  expect(mark?.getAttribute("aria-hidden")).toBe("true");
  expect(screen.getByRole("button", { name: "Data & tools" })).toBeDefined();
});

it("draws no mark when no option carries one", () => {
  const { container } = render(
    <SegmentedControl
      options={TABS}
      value="overview"
      onChange={() => undefined}
      labels={TAB_LABELS}
    />,
  );
  expect(container.querySelectorAll(".segmented-mark").length).toBe(0);
});

// `delayMs` is for a surface that re-reads as a reader types. Without it a
// placeholder flashes on every keystroke, reporting work that was already done
// — and it flashes in the accessibility tree too, which is why the spoken line
// is held back with the bars rather than announced early.
//
// Fake timers throughout: a real 300ms wait in a unit test is a test whose
// verdict depends on how busy the machine is.
it("holds a delayed pending body back until the wait is real", async () => {
  vi.useFakeTimers();
  try {
    const { container } = render(
      <PendingBody label="Searching…" lines={1} delayMs={300} />,
    );
    expect(container.querySelector(".pending")).toBeNull();
    expect(screen.queryByText("Searching…")).toBeNull();

    await act(async () => {
      vi.advanceTimersByTime(299);
    });
    expect(container.querySelector(".pending")).toBeNull();

    await act(async () => {
      vi.advanceTimersByTime(1);
    });
    expect(container.querySelector(".pending")).not.toBeNull();
    expect(screen.getByText("Searching…")).toBeTruthy();
  } finally {
    vi.useRealTimers();
  }
});

// Unset, the pending state is immediate — right for a surface a reader opened
// rather than one they are typing into. The default must not become "delayed by
// zero", which renders a frame late and makes every existing caller flicker.
it("shows an undelayed pending body on the first render", () => {
  const { container } = render(<PendingBody label="Loading…" lines={2} />);
  expect(container.querySelector(".pending")).not.toBeNull();
  expect(container.querySelectorAll(".pending-line").length).toBe(2);
});
