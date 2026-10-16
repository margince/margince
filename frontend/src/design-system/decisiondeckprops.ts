import type { ReactNode } from "react";
import type {
  DecisionApproval,
  DecisionCardLabels,
  DecisionCompactWords,
  DecisionDisplay,
} from "./decisioncard";
import type { DecisionSharedFacts } from "./decisiondeck.verdicts";
import type { SectionDetail, SectionState } from "./surfacestate";
import type { ConfidenceLevel, Provenance } from "./trust";

/** What a contact can say about one staged proposal. */
export type DeckVerdict = "accept" | "edit" | "reject" | "skip";

/**
 * One thing to decide: a proposal staged on its own, or one act's bundle. The
 * API decides a bundle as a unit, so it reads as a single decision.
 *
 * A bundle of one is not a bundle: the caller emits it as a `single`. A group
 * holding a single child hides the question it exists to present, and the
 * reader gains nothing for the click.
 */
export type DecisionDeckItem =
  | Readonly<{ kind: "single"; id: string; approval: DecisionApproval }>
  | Readonly<{
      kind: "bundle";
      id: string;
      bundleId: string;
      members: readonly DecisionApproval[];
    }>;

/** A verdict waiting in the tray, and what it answers. */
export type StagedDecision = Readonly<{
  id: string;
  verdict: DeckVerdict;
}>;

/**
 * The chips the caller owns on one card's meta line, plus its trust readings.
 * A function rather than a field on the item: each of these needs something
 * this tier does not hold. That is the agent tier map, the kind catalog, the
 * signed-in reader's own id, and the locale the countdown is spelled in.
 */
export type DecisionDeckChips = Readonly<{
  meta?: ReactNode;
  aside?: ReactNode;
  provenance?: Provenance;
  confidence?: ConfidenceLevel;
  /**
   * What this kind of proposal shows, in the reader's language. It rides here
   * for the same reason the chips do: resolving it needs the kind catalogue and
   * the locale, and this tier holds neither.
   */
  display?: readonly DecisionDisplay[];
}>;

/**
 * The words. All of them the caller's, for the reason `DecisionCardLabels` gives:
 * a primitive that carried its own copy would be the second author of the
 * product's vocabulary.
 *
 * The three counted ones are functions rather than templates. Plural agreement
 * belongs to the language and not to this component. A caller hands them the
 * count and gets its own catalog's answer back.
 */
export type DecisionDeckLabels = Readonly<{
  /** Forwarded to every card. */
  card: DecisionCardLabels;
  /** Names the deck as a region, and names its keyboard surface. */
  deckLabel: string;
  /** The `[Deck | List]` toggle: the group's name and its two options. */
  viewLabel: string;
  viewDeck: string;
  viewList: string;
  /** The keyboard legend. Drawn, not hidden: a shortcut nobody is told about is
   *  a shortcut for whoever wrote it.
   *
   *  It must say that the arrows stage. This deck is stage-then-commit: an
   *  arrow moves a verdict into the tray, and nothing leaves until `commit`
   *  runs. A legend reading "→ accept" beside "Enter send" suggests the arrow
   *  already sent it and Enter is a separate act. A reader who believes that
   *  presses four arrows, walks away, and has sent nothing. */
  keys: string;
  /** How many cards are still behind the live one. */
  behind: (count: number) => string;
  /** The tray and its controls, split as the commit splits it (see `StagingTray`). */
  staged: (count: number) => string;
  skipped: (count: number) => string;
  edited: (count: number) => string;
  commit: string;
  commitNothingToSend: string;
  unstage: string;
  /** The earned moment: the queue is clear, this many were decided, at this time. */
  clearedTitle: string;
  cleared: (count: number) => string;
  clearedTime: (atMs: number) => string;
  /** There was never anything here. The one sentence allowed to say that. */
  empty: string;
  /** A bundle, as one decision with N members behind it. */
  bundleSummary: (members: number) => string;
  bundleMembers: (members: number) => string;
  /**
   * The two words that turn the list form's rows dense, and the switch that
   * turns it on. A surface that has them gets one line per decision, with the
   * proposal behind a popover. One that has not keeps the full row.
   *
   * Words rather than a flag, for the reason `DecisionCompactWords` states. The
   * dense line hides the proposal behind one control and folds two verdicts
   * into another. Neither is reachable unnamed.
   */
  compactRow?: DecisionCompactWords;
}>;

