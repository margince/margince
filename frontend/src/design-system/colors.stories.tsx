// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { Heading } from "./heading";
import {
  specimenColumn,
  specimenIntro,
  specimenNote,
  specimenToken,
  type ThemedValue,
  useThemeValues,
} from "./tokenspecimen";

/**
 * Every colour token in `tokens.css`, grouped by what it is for and drawn in
 * both themes at once.
 *
 * Each row prints the token, what it is for (taken from its comment in
 * `tokens.css`, and left blank where the sheet says nothing), and the value it
 * resolves to in each theme. The values are read from the browser: the root is
 * flipped to each theme in turn inside one task and read back, so a retune in
 * `tokens.css` moves this page and a derived token is right in both columns.
 */
const meta = {
  title: "Foundations/Color/Color roles",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

type Theme = "light" | "dark";
type Kind = "fill" | "ink" | "edge" | "outline" | "glow";

type Entry = Readonly<{
  token: string;
  usage?: string;
  kind?: Kind;
  /** The token the specimen sits on; most tokens are drawn on --bgElevated. */
  on?: string;
  /** What to paint when the token is a number and not a colour. */
  paint?: string;
}>;

const THEMES: readonly Theme[] = ["light", "dark"];

const SURFACES: readonly Entry[] = [
  { token: "--bgPage", usage: "The reading ground." },
  {
    token: "--bgWell",
    usage: "A recess in an elevated surface, the well a board's stage is.",
  },
  { token: "--bgInset", usage: "A well inside a card: an inset row, a plate." },
  {
    token: "--bgElevated",
    usage: "A card, a panel, a menu.",
  },
  { token: "--bgHover", usage: "A pointer resting on a row on the page." },
  {
    token: "--pane",
    usage: "One translucent pane per zone, over the lit ground.",
  },
  {
    token: "--bgChip",
    usage:
      "A small inline fill drawn on a rung: a badge, a key-cap, a segmented strip, a meter trough. Translucent.",
  },
];

const TEXT: readonly Entry[] = [
  {
    token: "--textPrimary",
    kind: "ink",
    usage: "Headings, and a record's name where that name is the title.",
  },
  {
    token: "--textSecondary",
    kind: "ink",
    usage:
      "The one secondary neutral, for text that supports rather than carries. A placeholder is the first.",
  },
  { token: "--textOnAccent", kind: "ink", on: "--accent" },
  {
    token: "--textOnAccentControl",
    kind: "ink",
    on: "--accent",
    usage:
      "A control filled with the accent: a primary button, a step numeral.",
  },
  {
    token: "--textOnStatusControl",
    kind: "ink",
    on: "--dangerText",
    usage:
      "A control filled with a state's Text token: a destructive button, a completion disc, a primary badge.",
  },
];

const BORDERS: readonly Entry[] = [
  { token: "--borderSubtle", kind: "edge" },
  {
    token: "--borderStrong",
    kind: "edge",
    usage: "Divides content, where any visible hairline does the job.",
  },
  {
    token: "--borderControl",
    kind: "edge",
    usage:
      "The boundary of an interactive control, held to 3:1 against its ground.",
  },
  {
    token: "--paneEdge",
    kind: "edge",
    usage: "A pane's hairline edge. A border, never a fill.",
  },
];

const ACCENT: readonly Entry[] = [
  {
    token: "--accent",
    usage: "Brand and primary action, and nothing else.",
  },
  {
    token: "--accentText",
    usage: "The accent as text, split from the accent as a fill.",
  },
  {
    token: "--accentLight",
    usage: "The accent's translucent tint, and the colour of the focus glow.",
  },
  { token: "--accentMed" },
  {
    token: "--accentBrand",
    usage: "Brand emerald that stays put when the dark accent lightens.",
  },
  {
    token: "--teal",
    usage: "Identity, not status: marks our side of a diagram.",
  },
  {
    token: "--tealText",
    usage: "The teal as text, lifted for dark.",
  },
];

const AI_TOKENS: readonly Entry[] = [
  {
    token: "--ai",
    usage:
      "The one hue that means a machine did this: the agent's filled verb.",
  },
  { token: "--aiLight", usage: "The agent's tinted row." },
  {
    token: "--aiChip",
    usage: "The filled chip, and the hover of an indigo control.",
  },
  { token: "--aiMed", usage: "The edge of the agent's tint." },
  { token: "--aiText", usage: "The agent's label." },
];

const TAGS: readonly Entry[] = [
  "--tagTeal",
  "--tagAmber",
  "--tagRose",
  "--tagSlate",
  "--tagSky",
  "--tagViolet",
  "--tagLime",
  "--tagOrange",
].map((token) => ({ token }));

const AVATARS: readonly Entry[] = [
  { token: "--avatarGroundL", paint: "--avatarGround" },
  { token: "--avatarGroundC", paint: "--avatarGround" },
  { token: "--avatarBlobAL", paint: "--avatarBlobA" },
  { token: "--avatarBlobAC", paint: "--avatarBlobA" },
  { token: "--avatarBlobBL", paint: "--avatarBlobB" },
  { token: "--avatarBlobBC", paint: "--avatarBlobB" },
  { token: "--avatarInkL", paint: "--avatarInk" },
  { token: "--avatarInkC", paint: "--avatarInk" },
];

const RAIL: readonly Entry[] = [
  {
    token: "--bgRail",
    on: "--bgPage",
    usage: "The dark green field of the rail, the same in both themes.",
  },
  {
    token: "--railIconActive",
    on: "--bgRail",
    usage: "White icons and labels on the dark green field.",
  },
];

const ONBOARDING: readonly Entry[] = [
  {
    token: "--obDialogGlass",
    on: "--bgRail",
    usage: "The translucent white fill of the onboarding dialog's dark glass.",
  },
  {
    token: "--obDialogInkQuiet",
    on: "--bgRail",
    usage: "The dialog's secondary text on the dark glass.",
  },
  {
    token: "--obDialogShadow",
    on: "--bgPage",
    usage: "The tint of the dialog's drop shadow on the dark hero.",
  },
];

const ORBS: readonly Entry[] = [
  {
    token: "--orbBody",
    usage: "The working tone, declared as --ai so the two cannot drift.",
  },
  { token: "--orbGlow", usage: "A light end to glow." },
  { token: "--orbMid" },
  {
    token: "--orbBright",
    usage: "A bright end, for the one state with energy.",
  },
  {
    token: "--orbAmber",
    usage: "The agent is asking a contact for something.",
  },
  { token: "--orbRed", usage: "The run failed." },
  { token: "--orbInk" },
  { token: "--orbDeep", usage: "A dark to be seen against." },
];

const FOCUS: readonly Entry[] = [
  {
    token: "--focus-ring",
    kind: "outline",
    usage:
      "An outline for a control on a surface, drawn outside the box so it never changes the control's size.",
  },
  {
    token: "--focus-glow",
    kind: "glow",
    usage: "A glow for a field, which already has a boundary.",
  },
  {
    token: "--focus-ring-forced",
    kind: "outline",
    usage:
      "A transparent outline paired with the glow, so forced-colors mode still has a ring to paint.",
  },
  {
    token: "--focus-glow-danger",
    kind: "glow",
    usage: "A refused field keeps the glow and changes only its colour.",
  },
  {
    token: "--focus-glow-ai",
    kind: "glow",
    usage: "A field on a surface a machine authored.",
  },
];

const ATMOSPHERE: readonly Entry[] = [
  {
    token: "--overlayLight",
    on: "--bgInset",
    usage: "A highlight: pure white, unthemed, the way a glass edge works.",
  },
  {
    token: "--overlayDark",
    on: "--bgInset",
    usage: "A shade: pure black, unthemed.",
  },
  {
    token: "--overlayScrim",
    on: "--bgPage",
    usage: "The scrim behind a modal.",
  },
  {
    token: "--glowA",
    on: "--bgPage",
    usage: "The emerald light at the top-left corner, behind the sidebar.",
  },
  {
    token: "--glowB",
    on: "--bgPage",
    usage: "The indigo light at the top-right corner.",
  },
  {
    token: "--groundLit",
    on: "--bgPage",
    usage:
      "The page colour with both lights painted in, shared by the app shell and the buyer's Deal Room.",
  },
];

type State = "info" | "success" | "warning" | "danger" | "discovery";
const STATES: readonly State[] = [
  "info",
  "success",
  "warning",
  "danger",
  "discovery",
];
const STATE_ROLES: ReadonlyArray<{
  suffix: string;
  name: string;
  usage: string;
}> = [
  {
    suffix: "",
    name: "Base",
    usage:
      "A fill, a bar, a dot, a border: anything seen with nothing read on it.",
  },
  {
    suffix: "Text",
    name: "Text",
    usage:
      "Ink: badge lettering, an error line, a caption. Also the ground of a filled control.",
  },
  {
    suffix: "Surface",
    name: "Surface",
    usage: "The opaque tint, for a badge.",
  },
  {
    suffix: "Bg",
    name: "Bg",
    usage:
      "The same share left translucent, for a wash on a ground the token cannot know.",
  },
  { suffix: "Border", name: "Border", usage: "The hairline, the tint at 45%." },
];
const STATE_MEANING: Readonly<Record<State, string>> = {
  info: "The neutral report and work still in flight.",
  success: "A favourable outcome.",
  warning: "Caution before the fact.",
  danger: "The serious or irreversible one.",
  discovery: "What is new to this reader. Not a verdict about the record.",
};

const CHROME = [
  "--bgPage",
  "--bgElevated",
  "--textPrimary",
  "--textSecondary",
  "--borderSubtle",
];

function tokensOf(entries: readonly Entry[]): string[] {
  return [
    ...new Set([
      ...CHROME,
      ...entries.flatMap(({ token, paint, on }) => [
        token,
        paint ?? token,
        ...(on ? [on] : []),
      ]),
    ]),
  ];
}

type Values = ReadonlyMap<string, ThemedValue>;

function pick(values: Values, token: string, theme: Theme): string {
  return values.get(token)?.[theme] ?? "";
}

const chipBox: CSSProperties = {
  position: "relative",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  flex: "none",
  inlineSize: "var(--space-10)",
  blockSize: "28px",
  borderRadius: "var(--r-xs)",
};
const fillLayer: CSSProperties = {
  position: "absolute",
  inset: 0,
  borderRadius: "var(--r-xs)",
};

function Chip({
  kind = "fill",
  value,
  ground,
  line,
}: {
  kind?: Kind;
  value: string;
  ground: string;
  line: string;
}) {
  const shape: CSSProperties = {
    inlineSize: "var(--space-5)",
    blockSize: "var(--space-3)",
  };
  return (
    <div
      style={{ ...chipBox, background: ground, border: `1px solid ${line}` }}
    >
      {kind === "fill" ? (
        <div style={{ ...fillLayer, background: value }} />
      ) : null}
      {kind === "edge" ? (
        <div style={{ ...fillLayer, border: `3px solid ${value}` }} />
      ) : null}
      {kind === "ink" ? (
        <span style={{ color: value, font: "var(--fontHeadingXSmall)" }}>
          Aa
        </span>
      ) : null}
      {kind === "outline" ? <div style={{ ...shape, outline: value }} /> : null}
      {kind === "glow" ? <div style={{ ...shape, boxShadow: value }} /> : null}
    </div>
  );
}

const rowGrid: CSSProperties = {
  display: "grid",
  gridTemplateColumns:
    "minmax(220px, 1.1fr) minmax(240px, 1.3fr) minmax(240px, 1.3fr)",
};
const info: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  justifyContent: "center",
  gap: "var(--space-1)",
  minBlockSize: "48px",
  padding: "var(--space-2) var(--space-3) var(--space-2) 0",
  borderBlockEnd: "1px solid var(--borderSubtle)",
};
const cellBase: CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: "var(--space-3)",
  minBlockSize: "48px",
  padding: "var(--space-2) var(--space-3)",
};
const valueText: CSSProperties = {
  font: "var(--fontBodySmall)",
  overflowWrap: "anywhere",
};
const panelHeading: CSSProperties = {
  font: "var(--fontHeadingXSmall)",
  margin: 0,
};
const table: CSSProperties = { gap: 0, maxWidth: "1080px" };

