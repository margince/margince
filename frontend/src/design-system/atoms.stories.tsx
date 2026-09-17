import type { Meta, StoryObj } from "@storybook/react-vite";
import { Lock, Mail, Plus, RefreshCw, Trash2 } from "lucide-react";
import {
  type CSSProperties,
  type ReactNode,
  useEffect,
  useRef,
  useState,
} from "react";
import {
  Avatar,
  BADGE_TONES,
  Badge,
  Button,
  Card,
  Checkbox,
  DataTable,
  Disclosure,
  EmptyState,
  Field,
  Kbd,
  OverflowMenu,
  Radio,
  SearchField,
  SectionHeader,
  SegmentedControl,
  Skeleton,
  Textarea,
  TextInput,
} from "./atoms";
import { AvatarStack } from "./avatarstack";
import { Heading } from "./heading";
import { usePasswordReveal } from "./passwordreveal";
import { ProviderMark } from "./provider-mark";
import { Select } from "./select";

const meta: Meta = {
  title: "Design System/Atoms",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const row: CSSProperties = {
  display: "flex",
  gap: "0.75rem",
  alignItems: "center",
  flexWrap: "wrap",
};
const stack: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "1rem",
};

// Every axis of the button on one screen. A ghost taller than the primary, an
// icon at lucide's own 24px beside the label, a two-letter label shrunk to a
// pill and a missing focus ring each look fine one variant at a time; same-row
// height, icon size and width floor show them in one look. There is no size
// axis: one height, and the row beside a field is where that is visible.
export const Buttons: Story = {
  render: () => (
    <div style={stack}>
      <div style={stack}>
        <span className="t-label">Variants</span>
        <div style={row}>
          <Button variant="primary">Save</Button>
          <Button variant="ghost">Cancel</Button>
          <Button variant="danger">Delete</Button>
          <Button variant="ai">Draft a reply</Button>
          <Button variant="aiQuiet">Shorter</Button>
        </div>
      </div>
      {/* The text affordance beside a real Button it must not out-shout: a
          CLASS on an `<a>` or `<button>`, or `variant="link"` for Button's
          refusal and busy contracts. Its focus ring is a SOLID outline — with
          no fill, a low-alpha shadow has nothing to read against. Tab to it. */}
      <div style={stack}>
        <span className="t-label">The secondary text affordance</span>
        <div style={row}>
          <Button variant="primary">Save changes</Button>
          <a className="link-button" href="#link-button-story">
            Download the signed PDF
          </a>
          <button type="button" className="link-button">
            View existing
          </button>
          {/* With an icon: lucide hands over a 24px glyph, so the BUTTON sizes
              it — this label wrapped under its own icon until it did. */}
          <button type="button" className="link-button">
            <Plus aria-hidden />
            Add another
          </button>
          {/* The same affordance reached through Button, for a verb that also
              needs what only the component gives — here the write in flight.
              One appearance, one declaration: `.btn.btn-link` rides along on
              the class's own rules rather than declaring a look of its own. */}
          <Button variant="link">Make private</Button>
          <Button variant="link" pending>
            Make private
          </Button>
        </div>
      </div>
      <div style={stack}>
        <span className="t-label">With an icon</span>
        <div style={row}>
          <Button variant="primary">
            <Plus aria-hidden />
            Add contact
          </Button>
          <Button variant="ghost">
            <RefreshCw aria-hidden />
            Reconnect
          </Button>
        </div>
      </div>
      <div style={stack}>
        <span className="t-label">
          Icon only — square, and named for a reader
        </span>
        <div style={row}>
          <Button variant="primary" iconOnly aria-label="Add contact">
            <Plus aria-hidden />
          </Button>
          <Button variant="ghost" iconOnly aria-label="Reconnect">
            <RefreshCw aria-hidden />
          </Button>
        </div>
      </div>
      <div style={stack}>
        <span className="t-label">Short labels keep the width floor</span>
        <div style={row}>
          <Button variant="ghost">No</Button>
          <Button variant="primary">Yes</Button>
          <Button variant="ghost">Add</Button>
          <Button variant="primary">Save</Button>
          <Button variant="ghost">Disconnect this inbox</Button>
        </div>
      </div>
      <div style={stack}>
        <span className="t-label">Beside a field, which is the same box</span>
        <div style={row}>
          <TextInput
            defaultValue="ops@example.com"
            style={{ inlineSize: "16rem" }}
          />
          <Button variant="primary">Invite</Button>
        </div>
      </div>
      <div style={stack}>
        <span className="t-label">
          Refused, disabled, and the icon affordance
        </span>
        <div style={row}>
          <Button variant="primary" reason="Connect an inbox first.">
            Send
          </Button>
          <Button variant="ghost" disabled>
            Cancel
          </Button>
          <button type="button" className="iconbtn" aria-label="Remove">
            <Trash2 aria-hidden />
          </button>
        </div>
      </div>
      {/* Working and refused are opposite facts: full ink and a turning mark
          against a dimmed, barred pill, side by side so the difference shows.
          The label does not change — a "Saving…" here would rename a control
          the reader is standing on. */}
      <div style={stack}>
        <span className="t-label">
          Working — which must not read as refused
        </span>
        <div style={row}>
          <Button variant="primary" pending>
            Save
          </Button>
          <Button variant="ghost" pending>
            Reconnect
          </Button>
          <Button variant="primary" pending>
            Save
          </Button>
          <Button variant="ghost" iconOnly pending aria-label="Reconnect">
            <RefreshCw aria-hidden />
          </Button>
          <Button
            variant="primary"
            pending
            busyLabel="Signing you in — this can take a moment."
          >
            Sign in
          </Button>
          <Button variant="primary" disabled>
            Save
          </Button>
        </div>
      </div>
      {/* Refusal outranks busy in both its spellings, so neither draws a
          mark: a control nobody may press cannot also be mid-press. */}
      <div style={stack}>
        <span className="t-label">Refused wins over busy, both ways round</span>
        <div style={row}>
          <Button variant="primary" disabled pending>
            Save
          </Button>
          <Button variant="primary" pending reason="Connect an inbox first.">
            Send
          </Button>
        </div>
      </div>
      {/* The federated variant at the full width it is shaped for, in the
          three states sign-in draws: live, dimmed while the password form
          writes, and advertised with nothing behind it. Each mark keeps its
          company's colours — the one non-token colour in the product — so no
          dim state grayscales it: recolouring somebody's logo is not ours. */}
      <div style={stack}>
        <span className="t-label">
          Federated sign-in — offered, in flight, unavailable
        </span>
        <div style={{ ...stack, gap: "0.5rem", maxInlineSize: "20rem" }}>
          <Button variant="federated">
            <ProviderMark providerKey="google" />
            Continue with Google
          </Button>
          <Button variant="federated" disabled>
            <ProviderMark providerKey="google" />
            Continue with Google
          </Button>
          <Button variant="federated" unavailable>
            <ProviderMark providerKey="microsoft" />
            Continue with Microsoft
          </Button>
        </div>
      </div>
    </div>
  ),
};

