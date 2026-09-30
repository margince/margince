// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { en } from "../i18n/en";
import { ShortlistVerb } from "./bulkshortlist";
import { ValueControl } from "./filtervalue";
import { listsMe, shortlist } from "./lists.fixtures";
import type { LeafValue } from "./segmentpredicate";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

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
    fireEvent.change(days, { target: { value: "-2" } });
    expect(seen.at(-1)).toEqual({ days_ago: 0 });
    await user.click(
      screen.getByRole("button", { name: en["filters.date.on"] }),
    );
    expect(seen.at(-1)).toBe("");
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