function ThemeCell({
  entry,
  theme,
  values,
}: {
  entry: Entry;
  theme: Theme;
  values: Values;
}) {
  const at = (token: string) => pick(values, token, theme);
  return (
    <div
      style={{
        ...cellBase,
        background: at("--bgPage"),
        color: at("--textPrimary"),
        borderBlockEnd: `1px solid ${at("--borderSubtle")}`,
      }}
    >
      <Chip
        kind={entry.kind}
        value={at(entry.paint ?? entry.token)}
        ground={at(entry.on ?? "--bgElevated")}
        line={at("--borderSubtle")}
      />
      <code style={{ ...valueText, color: at("--textSecondary") }}>
        {at(entry.token)}
      </code>
    </div>
  );
}

function Rows({
  entries,
  intro,
}: {
  entries: readonly Entry[];
  intro: string;
}) {
  const values = useThemeValues(tokensOf(entries));
  return (
    <div className={specimenColumn} style={{ gap: "var(--space-4)" }}>
      <p className={specimenIntro}>{intro}</p>
      <div className={specimenColumn} style={table}>
        <div className={specimenNote} style={rowGrid}>
          <span style={{ padding: "var(--space-2) 0" }}>Token and use</span>
          <span style={{ padding: "var(--space-2) var(--space-3)" }}>
            Light
          </span>
          <span style={{ padding: "var(--space-2) var(--space-3)" }}>Dark</span>
        </div>
        {entries.map((entry) => (
          <div key={entry.token} style={rowGrid}>
            <div style={info}>
              <code className={specimenToken}>{entry.token}</code>
              {entry.usage ? (
                <span className={specimenNote}>{entry.usage}</span>
              ) : null}
            </div>
            {THEMES.map((theme) => (
              <ThemeCell
                key={theme}
                entry={entry}
                theme={theme}
                values={values}
              />
            ))}
          </div>
        ))}
      </div>
    </div>
  );
}

