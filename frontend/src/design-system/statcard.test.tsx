/** @vitest-environment happy-dom */

import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ts from "typescript";
import { afterEach, describe, expect, it } from "vitest";
import { colorValues, rulesOf } from "../../scripts/lib/css-rules";
import { sourceFileAt } from "../../scripts/lib/source-tree";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { STAT_CARD_ACTIONS, STAT_CARD_TONES, StatCard } from "./statcard";

// Action names belong to the primitive; a caller selects a supported action
// rather than inventing another label for the same control.

const here = dirname(fileURLToPath(import.meta.url));
// The figure's own rules. census/rules/statcard.census.ts reads every sheet.
const atoms = rulesOf([join(here, "atoms.css")]);

afterEach(cleanup);

function drawReading() {
  return render(
    <LocaleProvider initial="en">
      <StatCard
        label="Urgent"
        value="4"
        detail="somebody waiting"
        onOpen={() => {}}
      />
    </LocaleProvider>,
  );
}

describe("a reading that names what it counts", () => {
  const peek = {
    head: "Open deals on this account",
    items: [
      { key: "a", title: "Expansion Phase 2", sub: "Proposal", right: "€95k" },
    ],
  };

  it("opens its card when keyboard focus lands on the door, and the door keeps its press", async () => {
    let opened = 0;
    const user = userEvent.setup();
    render(
      <LocaleProvider initial="en">
        <StatCard
          label="Open deals"
          value="€244k"
          detail="4 open"
          peek={peek}
          onOpen={() => (opened += 1)}
        />
      </LocaleProvider>,
    );

    await user.tab();
    expect(
      screen.getByRole("region", { name: "Open deals on this account" }),
    ).toBeTruthy();
    await user.keyboard("{Enter}");

    expect(opened).toBe(1);
  });

  it("opens the card on a press where the tile has no door of its own", async () => {
    const user = userEvent.setup();
    render(
      <LocaleProvider initial="en">
        <StatCard label="Unanswered" value="2 emails" peek={peek} />
      </LocaleProvider>,
    );

    await user.click(screen.getByText("2 emails"));

    expect(
      screen.getByRole("region", { name: "Open deals on this account" }),
    ).toBeTruthy();
  });

  it("draws no card for a figure with nothing behind it", async () => {
    const user = userEvent.setup();
    render(
      <LocaleProvider initial="en">
        <StatCard
          label="Unanswered"
          value="None"
          peek={{ head: "Emails", items: [] }}
          onOpen={() => {}}
        />
      </LocaleProvider>,
    );

    await user.tab();

    expect(screen.queryByRole("region")).toBeNull();
  });
});

