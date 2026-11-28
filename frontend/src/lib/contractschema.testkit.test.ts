import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { propertyOf, typeMembers } from "./contractschema.testkit";

const parse = (source: string) => parseSource("schema.ts", source);

describe("contract schema owners", () => {
  it("keeps every matching declaration and intersection in source order", () => {
    const source = parse(`
      type First = { Record: { first: "one" } & { repeated: "early" } };
      type Second = { Record: { repeated: "late"; last: "two" } };
    `);
    expect(
      typeMembers(source, "Record").map((node) => node.name.getText(source)),
    ).toEqual(["first", "repeated", "repeated", "last"]);
    expect(propertyOf(source, "Record", "first")?.type?.getText(source)).toBe(
      '"one"',
    );
    expect(
      propertyOf(source, "Record", "repeated")?.type?.getText(source),
    ).toBe('"late"');
    expect(propertyOf(source, "Record", "absent")).toBeUndefined();
  });

  it("keeps separate syntax trees isolated even when their file names match", () => {
    const first = parse('type Schema = { Record: { value: "first" } }');
    const second = parse('type Schema = { Record: { value: "second" } }');
    expect(propertyOf(first, "Record", "value")?.type?.getText(first)).toBe(
      '"first"',
    );
    expect(propertyOf(second, "Record", "value")?.type?.getText(second)).toBe(
      '"second"',
    );
    typeMembers(first, "Record").pop();
    expect(propertyOf(first, "Record", "value")?.type?.getText(first)).toBe(
      '"first"',
    );
  });
});
