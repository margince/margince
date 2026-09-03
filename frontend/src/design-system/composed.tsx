import {
  CalendarClock,
  CheckSquare,
  Lock,
  Mail,
  MessageCircle,
  PencilLine,
  Phone,
  StickyNote,
} from "lucide-react";
import type { ReactNode } from "react";
import { Fragment, useLayoutEffect, useMemo, useRef, useState } from "react";
import { splitEmailBody } from "../format/emailtext";
import {
  formatDate,
  formatDuration,
  formatMoneyOrAbsent,
  formatNumber,
  formatTimeOfDay,
} from "../format/format";
import { type Locale, translatePlural, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { EmailEntry } from "./emailentry";

type RowTag = components["schemas"]["RowTag"];

import type { components } from "../api/schema";
import { Avatar, Badge, Button } from "./atoms";
import { PageZones, type PageZonesShape } from "./pagezones";
import { FieldGuard } from "./rbac";
import { RowTags } from "./rowtags";
import { useTruncationTooltip } from "./tooltip";
import { type Provenance, ProvenanceTag } from "./trust";
import "./composed.css";

// Composed surfaces (B-EP09.3b): the pipeline board and the record view — each
// consumes the 3a trust primitives so staged / real / human-typed stay three
// distinguishable styles through composition.

// ----- Pipeline board -----

export type BoardRecord = {
  id: string;
  name: string;
};

export type BoardDeal = BoardRecord & {
  /**
   * The company this deal is with, as a name a reader recognises. Empty for a
   * deal that names no company, which is the one reading that draws nothing.
   */
  org: string;
  /** The company's resolved mark. Absent leaves the monogram, which is the
   *  floor rather than a fallback. */
  orgLogoUrl?: string | null;
  /**
   * The company is not this reader's to read: the wire sent no id and named the
   * field in `masked_fields`, so the slot carries the MASK rather than a name.
   *
   * A flag rather than a node in `org`, for the reason `TimelineEntry.withheld`
   * is one: the withheld reading is this tier's to spell, and a caller handing
   * in its own words for it is how one reading ends up with two spellings. It
   * also keeps the mark honest — a monogram cut from the word for "withheld"
   * would be a mark no company has.
   */
  orgWithheld?: boolean;
  /**
   * The company's name could not be READ — the caller's lookup failed rather
   * than answering. A distinct flag from `orgWithheld`, because the two say
   * opposite things about the reader: withheld means the answer exists and is
   * not theirs, unreadable means nobody got an answer at all.
   *
   * It exists because the alternative is worse than either. An empty `org` is
   * the reading for a deal that names NO company, and a failed lookup falling
   * into it tells the reader the deal is unlinked when it is linked to a
   * company they simply could not fetch. The table's own company cell has had
   * this reading all along; this is the card's half of it.
   */
  orgUnreadable?: boolean;
  /**
   * The deal's money, as the two halves it actually has: an integer minor
   * amount and its ISO currency, either of which can be missing on a deal
   * nobody has priced. Required and nullable rather than optional, because a
   * caller owes the card an answer about the money — `null` is that answer
   * where there is no figure, and the card draws it as absent.
   *
   * Definite types here are what made every caller invent a currency: a prop
   * demanding a `string` leaves `?? "EUR"` as the only way to satisfy it, and
   * a euro sign on a figure that might be dong is worse than no figure.
   */
  valueMinor: number | null;
  currency: string | null;
  ageMs: number;
  /** How this deal is filed. The board draws the same chip strip a list row
   *  does, so a reader moving between the two reads one thing one way. */
  tags?: readonly RowTag[];
  stalled?: boolean;
  singleThreaded?: boolean;
  staged?: boolean;
  archived?: boolean;
};

export type BoardColumn<Record extends BoardRecord = BoardDeal> = {
  stage: string;
  label: string;
  deals: Record[];
  /**
   * The stage's true deal count, independent of how many `deals` (cards)
   * loaded — a caller with a capped/paginated card fetch (the Kanban board)
   * has a real count from a server aggregate that `deals.length` cannot
   * give it. Falls back to `deals.length` when absent, which is correct for
   * a caller whose card list IS the whole stage.
   */
  count?: number;
  /**
   * The stage holds deals in more than one currency, so it has no total to
   * state — native minor units are never summed across currencies. The column
   * then reports how many deals it holds and no figure at all, rather than a
   * zero that reads as an empty stage.
   */
  sumHidden?: boolean;
};

/**
 * A column on a MONEY board: the stage's figures, which its header states.
 *
 * Separate from the base rather than four optional fields on it, because
 * optional is not what these are — a deal column without them renders
 * `undefined%` in its header, which is worse than failing to compile. The
 * variant that reads them is the variant that requires them.
 *
 * The three money fields are nullable, which is not the same as optional: the
 * caller still owes every column an answer, and `null` is the answer where the
 * server stated no figure — an aggregate over unpriced deals, or a stage whose
 * rows name no currency. Drawn as absent, never as a zero: a stage total of
 * `€0.00` beside a real deal count reads as an empty stage.
 */
export type BoardMoneyColumn = BoardColumn<BoardDeal> & {
  probabilityPct: number;
  rawMinor: number | null;
  weightedMinor: number | null;
  currency: string | null;
};

/**
 * The company slot on a deal card, in the four readings a company has.
 *
 * Withheld is the MASK — the same `FieldGuard` control the deals table's
 * company cell draws, so one refusal has one spelling wherever it is read. A
 * name that could not be read says so, in the same words the shared reference
 * resolver uses for its own failed read. A company the caller named takes its
 * mark and its name. Only a deal that names no company draws nothing, which is
 * the one reading an empty slot states truthfully.
 */
function DealCardCompany({ deal }: Readonly<{ deal: BoardDeal }>) {
  const t = useT();
  if (deal.orgWithheld) {
    return (
      <span className="deal-org">
        <FieldGuard mode="masked" />
      </span>
    );
  }
  if (deal.orgUnreadable) {
    return (
      <span className="deal-org">
        <span className="deal-org-name">{t("ref.nameLoadFailed")}</span>
      </span>
    );
  }
  if (!deal.org) {
    return null;
  }
  return (
    <span className="deal-org">
      <Avatar name={deal.org} src={deal.orgLogoUrl} shape="organization" />
      {/* The name needs a box of its own to be truncated in: a bare text node
          has nothing for the ellipsis to apply to, and wraps under its own
          mark instead. */}
      <span className="deal-org-name">{deal.org}</span>
    </span>
  );
}

export function DealCard({
  deal,
  href,
  onOpen,
  dragHandlers,
}: Readonly<{
  deal: BoardDeal;
  /**
   * The deal's own address.
   *
   * An anchor and not a button, which is what this was: a card that opens a
   * record is a link, and drawn as a button it could not be opened in a new
   * tab, middle-clicked, or copied — while every other record row in the
   * product could. The board is the one surface where a rep wants three deals
   * open side by side, so it was the worst place to lose that.
   *
   * The address arrives as a prop because this tier holds no routes: it is the
   * same reason `OffsiteLink` takes an href and `ProjectLinks` takes an
   * adapter.
   */
  href: string;
  /**
   * What a press does BESIDE following the link, and the reason the event
   * comes with it: the board's card is draggable, and the click that ends a
   * drag must not also navigate. A caller that needs to refuse the press calls
   * `preventDefault` on the event it is handed.
   */
  onOpen?: (deal: BoardDeal, event: React.MouseEvent) => void;
  dragHandlers?: {
    draggable: true;
    onDragStart: (event: React.DragEvent) => void;
  };
}>) {
  const t = useT();
  const { locale } = useLocale();
  // No `stalled` class: the warn Badge below says it in words, and an edge
  // stripe saying the same thing is one statement drawn twice — the reader who
  // cannot see colour reads the badge, and the reader who can read both.
  const classes = [
    "deal-card",
    deal.staged ? "staged" : "",
    deal.archived ? "archived" : "",
  ]
    .filter(Boolean)
    .join(" ");
  return (
    <a
      href={href}
      className={classes}
      data-deal={deal.id}
      onClick={(event) => onOpen?.(deal, event)}
      {...dragHandlers}
    >
      <span className="deal-name">{deal.name}</span>
      <DealCardCompany deal={deal} />
      <RowTags tags={deal.tags} />
      <span className="deal-meta">
        <span className="deal-value">
          {formatMoneyOrAbsent(deal.valueMinor, deal.currency, locale)}
        </span>
        <span>{formatDuration(deal.ageMs, locale)}</span>
        {deal.archived && <Badge>{t("deal.archived")}</Badge>}
        {deal.stalled && <Badge tone="warn">{t("deal.stalled")}</Badge>}
        {deal.singleThreaded && (
          <Badge tone="danger">{t("deal.singleThreaded")}</Badge>
        )}
        {deal.staged && <Badge tone="ai">{t("deal.staged")}</Badge>}
      </span>
    </a>
  );
}

type BoardHandlers<Record extends BoardRecord> = {
  countLabel?: (count: number) => string;
  columnExtras?: (column: BoardColumn<Record>) => ReactNode;
  cardDragHandlers?: (
    record: Record,
    column: BoardColumn<Record>,
  ) => {
    draggable: true;
    onDragStart: (event: React.DragEvent) => void;
  };
  columnDropHandlers?: (column: BoardColumn<Record>) => {
    onDragOver: (event: React.DragEvent) => void;
    onDrop: (event: React.DragEvent) => void;
    onDragLeave: (event: React.DragEvent) => void;
  };
};

type PlainBoardProps<Record extends BoardRecord> = BoardHandlers<Record> & {
  variant: "plain";
  columns: BoardColumn<Record>[];
  renderCard: (record: Record, column: BoardColumn<Record>) => ReactNode;
};

type DealBoardProps = BoardHandlers<BoardDeal> & {
  variant?: "deal";
  columns: BoardMoneyColumn[];
  /**
   * Each card's own address. Required, because every card on this board opens a
   * deal and a card that opens a record is a link — see `DealCard`'s `href`.
   * A function rather than a field on the deal: the ADDRESS is the screen's
   * vocabulary and this tier holds no routes.
   */
  cardHref: (deal: BoardDeal) => string;
  onOpen?: (deal: BoardDeal, event: React.MouseEvent) => void;
};

type BoardLayoutProps<Record extends BoardRecord> = BoardHandlers<Record> & {
  columns: BoardColumn<Record>[];
  renderCard: (record: Record, column: BoardColumn<Record>) => ReactNode;
  moneyFor: (column: BoardColumn<Record>) => BoardMoneyColumn | undefined;
};

function BoardLayout<Record extends BoardRecord>({
  columns,
  renderCard,
  countLabel,
  columnExtras,
  columnDropHandlers,
  moneyFor,
}: Readonly<BoardLayoutProps<Record>>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <div className="board">
      {columns.map((column) => {
        const money = moneyFor(column);
        return (
          <section
            key={column.stage}
            className="board-col"
            data-stage={column.stage}
            aria-label={column.label}
            {...columnDropHandlers?.(column)}
          >
            {/* THE STAGE AND HOW MUCH IS IN IT, on one line and stuck to the
                top of the column. A board is scrolled, and a reader halfway
                down a long stage had nothing on screen saying which stage they
                were in — the head is the only thing that says it, so it holds
                its place. The count moved up here with it: it is the figure a
                reader compares ACROSS the board, and under the money totals it
                was the third figure on a two-line sub. */}
            <div className="board-col-head">
              <span className="stage">{column.label}</span>
              {/* TWO SPANS, not one composed string. The name is data of
                  unbounded length and truncates; the count is three characters
                  and must not. Written as "{label}: {count}" into the truncating
                  span, a long stage name ellipsised the figure away — which is
                  the one thing this head was rearranged to keep on screen.
                  Hidden from a screen reader, which is told "12 deals" below
                  with the unit this bare figure leaves out. */}
              <span className="board-col-count" aria-hidden="true">
                {formatNumber(column.count ?? column.deals.length, locale)}
              </span>
              {money && (
                <span className="prob">
                  {formatNumber(money.probabilityPct, locale)}%
                </span>
              )}
            </div>
            {/* The stage's total is the figure being scanned down the board, so it
              leads with the deal count beside it; the weighted figure is its own
              server-sourced total (not derived from the raw total) and reads
              underneath rather than competing on the line. */}
            <div className="board-col-sub">
              <span className="board-col-total">
                {money && !column.sumHidden && (
                  <span className="board-col-money">
                    {formatMoneyOrAbsent(
                      money.rawMinor,
                      money.currency,
                      locale,
                    )}
                  </span>
                )}
                {/* The count is in the head; what a screen reader needs here
                    is the UNIT the head's bare figure leaves out, so the column
                    announces "12 deals" rather than "Qualified: 12". */}
                <span className="sr-only">
                  {countLabel
                    ? countLabel(column.count ?? column.deals.length)
                    : t("board.count", {
                        count: formatNumber(
                          column.count ?? column.deals.length,
                          locale,
                        ),
                      })}
                </span>
              </span>
              {money && !column.sumHidden && (
                <span className="board-col-weighted">
                  {t("board.weighted", {
                    value: formatMoneyOrAbsent(
                      money.weightedMinor,
                      money.currency,
                      locale,
                    ),
                  })}
                </span>
              )}
              {/* A refused sum SAYS it was refused. Omitting the figure and
                  leaving the count alone is a blank where a total belongs, and a
                  blank cannot distinguish "these are in two currencies, so no
                  one total means anything" from "nobody has priced these" — a
                  column that has both a real reason and no words for it reads as
                  a column that failed to load. On a board whose stages hold
                  euros, dollars and dong, five of six columns are this state. */}
              {money && column.sumHidden && (
                <span className="board-col-weighted">
                  {t("board.mixedCurrencies")}
                </span>
              )}
            </div>
            {column.deals.map((record) => (
              <Fragment key={record.id}>{renderCard(record, column)}</Fragment>
            ))}
            {columnExtras?.(column)}
          </section>
        );
      })}
    </div>
  );
}