const BADGE_VARIANTS = ["soft", "primary"] as const;
// The tinted tones, walked from the component's own vocabulary: `default` is
// drawn on its own at the head of each row, and a grid that listed the rest
// again would stop showing the newest tone on the day it was added.
const BADGE_TINTS = BADGE_TONES.filter((tone) => tone !== "default");
const badgeDocs = (story: string) => ({ docs: { description: { story } } });

export const BadgeVariants: Story = {
  parameters: badgeDocs(`A badge states one status or label fact beside a name.
- **Soft** is the default. **Primary** is for the one status a reader must
  not miss, and for counts. One variant per context, never mixed in a column.
- An icon sits LEFT of the label, never right; a badge has no trailing slot.
- Soft has a tone hairline, primary none; add no border, caps or pill class.
- Don't make a badge interactive: a pressable fact is \`Chip\`, a filter is
  \`FilterPills\`, a verb is \`Button\`.
- No new colours. \`ai\` (always with Sparkles) means an agent proposed it, and
  \`discovery\` means something new rather than something going well.`),
  render: () => (
    <div style={stack}>
      {BADGE_VARIANTS.map((variant) => (
        <div key={variant} style={row}>
          <Badge variant={variant}>default</Badge>
          {BADGE_TINTS.map((tone) => (
            <Badge key={tone} variant={variant} tone={tone}>
              {tone}
            </Badge>
          ))}
        </div>
      ))}
    </div>
  ),
};

