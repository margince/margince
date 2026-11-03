/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ChoiceList } from "./choicelist";

// This lane carries no auto-cleanup and no jest-dom: the DOM is torn down by
// hand, and state is read off the elements (`:checked`, the disabled attribute)
// rather than through matchers that are not installed here.
afterEach(cleanup);

// The question is a GROUP, every answer is readable at rest, and the words are
// part of the click target. Those three are the reasons this exists rather than
// ten hand-rolled wrappers around the Radio atom.

const CHOICES = [
  {
    value: "everyone_except" as const,
    label: "Everyone I talk to — except the contacts I leave out",
    description: "Every conversation goes in until you name somebody.",
  },
  {
    value: "only_chosen" as const,
    label: "Only the contacts I choose",
    description: "Nothing goes in until you name somebody.",
  },
];

describe("ChoiceList", () => {
  it("names the group, so the answers are one question rather than loose radios", () => {
    render(
      <ChoiceList
        legend="Which conversations go into the CRM?"
        value="only_chosen"
        choices={CHOICES}
        onChange={() => {}}
      />,
    );

    expect(
      screen.getByRole("group", {
        name: "Which conversations go into the CRM?",
      }),
    ).toBeTruthy();
    // Both answers are on screen at rest — the whole difference from a Select.
    expect(screen.getAllByRole("radio")).toHaveLength(2);
    expect(
      screen.getByRole("radio", { name: /Only the contacts I choose/ }),
    ).toBeTruthy();
    expect(
      screen.getAllByRole("radio").filter((radio) => radio.matches(":checked")),
    ).toHaveLength(1);
  });

  it("hides the legend from the page without taking it from the group's name", () => {
    render(
      <ChoiceList
        legend="Which conversations go into the CRM?"
        hideLegend
        value=""
        choices={CHOICES}
        onChange={() => {}}
      />,
    );

    const group = screen.getByRole("group", {
      name: "Which conversations go into the CRM?",
    });
    expect(group).toBeTruthy();
    expect(group.querySelector("legend")?.classList.contains("sr-only")).toBe(
      true,
    );
  });

  it("leaves every answer unchosen when nothing has been answered", () => {
    render(
      <ChoiceList
        legend="Which conversations go into the CRM?"
        value=""
        choices={CHOICES}
        onChange={() => {}}
      />,
    );

    // A pre-selected answer is one a reader can save without having read it.
    expect(
      screen.getAllByRole("radio").filter((radio) => radio.matches(":checked")),
    ).toHaveLength(0);
  });

  it("reads the description as part of the answer, so the help line is clickable", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <ChoiceList
        legend="Which conversations go into the CRM?"
        value="only_chosen"
        choices={CHOICES}
        onChange={onChange}
      />,
    );

    // The accessible name carries both halves: a reader hearing the option hears
    // what choosing it does.
    const option = screen.getByRole("radio", {
      name: /Everyone I talk to.*Every conversation goes in until you name somebody\./,
    });
    // And the words are the other half of the click target: pressing the help
    // line chooses the option it belongs to.
    await user.click(
      screen.getByText("Every conversation goes in until you name somebody."),
    );
    expect(onChange).toHaveBeenCalledWith("everyone_except");
    expect(option).toBeTruthy();
  });

  it("refuses every answer when the group is disabled", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <ChoiceList
        legend="Which conversations go into the CRM?"
        value="only_chosen"
        choices={CHOICES}
        disabled
        onChange={onChange}
      />,
    );

    for (const radio of screen.getAllByRole("radio")) {
      expect(radio.hasAttribute("disabled")).toBe(true);
    }
    await user.click(screen.getByText(/Everyone I talk to/));
    expect(onChange).not.toHaveBeenCalled();
  });

  it("refuses one answer alone, reading its reason with it", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <ChoiceList
        legend="Which conversations go into the CRM?"
        value="only_chosen"
        choices={[
          {
            ...CHOICES[0],
            disabled: true,
            description: "Connect a mailbox first.",
          },
          CHOICES[1],
        ]}
        onChange={onChange}
      />,
    );

    const refused = screen.getByRole("radio", {
      name: /Everyone I talk to.*Connect a mailbox first\./,
    });
    expect(refused.hasAttribute("disabled")).toBe(true);
    expect(
      screen
        .getByRole("radio", { name: /Only the contacts I choose/ })
        .hasAttribute("disabled"),
    ).toBe(false);
    await user.click(screen.getByText("Connect a mailbox first."));
    expect(onChange).not.toHaveBeenCalled();
  });

  it("groups two lists apart, so one page's two questions are not one", () => {
    render(
      <>
        <ChoiceList
          legend="First question"
          value="only_chosen"
          choices={CHOICES}
          onChange={() => {}}
        />
        <ChoiceList
          legend="Second question"
          value="everyone_except"
          choices={CHOICES}
          onChange={() => {}}
        />
      </>,
    );

    // Two names, and each group holds its own answer: with one shared radio
    // name the second would have unchecked the first.
    const names = new Set(
      screen
        .getAllByRole("radio")
        .map((radio) => radio.getAttribute("name") ?? ""),
    );
    expect(names.size).toBe(2);
    expect(
      screen.getAllByRole("radio").filter((r) => r.matches(":checked")),
    ).toHaveLength(2);
  });
});