/**
 * The variant decides what a board column must carry. Deal boards require
 * money and probability; plain boards accept any identified record and make
 * their card renderer explicit, so a lead never pretends to be a deal.
 */
export function PipelineBoard<Record extends BoardRecord>(
  props: Readonly<PlainBoardProps<Record>>,
): ReactNode;
export function PipelineBoard(props: Readonly<DealBoardProps>): ReactNode;
export function PipelineBoard<Record extends BoardRecord>(
  props: Readonly<PlainBoardProps<Record> | DealBoardProps>,
): ReactNode {
  if (props.variant === "plain") {
    return <BoardLayout {...props} moneyFor={() => undefined} />;
  }
  return (
    <BoardLayout
      {...props}
      renderCard={(deal, column) => (
        <DealCard
          deal={deal}
          href={props.cardHref(deal)}
          onOpen={props.onOpen}
          dragHandlers={props.cardDragHandlers?.(deal, column)}
        />
      )}
      moneyFor={(column) =>
        props.columns.find((candidate) => candidate.stage === column.stage)
      }
      cardDragHandlers={undefined}
    />
  );
}

// ----- Record view + timeline -----

/**
 * TimelineGroup is a run of entries the reader sees as ONE event: a
 * conversation, or one message sent to several people. It lives here with the
 * component that renders it — the rules that BUILD one are a screen concern,
 * but the shape is the list's own vocabulary.
 */