/** The surface ladder in the order a page stacks it. */
export const SurfacesAndBackgrounds: Story = {
  name: "Surfaces and backgrounds",
  render: () => (
    <Rows
      intro="Surfaces are a ladder and the order carries the meaning: a card is separated from the page by its own lightness, so the resting shadow can stay slight. Choose the rung by what the thing is: the page to read on, a well for a recess, an elevated surface for what lifts off it, a pane or chip for what sits on top. On a dark ground every surface lifts toward the light, so the ladder runs the other way and only the direction of each step is shared."
      entries={SURFACES}
    />
  ),
};

/** Two neutrals, one that carries and one that supports, and the fill inks. */
export const Text: Story = {
  render: () => (
    <Rows
      intro="Two neutral inks and the inks that sit on a fill. --textPrimary carries: names, headings, body. --textSecondary supports, and there is no third neutral. On a filled control, take the ink for the fill: --textOnAccentControl on the accent, --textOnStatusControl on a state's Text token. Each is drawn on the ground it is meant for."
      entries={TEXT}
    />
  ),
};

/** Hairlines, and a control's boundary, which is its own token. */
export const Borders: Story = {
  render: () => (
    <Rows
      intro="Choose a hairline by its job. A divider between content takes --borderStrong, where any visible line will do. A control's boundary takes --borderControl, held to 3:1 against its ground because it is often the only thing telling a reader that a rectangle accepts typing."
      entries={BORDERS}
    />
  ),
};