const SERVICES = [
  { value: "mail" as const, label: "Mail" },
  { value: "calendar" as const, label: "Calendar" },
];

function Services({
  initial,
  onChange,
}: Readonly<{
  initial: ("mail" | "calendar")[];
  onChange: (next: ("mail" | "calendar")[]) => void;
}>) {
  const [value, setValue] = useState(initial);
  return (
    <ChoiceList
      legend="What to connect"
      multiple
      layout="cards"
      value={value}
      choices={SERVICES}
      onChange={(next) => {
        setValue(next);
        onChange(next);
      }}
    />
  );
}

describe("ChoiceList groups", () => {
  const GROUPED = [
    { value: "site" as const, label: "Your website", group: "No developer" },
    { value: "server" as const, label: "Website server", group: "Developers" },
    { value: "hook" as const, label: "Webhook", group: "Developers" },
  ];

  it("names each run of one group and keeps every radio in the one question", () => {
    render(
      <ChoiceList
        legend="Where is your form?"
        layout="cards"
        value="site"
        choices={GROUPED}
        onChange={() => {}}
      />,
    );

    const question = screen.getByRole("group", { name: "Where is your form?" });
    const developers = screen.getByRole("group", { name: "Developers" });
    expect(
      screen.getAllByRole("radio").every((r) => question.contains(r)),
    ).toBe(true);
    expect(developers.querySelectorAll("input[type=radio]")).toHaveLength(2);
    const names = new Set(
      screen.getAllByRole("radio").map((r) => r.getAttribute("name")),
    );
    expect(names.size).toBe(1);
  });
});

describe("ChoiceList tiles", () => {
  it("keeps a real radio under each tile, so a press on the tile answers", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <ChoiceList
        legend="Where is your form?"
        layout="tiles"
        value="site"
        choices={[
          {
            value: "site" as const,
            label: "Your website",
            description: "Posts",
          },
          { value: "hook" as const, label: "Webhook", description: "Signed" },
        ]}
        onChange={onChange}
      />,
    );

    await user.click(screen.getByText("Webhook"));
    expect(onChange).toHaveBeenLastCalledWith("hook");
    // The tick is drawn, never read: the radio's own checked state says it.
    expect(
      screen.getByRole("radio", { name: /Your website/ }).matches(":checked"),
    ).toBe(true);
    expect(screen.queryByRole("img")).toBeNull();
  });
});

describe("ChoiceList multiple", () => {
  it("draws checkboxes in one group named by the legend", () => {
    render(<Services initial={["mail"]} onChange={() => {}} />);

    const group = screen.getByRole("group", { name: "What to connect" });
    expect(screen.queryAllByRole("radio")).toHaveLength(0);
    const boxes = screen.getAllByRole("checkbox");
    expect(boxes).toHaveLength(2);
    expect(boxes.every((box) => box.closest("fieldset") === group)).toBe(true);
    expect(boxes.map((box) => box.matches(":checked"))).toEqual([true, false]);
  });

  it("reports the picks in choice order, not press order", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Services initial={[]} onChange={onChange} />);

    await user.click(screen.getByRole("checkbox", { name: "Calendar" }));
    await user.click(screen.getByRole("checkbox", { name: "Mail" }));
    expect(onChange).toHaveBeenLastCalledWith(["mail", "calendar"]);

    await user.click(screen.getByRole("checkbox", { name: "Calendar" }));
    expect(onChange).toHaveBeenLastCalledWith(["mail"]);
  });

  it("refuses a disabled answer and reads its reason with it", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(
      <ChoiceList
        legend="What to connect"
        multiple
        value={["mail"]}
        choices={[
          SERVICES[0],
          {
            ...SERVICES[1],
            disabled: true,
            description: "IMAP has no calendar.",
          },
        ]}
        onChange={onChange}
      />,
    );

    const calendar = screen.getByRole("checkbox", {
      name: /Calendar.*IMAP has no calendar\./,
    });
    expect(calendar.hasAttribute("disabled")).toBe(true);
    await user.click(calendar);
    expect(onChange).not.toHaveBeenCalled();
  });
});
