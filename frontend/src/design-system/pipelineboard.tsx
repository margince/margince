import type { ReactNode } from "react";
import { Fragment, useRef } from "react";
import {
  formatDate,
  formatMoneyOrAbsent,
  formatNumber,
} from "../format/format";
import { translatePlural, useLocale, useT } from "../i18n";
import { Button, OptionCount } from "./atoms";
import { Callout } from "./callout";
import "./composed.css";

import { type BoardDealMail, DealCard, type DealCardHooks } from "./dealcard";
import {
  matchesTitle,
  matchingColumns,
  scrollColumnIn,
  useOffScreenMatches,
} from "./pipelinereveal";

export type BoardRecord = {
  id: string;
  name: string;
};

export type BoardDeal = BoardRecord & {
  /**
   * The company this deal is with, as a name a reader recognises, and the id
   * keying its mark. Both are empty for a deal naming no company, which draws
   * none.
   */
  company: string;
  companyId: string;
  /**
   * The company's own address, when the caller has one to give.
   *
   * An href and not an id, for the reason the deal's own `href` is one: this
   * tier holds no routes. A design-system card that knew how to build
   * `#/companies/{id}` would be the app's routing table living in two places.
   * Absent renders the company as prose, which is what a caller that cannot
   * link it is saying.
   */
  companyHref?: string;
  /** The company's resolved mark. Absent leaves the monogram, which is the
   *  floor rather than a fallback. */
  companyLogoUrl?: string | null;
  /**
   * The company is not this reader's to read. The wire sent no id and named
   * the field in `masked_fields`, so the slot carries the mask, not a name.
   *
   * A flag rather than a node in `company`, for the reason
   * `TimelineEntry.withheld` is one: the withheld reading is this tier's to
   * spell. A caller handing in its own words for it is how one reading ends up
   * with two spellings. It also keeps the mark true: a monogram cut from the
   * word for "withheld" would be a mark no company has.
   */
  companyWithheld?: boolean;
  /**
   * The company's name could not be read: the caller's lookup failed rather
   * than answering. It is a distinct flag from `companyWithheld`, because the
   * two say opposite things about the reader. Withheld means the answer exists
   * and is not theirs; unreadable means nobody got an answer at all.
   *
   * It exists because the alternative is worse than either. An empty `company`
   * is the reading for a deal that names no company. A failed lookup falling
   * into it tells the reader the deal is unlinked, when it is linked to a
   * company they could not fetch. The table's own company cell has had this
   * reading all along; this is the card's half of it.
   */
  companyUnreadable?: boolean;
  /**
   * The deal's money, as the two halves it has: an integer minor amount and
   * its ISO currency. Either can be missing on a deal nobody has priced.
   * Required and nullable rather than optional, because a caller owes the card
   * an answer about the money. `null` is that answer where there is no figure,
   * and the card draws it as absent.
   *
   * Definite types here are what made every caller invent a currency. A prop
   * demanding a `string` leaves `?? "EUR"` as the only way to satisfy it. A
   * euro sign on a figure that might be dong is worse than no figure.
   */
  valueMinor: number | null;
  currency: string | null;
  ageMs: number;
  /**
   * When the deal is expected to close, as the calendar day the record holds.
   * Null on a deal nobody has dated, and the card then states no date rather
   * than a guess. `closeDateProvisional` says the day is the nightly run's
   * replacement for one that aged into the past. No human has confirmed it. A
   * date that reads as agreed with the buyer when it was not is the quiet kind
   * of wrong, so the card marks it.
   */
  closeDate?: string | null;
  closeDateProvisional?: boolean;
  /**
   * Who carries the deal: the seat's name, and its user id to key the chip.
   * Null for a deal nobody owns and for one whose owner the caller could not
   * name. The card draws nothing for either; the owner column tells them apart.
   */
  owner?: Readonly<{ id: string; name: string }> | null;
  /**
   * The newest email on the deal that the whole workspace may see, as how long
   * ago it was and which way it went. A span rather than an instant, for the
   * reason `ageMs` is one: the caller owns the clock. A card that read
   * `Date.now()` would be a card no test could pin. Absent or null on a deal
   * nobody has mailed about, and the card then draws no mail line at all.
   * "No mail yet" is a fact the timeline states, not one worth a row on every
   * fresh card in a column.
   */
  lastEmail?: BoardDealMail | null;
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
   * loaded. A caller with a capped/paginated card fetch (the Kanban board) has
   * a real count from a server aggregate that `deals.length` cannot give it.
   * Falls back to `deals.length` when absent, which is correct for a caller
   * whose card list is the whole stage.
   */
  count?: number;
  /** Totals withheld because the aggregate cannot measure the card filters. */
  sumHidden?: boolean;
  /** The caller's explanation of the withheld total. */
  sumHiddenReason?: string;
  /**
   * Draw the column narrow: its head and its count, and none of its cards.
   *
   * For a stage that is a destination more than a queue. A lead board's
   * Qualified and Disqualified hold every lead that ever left the pipeline.
   * Expanded, they would be the longest columns on the board and the least
   * read. Collapsed, they still state their count and still take a drop, which
   * is what they are there for.
   *
   * The cards are not rendered at all rather than hidden with CSS. A column
   * nobody can see should not cost a render, and `count` already carries the
   * figure the head states without them.
   */
  collapsed?: boolean;
};

