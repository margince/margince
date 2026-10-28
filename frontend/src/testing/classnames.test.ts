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

  it("reads each component's own list whichever declares the name first", () => {
    const code = `${AVATAR}\n${BUTTON}`;
    expect(on(code, "button")).toEqual(["btn", "btn-wide"]);
    expect(on(code, "span")).toEqual(["avatar", "avatar-mesh"]);
  });

  it("reads nothing through a name a nearer class or enum shadows", () => {
    const code = `const Foo = "outer";
    const Tone = "outer";
    function C() {
      class Foo {}
      enum Tone { A }
      return <><i className={Foo} /><b className={Tone} /></>;
    }`;
    expect(on(code, "i")).toEqual([]);
    expect(on(code, "b")).toEqual([]);
  });

  it("reads a function expression's own name as that function", () => {
    const code = `const f = "outer";
    const g = function f() { return <i className={f} />; };`;
    expect(on(code, "i")).toEqual([]);
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
    const given = (read: ts.Expression) =>
      read.getText() === "size" ? ["s", "l"] : undefined;
    expect(
      classVariants(source, classAttribute(source, "i"), 16, given),
    ).toEqual([["s"], ["l"]]);
    expect(
      classVariants(source, classAttribute(source, "p"), 16, given),
    ).toEqual([["huge"]]);
  });
});

describe("a call in a className reads what the function returns", () => {
  const variants = (code: string, element: string) => {
    const source = parsed(code);
    return classVariants(source, classAttribute(source, element));
  };

  it("reads each branch a local function returns, not its inputs", () => {
    const code = `function look(state) { return state === "x" ? "tone-x" : "tone-y"; }
    const C = () => <i className={look(state)} />;`;
    expect(variants(code, "i")).toEqual([["tone-x"], ["tone-y"]]);
  });

  it("reads the function useMemo is handed", () => {
    const code = `function C() {
      const cls = useMemo(() => (open ? "row-open" : "row"), [open]);
      return <i className={cls} />;
    }`;
    expect(variants(code, "i")).toEqual([["row-open"], ["row"]]);
  });

  it("reads the classes it is handed when its returns name none", () => {
    const code = `const join = (...parts) => parts.join(" ");
    const C = () => <i className={join("a", "b")} />;`;
    expect(variants(code, "i")).toEqual([["a", "b"]]);
  });

  it("keeps the spaces a returned text carries inside a template", () => {
    const code = `function look(on) { return on ? " mark-on" : ""; }
    const C = () => <i className={\`mark\${look(on)}\`} />;`;
    expect(on(code, "i")).toEqual(["mark", "mark-on"]);
  });

  it("reads a list filled after its declaration as unread, not as its start", () => {
    const code = `function C() {
      const classes = ["bar"];
      if (dense) classes.push("bar-dense");
      let tone = "ink";
      if (warn) tone += " ink-warn";
      return <><i className={classes.join(" ")} /><b className={tone} /></>;
    }`;
    expect(variants(code, "i")).toEqual([[]]);
    expect(variants(code, "b")).toEqual([[]]);
  });
});