export const BadgeWithIcon: Story = {
  parameters: badgeDocs("A glyph names the kind of status; ai's is Sparkles."),
  render: () => (
    <div style={stack}>
      {BADGE_VARIANTS.map((variant) => (
        <div key={variant} style={row}>
          <Badge variant={variant} tone="success" icon={Mail}>
            Replied
          </Badge>
          <Badge variant={variant} tone="danger" icon={Lock}>
            Restricted
          </Badge>
          <Badge variant={variant} tone="ai">
            Drafted
          </Badge>
        </div>
      ))}
    </div>
  ),
};

export const BadgeLive: Story = {
  parameters: badgeDocs("Happening now; the dot stops under reduced motion."),
  render: () => (
    <div style={row}>
      <Badge tone="success" live>
        Live
      </Badge>
      <Badge variant="primary" tone="accent" live>
        Publishing
      </Badge>
    </div>
  ),
};

export const BadgeLongLabel: Story = {
  parameters: badgeDocs("At 200px, alone and beside a sibling: an ellipsis."),
  render: () => (
    <div style={{ ...stack, alignItems: "flex-start", inlineSize: 200 }}>
      <Badge tone="warning">
        extensions/acme/routes/partner-portal/settings
      </Badge>
      <div style={{ ...row, flexWrap: "nowrap", inlineSize: "100%" }}>
        <span>Route</span>
        <Badge icon={Lock}>extensions/acme/routes/partner-portal</Badge>
      </div>
    </div>
  ),
};

export const BadgeInsideUppercaseParent: Story = {
  parameters: badgeDocs("A parent's case, tracking and face stop at its edge."),
  render: () => (
    <div style={stack}>
      <Heading size="medium" className="t-eyebrow">
        Pipeline <Badge tone="accent">Three open</Badge>
      </Heading>
      <code>
        run 4f2a <Badge tone="success">Passed</Badge>
      </code>
    </div>
  ),
};

// The chip is an IDENTIFIER, so the states that matter are the ones where two
// chips must be told apart or recognised as one record: every size, the tint a
// record keeps on every page, and a name with no space in it.
export const Avatars: Story = {
  render: () => (
    <div style={stack}>
      <div style={stack}>
        <span className="t-label">The four sizes</span>
        <div style={row}>
          <Avatar name="Alice Müller" size="sm" />
          <Avatar name="Alice Müller" size="md" />
          <Avatar name="Alice Müller" size="lg" />
          <Avatar name="Alice Müller" size="xl" />
        </div>
      </div>
      <div style={stack}>
        <span className="t-label">
          Six tones, picked from the record and never stored
        </span>
        <div style={row}>
          <Avatar name="Alice Müller" />
          <Avatar name="Bob Schmidt" />
          <Avatar name="Carol Wagner" />
          <Avatar name="Voltaq Systems" />
          <Avatar name="Northwind Handel" />
          <Avatar name="Dara O'Brien" />
        </div>
      </div>
      <div style={stack}>
        <span className="t-label">
          The names a monogram rule usually gets wrong
        </span>
        <div style={row}>
          <Avatar name="jane.doe@example.com" />
          <Avatar name="Müller" />
          <Avatar name="van der Berg" />
          <Avatar name="李" />
          <Avatar name="Ana-Sofía Ruiz" />
        </div>
      </div>
      <div style={stack}>
        <span className="t-label">
          Same record, same colour — keyed on an id, so a rename does not move
          it
        </span>
        <div style={row}>
          <Avatar identity="company_7f3" name="Voltaq Systems" />
          <Avatar identity="company_7f3" name="Voltaq Systems GmbH" size="md" />
        </div>
      </div>
      <div style={stack}>
        <span className="t-label">
          A logo sits ON the monogram; a broken one leaves it standing
        </span>
        <div style={row}>
          <Avatar
            name="Northwind Handel"
            size="md"
            src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'%3E%3Ccircle cx='32' cy='32' r='30' fill='%230b7a53'/%3E%3C/svg%3E"
          />
          {/* A data URI that is a well-formed URL and not a decodable image, so
              the failure this story is about happens in the DECODER. A path
              that 404s would exercise the same fallback, but it also puts a
              failed request in the console, and the capture gate reads a failed
              request as a story that did not render clean. */}
          <Avatar
            name="Northwind Handel"
            size="md"
            src="data:image/png;base64,AAAA"
          />
        </div>
      </div>
      <div style={stack}>
        <span className="t-label">
          Stacked — the ring costs the chip no size, so a folded face sits on
          the same line as a lone one
        </span>
        <div style={row}>
          <AvatarStack
            contacts={[
              { name: "Alice Müller" },
              { name: "Bob Schmidt" },
              { name: "Carol Wagner" },
              { name: "Dara O'Brien" },
              { name: "Eve Lindqvist" },
              { name: "Frank Osei" },
            ]}
          />
          <Avatar name="Alice Müller" />
        </div>
      </div>
    </div>
  ),
};