export type TimelineGroup = {
  /** The newest member's id; the list keys on it. */
  id: string;
  kind: "thread" | "bulk" | "single";
  /** Newest first, like the list itself. */
  entries: TimelineEntry[];
  /** This group may continue past what the page holds. */
  partial: boolean;
};

type EmailSummary = components["schemas"]["EmailSummary"];

export type TimelineEntry = {
  id: string;
  // The backend's activity kinds, not a reduced set: collapsing call, task
  // and the chat kinds into "note" told the reader an email was a note.
  //
  // `change` is not an activity: it is a field edit projected from the audit
  // spine. It rides the same list because what was said to an account and what
  // was changed about it are one chronology to the person reading them — kept
  // apart, a rep comparing "we told them X" against "someone set stage to Y"
  // had to hold two orderings in their head.
  kind: "email" | "meeting" | "note" | "call" | "task" | "message" | "change";
  title: string;
  atIso: string;
  provenance: Provenance;
  // A right-aligned per-row action slot (Reply / Relink). Absent on rows with
  // no affordance, which render exactly as before.
  actions?: ReactNode;
  // The records this entry is about, as the backend's links[] reports them —
  // already pruned to what the reader may see.
  via?: ReactNode;
  /**
   * Which way it went, when the record knows: `outbound` is us reaching out,
   * `inbound` is them coming back.
   *
   * A single undifferentiated stream reads as "things happened here" and hides
   * the one shape a rep is looking for before they reach out — whether the last
   * few moves were all ours. Absent on kinds that have no direction (a note, a
   * task), which render exactly as before.
   */
  direction?: "inbound" | "outbound" | null;
  /**
   * Who was on the other end, already resolved to names by the caller.
   *
   * "We sent" answers half a question. The half a reader came for is WHO it
   * went to: an account with four contacts has four different meanings of "we
   * sent", and the row that does not say which one is a row they have to open.
   */
  counterparts?: string;
  /**
   * The server's own row model for an email, present exactly when `kind` is
   * `email`. It is what EmailEntry draws, so the timeline hands the canonical
   * row its data rather than re-deriving a reading of the message here — the
   * four independent readings this component was one of are what the canonical
   * row exists to replace.
   */
  emailSummary?: EmailSummary;
  /**
   * Opens the canonical detail for this message. The caller's, because the
   * drawer is mounted by the screen rather than by the list — absent leaves
   * the row readable and not openable, which is what a surface with nowhere to
   * open it should draw.
   */
  onOpenEmail?: () => void;
  /**
   * What KIND of thing happened to the record, in the words a reader uses:
   * "field updated", "completed". It sits where an exchange's direction sits,
   * because it answers the same question — the badge says what sort of entry
   * this is, and this says what it did.
   */
  qualifier?: string;
  /**
   * The provider's own conversation id, when capture stamped one. It is what
   * makes a thread a thread — a subject match would merge two unrelated
   * "Re: Update" exchanges and split one renamed mid-conversation.
   */
  threadKey?: string | null;
  /**
   * The SENDER declared this message bulk (RFC 2369 List-Unsubscribe). Per
   * message, never per sender: the same address sends a newsletter and a reply.
   */
  bulkAttested?: boolean;
  /**
   * The message's own subject, when it had one — NOT the rendered `title`,
   * which falls back to the body (or to the kind) so a subjectless row still
   * has something to show. Bulk grouping keys on this: keyed on the title, two
   * subjectless messages that happen to render the same text would fold into
   * one summary and hide each other.
   */
  subject?: string | null;
  /**
   * What the message actually said.
   *
   * A timeline of subject lines is a list of things you cannot read: the rep
   * knows an email happened and still has to leave for their mail client to
   * find out what was in it. The body rides along in the same composite read
   * the row came from, so showing it costs nothing.
   *
   * Legitimately absent on a row whose body was erased under retention or an
   * Art. 17 request, which is why this is optional rather than empty string.
   */
  body?: string | null;
  /**
   * The row is discoverable but its CONTENT is not this reader's: the
   * activity's audience was limited by a human and the reader is not in it.
   * The row keeps its place — date, direction, kind — and says so, rather
   * than vanishing into what reads as silence (the withheld state of
   * README § "Absent, disabled, or withheld").
   */
  withheld?: boolean;
  /**
   * Rendered content for a row whose substance is not prose — the old→new
   * diff on a `change` row. Sits where the body would, so a change reads at
   * the same place in the row as a message does.
   */
  detail?: ReactNode;
};