export type DecisionDeckProps = Readonly<{
  /** Everything still waiting. The deck reorders nothing. */
  items: readonly DecisionDeckItem[];
  /**
   * The instant deadlines are judged against, in wall-clock ms. A prop for the
   * reason `DecisionCard` takes one: a story pins it. A test then does not
   * depend on what time the suite runs at.
   */
  now: number;
  labels: DecisionDeckLabels;
  /**
   * The deck's own heading, drawn on the same row as the Deck/List toggle. A
   * prop rather than a caller-side `SectionHeader`: a title above the deck and
   * a toggle inside it are two rows saying one thing. The row the toggle
   * belongs to is the one that names what it switches. Omitted, the deck
   * carries no heading and the toggle keeps the row to itself.
   */
  title?: string;
  /** Fired once, on the explicit commit, with what is in the tray. */
  onCommit: (staged: readonly StagedDecision[]) => void;
  /**
   * Told when a contact puts a verdict in the tray or takes it back out, for a
   * caller keeping a count of its own. Neither sends anything.
   *
   * User-triggered only. The tray also empties on its own: when a committed
   * item leaves the queue, and when a verdict that sends nothing is committed.
   * Reporting those as un-staging would tell a caller that somebody changed
   * their mind about a decision that has already gone.
   */
  onStage?: (staged: StagedDecision) => void;
  onUnstage?: (staged: StagedDecision) => void;
  /**
   * Whether the commit the caller was handed is still in flight, or came back
   * refused. A refused commit keeps the tray. The verdicts are the only copy of
   * a contact's answers, and clearing them on failure would ask for all of them
   * again.
   */
  commitState?: "idle" | "sending" | "failed";
  /** What the caller says about the read behind these items. `ready` defers to
   *  what the deck can see; anything else wins. */
  state?: SectionState;
  /** What the four states other than `ready` need to be actionable. Above all
   *  a retry: without one, `failed` is `unavailable` with extra words. */
  stateDetail?: SectionDetail;
  // Required, like the SurfaceState it forwards to. This deck cannot know what
  // its caller is waiting for. The required prop one level down exists to
  // prevent a generic line.
  loadingLabel: string;
  /** Under the tray: what a refused commit said, in the caller's words. */
  notice?: ReactNode;
  chips?: (
    approval: DecisionApproval,
    shared: DecisionSharedFacts,
  ) => DecisionDeckChips;
  /**
   * Lets the surface draw the deck's chrome. It is handed the view toggle and
   * the deck's whole content, and decides where each goes.
   *
   * For a deck inside a `Panel`, where the toggle belongs in the header band
   * beside the panel's title. One band, one interval, the same shape as every
   * other zone on the page. Without this the deck draws its own heading row.
   * A panel around it would then carry two headings: its title and a second
   * one inside its body.
   *
   * It does not lift the deck's state. The toggle is the deck's own control,
   * already wired; the frame only says where it stands.
   */
  frame?: (parts: DeckFrame) => ReactNode;
  /**
   * How many rows the list form draws before it defers to the fuller surface.
   *
   * Absent draws them all, which is right for a deck that is its own page. A
   * capped list is for a page that opens with the decisions and goes on to
   * something else. It shows three questions a reader can answer on the way
   * past, and the rest where every one of them is. The cap never reaches the
   * deck form. A stack hides what is behind the live card anyway. Capping it
   * would leave a count that says "4 more behind" over a pile that holds three.
   */
  listCap?: number;
  /** The way to the rest, drawn under a list the cap cut. */
  listRest?: (hidden: number) => ReactNode;
}>;

/**
 * The deck's parts, for a surface that frames them.
 *
 * Two rather than a slot per piece. The toggle is what a header band wants.
 * Everything else (the body, the tray, the notice) is one block that belongs
 * under it in that order. A frame that could reorder those would be a second
 * opinion about what a reader reads first.
 */
export type DeckFrame = Readonly<{
  /** The Deck/List control, or null while there is nothing to switch between. */
  toggle: ReactNode;
  /**
   * The queue itself, and `null` while every card is staged. That state has
   * nothing to draw, and it differs from an empty queue. The tray below says
   * what is true. A body reading "nothing is waiting on you" over a reader's
   * own staged verdicts would contradict it.
   */
  content: ReactNode;
  /**
   * The staging tray, and anything a refused commit said under it. Null while
   * the tray is empty and the last commit was clean.
   *
   * Apart from `content` because of where it goes. It belongs to the whole
   * zone rather than to the queue, so a surface puts it in its own band. In a
   * `Panel` that is the foot, edge to edge with a hairline over it. The tray
   * then stops drawing the standalone box it needs when it floats at a page's
   * foot.
   */
  tray: ReactNode;
}>;
