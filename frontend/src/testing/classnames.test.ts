// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type ts from "typescript";
import { describe, expect, it } from "vitest";
import { attr, elementsIn, tag } from "../../scripts/lib/dialoglayout";
import { parseSource } from "../../scripts/lib/source-tree";
import { classNamesOn, classVariants } from "./classnames";

const parsed = (code: string) => parseSource("probe.tsx", code);
const on = (code: string, element: string) =>
  classNamesOn(parsed(code), element).map((one) => one.name);

const classAttribute = (source: ts.SourceFile, name: string) => {
  const element = elementsIn(source).find((e) => tag(e) === name);
  return element && attr(element, "className")?.initializer;
};

const BUTTON = `function Button() {
  const classes = ["btn", wide ? "btn-wide" : ""].filter(Boolean).join(" ");
  return <button className={classes} />;
}`;
const AVATAR = `function Avatar() {
  const classes = ["avatar", "avatar-mesh"];
  return <span className={classes.join(" ")} />;
}`;

describe("a name in a className reads the binding in its own scope", () => {
  it("reads each component's own list when two declare the same name", () => {
    const code = `${BUTTON}\n${AVATAR}`;
    expect(on(code, "button")).toEqual(["btn", "btn-wide"]);
    expect(on(code, "span")).toEqual(["avatar", "avatar-mesh"]);
  });

  it("keeps a component's reading when a later one declares its name", () => {
    expect(on(`${BUTTON}\n${AVATAR}`, "button")).toEqual(on(BUTTON, "button"));
  });

  it("reads an inner block's binding over the outer one it shadows", () => {
    const code = `function Row() {
      const c = "row";
      if (open) {
        const c = "row-open";
        return <i className={c} />;
      }
      return <b className={c} />;
    }`;
    expect(on(code, "i")).toEqual(["row-open"]);
    expect(on(code, "b")).toEqual(["row"]);
  });

  it("follows a joined list's subject to the list in its own scope", () => {
    const code = `${AVATAR}\nfunction Chip() {
      const classes = ["chip"];
      return <em className={classes.join(" ")} />;
    }`;
    expect(on(code, "em")).toEqual(["chip"]);
    expect(on(code, "span")).toEqual(["avatar", "avatar-mesh"]);
  });

  it("hands a prop its call's values where another component declares the name", () => {
    const source = parsed(`function Other() {
      const size = "huge";
      return <p className={size} />;
    }
    function Chip({ size }) {
      return <i className={size} />;
    }`);
    const given = (name: string) => (name === "size" ? ["s", "l"] : undefined);
    expect(
      classVariants(source, classAttribute(source, "i"), 16, given),
    ).toEqual([["s"], ["l"]]);
    expect(
      classVariants(source, classAttribute(source, "p"), 16, given),
    ).toEqual([["huge"]]);
  });
});
