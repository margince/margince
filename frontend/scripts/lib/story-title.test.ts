// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { storyTitle } from "./story-title";

describe("storyTitle reads what Storybook files a story under", () => {
  const probe = "probe.stories.tsx";
  const docs = "probe.mdx";

  it("reads the title off the default export, not the first match", () => {
    const source = [
      'const fixture = { title: "Commercial terms v4" };',
      'const meta = { title: "Records/Deal room/Documents and threads" };',
      "export default meta;",
    ].join("\n");
    expect(storyTitle(probe, source)).toBe(
      "Records/Deal room/Documents and threads",
    );
  });

  it("reads a title off an inline default export", () => {
    expect(
      storyTitle(probe, 'export default { title: "Shell/Top bar" };'),
    ).toBe("Shell/Top bar");
  });

  it("reads a title through the type-only wrappers", () => {
    // Wrappers that leave the object unchanged; the reader must see through each.
    for (const meta of [
      'const meta = { title: "Shell/Top bar" } satisfies Meta<typeof Bar>;',
      'const meta = { title: "Shell/Top bar" } as Meta<typeof Bar>;',
      'const meta = ({ title: "Shell/Top bar" });',
    ]) {
      expect(storyTitle(probe, `${meta}\nexport default meta;`)).toBe(
        "Shell/Top bar",
      );
    }
    expect(
      storyTitle(
        probe,
        'export default { title: "Shell/Top bar" } satisfies Meta<typeof Bar>;',
      ),
    ).toBe("Shell/Top bar");
  });

  it("reads a title whichever way the key and value are written", () => {
    for (const meta of [
      'const meta = { "title": "Shell/Top bar" };',
      'const meta = { title: "Shell/Top bar" as const };',
      'const meta = { title: ("Shell/Top bar") };',
      'const meta = { "title": "Shell/Top bar" as const };',
    ]) {
      expect(storyTitle(probe, `${meta}\nexport default meta;`)).toBe(
        "Shell/Top bar",
      );
    }
  });

  it("reports no title rather than resolving a computed key", () => {
    expect(
      storyTitle(
        probe,
        'const k = "title";\nconst meta = { [k]: "Shell/Top bar" };\nexport default meta;',
      ),
    ).toBe(null);
  });

  it("reports no title rather than guessing when there is no default export", () => {
    expect(storyTitle(probe, 'const meta = { title: "Shell/Top bar" };')).toBe(
      null,
    );
  });

  it("reports no title for a meta a literal read cannot resolve", () => {
    for (const meta of [
      `const meta = { title: \`Shell/\${bar}\` };\nexport default meta;`,
      'const meta = { title: "Shell/" + bar };\nexport default meta;',
      "const meta = { title: TITLE };\nexport default meta;",
      "const meta = { title } as Meta;\nexport default meta;",
      "const meta = {};\nexport default meta;",
      'const meta = { title: "Shell/Top bar" };\nexport { meta as default };',
    ]) {
      expect(storyTitle(probe, meta)).toBe(null);
    }
  });

  it("reads the title off an MDX page's one Meta", () => {
    const page = [
      "{/* SPDX-License-Identifier: BUSL-1.1 */}",
      'import { Meta } from "@storybook/addon-docs/blocks";',
      "",
      '<Meta title="Get started/Introduction" />',
      "",
      "# Introduction",
    ].join("\n");
    expect(storyTitle(docs, page)).toBe("Get started/Introduction");
  });

  it("reads past a Meta in a comment or a code fence", () => {
    for (const aside of [
      '{/* <Meta title="Shell/Old" /> */}',
      '```mdx\n<Meta title="Shell/Example" />\n```',
    ]) {
      expect(
        storyTitle(docs, `${aside}\n<Meta title="Get started/Introduction" />`),
      ).toBe("Get started/Introduction");
    }
  });

  it("reports no title for an MDX page it cannot read strictly", () => {
    for (const page of [
      "# No meta at all",
      '<Meta title="Shell/One" />\n<Meta title="Shell/Two" />',
      "<Meta title={TITLE} />",
      "<Meta of={Stories} />",
      '<Meta of={Stories} title="Shell/Top bar" />',
      "<Meta title='Shell/Top bar' />",
    ]) {
      expect(storyTitle(docs, page)).toBe(null);
    }
  });
});