// The field controls stacked the way a form stacks them, the only way to see
// that type size, padding, height and focus ring agree. The dropdown is the
// Select from select.tsx; its own states live in select.stories.tsx.
export const Fields: Story = {
  render: () => <FieldsRow />,
};

function FieldsRow() {
  const [stage, setStage] = useState("proposal");
  return (
    <div className="form-stack" style={{ maxWidth: "22rem" }}>
      <Field label="Deal name">
        {(control) => <TextInput {...control} defaultValue="Globex renewal" />}
      </Field>
      <Field label="Stage" required>
        {(control) => (
          <Select
            {...control}
            options={[
              { value: "qualify", label: "Qualify" },
              { value: "proposal", label: "Proposal" },
              { value: "won", label: "Won" },
            ]}
            value={stage}
            onChange={setStage}
          />
        )}
      </Field>
      <Field label="Note" hint="Only the deal's followers will see this.">
        {(control) => (
          <Textarea
            {...control}
            rows={3}
            defaultValue="Renewal terms agreed on the call."
          />
        )}
      </Field>
    </div>
  );
}

// A field's three slots: a leading glyph and a trailing control both sit INSIDE
// one outline, so a reveal button is in the ring. `error` is its own slot
// rather than a message through `hint`, because a refusal and the rule it broke
// say different things and must not share one meta-grey.
export const FieldStates: Story = {
  name: "Fields — affordances and refusal",
  render: () => <FieldStatesColumn />,
};

function FieldStatesColumn() {
  const reveal = usePasswordReveal({
    show: "Show password",
    hide: "Hide password",
  });
  const revealShort = usePasswordReveal({
    show: "Show password",
    hide: "Hide password",
  });
  return (
    <div className="form-stack" style={{ maxWidth: "22rem" }}>
      <Field label="Work address" icon={<Mail aria-hidden />}>
        {(control) => (
          <TextInput
            {...control}
            type="email"
            defaultValue="ops@example.com"
            autoComplete="username"
          />
        )}
      </Field>
      <Field
        label="Password"
        icon={<Lock aria-hidden />}
        labelEnd={
          <button type="button" className="link-button">
            Forgot?
          </button>
        }
        hint="At least 12 characters."
        trailing={reveal.trailing}
      >
        {(control) => (
          <TextInput
            {...control}
            type={reveal.type}
            defaultValue="correct horse battery"
            autoComplete="current-password"
          />
        )}
      </Field>
      <Field
        label="New password"
        required
        error="Too short. Use at least 12 characters."
        trailing={revealShort.trailing}
      >
        {(control) => (
          <TextInput
            {...control}
            type={revealShort.type}
            defaultValue="short"
            autoComplete="new-password"
          />
        )}
      </Field>
      <Field
        label="Confirm"
        required
        error="These two don't match."
        hint="Both fields have to say the same thing."
      >
        {(control) => (
          <TextInput
            {...control}
            type="password"
            defaultValue="something else"
            autoComplete="new-password"
          />
        )}
      </Field>
    </div>
  );
}