const matrixGrid: CSSProperties = {
  display: "grid",
  gridTemplateColumns: "minmax(150px, 190px) repeat(5, minmax(150px, 1fr))",
  gap: "var(--space-3)",
  alignItems: "start",
};

function StatusPanel({ theme, values }: { theme: Theme; values: Values }) {
  const at = (token: string) => pick(values, token, theme);
  return (
    <section
      className={specimenColumn}
      style={{
        padding: "var(--space-4)",
        background: at("--bgPage"),
        color: at("--textPrimary"),
        border: `1px solid ${at("--borderSubtle")}`,
        borderRadius: "var(--r-sm)",
      }}
    >
      <Heading size="xsmall" as="h3" style={panelHeading}>
        {theme === "light" ? "Light" : "Dark"}
      </Heading>
      <div style={matrixGrid}>
        <span />
        {STATES.map((state) => (
          <div
            key={state}
            className={specimenColumn}
            style={{ gap: "var(--space-1)" }}
          >
            <strong style={panelHeading}>{state}</strong>
            <span
              className={specimenNote}
              style={{ color: at("--textSecondary") }}
            >
              {STATE_MEANING[state]}
            </span>
          </div>
        ))}
        {STATE_ROLES.map((role) => (
          <StatusRow
            key={role.suffix}
            role={role}
            theme={theme}
            values={values}
          />
        ))}
      </div>
    </section>
  );
}

function StatusRow({
  role,
  theme,
  values,
}: {
  role: (typeof STATE_ROLES)[number];
  theme: Theme;
  values: Values;
}) {
  const at = (token: string) => pick(values, token, theme);
  return (
    <>
      <div className={specimenColumn} style={{ gap: "var(--space-1)" }}>
        <strong style={panelHeading}>{role.name}</strong>
        <span className={specimenNote} style={{ color: at("--textSecondary") }}>
          {role.usage}
        </span>
      </div>
      {STATES.map((state) => {
        const token = `--${state}${role.suffix}`;
        return (
          <div
            key={token}
            className={specimenColumn}
            style={{ gap: "var(--space-1)" }}
          >
            <div style={{ ...cellBase, padding: 0 }}>
              <Chip
                value={at(token)}
                ground={at("--bgElevated")}
                line={at("--borderSubtle")}
              />
              <code style={{ ...valueText, color: at("--textPrimary") }}>
                {token}
              </code>
            </div>
            <code style={{ ...valueText, color: at("--textSecondary") }}>
              {at(token)}
            </code>
          </div>
        );
      })}
    </>
  );
}