// One icon for every kind, and ONE for every messaging transport: since
// ADR-0107/A158 the timeline no longer knows a Telegram row from a WhatsApp one
// by its kind, so a per-transport icon here would be a map this file cannot
// keep — an extension unit's transport has no entry and never will. The row
// names its transport in the label instead.
// The kind in a word, beside the row rather than as an icon alone: a glyph is
// a thing to recognise and a word is a thing to read, and a chronology mixing
// mail, meetings and field edits is scanned by the second.
const TIMELINE_KIND_LABEL = {
  email: "timeline.kind.email",
  meeting: "timeline.kind.meeting",
  note: "timeline.kind.note",
  call: "timeline.kind.call",
  task: "timeline.kind.task",
  message: "timeline.kind.message",
  change: "timeline.kind.change",
} as const satisfies Record<TimelineEntry["kind"], MessageKey>;

/**
 * The mark on the axis: solid for something that was SAID, hollow for
 * something that merely changed.
 *
 * A reader following a conversation can then track the solid dots straight
 * past the field edits between them without reading either — which is the
 * whole reason the two kinds share one column instead of two tabs.
 */
// Solid for something SAID, hollow for something that merely changed, and
// indigo for a change the agent made: the rail says who wrote before a reader
// reads a word of it.
function dotClass(entry: TimelineEntry): string {
  if (entry.kind !== "change") {
    return "tl-dot";
  }
  return entry.provenance.kind === "agent"
    ? "tl-dot tl-dot-agent"
    : "tl-dot tl-dot-quiet";
}

const TIMELINE_ICON = {
  email: Mail,
  meeting: CalendarClock,
  note: StickyNote,
  call: Phone,
  task: CheckSquare,
  message: MessageCircle,
  change: PencilLine,
} as const;

// Where a record's verbs land. Three places can hold them — beside the
// standing column, on the identity's own row, or in a band under the header —
// and the choice is made once here rather than restated as a condition at
// each of the three, where a reader had to hold all three at once to know
// which one wins.
function actionsPlacement(
  actions: ReactNode,
  inline: boolean | undefined,
  controls: ReactNode,
): "none" | "inline" | "controls" | "below" {
  if (!actions) {
    return "none";
  }
  if (inline) {
    return "inline";
  }
  return controls ? "controls" : "below";
}

// The identity block: who this record is, and the verbs and standing that
// belong beside the name rather than under it. Split from RecordView because
// the two answer different questions — this one what the record IS, the other
// how its columns are laid out — and reading either meant holding both.
function RecordHead({
  name,
  avatarSrc,
  nameBadge,
  subtitle,
  pulse,
  badges,
  controls,
  actions,
  actionsAt,
  wide,
  markShape,
}: Readonly<{
  name: string;
  avatarSrc?: string | null;
  nameBadge?: ReactNode;
  subtitle?: ReactNode;
  pulse?: ReactNode;
  badges?: ReactNode;
  controls?: ReactNode;
  actions?: ReactNode;
  actionsAt: "none" | "inline" | "controls" | "below";
  wide: boolean;
  markShape: "person" | "organization";
}>) {
  const nameTip = useTruncationTooltip<HTMLHeadingElement>(name);
  return (
    <header className={wide ? "record-head record-head-wide" : "record-head"}>
      {/* The wide header's chip is the page's own mark rather than a marker
          beside a name, so it takes the largest rung. This used to be a
          `.record-head-wide .avatar` override in composed.css, which meant the
          chip's size was decided by a class on its parent and the `size` prop
          said something that was not true. */}
      <Avatar
        name={name}
        src={avatarSrc}
        size={wide ? "xl" : "md"}
        shape={markShape}
      />
      <div className="record-id">
        {/* The record page's name, and the one badge that belongs on ITS
            OWN line — a record's standing, read immediately after what it
            is named, not one fact among the others under it. A div, not a
            p, for the same reason as record-sub below: a caller passing
            structure there must not land inside a paragraph the browser
            silently un-nests. */}
        <div className="record-name-row">
          {/* The shell's page head yields to it on a record route — it
              prints the trail that leads here and nothing at heading
              level, so this stays the page's one h1. */}
          {/* A record's name is user data of unbounded length. It is drawn on
              one line and truncated rather than allowed to grow the header;
              the tooltip is what carries the whole of it, and appears only
              when there was more name than row. */}
          <h1 ref={nameTip.ref} {...nameTip.trigger}>
            {name}
            {nameTip.tip}
          </h1>
          {nameBadge}
        </div>
        {/* A div, not a p: a caller passing structure — the company page's
            description line plus its chip row — would otherwise nest block
            elements inside a paragraph, which the browser silently un-nests,
            leaving the chips outside the header they belong to. */}
        {subtitle && <div className="record-sub">{subtitle}</div>}
        {pulse && <div className="record-pulse">{pulse}</div>}
      </div>
      {badges && <div className="record-badges">{badges}</div>}
      {/* The record's standing and its verbs, stacked at the top right. Only a
          caller that passes `controls` gets this column: every other record
          keeps the action row under the header, which is where its own layout
          puts it. */}
      {controls && (
        <div className="record-controls">
          {controls}
          {actionsAt === "controls" && (
            <div className="record-actions">{actions}</div>
          )}
        </div>
      )}
      {actionsAt === "inline" && (
        <div className="record-actions record-actions-inline">{actions}</div>
      )}
    </header>
  );
}