/**
 * A column on a money board: the stage's figures, which its header states.
 *
 * Separate from the base rather than four optional fields on it, because
 * optional is not what these are. A deal column without them renders
 * `undefined%` in its header, which is worse than failing to compile. The
 * variant that reads them is the variant that requires them.
 *
 * The three money fields are nullable, which is not the same as optional. The
 * caller still owes every column an answer. `null` is the answer where the
 * server stated no figure: an aggregate over unpriced deals, or a stage whose
 * rows name no currency. Drawn as absent, never as a zero: a stage total of
 * `€0.00` beside a real deal count reads as an empty stage.
 */
export type BoardMoneyColumn = BoardColumn<BoardDeal> & {
  probabilityPct: number;
  rawMinor: number | null;
  weightedMinor: number | null;
  currency: string | null;
  asOf?: string | null;
  timezone?: string | null;
  unpricedCount?: number | null;
};

type BoardHandlers<Record extends BoardRecord> = {
  countLabel?: (count: number) => string;
  /**
   * The search the cards answer. When it changes and no column holding a card
   * shows, the board scrolls the first into view and says where the matches
   * are. Otherwise a search matching only an off-screen stage reads as empty.
   */
  revealKey?: string;
  /**
   * Fold or unfold one column. Given only by a board that has a collapsed
   * column. Without it the head is inert text, so a board with no such column
   * grows no button a reader can press to no effect.
   */
  onToggleColumn?: (column: BoardColumn<Record>) => void;
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

type DealBoardProps = BoardHandlers<BoardDeal> &
  DealCardHooks & {
    variant?: "deal";
    columns: BoardMoneyColumn[];
    /**
     * Each card's own address. Required, because every card on this board opens a
     * deal and a card that opens a record is a link (see `DealCard`'s `href`).
     * A function rather than a field on the deal: the address is the screen's
     * vocabulary and this tier holds no routes.
     */
    cardHref: (deal: BoardDeal) => string;
    onOpen?: (deal: BoardDeal, event: React.MouseEvent) => void;
    /** The zone a close date is read in; see `DealCard`'s `zone`. */
    zone: string;
  };

type BoardLayoutProps<Record extends BoardRecord> = BoardHandlers<Record> & {
  columns: BoardColumn<Record>[];
  renderCard: (record: Record, column: BoardColumn<Record>) => ReactNode;
  moneyFor: (column: BoardColumn<Record>) => BoardMoneyColumn | undefined;
  zone?: string;
};

function MoneyNotes({
  money,
  hidden,
  zone,
}: Readonly<{
  money: BoardMoneyColumn | undefined;
  hidden?: boolean;
  zone?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  if (!money || hidden) return null;
  const unpriced = money.unpricedCount ?? 0;
  const reportZone = money.timezone ?? zone;
  return (
    <>
      {money.asOf && reportZone && (
        <span className="board-col-refusal">
          {t("board.convertedAt", {
            date: formatDate(money.asOf, locale, reportZone),
          })}
        </span>
      )}
      {unpriced > 0 && (
        <span className="board-col-refusal">
          {translatePlural(locale, "brief.readings.unpricedCount", unpriced, {
            count: formatNumber(unpriced, locale),
          })}
        </span>
      )}
    </>
  );
}

function BoardLayout<Record extends BoardRecord>({
  columns,
  renderCard,
  revealKey,
  countLabel,
  columnExtras,
  columnDropHandlers,
  onToggleColumn,
  moneyFor,
  zone,
}: Readonly<BoardLayoutProps<Record>>) {
  const t = useT();
  const { locale } = useLocale();
  const sections = useRef(new Map<string, HTMLElement>());
  const board = useRef<HTMLDivElement>(null);
  const matches = revealKey ? matchingColumns(columns) : [];
  const offScreen = useOffScreenMatches(
    board,
    sections,
    revealKey,
    matches.map((column) => column.stage),
  );
  // The head's interactive half, or nothing at all. Written as a function so
  // the column's JSX stays one line of props rather than a spread conditional
  // the reader has to unpick.
  function foldProps(column: BoardColumn<Record>) {
    if (!onToggleColumn || column.collapsed === undefined) return {};
    return {
      role: "button",
      tabIndex: 0,
      "aria-expanded": !column.collapsed,
      onClick: () => onToggleColumn(column),
      onKeyDown: (event: React.KeyboardEvent) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onToggleColumn(column);
        }
      },
    };
  }
  return (
    <>
      {offScreen && matches.length > 0 && (
        // The host's gutter comes from this wrapper, as a Callout takes no class.
        <div className="board-reveal">
          <Callout
            kind="outcome"
            title={matchesTitle(matches, locale)}
            actions={
              <Button
                variant="ghost"
                onClick={() =>
                  scrollColumnIn(sections.current.get(matches[0]?.stage ?? ""))
                }
              >
                {t("board.showMatches")}
              </Button>
            }
          />
        </div>
      )}
      <div className="board" ref={board}>
        {columns.map((column) => {
          const money = moneyFor(column);
          const dealCount = column.count ?? column.deals.length;
          return (
            <section
              key={column.stage}
              className={
                column.collapsed ? "board-col board-col-collapsed" : "board-col"
              }
              data-stage={column.stage}
              ref={(node) => {
                if (node) sections.current.set(column.stage, node);
                else sections.current.delete(column.stage);
              }}
              aria-label={column.label}
              // listtable.css gives every column `overflow-y: auto` so a stage
              // with many deals scrolls its own cards instead of stretching the
              // rest to match it. A column with too few cards to overflow, or
              // none at all, is still declared scrollable. A card's own link
              // makes a full column keyboard-reachable but says nothing about an
              // empty one, which axe's scrollable-region-focusable rule catches.
              // The section is the scroller, so it takes the fallback stop
              // itself rather than depending on what is inside it. That is the
              // WCAG-sanctioned fix for a non-interactive scrollable region. It
              // is also why the a11y linter's default posture forbids tabIndex
              // on a section at all.
              // biome-ignore lint/a11y/noNoninteractiveTabindex: the scrollable region itself needs the keyboard stop; see the comment above.
              tabIndex={0}
              {...columnDropHandlers?.(column)}
            >
              {/* The stage and how much is in it, on one line and stuck to the
                top of the column. A board is scrolled, and a reader halfway
                down a long stage had nothing on screen saying which stage they
                were in. The head is the only thing that says it, so it holds
                its place. The count moved up here with it: it is the figure a
                reader compares across the board, and under the money totals it
                was the third figure on a two-line sub. */}
              {/* A button only where there is something to press, and that is
                per column rather than per board. A column says it can fold by
                carrying `collapsed` at all; one that never folds keeps the
                plain div it has always been, even on a board that has a
                foldable column beside it. A control that does nothing is worse
                than no control, and a lead board would otherwise have made its
                three open stages focusable to no effect. */}
              <div className="board-col-head" {...foldProps(column)}>
                <span className="stage">{column.label}</span>
                {/* Its own node, not one composed string. The name is data of
                  unbounded length and truncates; the count is three characters
                  and must not. Written as "{label}: {count}" into the
                  truncating span, a long stage name ellipsised the figure away.
                  That figure is the one thing this head was rearranged to keep
                  on screen. `OptionCount` is the product's one count chip, so
                  the figure beside a stage is drawn as the figure beside a tab.
                  The wrapper is what carries `aria-hidden`: the chip speaks its
                  own figure. This column already tells a screen reader "12
                  deals" below, with the unit a bare figure leaves out. So the
                  whole chip, separator included, stays out of the accessible
                  tree rather than announcing the number twice. */}
                <span className="board-col-count" aria-hidden="true">
                  <OptionCount count={dealCount} />
                </span>
                {money && (
                  <span className="prob">
                    {formatNumber(money.probabilityPct, locale)}%
                  </span>
                )}
              </div>
              {/* The stage's total is the figure being scanned down the board,
              so it leads with the deal count beside it. The weighted figure is
              its own server-sourced total (not derived from the raw total). It
              reads underneath rather than competing on the line. */}
              <div className="board-col-sub">
                <span className="board-col-total">
                  {money && !column.sumHidden && (
                    <span className="board-col-money">
                      {money.rawMinor != null && money.currency ? "≈ " : ""}
                      {formatMoneyOrAbsent(
                        money.rawMinor,
                        money.currency,
                        locale,
                      )}
                    </span>
                  )}
                  {/* The count is in the head; what a screen reader needs here
                    is the unit the head's bare figure leaves out, so the column
                    announces "12 deals" rather than "Qualified: 12". */}
                  <span className="sr-only">
                    {countLabel
                      ? countLabel(dealCount)
                      : translatePlural(locale, "board.count", dealCount, {
                          count: formatNumber(dealCount, locale),
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
                <MoneyNotes
                  money={money}
                  hidden={column.sumHidden}
                  zone={zone}
                />
                {money && column.sumHidden && dealCount > 0 && (
                  <span className="board-col-refusal">
                    {column.sumHiddenReason}
                  </span>
                )}
              </div>
              {!column.collapsed &&
                column.deals.map((record) => (
                  <Fragment key={record.id}>
                    {renderCard(record, column)}
                  </Fragment>
                ))}
              {!column.collapsed && columnExtras?.(column)}
            </section>
          );
        })}
      </div>
    </>
  );
}

/**
 * The variant decides what a board column must carry. Deal boards require
 * money and probability. Plain boards accept any identified record and make
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
          zone={props.zone}
          onOpen={props.onOpen}
          mailAside={props.mailAside}
          actions={props.cardActions?.(deal)}
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
