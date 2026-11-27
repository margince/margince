import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import {
  catalogAssertionValues,
  literalCatalogAssertions,
} from "./catalog-assertions.census";

const values = catalogAssertionValues(
  parseSource(
    "en.ts",
    'export const en={"session.signOut":"Sign out of Margince","record.empty":"Nothing here","duplicate":"Nothing here"};',
  ),
);
function found(text: string) {
  return literalCatalogAssertions(
    parseSource("planted.test.tsx", text),
    values,
  ).map(({ node }) => node.text);
}

describe("rendered assertions follow the catalog", () => {
  it("reads positive, absent, role-name and matcher assertions", () => {
    expect(
      found(
        'screen.getByText("Sign out of Margince"); screen.queryByText("Nothing here"); screen.findByRole("button",{name:"Nothing here"}); expect(element).not.toHaveTextContent("Nothing here");',
      ),
    ).toEqual([
      "Sign out of Margince",
      "Nothing here",
      "Nothing here",
      "Nothing here",
    ]);
  });
  it("keeps duplicate catalog values in scope", () => {
    expect(values.get("Nothing here")).toEqual(["record.empty", "duplicate"]);
  });
  it("keeps fixture data, comments and catalog reads", () => {
    expect(
      found(
        'const fixture={name:"Nothing here"}; // Nothing here\nscreen.getByText(en["record.empty"]); expect(element).not.toHaveTextContent(en["record.empty"]);',
      ),
    ).toEqual([]);
  });
  it("keeps assertions on explicit fixture labels independent of catalog copy", () => {
    expect(
      found(
        'const fixture={name:"Nothing here"};screen.getByText("Nothing here");render(<Demo label="Sign out of Margince"/>);screen.getByText("Sign out of Margince");',
      ),
    ).toEqual([]);
  });
  it("hears a changed catalog value and rejects its new literal", () => {
    const changed = catalogAssertionValues(
      parseSource("en.ts", 'const en={"record.empty":"No records captured"};'),
    );
    expect(
      literalCatalogAssertions(
        parseSource("x.test.tsx", 'screen.queryByText("No records captured")'),
        changed,
      ),
    ).toHaveLength(1);
  });
});