export function RecordView({
  name,
  avatarSrc,
  nameBadge,
  subtitle,
  badges,
  pulse,
  actions,
  controls,
  wide,
  markShape = "person",
  actionsInline,
  band,
  rail,
  railLabel,
  aside,
  asideLabel,
  timeline,
  timelineGroups,
  onOpenThread,
  timelineHeader,
  timelineFooter,
  timelineNotice,
  tabs,
  zone,
  children,
}: Readonly<{
  name: string;
  // The record's own image for the header chip — a company's resolved logo.
  // Null or absent renders the deterministic monogram, which is the floor for
  // every record type that has no image at all.
  avatarSrc?: string | null;
  // The record's standing, read on the SAME line as its name rather than as
  // one more fact under it — the company page's editable lifecycle badge.
  // Absent on every record that has no such single, always-shown value.
  nameBadge?: ReactNode;
  // A string for the records whose subtitle IS one line of joined facts, or a
  // node for a record that needs structure under its name — the company page's
  // editable description plus its row of attribute chips.
  subtitle?: ReactNode;
  badges?: ReactNode;
  // A one-line "state of this record" strip under the name — warmth, last
  // touch, owner. Absent on records that have no such summary.
  pulse?: ReactNode;
  // The record's verbs, kept beside the identity rather than scattered
  // through the body.
  actions?: ReactNode;
  // The record's standing — the values a reader changes in place rather than
  // acts on: lifecycle, owner. Passing it moves the action row up beside them,
  // which is the company page's layout; a record that passes none keeps the
  // action row under the header.
  controls?: ReactNode;
  // What KIND of record this is, which decides whether its mark is drawn round
  // like a face or as a rounded square like a logo. Defaults to `person`,
  // which is what every record but an organization is.
  markShape?: "person" | "organization";
  // Forces the record-head-wide sizing (display-size name, the page's own
  // mark, wrapping identity block) independent of `controls`. For a record whose standing
  // lives inside `pulse` itself rather than in a stacked controls column —
  // the company page's shape — and still wants the same scale.
  wide?: boolean;
  // Puts `actions` on the SAME row as the identity block, right-aligned,
  // instead of the default full-width row underneath the header (or the
  // stacked column `controls` produces). An explicit opt-in: every other
  // record keeps its actions where its own layout already puts them.
  actionsInline?: boolean;
  // The three-zone record page: rail is the left column (what this record
  // IS), children the middle (what is happening), aside the right (the
  // business around it). With neither rail nor aside the layout collapses
  // to the single column every existing caller already renders.
  // Full-width content between the identity and the columns: the account's
  // readings and its tab bar. Absent on a record that has neither.
  band?: ReactNode;
  rail?: ReactNode;
  // What the rail column IS, on the same rule as asideLabel below: it defaults
  // to the record's profile because that is what a rail usually holds, and a
  // page whose rail holds something else names it. A record page that also has
  // a Profile TAB is exactly that case — two regions called "Profile", one of
  // them wrong, is a dead end for anyone navigating by landmark.
  railLabel?: string;
  aside?: ReactNode;
  // What the aside column IS, for a reader navigating by landmark. Defaults to
  // the record's context; a page whose aside holds something else names it,
  // because two regions with one name is a dead end for anyone moving between
  // them.
  asideLabel?: string;
  // The entries, or undefined when this view has NO timeline at all. The
  // distinction is the same one every card on a record page keeps: absent is
  // not empty. `[]` renders the section with its honest "nothing logged yet";
  // undefined omits the section, for a view whose body is not a history.
  timeline?: TimelineEntry[];
  /**
   * When set, the timeline renders CONVERSATIONS rather than messages. The
   * flat list stays the default: a person's timeline is a handful of rows and
   * grouping it would collapse events that were never one.
   */
  timelineGroups?: readonly TimelineGroup[];
  onOpenThread?: (threadKey: string) => void;
  // Controls above the timeline list (filters), and below it (load more).
  timelineHeader?: ReactNode;
  timelineFooter?: ReactNode;
  // When set, replaces the timeline list — e.g. an overlay-mode "not available"
  // note, since the mirror cannot serve entity-scoped activity reads. Keeps the
  // section honest instead of rendering an empty list that reads as "no activity".
  timelineNotice?: ReactNode;
  // The bar that chooses which part of the record is below it. It runs the
  // full width over the columns, because the details pane opens under it
  // (DESIGN.md §6): the switch at the row's end governs the column beside the
  // work, and a strip confined to the work column would end where the pane it
  // opens begins. The slot exists so the interval under it and the
  // one-row-that-scrolls behaviour are the record page's, spelled once — two
  // pages had already written the same wrapper under two names.
  tabs?: ReactNode;
  zone: string;
  children?: ReactNode;
}>) {
  const t = useT();
  // The grid follows which slots are actually filled, because a three-column
  // template with an empty column does not collapse: it reserves the space and
  // leaves the story narrower than the rail beside it.
  const shape = zonesShape(Boolean(rail), Boolean(aside));
  // Also when the verbs sit on the identity's own row: that record's block is
  // a name over a description, a chip row and a meta line, and centring the
  // mark against a stack that tall floats it to the middle of the chips
  // instead of beside the name it belongs to.
  const headerWide = Boolean(controls) || Boolean(actionsInline) || wide;
  const actionsAt = actionsPlacement(actions, actionsInline, controls);
  return (
    /* The record's own blocks arrive in order — head, then actions, then the
       band, then the zones — rather than the whole record fading in as one
       plate. It is an `.arrive-stack` and therefore not itself an arriving
       block (design-system/enter.css), which is what keeps the two fades from
       multiplying. */
    <div className="arrive-stack">
      <RecordHead
        name={name}
        avatarSrc={avatarSrc}
        nameBadge={nameBadge}
        subtitle={subtitle}
        pulse={pulse}
        badges={badges}
        controls={controls}
        actions={actions}
        actionsAt={actionsAt}
        wide={Boolean(headerWide)}
        markShape={markShape}
      />
      {actionsAt === "below" && <div className="record-actions">{actions}</div>}
      {/* The band runs the full width of the record, between the identity and
          the columns. What describes the WHOLE account — its readings, the bar
          that chooses which part of it to read — belongs here rather than in
          the work column, where it would sit beside the rail as though it were
          one more thing to read rather than the frame around all of them. */}
      {band && <div className="record-band">{band}</div>}
      {tabs && <div className="record-tabs">{tabs}</div>}
      <PageZones
        shape={shape}
        className={zonesClassName(shape)}
        rail={rail}
        railLabel={railLabel ?? t("record.profile")}
        railClassName="record-rail"
        /* An `.arrive-stack`: the work column's blocks arrive one after the
           next, and — because a tab's panel is a fresh element while the strip
           above it is not — switching tabs fades the new panel in without
           touching the strip. That is the whole tab-panel transition; no
           wrapper, no state, and nothing to keep in step with the strip. */
        mainClassName="arrive-stack"
        main={
          <>
            {children}
            {timeline && (
              <section aria-label={t("record.timeline")}>
                <h2 className="t-sub">{t("record.timeline")}</h2>
                {/* The dials above the list are one block with one rhythm: the
                    cuts through the chronology, then the narrowing of whichever
                    cut is open. Rendered as bare siblings they touched, and two
                    rows of controls with no interval between them read as one
                    control that has wrapped. */}
                {timelineHeader && (
                  <div className="timeline-header">{timelineHeader}</div>
                )}
                {/* The chronology sits on a card of its own, like every other
                    body on the page. Loose on the page ground it read as the
                    page's own text rather than as one of the record's
                    sections, and the rail down its left had nothing to run
                    inside. The notice takes no card: a sentence about why
                    there are no rows is not a list of them. */}
                {timelineNotice ?? (
                  <div className="timeline-card">
                    {timelineGroups ? (
                      <GroupedTimelineList
                        groups={timelineGroups}
                        zone={zone}
                        onOpenThread={onOpenThread}
                      />
                    ) : (
                      <TimelineList entries={timeline} zone={zone} />
                    )}
                  </div>
                )}
                {timelineFooter}
              </section>
            )}
          </>
        }
        aside={aside}
        asideLabel={asideLabel ?? t("record.context")}
        asideClassName="record-aside"
      />
    </div>
  );
}

