// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, renderHook, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type ReactNode, useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import type { LeafValue } from "../../lib/segmentpredicate";
import { StoryProviders } from "../../testing/story-utils";
import { ValueControl } from "./filtervalue";
import { useFieldValueText } from "./listexplain";

afterEach(cleanup);

function IndustryPicker({ seen }: Readonly<{ seen: LeafValue[] }>) {
  const [value, setValue] = useState<LeafValue>("");
  return (
    <ValueControl
      type="picklist"
      field="industry"
      references={undefined}
      options={["healthcare", "software"]}
      op="eq"
      value={value}
      label="Industry"
      onChange={(next) => {
        seen.push(next);
        setValue(next);
      }}
    />
  );
}

describe("the saved-view industry picker", () => {
  it("shows the words of each code and stores the code", async () => {
    const seen: LeafValue[] = [];
    render(
      <StoryProviders>
        <IndustryPicker seen={seen} />
      </StoryProviders>,
    );
    const user = userEvent.setup();
    await pickOption(user, screen.getByRole("combobox"), "Healthcare");

    expect(seen.at(-1)).toBe("healthcare");
    expect(screen.getByRole("combobox")).toHaveTextContent("Healthcare");
  });
});

describe("a saved view's value text", () => {
  it("words an industry code in the explanation and the member columns", () => {
    const { result } = renderHook(
      () => useFieldValueText("company", [], false),
      {
        wrapper: ({ children }: { children: ReactNode }) => (
          <StoryProviders>{children}</StoryProviders>
        ),
      },
    );
    expect(result.current("industry", "healthcare")).toBe("Healthcare");
    expect(result.current("city", "healthcare")).toBe("healthcare");
  });
});
