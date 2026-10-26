// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { en } from "../../i18n/en";
import type { LeafValue } from "../../lib/segmentpredicate";
import { StoryProviders } from "../../testing/story-utils";
import { ValueControl } from "./filtervalue";

afterEach(cleanup);

function typeInto(seen: LeafValue[], initial: LeafValue = "") {
  render(
    <StoryProviders>
      <ValueControl
        type="distance"
        references={undefined}
        options={undefined}
        op="within_radius"
        value={initial}
        onChange={(next) => seen.push(next)}
      />
    </StoryProviders>,
  );
  return (label: keyof typeof en, text: string) =>
    fireEvent.change(screen.getByLabelText(en[label]), {
      target: { value: text },
    });
}

describe("the radius value editor", () => {
  it("holds the empty value until the centre and distance are all given", () => {
    const seen: LeafValue[] = [];
    const type = typeInto(seen);
    type("filters.radius.lat", "48.77");
    type("filters.radius.lon", "9.18");
    expect(seen).toEqual(["", ""]);
    type("filters.radius.km", "50");
    expect(seen.at(-1)).toEqual({ lat: 48.77, lon: 9.18, radius_km: 50 });
  });

  it("carries the place name as the label and refuses what the server refuses", () => {
    const seen: LeafValue[] = [];
    const type = typeInto(seen, {
      lat: 48.77,
      lon: 9.18,
      radius_km: 50,
    });
    type("filters.radius.place", "Stuttgart");
    expect(seen.at(-1)).toEqual({
      lat: 48.77,
      lon: 9.18,
      radius_km: 50,
      label: "Stuttgart",
    });
    type("filters.radius.km", "0");
    expect(seen.at(-1)).toBe("");
    type("filters.radius.km", "5");
    type("filters.radius.lat", "91");
    expect(seen.at(-1)).toBe("");
  });

  it("shows the tree's operand again when the tree changes it without the editor", () => {
    const stored = { lat: 48.77, lon: 9.18, radius_km: 50 };
    const view = (value: LeafValue) => (
      <StoryProviders>
        <ValueControl
          type="distance"
          references={undefined}
          options={undefined}
          op="within_radius"
          value={value}
          onChange={() => {}}
        />
      </StoryProviders>
    );
    const { rerender } = render(view(stored));
    fireEvent.change(screen.getByLabelText(en["filters.radius.km"]), {
      target: { value: "7" },
    });
    rerender(view({ ...stored }));
    expect(screen.getByLabelText(en["filters.radius.km"])).toHaveValue("50");
    rerender(view(""));
    expect(screen.getByLabelText(en["filters.radius.lat"])).toHaveValue("");
  });
});
