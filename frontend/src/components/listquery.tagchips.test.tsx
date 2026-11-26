/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ChipValueList } from "../design-system/listfiltervalues";
import { en } from "../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { useTagChips } from "./listquery";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function TagValues() {
  const [chip] = useTagChips();
  return chip ? (
    <ChipValueList chip={chip} value="" onPick={() => undefined} />
  ) : null;
}

describe("the Tags filter of a list", () => {
  it("narrows its words with a search box, as Owner and Team do", async () => {
    installFetchStub({
      "GET /tags": () =>
        jsonResponse({
          data: [
            { id: "t-1", name: "VIP" },
            { id: "t-2", name: "Partner" },
          ],
          page: { next_cursor: null, has_more: false },
        }),
    });
    const user = userEvent.setup();
    render(
      <StoryProviders>
        <TagValues />
      </StoryProviders>,
    );

    const box = await screen.findByRole("searchbox", {
      name: en["table.filterValueSearch"].replace(
        "{filter}",
        en["tags.columnHeader"],
      ),
    });
    expect(screen.getByRole("radio", { name: "Partner" })).toBeInTheDocument();
    await user.type(box, "vi");
    expect(screen.getByRole("radio", { name: "VIP" })).toBeInTheDocument();
    expect(screen.queryByRole("radio", { name: "Partner" })).toBeNull();
  });
});
