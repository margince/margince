import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { DataTable } from "./datatable";
import { KeyCode, KeyedName } from "./keyedname";

const meta = {
  title: "Components/Text and data display/Keyed name",
  component: KeyedName,
  args: { name: "Everyday cloud", code: "cheap_cloud" },
} satisfies Meta<typeof KeyedName>;
export default meta;

type Story = StoryObj<typeof meta>;

export const NameOverKey: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("Everyday cloud");
    await expect(canvas.getByText("cheap_cloud").tagName).toBe("CODE");
  },
};

export const KeyWithoutName: Story = {
  args: { name: "nightly_batch", code: "nightly_batch" },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findAllByText("nightly_batch")).toHaveLength(1);
  },
};

/** `KeyCode`: the key alone under a field, said as a key. */
export const KeyUnderAField: Story = {
  render: (args) => <KeyCode code={args.code} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const key = await canvas.findByText("cheap_cloud");
    await expect(key.tagName).toBe("CODE");
    await expect(key.parentElement).toHaveTextContent("Key: cheap_cloud");
  },
};

type Job = { name: string; key: string; waiting: number; running: number };

const JOBS: Job[] = [
  {
    name: "Expired confirmation link cleanup",
    key: "comms_controller_payload_sweep",
    waiting: 1,
    running: 0,
  },
  {
    name: "Mail confidentiality check",
    key: "capture_confidentiality_verdict",
    waiting: 12,
    running: 3,
  },
];

// Each run between two `<wbr>` is its own text node. A run laid out on more
// than one line broke inside a word.
function keyLayout(key: HTMLElement) {
  const walker = key.ownerDocument.createTreeWalker(key, NodeFilter.SHOW_TEXT);
  const lines = new Set<number>();
  const brokenMidWord: string[] = [];
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    const range = key.ownerDocument.createRange();
    range.selectNodeContents(node);
    const tops = new Set([...range.getClientRects()].map((rect) => rect.top));
    for (const top of tops) lines.add(top);
    if (tops.size > 1) brokenMidWord.push(node.textContent ?? "");
  }
  return { lines: lines.size, brokenMidWord };
}

// Long keys in a table squeezed narrower than they are: each key wraps, and
// only after an underscore.
export const LongKeysUnderWidthPressure: Story = {
  render: () => (
    <div style={{ width: 160 }}>
      <DataTable
        label="Background jobs"
        rows={JOBS}
        rowKey={(row) => row.key}
        columns={[
          {
            key: "job",
            header: "Job",
            render: (row) => <KeyedName name={row.name} code={row.key} />,
          },
          {
            key: "waiting",
            header: "Waiting",
            align: "end",
            render: (row) => row.waiting,
          },
          {
            key: "running",
            header: "Running",
            align: "end",
            render: (row) => row.running,
          },
        ]}
      />
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("table", { name: "Background jobs" });
    for (const job of JOBS) {
      const key = canvas.getByText(job.key);
      const layout = keyLayout(key);
      // Wrapped at all, or the squeeze proves nothing.
      await expect(layout.lines).toBeGreaterThan(1);
      await expect(layout.brokenMidWord).toEqual([]);
    }
  },
};

type Tier = { name: string; key: string; model: string };

const TIERS: Tier[] = [
  { name: "Small local", key: "local_small", model: "gemma3" },
  { name: "Everyday cloud", key: "cheap_cloud", model: "gemini-3-flash" },
];

export const InATable: Story = {
  render: () => (
    <DataTable
      label="Model tiers"
      rows={TIERS}
      rowKey={(row) => row.key}
      columns={[
        {
          key: "tier",
          header: "Tier",
          render: (row) => <KeyedName name={row.name} code={row.key} />,
        },
        { key: "model", header: "Model", render: (row) => row.model },
      ]}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("table");
    await expect(canvas.getByText("local_small")).toBeTruthy();
  },
};
