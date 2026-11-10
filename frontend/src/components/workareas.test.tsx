/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { areasChange } from "./passport-areas";
import {
  ALL_AREAS,
  areasComplete,
  areasFormValue,
  type PassportAreas,
  type WorkArea,
  WorkAreasChoice,
} from "./workareas";

afterEach(cleanup);

const THREE: WorkArea[] = ["business", "compliance", "own_account"];

// Renders the section over its own state and reports each value it sets, so
// a test reads what a caller would send.
function renderChoice(reachable: readonly WorkArea[], initial = ALL_AREAS) {
  const seen: PassportAreas[] = [];
  function Host() {
    const [value, setValue] = useState(initial);
    return (
      <WorkAreasChoice
        reachable={reachable}
        value={value}
        onChange={(next) => {
          seen.push(next);
          setValue(next);
        }}
      />
    );
  }
  render(
    <LocaleProvider initial="en">
      <Host />
    </LocaleProvider>,
  );
  return seen;
}

describe("WorkAreasChoice", () => {
  it("draws nothing for a role that reaches fewer than two areas", () => {
    renderChoice(["business"]);
    expect(screen.queryByRole("radio")).toBeNull();
    renderChoice([]);
    expect(screen.queryByRole("radio")).toBeNull();
  });

  it("starts at all areas, with no area to tick", () => {
    renderChoice(THREE);
    expect(screen.getByRole("radio", { name: /All areas/ })).toBeChecked();
    expect(screen.queryByRole("checkbox")).toBeNull();
  });

  it("chooses areas from every reachable one, and drops what is unticked", async () => {
    const seen = renderChoice(THREE);
    const user = userEvent.setup();
    await user.click(screen.getByRole("radio", { name: /Choose areas/ }));
    for (const box of screen.getAllByRole("checkbox")) {
      expect(box).toBeChecked();
    }
    await user.click(
      screen.getByRole("checkbox", { name: /Compliance and privacy/ }),
    );
    expect(seen.at(-1)).toEqual({
      mode: "custom",
      areas: ["business", "own_account"],
    });
  });

  it("keeps the role's order whatever order the areas are ticked in", async () => {
    const seen = renderChoice(THREE, {
      mode: "custom",
      areas: ["own_account"],
    });
    const user = userEvent.setup();
    await user.click(screen.getByRole("checkbox", { name: /Sales work/ }));
    expect(seen.at(-1)?.areas).toEqual(["business", "own_account"]);
  });
});

describe("what a grant of areas sends", () => {
  it("needs at least one area when areas are chosen", () => {
    expect(areasComplete(ALL_AREAS)).toBe(true);
    expect(areasComplete({ mode: "custom", areas: [] })).toBe(false);
    expect(areasComplete({ mode: "custom", areas: ["business"] })).toBe(true);
  });

  it("posts the literal all, or the chosen keys separated by single spaces", () => {
    expect(areasFormValue(ALL_AREAS)).toBe("all");
    expect(
      areasFormValue({ mode: "custom", areas: ["business", "own_account"] }),
    ).toBe("business own_account");
  });
});

describe("areasChange", () => {
  it("counts a switch to chosen areas over every area as a change with nothing lost", () => {
    expect(
      areasChange(ALL_AREAS, { mode: "custom", areas: THREE }, THREE),
    ).toEqual({ gains: [], loses: [], changed: true });
  });

  it("names what is gained and lost between two choices", () => {
    expect(
      areasChange(
        { mode: "custom", areas: ["business"] },
        { mode: "custom", areas: ["compliance"] },
        THREE,
      ),
    ).toEqual({ gains: ["compliance"], loses: ["business"], changed: true });
  });

  it("is no change when the same areas come back in another order", () => {
    expect(
      areasChange(
        { mode: "custom", areas: ["business", "own_account"] },
        { mode: "custom", areas: ["own_account", "business"] },
        THREE,
      ).changed,
    ).toBe(false);
  });
});

describe("a chosen area the role no longer reaches", () => {
  it("counts as lost when another area is edited, since the server refuses to keep it", () => {
    const before: PassportAreas = {
      mode: "custom",
      areas: ["business", "admin_settings"],
    };
    const after: PassportAreas = {
      mode: "custom",
      areas: ["business", "compliance"],
    };
    const change = areasChange(before, after, THREE);
    expect(change.gains).toEqual(["compliance"]);
    expect(change.loses).toEqual(["admin_settings"]);
    expect(change.changed).toBe(true);
  });
});
