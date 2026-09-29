// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { storyTitle } from "./story-title";

describe("storyTitle reads what Storybook files a story under", () => {
  const probe = "probe.stories.tsx";
  const docs = "probe.mdx";

  it("reads the title off the default export, not the first match", async () => {
    const source = [
      'const fixture = { title: "Commercial terms v4" };',
      'const meta = { title: "Records/Deal room/Documents and threads" };',
      "export default meta;",
    ].join("\n");
    expect(await storyTitle(probe, source)).toBe(
      "Records/Deal room/Documents and threads",
    );
  });

  it("reads a title off an inline default export", async () => {
    expect(
      await storyTitle(probe, 'export default { title: "Shell/Top bar" };'),
    ).toBe("Shell/Top bar");
  });

  it("reads a title through the type-only wrappers", async () => {
    // A title behind a wrapper the reader cannot see through walks past the root check.
    for (const meta of [
      'const meta = { title: "Shell/Top bar" } satisfies Meta<typeof Bar>;',
      'const meta = { title: "Shell/Top bar" } as Meta<typeof Bar>;',
      'const meta = ({ title: "Shell/Top bar" });',
    ]) {
      expect(await storyTitle(probe, `${meta}\nexport default meta;`)).toBe(
        "Shell/Top bar",
      );
    }
    expect(
      await storyTitle(
        probe,
        'export default { title: "Shell/Top bar" } satisfies Meta<typeof Bar>;',
      ),
    ).toBe("Shell/Top bar");
  });

  it("reads a title whichever way the key and value are written", async () => {
    for (const meta of [
      'const meta = { "title": "Shell/Top bar" };',
      'const meta = { title: "Shell/Top bar" as const };',
      'const meta = { title: ("Shell/Top bar") };',
      'const meta = { "title": "Shell/Top bar" as const };',
    ]) {
      expect(await storyTitle(probe, `${meta}\nexport default meta;`)).toBe(
        "Shell/Top bar",
      );
    }
  });

  it("reports no title rather than resolving a computed key", async () => {
    expect(
      await storyTitle(
        probe,
        'const k = "title";\nconst meta = { [k]: "Shell/Top bar" };\nexport default meta;',
      ),
    ).toBe(null);
  });

  it("reports no title rather than guessing when there is no default export", async () => {
    expect(
      await storyTitle(probe, 'const meta = { title: "Shell/Top bar" };'),
    ).toBe(null);
  });

  it("reports no title for a meta a literal read cannot resolve", async () => {
    for (const meta of [
      `const meta = { title: \`Shell/\${bar}\` };\nexport default meta;`,
      'const meta = { title: "Shell/" + bar };\nexport default meta;',
      "const meta = { title: TITLE };\nexport default meta;",
      "const meta = { title } as Meta;\nexport default meta;",
      "const meta = {};\nexport default meta;",
      'const meta = { title: "Shell/Top bar" };\nexport { meta as default };',
    ]) {
      expect(await storyTitle(probe, meta)).toBe(null);
    }
  });

  const blocks = 'import { Meta } from "@storybook/addon-docs/blocks";\n\n';

  it("reads the title off an MDX page's Meta, as Storybook does", async () => {
    const page = [
      "{/* SPDX-License-Identifier: BUSL-1.1 */}",
      blocks,
      '<Meta title="Get started/Introduction" />',
      "",
      "# Introduction",
    ].join("\n");
    expect(await storyTitle(docs, page)).toBe("Get started/Introduction");
  });

  it("reads past a Meta in a comment or a code fence", async () => {
    for (const aside of [
      '{/* <Meta title="Shell/Old" /> */}',
      '```mdx\n<Meta title="Shell/Example" />\n```\n',
    ]) {
      expect(
        await storyTitle(
          docs,
          `${blocks}${aside}\n<Meta title="Get started/Introduction" />`,
        ),
      ).toBe("Get started/Introduction");
    }
  });

  it("reports no title for an MDX page Storybook files no title from", async () => {
    for (const page of [
      "# No meta at all",
      '<div><Meta title="Shell/Inside" /></div>',
      '<Meta title="Shell/One" />\n\n<Meta title="Shell/Two" />',
      "<Meta title={TITLE} />",
      'import * as Stories from "./x.stories";\n\n<Meta of={Stories} />',
    ]) {
      expect(await storyTitle(docs, `${blocks}${page}`)).toBe(null);
    }
  });
});