// Which columns this record actually has. The grid itself is `PageZones` — a
// record page is one page shape among others, and the ratios and the folds are
// not the record's to own.
function zonesShape(hasRail: boolean, hasAside: boolean): PageZonesShape {
  if (hasRail && hasAside) {
    return "both";
  }
  if (hasRail) {
    return "rail";
  }
  if (hasAside) {
    return "aside";
  }
  return "single";
}

// What the record adds to the grid container on top of the layout.
//
// `arrive-stack` on every shape, including the one with no columns: a record's
// blocks arrive individually (design-system/enter.css), and a container that is
// a stack does not itself arrive. Leaving one link of that chain unmarked is
// what makes a block fade in BEHIND a parent that is still fading in — two
// fades multiplied, which reads as the content being dim rather than as it
// arriving.
//
// `record-zones` carries only the phone bottom clearance for the sticky action
// bar (composed.css), which is why the single-column shape does not get it: a
// record with no columns never had it either.
function zonesClassName(shape: PageZonesShape): string {
  return shape === "single" ? "arrive-stack" : "record-zones arrive-stack";
}

// A link inside a captured message, rendered as an element rather than as
// markup: the body is escaped text and stays that way. The visible label is the
// URL itself, so the destination a reader checks is the destination they get —
// a mail we did not write is not a place to render a friendly label over a
// different address.
const URL_PATTERN = /https?:\/\/[^\s<>"')\]]+[^\s<>"')\].,;:!?]/g;

function linkify(text: string): ReactNode[] {
  const out: ReactNode[] = [];
  let last = 0;
  for (const match of text.matchAll(URL_PATTERN)) {
    const at = match.index;
    if (at > last) {
      out.push(text.slice(last, at));
    }
    out.push(
      <a
        key={`${at}-${match[0]}`}
        href={match[0]}
        target="_blank"
        rel="noopener noreferrer nofollow"
        className="tl-text-link"
      >
        {match[0]}
      </a>,
    );
    last = at + match[0].length;
  }
  if (last < text.length) {
    out.push(text.slice(last));
  }
  return out;
}

/**
 * TimelineText is the message itself, two lines by default and the whole of it
 * on request.
 *
 * Two lines is enough to recognise a thread; the full text is one click away
 * rather than one application away. Collapsed by default because a timeline
 * where every row is a full email is a mailbox, and the point of the row is
 * still the sequence.
 *
 * On a mail row the reader gets the sentence the sender wrote. The sign-off and
 * the quoted history below it are folded into a second control instead of being
 * dropped, because the split is a heuristic: when it takes too much, the text is
 * one click away rather than gone. `email` gates that, since a note may open
 * with "Viele Grüße" or carry a "> " line as ordinary prose.
 */
function TimelineText({
  text,
  email = false,
}: Readonly<{ text: string; email?: boolean }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const [tailOpen, setTailOpen] = useState(false);
  // Whether the clamp is actually cutting the text off, measured rather than
  // guessed. Counting characters was wrong in the one direction that matters:
  // the clamp is two VISUAL lines at whatever width the column happens to be,
  // so a message short enough to look safe still wrapped past it in a narrow
  // column, got clipped by CSS, and — having failed the character test — was
  // given no way to expand. Text the reader could not reach.
  const [clipped, setClipped] = useState(false);
  const bodyRef = useRef<HTMLSpanElement>(null);
  // The tail is what the reader is spared; `trimmed` is what the row shows and
  // what the clamp measures, so the split has to happen before that effect.
  const parts = useMemo(
    () => (email ? splitEmailBody(text) : null),
    [email, text],
  );
  const trimmed = parts
    ? [parts.header, parts.main].filter(Boolean).join("\n\n")
    : text.trim();
  const tail = parts?.trimmed ?? "";
  // A row is keyed by activity id, so React keeps this component mounted when
  // the entry it renders is replaced. Without this the next mail's signature
  // would already be open, revealed by a click the reader made on a different
  // message.
  const [shownFor, setShownFor] = useState(text);
  if (shownFor !== text) {
    setShownFor(text);
    setTailOpen(false);
  }

  useLayoutEffect(() => {
    const el = bodyRef.current;
    // Nothing to measure while expanded: scrollHeight equals clientHeight, and
    // re-measuring there would drop the control that collapses it again. Empty
    // text renders nothing, so there is nothing that could be clipped.
    if (!el || open || !trimmed) {
      return;
    }
    const measure = () => setClipped(el.scrollHeight > el.clientHeight + 1);
    measure();
    // The column is resizable, so a width change can start or stop the
    // clipping. Guarded because jsdom has no ResizeObserver.
    if (typeof ResizeObserver === "undefined") {
      return;
    }
    const observer = new ResizeObserver(measure);
    observer.observe(el);
    return () => observer.disconnect();
  }, [open, trimmed]);

  if (!trimmed) {
    return null;
  }
  return (
    <span className="tl-text">
      <span ref={bodyRef} className={open ? "tl-text-full" : "tl-text-clamp"}>
        {linkify(trimmed)}
      </span>
      {(clipped || open) && (
        <button
          type="button"
          className="tl-text-toggle"
          aria-expanded={open}
          onClick={() => setOpen(!open)}
        >
          {open ? t("timeline.textLess") : t("timeline.textMore")}
        </button>
      )}
      {/* Offered whenever something was folded away, not only once the message
          is expanded: a mail short enough never to clip still has a signature,
          and gating this on the clamp would put that text out of reach. */}
      {tail && (
        <>
          <button
            type="button"
            className="tl-text-toggle tl-text-tail-toggle"
            aria-expanded={tailOpen}
            onClick={() => setTailOpen(!tailOpen)}
          >
            {tailOpen ? t("timeline.tailLess") : t("timeline.tailMore")}
          </button>
          {tailOpen && <span className="tl-text-tail">{linkify(tail)}</span>}
        </>
      )}
    </span>
  );
}

// directionClass tracks the row to one side of the spine: ours or theirs.
function directionClass(direction: TimelineEntry["direction"]): string {
  if (direction === "outbound") {
    return "tl-out";
  }
  return direction === "inbound" ? "tl-in" : "";
}

function noteClass(entry: TimelineEntry): string {
  return entry.kind === "note" ? "tl-note" : "";
}

// A conversation is an exchange somebody can answer: mail, or a message on a
// connected transport. Calls, meetings and notes are events and asides. ONE
// spelling — the Conversations cut and the whose-move flag both ask it, and
// two copies would let a new transport join one answer and not the other.
const CONVERSATION_KINDS: ReadonlySet<string> = new Set(["email", "message"]);

export function isConversationKind(kind: string): boolean {
  return CONVERSATION_KINDS.has(kind);
}

/**
 * MoveFlag is whose move a conversation is waiting on, read off its newest
 * message: their word standing last means the move is ours. ONE spelling for
 * every cut — the same chip on the row in the full chronology and in the
 * Conversations reading, because a thread that read "your move" on one cut
 * and said nothing on the other would be two claims about one conversation.
 *
 * Only on the row that STANDS FOR the conversation — a collapsed thread, or
 * a lone message — never on the members inside an expanded thread, where
 * every reply would otherwise carry a verdict about the whole exchange.
 */
// Whether the entry carries a direction a move can be read off — the guard
// both flag sites share, so a wrapper cannot render an empty chip row.
function conversationDirection(
  entry: TimelineEntry,
): "inbound" | "outbound" | undefined {
  if (!CONVERSATION_KINDS.has(entry.kind)) {
    return undefined;
  }
  return entry.direction === "inbound" || entry.direction === "outbound"
    ? entry.direction
    : undefined;
}

function MoveFlag({ entry }: Readonly<{ entry: TimelineEntry }>) {
  const t = useT();
  const direction = conversationDirection(entry);
  if (direction === "inbound") {
    return <Badge tone="warn">{t("convo.yourMove")}</Badge>;
  }
  if (direction === "outbound") {
    return <Badge quiet>{t("convo.waitingOnThem")}</Badge>;
  }
  return null;
}

function TimelineList({
  entries,
  zone,
}: Readonly<{ entries: TimelineEntry[]; zone: string }>) {
  return (
    <ul className="timeline">
      {entries.map((entry) => (
        <TimelineRow key={entry.id} entry={entry} zone={zone} />
      ))}
    </ul>
  );
}

/**
 * GroupedTimelineList renders conversations rather than messages.
 *
 * A collapsed group states what it IS before what it says — "5 messages" or
 * "sent to 3 people" — because the reader is scanning for an event, not for a
 * sentence. Expanding shows the same rows the flat list would have shown, from
 * the same component, so the two can never drift.
 *
 * A group that may continue past the page says so. A summary that implied it
 * was whole would be a worse answer than the repetition this replaced.
 */
export function GroupedTimelineList({
  groups,
  zone,
  onOpenThread,
}: Readonly<{
  groups: readonly TimelineGroup[];
  zone: string;
  // Fetches the rest of a conversation the page holds only part of. Absent for
  // a caller that cannot complete it — a bulk group has no thread to ask for.
  onOpenThread?: (threadKey: string) => void;
}>) {
  return (
    <ul className="timeline">
      {groups.map((group) =>
        group.kind === "single" ? (
          <TimelineRow
            key={group.id}
            entry={group.entries[0]}
            zone={zone}
            flag={<MoveFlag entry={group.entries[0]} />}
          />
        ) : (
          <TimelineGroupRow
            key={group.id}
            group={group}
            zone={zone}
            onOpenThread={onOpenThread}
          />
        ),
      )}
    </ul>
  );
}

// groupCountLabel counts the group's members in words that read. A group of
// one is reachable both ways — a thread whose other messages are on another
// page, and a single message the sender attested as a bulk send — and the
// plural forms rendered "1 messages" and "sent to 1 people" for it.
function groupCountLabel(group: TimelineGroup, locale: Locale): string {
  const count = group.entries.length;
  const base =
    group.kind === "bulk" ? "timeline.group.bulk" : "timeline.group.thread";
  return translatePlural(locale, base, count, {
    count: formatNumber(count, locale),
  });
}

function TimelineGroupRow({
  group,
  zone,
  onOpenThread,
}: Readonly<{
  group: TimelineGroup;
  zone: string;
  onOpenThread?: (threadKey: string) => void;
}>) {
  const { locale } = useLocale();
  const t = useT();
  const [open, setOpen] = useState(false);
  const newest = group.entries[0];
  const Icon = TIMELINE_ICON[newest.kind];
  const threadKey = newest.threadKey;
  return (
    <li className={directionClass(newest.direction)}>
      {/* A group sits on the SAME three columns a single row does — date,
          rail, what happened — because it IS a row of the chronology, and one
          that stepped out of the axis read as a different list wedged into
          this one. Its mark is the kind's own icon: the whole conversation is
          one thing that happened, and a dot would say it was one message. */}
      <span className="tl-when t-mono">
        {formatDate(newest.atIso, locale, zone)}
      </span>
      <span className="tl-rail" aria-hidden="true">
        <span className="tl-icon">
          <Icon aria-hidden />
        </span>
      </span>
      <div className="tl-body">
        {/* Whose move the conversation waits on, on the row that stands for
            it. A bulk group is one outbound send with no reply expected, so
            it carries no claim. */}
        {group.kind === "thread" && conversationDirection(newest) && (
          <span className="tl-head">
            <MoveFlag entry={newest} />
          </span>
        )}
        {/* What the conversation is, while it is closed.

            A thread of emails stands for its newest message, and that message
            is drawn by the SAME component a lone one is — subject, who, the
            preview the server composed, the access badge. Writing the summary
            by hand here was the defect: a company timeline of grouped
            conversations showed old-style rows beside canonical ones on the
            same page, because a group never reached EmailEntry.
            Expanded, the members draw themselves below and a summary above
            them would say the newest one twice. */}
        {open ? (
          <span className="tl-title">{newest.title}</span>
        ) : newest.kind === "email" && newest.emailSummary ? (
          <EmailEntry
            summary={newest.emailSummary}
            timestamp={formatTimeOfDay(newest.atIso, locale, zone)}
            onOpen={newest.onOpenEmail}
          />
        ) : (
          <>
            <span className="tl-title">{newest.title}</span>
            {/* Never for a withheld entry — a summary row must not show a
                reader words the row itself refuses. */}
            {newest.body && !newest.withheld && (
              <TimelineText text={newest.body} email={newest.kind === "email"} />
            )}
          </>
        )}
        <span className="tl-meta">
          <span className="tl-group-count">
            {groupCountLabel(group, locale)}
          </span>
          <ProvenanceTag provenance={newest.provenance} />
          <Button small aria-expanded={open} onClick={() => setOpen(!open)}>
            {open ? t("timeline.group.collapse") : t("timeline.group.expand")}
          </Button>
          {/* Only a real conversation can be completed: a bulk group is one
              send with no thread to ask the server for. Where it cannot be
              completed — a bulk group, or a page that passed no handler — the
              notice still stands. Rendering neither would present a group cut
              off by the page edge as the whole of it. */}
          {group.partial &&
            (threadKey && onOpenThread ? (
              <Button small onClick={() => onOpenThread(threadKey)}>
                {t("timeline.group.openThread")}
              </Button>
            ) : (
              <span className="t-caption">
                {t("timeline.group.mayContinue")}
              </span>
            ))}
        </span>
        {open && (
          <ul className="timeline tl-group-members">
            {group.entries.map((entry) => (
              <TimelineRow key={entry.id} entry={entry} zone={zone} />
            ))}
          </ul>
        )}
      </div>
      {/* The newest member's verbs stand for the conversation: Relink on it
          offers to move the rest of the thread, so a mis-filed conversation
          is fixed from its summary row without opening it first. */}
      {newest.actions && <span className="tl-actions">{newest.actions}</span>}
    </li>
  );
}

// Which way it went, and with whom.
//
// The name is dropped rather than guessed at when nothing resolved it: a row
// that says "Sent to" and stops has lost the reader's trust more thoroughly
// than one that only says "Sent".
function directionPhrase(
  entry: TimelineEntry,
  t: ReturnType<typeof useT>,
): string {
  const who = entry.counterparts;
  if (!who) {
    return entry.direction === "outbound"
      ? t("timeline.sent")
      : t("timeline.received");
  }
  if (entry.direction === "outbound") {
    return t("timeline.sentTo", { who });
  }
  if (entry.direction === "inbound") {
    return t("timeline.receivedFrom", { who });
  }
  // A meeting or a note has no side. It still has people in it, and naming
  // them is the whole reason this line exists.
  return t("timeline.withWhom", { who });
}

/**
 * TimelineRow is one entry. Split out of the list so a grouped view can render
 * the SAME row inside an expanded conversation — a second rendering of a
 * message would drift from this one the first time either changed.
 */
export function TimelineRow({
  entry,
  zone,
  flag,
}: Readonly<{
  entry: TimelineEntry;
  zone: string;
  // A chip qualifying the whole exchange this row STANDS FOR — the whose-move
  // flag on a lone message. Passed by the grouped list rather than derived
  // here, because the same row rendered as a member of an expanded thread
  // must not carry a verdict about the conversation it is one reply in.
  flag?: ReactNode;
}>) {
  const { locale } = useLocale();
  const t = useT();
  // A note is the rep's own words about the record, not an exchange with it,
  // and the row says so: its body sits on a plate of its own instead of
  // running in the message rhythm around it.
  const rowClass = [directionClass(entry.direction), noteClass(entry)]
    .filter(Boolean)
    .join(" ");
  // An email draws the canonical row, which is the one reading of a message in
  // the product. It keeps the date gutter and the rail — those place the row on
  // the chronology and are the timeline's, not the message's — and hands
  // everything the message itself says to EmailEntry.
  //
  // Every other kind keeps the row below. A note is the rep's own words, a
  // change is a field edit, a meeting has a transcript: none of them has an
  // email's shape, and giving them one would be the collapse this component
  // already made once by rendering a call as a note.
  if (entry.kind === "email" && entry.emailSummary) {
    return (
      <li className={rowClass}>
        <span className="tl-when t-mono">
          {formatDate(entry.atIso, locale, zone)}
        </span>
        <span className="tl-rail" aria-hidden="true">
          <span className={dotClass(entry)} />
        </span>
        <div className="tl-body">
          {/* The TIME, not the date: the gutter beside this already carries
              the day, and printing it twice on one row spends the space that
              tells two messages on one subject apart. */}
          <EmailEntry
            summary={entry.emailSummary}
            timestamp={formatTimeOfDay(entry.atIso, locale, zone)}
            onOpen={entry.onOpenEmail}
          />
          {flag}
        </div>
        {entry.actions && <span className="tl-actions">{entry.actions}</span>}
      </li>
    );
  }
  return (
    <li className={rowClass}>
      {/* The date leads the row, in its own gutter. A chronology is read down
          the dates — a reader looking for "what happened in August" scans one
          column rather than the end of every line — and the mono face keeps
          that column straight whatever each date's digits are. */}
      <span className="tl-when t-mono">
        {formatDate(entry.atIso, locale, zone)}
      </span>
      {/* The axis, and this row's place on it. The rail runs THROUGH the row
          rather than a rule sitting under it: a chronology is one thread, and a
          border per entry drew it as a stack of unrelated cards. Filled for
          something that was SAID, hollow for something that merely changed, so
          a reader tracking a conversation follows the solid dots straight past
          the field edits between them. */}
      <span className="tl-rail" aria-hidden="true">
        <span className={dotClass(entry)} />
      </span>
      {/* A div, not a span: a change row's detail is a field diff whose
                long-value side is a focusable region — flow content, invalid
                inside phrasing content. The row lays out identically, because
                .tl-body is a flex column either way. */}
      <div className="tl-body">
        {/* What KIND of thing this was, and which way it went — one line above
            the headline, because both qualify it and set inline they read as
            the first words of the subject. */}
        <span className="tl-head">
          <Badge>{t(TIMELINE_KIND_LABEL[entry.kind])}</Badge>
          {/* What the record DID, for a row that is not an exchange: the badge
              says this is a record entry, and this says what happened to it. */}
          {entry.qualifier && (
            <span className="tl-direction">{entry.qualifier}</span>
          )}
          {/* Which way it went and who was at the other end, as one phrase.
              The direction alone is a fact about us; with the name it is a
              fact about the relationship, which is what the row is for.
              A WITHHELD row keeps the direction and loses the name: "Received
              from Ana" beside a message whose subject the row just refused
              still says who this record is talking to, which is the thing the
              audience limited. */}
          {(entry.direction || entry.counterparts) && (
            <span className="tl-direction">
              {directionPhrase(
                entry.withheld ? { ...entry, counterparts: undefined } : entry,
                t,
              )}
            </span>
          )}
          {entry.via}
          {flag}
        </span>
        {entry.withheld ? (
          <span className="tl-title tl-withheld">
            <Lock aria-hidden />
            {t("timeline.withheld")}
          </span>
        ) : (
          <span className="tl-title">{entry.title}</span>
        )}
        {/* Never for a withheld entry: the title above already says the
            content is for participants only, and a row must not show the
            words it just refused. The server withholds bodies upstream; this
            is the row keeping its own promise whatever it is handed.
            An email WITH its summary drew EmailEntry above and never reaches
            here; one without still needs the splitter, because a reader whose
            server has not caught up should not lose the fold. */}
        {entry.body && !entry.withheld && (
          <TimelineText text={entry.body} email={entry.kind === "email"} />
        )}
        {entry.detail}
        <span className="tl-meta">
          <ProvenanceTag provenance={entry.provenance} />
        </span>
      </div>
      {entry.actions && <span className="tl-actions">{entry.actions}</span>}
    </li>
  );
}