// A disabled control and a long label that wraps — the two states a field
// catalog usually omits and a real form always reaches.
export const Toggles: Story = {
  render: () => (
    <div className="form-stack" style={{ maxWidth: "22rem" }}>
      <Checkbox label="Replace the existing link" defaultChecked />
      <Checkbox label="Include archived records" />
      <Checkbox label="Notify the deal owner" disabled />
      <fieldset className="field-multiselect">
        <legend className="t-label">Target</legend>
        <Radio name="owner-side" label="Owner" defaultChecked />
        <Radio name="owner-side" label="Team" />
      </fieldset>
    </div>
  ),
};

// The card surfaces side by side. The third is the shape most screens want and
// why the header is props rather than a hand-placed child: one title, one
// description under it, and the section's actions beside them.
export const Cards: Story = {
  render: () => (
    <div style={stack}>
      <Card>
        <p className="t-caption">
          The standing surface: a card carries a section of a record.
        </p>
      </Card>
      <Card inset>
        <p className="t-caption">
          The inset variant sits inside another surface, so it recedes instead
          of stacking a second raised edge on the first.
        </p>
      </Card>
      <Card title="Passports" actions={<Button>Mint</Button>}>
        <p className="t-caption">
          The header comes from props: title over description across the full
          width, actions beside the pair.
        </p>
      </Card>
    </div>
  ),
};

// Loading and empty, the same moment of a screen seen twice: a skeleton that
// outlives the request and an empty state that says nothing both read broken.
export const Placeholders: Story = {
  render: () => (
    <div style={stack}>
      <Card>
        <div
          style={{ display: "flex", flexDirection: "column", gap: "0.6rem" }}
        >
          <Skeleton width="40%" height={18} />
          <Skeleton width="100%" />
          <Skeleton width="86%" />
          <Skeleton width={140} height={10} />
        </div>
      </Card>
      <EmptyState>No deals match these filters yet.</EmptyState>
      <EmptyState
        title="No projects yet"
        action={<Button variant="primary">New project</Button>}
      >
        <p>
          A project is the body of work a deal is about. It starts during the
          deal, in the initiative phase, and outlives close-won: delivery is
          tracked here after the pipeline has let go.
        </p>
      </EmptyState>
      {/* The plate: an empty GROUP inside a pane, dashed because the space is
          waiting rather than broken. Its verb lives in the group's head. */}
      <EmptyState plate title="No open deals">
        A deal is a sale in progress on this account, with its stage and its
        expected close.
      </EmptyState>
    </div>
  ),
};

export const Sections: Story = {
  render: () => (
    <div style={stack}>
      <SectionHeader title="Pipeline" />
      <SectionHeader title="Pipeline" />
      <SectionHeader
        title="Reporting currency"
        actions={<Button>Change</Button>}
      />
      <Card>
        <SectionHeader title="Contacts" />
        <p className="t-caption">Carol Wagner · Bob Schmidt · Alice Müller</p>
      </Card>
      <Card>
        <SectionHeader title="Delivery" />
        <SectionHeader title="Endpoints" level={3} />
        <p className="t-caption">Two subscriptions, both healthy.</p>
        <SectionHeader title="Dead-lettered" level={3} />
        <p className="t-caption">Nothing waiting.</p>
      </Card>
      <Disclosure summary="Matching rules">
        <p className="t-caption">
          Closed by default for details the reader rarely needs.
        </p>
      </Disclosure>
      <Disclosure summary="Import log" open>
        <p className="t-caption">Open for a run or a new result.</p>
      </Disclosure>
    </div>
  ),
};

const RANGES = ["month", "quarter", "year"] as const;
type Range = (typeof RANGES)[number];
const RANGE_LABELS: Record<Range, string> = {
  month: "Month",
  quarter: "Quarter",
  year: "Year",
};

const SIDES = ["owner", "team"] as const;
type Side = (typeof SIDES)[number];
const SIDE_LABELS: Record<Side, string> = { owner: "Owner", team: "Team" };

