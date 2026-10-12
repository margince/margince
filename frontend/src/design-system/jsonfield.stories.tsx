// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { JsonField, lineOfPath, parseProblem } from "./jsonfield";

const meta: Meta<typeof JsonField> = {
  title: "Components/Forms and input/Json field",
  component: JsonField,
};
export default meta;

type Story = StoryObj<typeof JsonField>;

const VALID = `{
  "provider": {
    "sort": "latency",
    "quantizations": ["fp16", "bf16"]
  }
}`;

const REFUSED = `{
  "provider": {
    "sort": { "by": "fastest" },
    "zdr": true
  }
}`;

function Editable({
  initial,
  paths = [],
}: Readonly<{ initial: string; paths?: string[] }>) {
  const [value, setValue] = useState(initial);
  const parse = parseProblem(value);
  const lines = parse?.line
    ? [parse.line]
    : paths.flatMap((p) => lineOfPath(value, p) ?? []);
  return (
    <div style={{ maxWidth: 520 }}>
      <JsonField
        aria-label="Serving JSON"
        value={value}
        onChange={setValue}
        problemLines={lines}
        invalid={lines.length > 0}
      />
    </div>
  );
}

export const Valid: Story = { render: () => <Editable initial={VALID} /> };

// Two problems the server named by path, found in the text by the gutter.
export const RefusedByPath: Story = {
  render: () => (
    <Editable initial={REFUSED} paths={["provider.sort.by", "provider.zdr"]} />
  ),
};

// Text that does not parse: the parser's own line is marked.
export const DoesNotParse: Story = {
  render: () => (
    <Editable initial={`{\n  "provider": {\n    "sort": "latency",\n  }\n}`} />
  ),
};

export const Empty: Story = {
  render: () => <Editable initial="" />,
};