const STATUS_TOKENS: readonly Entry[] = STATES.flatMap((state) =>
  STATE_ROLES.map((role) => ({ token: `--${state}${role.suffix}` })),
);

/** Five states, each with one base and everything else derived from it. */
export const Status: Story = {
  render: () => {
    const values = useThemeValues(tokensOf(STATUS_TOKENS));
    return (
      <div className={specimenColumn}>
        <p className={specimenIntro}>
          Five states and never a sixth: information, success, warning, danger
          and discovery. Each has one base, the only value anybody picks, and
          the rest is derived from it in tokens.css. Choose by what the colour
          is doing: the base for what is seen with nothing read on it, Text for
          ink, Surface for a badge, Bg for a wash, Border for a hairline.
          Success stays a brighter green than the brand emerald on purpose.
        </p>
        {THEMES.map((theme) => (
          <StatusPanel key={theme} theme={theme} values={values} />
        ))}
      </div>
    );
  },
};

/** Emerald is brand and primary action, teal is identity. */
export const AccentAndBrand: Story = {
  name: "Accent and brand",
  render: () => (
    <Rows
      intro="Emerald means primary action and brand, and nothing else: at most one filled control in view wears it, and a selected row takes its wash. The teal pair marks our side of a diagram apart from the agent's indigo and the counterparty's neutral. It says nothing about how anything is going."
      entries={ACCENT}
    />
  ),
};

/** Indigo is a claim about provenance. */
export const AI: Story = {
  render: () => (
    <Rows
      intro="Indigo means a machine did this: wrote the draft, found the thing, or holds the evidence behind a claim. It marks authorship and never status, so nothing good, bad or urgent is indigo and nothing a human wrote is. Choose the step by its shape: the base for a filled verb, Light for a tinted row, Chip for a chip, Med for an edge, Text for a label."
      entries={AI_TOKENS}
    />
  ),
};

/** Eight hues that carry no meaning. */
export const Tags: Story = {
  render: () => (
    <Rows
      intro="Eight hues an admin picks from when naming a tag, and the only colours in the product that carry no meaning: a tag's colour says not that other tag. They are a family of their own so a retune of the state colours never recolours every tag, and they are kept apart at 7px and clear of 3:1 on the card ground."
      entries={TAGS}
    />
  ),
};

/** The monogram mesh, as OKLCh numbers. */
export const Avatars: Story = {
  render: () => (
    <Rows
      intro="A monogram sits on a quiet mesh of two neighbouring hues keyed on the record's id: never indigo, never danger red, never moving. The tokens are OKLCh lightness and chroma, so one record keeps its composition in both themes and only these numbers move. Avatar chooses them; the chip shows the colour each pair draws for a sample set of hues."
      entries={AVATARS}
    />
  ),
};

/** The rail is the same in both themes. */
export const RailAndWorkspaceChrome: Story = {
  name: "Rail and workspace chrome",
  render: () => (
    <Rows
      intro="The rail is deliberately unthemed: white-alpha icons on a dark green field in both themes, so the two columns match."
      entries={RAIL}
    />
  ),
};

/** The onboarding dialog's glass. */
export const OnboardingDialog: Story = {
  name: "Onboarding dialog",
  render: () => (
    <Rows
      intro="The onboarding dialog floats on the dark hero as glass: a translucent white fill, a quiet ink for secondary text and a shadow tinted to the hero's own ground. Hosted in the workbench it switches to the content inks instead."
      entries={ONBOARDING}
    />
  ),
};

/** The Core's palette, and the focus shapes. */
export const OrbsAndFocus: Story = {
  name: "Orbs and focus",
  render: () => (
    <Rows
      intro="The orb palette belongs to the Core, the agent's orb, and nothing else in the product. The working tones sit around the AI indigo, and amber and red mean an outcome, not provenance. The orb's red is deliberately not the danger red: it has to survive being a 34px glowing ball. Focus comes in two shapes: an outline for a control on a surface, a glow for a field that already has a boundary."
      entries={[...ORBS, ...FOCUS]}
    />
  ),
};

/** Material effects, not brand colour. */
export const OverlaysAndAtmosphere: Story = {
  name: "Overlays and atmosphere",
  render: () => (
    <Rows
      intro="Material effects, not brand colour: a highlight and a scrim are pure white and pure black at low alpha, unthemed so white stays white when the surface under it goes dark. The two glows are the chrome, the only decoration on the page, and atmosphere that moves is a feature, so they never animate."
      entries={ATMOSPHERE}
    />
  ),
};