// SegmentedControl is controlled: without state here the buttons never move.
function ToolbarDemo() {
  const [range, setRange] = useState<Range>("quarter");
  const [side, setSide] = useState<Side>("owner");
  const [tab, setTab] = useState<Tab>("overview");
  return (
    <div style={stack}>
      <div style={{ ...row, justifyContent: "space-between" }}>
        <SegmentedControl
          options={RANGES}
          value={range}
          onChange={setRange}
          labels={RANGE_LABELS}
          label="Reporting range"
        />
        <span className="t-caption">
          Press <Kbd>/</Kbd> to search, <Kbd>Ctrl</Kbd> <Kbd>K</Kbd> for the
          command bar, <Kbd>Esc</Kbd> to close.
        </span>
      </div>
      <SegmentedControl
        options={SIDES}
        value={side}
        onChange={setSide}
        labels={SIDE_LABELS}
        label="Target amount"
      />
      {/* `counts`: how much is behind each option, for a strip that chooses
          between bodies of a record. Partial on purpose — the first option
          here has none, which is what a section that is not a list of things
          needs, and it is NOT the same as the explicit zero on the second. */}
      <SegmentedControl
        options={TABS}
        value={tab}
        onChange={setTab}
        labels={TAB_LABELS}
        counts={{ contacts: 6, deals: 0 }}
        label="Record section"
      />
    </div>
  );
}

const TABS = ["overview", "contacts", "deals"] as const;
type Tab = (typeof TABS)[number];
const TAB_LABELS: Record<Tab, string> = {
  overview: "360",
  contacts: "Contacts",
  deals: "Deals",
};

// The toolbar pair: the segmented switch that scopes a screen and the key
// legend that sits beside it. Two options and three options are both here —
// a two-up control has no middle segment, which is where the divider rules
// break if they were written for three.
export const Toolbar: Story = {
  render: () => <ToolbarDemo />,
};

const RECORD_TABS = ["overview", "research", "documents"] as const;
type RecordTab = (typeof RECORD_TABS)[number];
const RECORD_TAB_LABELS: Record<RecordTab, string> = {
  overview: "Overview",
  research: "Data & tools",
  documents: "Documents",
};

function MarkedTabsDemo() {
  const [tab, setTab] = useState<RecordTab>("overview");
  return (
    <SegmentedControl
      options={RECORD_TABS}
      value={tab}
      onChange={setTab}
      labels={RECORD_TAB_LABELS}
      label="Record sections"
      marks={{ research: true }}
    />
  );
}

// A dot on an option says something waits behind it. It is `aria-hidden` and
// never the only carrier: the surface it points at states the fact in words.
export const MarkedOption: Story = {
  render: () => <MarkedTabsDemo />,
};

type DemoDeal = {
  id: string;
  name: string;
  stage: string;
  weighted: string;
};

const DEMO_DEALS: DemoDeal[] = [
  {
    id: "dl_1",
    name: "Globex renewal",
    stage: "Proposal",
    weighted: "48,000 EUR",
  },
  {
    id: "dl_2",
    name: "Initech platform",
    stage: "Qualify",
    weighted: "12,500 EUR",
  },
  {
    id: "dl_3",
    name: "Umbrella expansion",
    stage: "Negotiation",
    weighted: "156,000 EUR",
  },
];

const DEAL_COLUMNS = [
  { key: "name", header: "Deal", render: (deal: DemoDeal) => deal.name },
  {
    key: "stage",
    header: "Stage",
    render: (deal: DemoDeal) => <Badge tone="accent">{deal.stage}</Badge>,
  },
  {
    key: "weighted",
    header: "Weighted",
    render: (deal: DemoDeal) => <span className="t-num">{deal.weighted}</span>,
  },
];

// onRowClick is what turns a row into a link, so the story has to supply one
// and show that it fired — a cursor change alone is not evidence.
function DealTableDemo() {
  const [opened, setOpened] = useState<DemoDeal | null>(null);
  return (
    <div style={stack}>
      <DataTable
        label={"Deals"}
        columns={DEAL_COLUMNS}
        rows={DEMO_DEALS}
        rowKey={(deal) => deal.id}
        onRowClick={setOpened}
      />
      <span className="t-caption">
        {opened
          ? `Row opened: ${opened.name}`
          : "Click a row — onRowClick is what makes it a link."}
      </span>
    </div>
  );
}

