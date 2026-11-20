// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { ShortlistVerb } from "../../components/bulkshortlist";
import { en } from "../../i18n/en";
import {
  isComplete,
  type LeafValue,
  newLeaf,
} from "../../lib/segmentpredicate";
import { listsMe, shortlist } from "../../testing/lists.fixtures";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ValueControl } from "./filtervalue";

afterEach(cleanup);

function DateClause({ seen }: Readonly<{ seen: LeafValue[] }>) {
  const [value, setValue] = useState<LeafValue>("");
  return (
    <ValueControl
      type="date"
      references={undefined}
      options={undefined}
      op="lt"
      value={value}
      label="Last touch"
      onChange={(next) => {
        seen.push(next);
        setValue(next);
      }}
    />
  );
}

describe("a date clause", () => {
  it("switches to a count of days before today and back", async () => {
    const seen: LeafValue[] = [];
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <DateClause seen={seen} />
      </StoryProviders>,
    );
    await user.click(
      screen.getByRole("button", { name: en["filters.date.daysAgo"] }),
    );
    const days = screen.getByRole("textbox", {
      name: "Last touch: days before today",
    });
    fireEvent.change(days, { target: { value: "45" } });
    expect(seen.at(-1)).toEqual({ days_ago: 45 });
    await user.click(
      screen.getByRole("button", { name: en["filters.date.on"] }),
    );
    expect(seen.at(-1)).toBe("");
  });
});

describe("a count of days the engine cannot take", () => {
  const REFUSAL = "Enter a whole number of days from 0 to 36,600.";

  it.each(["-1", "1.5", "abc", "36601"])(
    "keeps %s as typed, says why, and holds the clause back",
    async (typed) => {
      const seen: LeafValue[] = [];
      const user = userEvent.setup();
      render(
        <StoryProviders>
          <DateClause seen={seen} />
        </StoryProviders>,
      );
      await user.click(
        screen.getByRole("button", { name: en["filters.date.daysAgo"] }),
      );
      const days = screen.getByRole("textbox", {
        name: "Last touch: days before today",
      });
      fireEvent.change(days, { target: { value: typed } });

      expect(days).toHaveValue(typed);
      expect(days).toHaveAttribute("aria-invalid", "true");
      expect(days).toHaveAccessibleDescription(REFUSAL);
      expect(
        screen
          .getByRole("button", { name: en["filters.date.daysAgo"] })
          .getAttribute("aria-pressed"),
      ).toBe("true");
      const held = seen.at(-1) ?? "";
      expect(held).toBe("");
      expect(isComplete(newLeaf("last_touch", "lt", held))).toBe(false);
    },
  );

  it.each([
    ["0", 0],
    ["36600", 36600],
  ])("takes %s as a count", async (typed, count) => {
    const seen: LeafValue[] = [];
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <DateClause seen={seen} />
      </StoryProviders>,
    );
    await user.click(
      screen.getByRole("button", { name: en["filters.date.daysAgo"] }),
    );
    const days = screen.getByRole("textbox", {
      name: "Last touch: days before today",
    });
    fireEvent.change(days, { target: { value: "x" } });
    fireEvent.change(days, { target: { value: typed } });

    expect(seen.at(-1)).toEqual({ days_ago: count });
    expect(days).not.toHaveAttribute("aria-invalid");
    expect(screen.queryByText(REFUSAL)).toBeNull();
  });

  it("goes back to a calendar day from a count being corrected", async () => {
    const seen: LeafValue[] = [];
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <DateClause seen={seen} />
      </StoryProviders>,
    );
    await user.click(
      screen.getByRole("button", { name: en["filters.date.daysAgo"] }),
    );
    fireEvent.change(
      screen.getByRole("textbox", { name: "Last touch: days before today" }),
      { target: { value: "-1" } },
    );
    await user.click(
      screen.getByRole("button", { name: en["filters.date.on"] }),
    );

    expect(
      screen.queryByRole("textbox", { name: "Last touch: days before today" }),
    ).toBeNull();
    expect(screen.queryByText(REFUSAL)).toBeNull();
  });
});

describe("Add to Shortlist over a selection", () => {
  it("offers the reader's own Shortlists and hands back the one picked", async () => {
    installFetchStub({
      "GET /me": listsMe(true),
      "GET /lists": () =>
        jsonResponse({
          data: [
            shortlist,
            { ...shortlist, id: "other", name: "Not mine", can_edit: false },
          ],
          page: { has_more: false },
        }),
    });
    const picked: string[] = [];
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <ShortlistVerb
          recordType="company"
          disabled={false}
          onPick={(list) => picked.push(list.name)}
        />
      </StoryProviders>,
    );
    await user.click(
      await screen.findByRole("combobox", { name: en["bulk.addToShortlist"] }),
    );
    expect(screen.queryByRole("option", { name: "Not mine" })).toBeNull();
    await user.click(
      await screen.findByRole("option", { name: shortlist.name }),
    );
    expect(picked).toEqual([shortlist.name]);
  });
});