describe("the door says Open, or View records for records in place", () => {
  it("letters the foot's button with the stat.open message and no other word", () => {
    const { container } = drawReading();

    const door = screen.getByRole("button");
    expect(door.textContent?.replace(/→/g, "").trim()).toBe(en["stat.open"]);
    // The foot holds that button and nothing beside it: a second word here is
    // the caller's label arriving under another name.
    const foot = container.querySelector(".stat-card-foot");
    expect(foot?.textContent?.replace(/→/g, "").trim()).toBe(en["stat.open"]);
  });

  it("names the ACTION and describes the reading, never both in the name", () => {
    const { container } = drawReading();

    const door = screen.getByRole("button");
    const described = (door.getAttribute("aria-describedby") ?? "")
      .split(/\s+/)
      .map((id) => container.ownerDocument.getElementById(id)?.textContent)
      .join(" ");
    expect(described).toContain("Urgent");
    expect(door.getAttribute("aria-label")).toBeNull();
  });

  it("draws a door given an href as a link with the same word and description", () => {
    render(
      <LocaleProvider initial="en">
        <StatCard label="Best route" value="Ada" href="#/contacts/c-1" />
      </LocaleProvider>,
    );

    const door = screen.getByRole("link", {
      name: en["stat.open"],
      description: "Best route",
    });
    expect(door.getAttribute("href")).toBe("/#/contacts/c-1");
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("says View records, with no arrow, for a door that lists the records in place", () => {
    const { container } = render(
      <LocaleProvider initial="en">
        <StatCard label="Urgent" value="4" opens="records" onOpen={() => {}} />
      </LocaleProvider>,
    );

    const door = screen.getByRole("button", { description: "Urgent" });
    expect(door.textContent?.trim()).toBe(en["stat.records"]);
    expect(container.querySelector(".stat-card-arrow")).toBeNull();
  });

  it("keeps Open on a link, which always goes to a page", () => {
    render(
      <LocaleProvider initial="en">
        <StatCard label="Best route" value="Ada" opens="records" href="#/c" />
      </LocaleProvider>,
    );

    expect(screen.getByRole("link", { name: en["stat.open"] })).not.toBeNull();
  });

  it("names a forecast drill with the shared action", () => {
    render(
      <LocaleProvider initial="en">
        <StatCard
          label="Expected finish"
          value="12,000 EUR"
          opens="forecast"
          href="#/analytics/forecast"
        />
      </LocaleProvider>,
    );
    expect(
      screen.getByRole("link", { name: en["stat.forecast"] }),
    ).not.toBeNull();
  });

  it("pins the English word, which is what every door in the product reads", () => {
    expect(en["stat.open"]).toBe("Open");
  });
});

describe("a toned figure is read, so it takes the ink that clears contrast", () => {
  const declared = (selector: string) =>
    atoms
      .filter((rule) => rule.selector === selector)
      .map((rule) => rule.body)
      .join("\n");

  it.each(STAT_CARD_TONES)(
    "letters a %s figure in its Text token and fills its bar with the base",
    (tone) => {
      expect(colorValues(declared(`.stat-card-${tone}`))).toEqual([
        `var(--${tone}Text)`,
      ]);
      expect(
        declared(`.stat-card-${tone} ~ .stat-card-meter .stat-card-meter-fill`),
      ).toMatch(new RegExp(`background:\\s*var\\(--${tone}\\);`));
    },
  );
});

describe("a reading's name and figure are never cut", () => {
  it("keeps the whole figure on the card's tooltip", () => {
    render(
      <LocaleProvider initial="en">
        <StatCard label="Reading" value={"1.234.567,89\u00a0€"} />
      </LocaleProvider>,
    );
    expect(document.querySelector(".stat-card")?.getAttribute("title")).toBe(
      "1.234.567,89\u00a0€",
    );
  });

  it("breaks a long compound at a syllable before anywhere", () => {
    const rule = atoms.find(
      (one) =>
        one.selector === ".stat-card-label-text" && /hyphens/.test(one.body),
    );
    expect(rule?.body).toMatch(/hyphens:\s*auto;/);
  });
});

describe("the foot speaks once per action", () => {
  it.each(Object.values(STAT_CARD_ACTIONS))(
    "uses the shared action key %s",
    (key) => {
      expect(en[key].length).toBeGreaterThan(0);
    },
  );

  it("writes one message per action into the card's foot", () => {
    const path = join(here, "statcard.tsx");
    const source = sourceFileAt(path);

    const foot = findElementByClass(source, "stat-card-foot");
    expect(foot).not.toBeUndefined();
    if (foot === undefined) return;

    expect(messageLookups(foot)).toHaveLength(1);
    expect(literalWords(foot)).toEqual([]);
  });
});

/** The JSX element whose className attribute is exactly `name`. */
function findElementByClass(
  source: ts.SourceFile,
  name: string,
): ts.Node | undefined {
  let found: ts.Node | undefined;
  const visit = (node: ts.Node): void => {
    if (found !== undefined) return;
    const opening = ts.isJsxElement(node)
      ? node.openingElement
      : ts.isJsxSelfClosingElement(node)
        ? node
        : undefined;
    if (opening !== undefined && classNameOf(opening) === name) {
      found = node;
      return;
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

function classNameOf(
  opening: ts.JsxOpeningElement | ts.JsxSelfClosingElement,
): string | undefined {
  for (const attribute of opening.attributes.properties) {
    if (
      ts.isJsxAttribute(attribute) &&
      ts.isIdentifier(attribute.name) &&
      attribute.name.text === "className" &&
      attribute.initializer !== undefined &&
      ts.isStringLiteral(attribute.initializer)
    ) {
      return attribute.initializer.text;
    }
  }
  return undefined;
}

/** Every `t("…")` call written inside `root`, as source text. */
function messageLookups(root: ts.Node): string[] {
  const calls: string[] = [];
  const visit = (node: ts.Node): void => {
    if (
      ts.isCallExpression(node) &&
      ts.isIdentifier(node.expression) &&
      node.expression.text === "t"
    ) {
      calls.push(node.getText());
    }
    ts.forEachChild(node, visit);
  };
  visit(root);
  return calls;
}

/** Every word written into `root` as text rather than looked up. */
function literalWords(root: ts.Node): string[] {
  const words: string[] = [];
  const visit = (node: ts.Node): void => {
    if (ts.isJsxText(node) && node.text.trim() !== "") {
      words.push(node.text.trim());
    }
    // Only a string the JSX DRAWS counts: one inside a `{…}` child. A class
    // name, a DOM flag and the key handed to `t()` are all strings too, and
    // none of them is a word the reader meets. Nor is the arrow — it is
    // `aria-hidden` on its own span, so a screen reader never reaches it.
    if (
      ts.isStringLiteral(node) &&
      node.parent !== undefined &&
      ts.isJsxExpression(node.parent) &&
      node.text.trim() !== "" &&
      node.text.trim() !== "→"
    ) {
      words.push(node.text.trim());
    }
    ts.forEachChild(node, visit);
  };
  visit(root);
  return words;
}