// Rows and no rows. The empty table is the state a screen actually reaches
// first, and it is header-only by design: DataTable never invents a message,
// so the screen pairs it with an EmptyState of its own.
export const Tables: Story = {
  render: () => (
    <div style={stack}>
      <DealTableDemo />
      <SectionHeader title="No rows" />
      <DataTable
        label={"Deals"}
        columns={DEAL_COLUMNS}
        rows={[]}
        rowKey={(deal) => deal.id}
      />
      <EmptyState>No deals in this pipeline yet.</EmptyState>
    </div>
  ),
};

// OverflowMenu mounts its items only once opened, so the story presses the
// trigger on mount rather than giving the component a prop it does not have.
function OverflowMenuDemo({
  openOnMount,
  children,
}: Readonly<{ openOnMount: boolean; children: ReactNode }>) {
  const wrap = useRef<HTMLDivElement>(null);
  const pressed = useRef(false);
  useEffect(() => {
    // The trigger TOGGLES, so this must happen exactly once — a mount effect
    // invoked twice would open the menu and close it again.
    if (!openOnMount || pressed.current) {
      return;
    }
    pressed.current = true;
    // The trigger is the only element the wrapper holds — the panel is
    // portalled to the body — so its own container names it.
    wrap.current
      ?.querySelector<HTMLButtonElement>(".overflow-menu > button")
      ?.click();
  }, [openOnMount]);
  return (
    // The panel is anchored to its trigger and hangs off its END, as a record
    // header carries it — so the story puts the trigger at the right edge (it
    // opens inward, not off the page) and reserves the height it drops into.
    <div
      ref={wrap}
      style={{
        display: "flex",
        justifyContent: "flex-end",
        alignItems: "flex-start",
        minHeight: "18rem",
      }}
    >
      <OverflowMenu label="More actions">{children}</OverflowMenu>
    </div>
  );
}

// The resting state, which is the one a reader sees for most of a record's
// life: a single ghost square in the header, saying only that there is more.
export const OverflowClosed: Story = {
  render: () => (
    <OverflowMenuDemo openOnMount={false}>
      <Button>Merge with…</Button>
      <Button variant="danger">Archive</Button>
    </OverflowMenuDemo>
  ),
};

// The open panel with every item shape one menu can hold, each invisible with
// three tidy verbs in it: a label LEADING WITH A GLYPH (the words start on one
// x); a label past the ceiling, which wraps, since an item nobody can finish
// reading cannot be chosen; a SET row, drawn in the accent, that the menu does
// not close under; a refused row, listed because the refusal is information;
// and the destructive verb, red without its fill, below the seam.
export const Overflow: Story = {
  render: () => (
    <OverflowMenuDemo openOnMount>
      <Button>
        <Mail aria-hidden="true" />
        Email everyone on this account
      </Button>
      <Button>Merge with…</Button>
      <Button aria-expanded="true">
        <RefreshCw aria-hidden="true" />
        Runs
      </Button>
      <Button>
        Set up the partner programme for this account and its subsidiaries
      </Button>
      <Button reason="An archived account takes no writes.">Export</Button>
      <Button variant="danger">
        <Trash2 aria-hidden="true" />
        Archive
      </Button>
    </OverflowMenuDemo>
  ),
};

// The one spelling of a search input, empty and filled: the affordance is the
// icon and the type="search" clear control, and only a filled field shows it.
export const Search: Story = {
  render: () => (
    <div style={{ display: "grid", gap: "var(--space-3)", maxWidth: "22rem" }}>
      <Field label="Find a company">
        {(control) => <SearchField {...control} placeholder="Search…" />}
      </Field>
      <Field label="Find a contact">
        {(control) => <SearchField {...control} defaultValue="Anna Brandt" />}
      </Field>
    </div>
  ),
};

function ControlledDisclosureExample() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onClick={() => setOpen(true)}>Open details</Button>
      <Disclosure summary="Editable details" open={open} onToggle={setOpen}>
        <p>The reader can close and reopen this section.</p>
      </Disclosure>
    </>
  );
}

export const ControlledDisclosure: Story = {
  render: () => <ControlledDisclosureExample />,
};
