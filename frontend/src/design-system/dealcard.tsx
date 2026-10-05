// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ListPlus, Mail, Send, Sparkles } from "lucide-react";
import type { ReactNode } from "react";
import { calendarDay, middayInstant } from "../format/calendarday";
import {
  formatDayMonth,
  formatDuration,
  formatMoneyCompact,
  MONEY_ABSENT,
} from "../format/format";
import { formatMoneyOrWord } from "../format/moneyword";
import { formatElapsed } from "../format/now";
import { useLocale, useT } from "../i18n";
import { Avatar, Badge } from "./atoms";
import type { BoardDeal } from "./composed";
import { IconAction } from "./iconaction";
import { Popover } from "./popover";
import { FieldGuard } from "./rbac";
import { Chip } from "./readings";
import { useTooltip } from "./tooltip";

// One deal, as a card on the pipeline board (composed.tsx draws the board).
// The card's vocabulary — `BoardDeal` — stays with the board, because a column
// is a list of them and the board is what a caller assembles.

export type BoardDealMail = {
  agoMs: number;
  /** Null on a logged email that named no direction. */
  direction: "inbound" | "outbound" | null;
};

/**
 * The verbs a rep reaches for on a card without opening the deal. Each one the
 * caller passes is drawn, and one it leaves out is absent rather than refused:
 * whether a reader may write mail or file a task is the screen's to decide.
 */
export type DealCardActions = Readonly<{
  onSummary?: () => void;
  onEmail?: () => void;
  onAddTask?: () => void;
}>;

/** What a caller hangs on every card of a board, keyed by the deal. */
export type DealCardHooks = Readonly<{
  /** Each card's mail flyout — see `DealCard`'s `mailAside`. */
  mailAside?: (deal: BoardDeal) => ReactNode;
  /** Each card's verbs — see `DealCard`'s `actions`. */
  cardActions?: (deal: BoardDeal) => DealCardActions | undefined;
}>;

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
function DealCardCompany({
  deal,
  onOpen,
}: Readonly<{
  deal: BoardDeal;
  // The same press hook the deal's own link takes, for the same reason: a
  // click that ends a drag is not a click on the company either, and a card
  // that suppressed one link and not the other would open the account a rep
  // was only moving.
  onOpen?: (deal: BoardDeal, event: React.MouseEvent) => void;
}>) {
  const t = useT();
  if (deal.companyWithheld) {
    return (
      <span className="deal-company">
        <FieldGuard mode="masked" />
      </span>
    );
  }
  if (deal.companyUnreadable) {
    return (
      <span className="deal-company">
        <span className="deal-company-name">{t("ref.nameLoadFailed")}</span>
      </span>
    );
  }
  if (!deal.company) {
    return null;
  }
  return (
    <span className="deal-company">
      <Avatar
        name={deal.company}
        identity={deal.companyId}
        src={deal.companyLogoUrl}
      />
      {/* The name needs a box of its own to be truncated in: a bare text node
          has nothing for the ellipsis to apply to, and wraps under its own
          mark instead. */}
      {deal.companyHref ? (
        // The company's own door, beside the deal's. The whole card used to be
        // one anchor to the deal, so a rep looking at a board of deals could
        // not reach the account behind any of them without opening a deal
        // first.
        //
        // stopPropagation keeps the click off the card's stretched anchor.
        // preventDefault would be wrong: it would leave the card's own
        // navigation to fire while this link did nothing.
        <a
          className="deal-company-name deal-company-link"
          href={deal.companyHref}
          onClick={(event) => {
            event.stopPropagation();
            onOpen?.(deal, event);
          }}
        >
          {deal.company}
        </a>
      ) : (
        <span className="deal-company-name">{deal.company}</span>
      )}
    </span>
  );
}

/**
 * Who carries the deal, as a mark at the edge of the company line.
 *
 * A monogram is not an answer on its own — a teammate has to decode it — so the
 * mark carries the full name as its label, which a screen reader gets as part
 * of the card's own name and a pointer gets as a tip. The tip grants no tab
 * stop of its own: the card is a link and already has one, and a second stop
 * inside it would be a control that does nothing.
 */
function DealOwner({
  owner: { id, name },
}: Readonly<{ owner: Readonly<{ id: string; name: string }> }>) {
  const tip = useTooltip<HTMLSpanElement>(name);
  return (
    <span
      className="deal-owner"
      role="img"
      aria-label={name}
      ref={tip.ref}
      {...tip.trigger}
    >
      <Avatar name={name} identity={id} />
      {tip.tip}
    </span>
  );
}

/**
 * When the deal closes, on the card's figure line.
 *
 * Read as a calendar day in the record's zone, never as the instant midnight
 * UTC — that instant is the previous date for half the world. The tone marks
 * the DATE when it is one nobody confirmed or one already behind us, and says
 * so in words for a reader who does not get the colour; the deal strip states
 * the same fact the same two ways.
 */
function DealCloses({
  day,
  provisional,
  zone,
}: Readonly<{ day: string; provisional: boolean; zone: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const overdue = day < calendarDay(new Date(), zone);
  const classes =
    provisional || overdue ? "deal-closes deal-closes-warning" : "deal-closes";
  return (
    <span className={classes}>
      {t("deal.closes", {
        date: formatDayMonth(middayInstant(day, zone), locale, zone),
      })}
      {provisional && (
        <span className="sr-only"> · {t("deal.closesProvisional")}</span>
      )}
    </span>
  );
}

/**
 * When mail last moved on this deal: at the foot of the card, and in the deals
 * table's own column, which is why it is exported — one chip, one flyout, on
 * both views of the same deal.
 *
 * The one line a rep triages a column by after the money: a deal they wrote
 * to ten days ago with no reply and a deal the buyer wrote to yesterday sit in
 * the same stage and are not the same deal. The glyph says which way the last
 * mail went, the words say how long ago, and the name of the chip is spoken to
 * a screen reader rather than drawn — a sighted reader knows an envelope.
 *
 * With an `aside` it is the trigger of a hover flyout — the last few messages,
 * for a reader who wants the subjects without opening the deal — and without
 * one it is plain text. The flyout's CONTENT is the caller's, because it is
 * read from the timeline and this tier fetches nothing.
 */
export function DealMailChip({
  mail,
  aside,
}: Readonly<{ mail: BoardDealMail; aside?: ReactNode }>) {
  const t = useT();
  const { locale } = useLocale();
  // A Chip, because that is what this is: one fact about the record with the
  // glyph naming its kind — the same pill a company's domain or a file's type
  // is drawn as, dense so it sits at the badges' geometry rather than a rung
  // above the name.
  const label = (
    <>
      <span className="sr-only">{t("deal.lastMail")}: </span>
      <Chip icon={mail.direction === "outbound" ? Send : Mail} dense>
        {formatElapsed(mail.agoMs, t, locale)}
      </Chip>
    </>
  );
  if (!aside) {
    return <span className="deal-mail">{label}</span>;
  }
  return (
    <Popover label={label} className="deal-mail" onHover>
      {aside}
    </Popover>
  );
}

/**
 * The card's last line: how the deal is moving, and the verbs a rep reaches
 * for without opening it.
 *
 * What is wrong with a deal lands HERE rather than in a line above the name,
 * so a card that needs the reader keeps its name and its figure level with the
 * cards beside it — a column is read down, and a flag line that came and went
 * moved every slot under it.
 */
function DealCardFoot({
  deal,
  mailAside,
  actions,
}: Readonly<{
  deal: BoardDeal;
  mailAside?: (deal: BoardDeal) => ReactNode;
  actions?: DealCardActions;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const verbs =
    actions !== undefined &&
    [actions.onSummary, actions.onEmail, actions.onAddTask].some(Boolean);
  if (!deal.stalled && !deal.lastEmail && !deal.singleThreaded && !verbs) {
    return null;
  }
  return (
    <span className="deal-foot">
      {deal.stalled && (
        <span className="deal-stall">
          <Badge tone="warning">{t("deal.stalledBadge")}</Badge>
          {/* How long it has sat is the size of the stall, and only then: on a
              healthy card the number is a fact nobody acts on. */}
          <span className="deal-age">{formatDuration(deal.ageMs, locale)}</span>
        </span>
      )}
      {deal.lastEmail && (
        <DealMailChip mail={deal.lastEmail} aside={mailAside?.(deal)} />
      )}
      {deal.singleThreaded && (
        <Badge tone="danger">{t("deal.singleThreaded")}</Badge>
      )}
      {verbs && <DealCardVerbs actions={actions} />}
    </span>
  );
}

/**
 * The card's verbs, as named glyphs at the trailing end of its last line.
 *
 * They keep their room at rest and show under the pointer or on focus (see
 * composed.css): twenty cards of glyphs that never go away is a column of
 * noise, and a line that only made room for them on hover would push every
 * card under it down a step.
 */
function DealCardVerbs({ actions }: Readonly<{ actions: DealCardActions }>) {
  const t = useT();
  return (
    <span className="deal-actions">
      {actions.onSummary && (
        <IconAction
          inline
          variant="ghost"
          label={t("deal360.brief")}
          icon={<Sparkles aria-hidden="true" />}
          onClick={actions.onSummary}
        />
      )}
      {actions.onEmail && (
        <IconAction
          inline
          variant="ghost"
          label={t("contact.action.email")}
          icon={<Mail aria-hidden="true" />}
          onClick={actions.onEmail}
        />
      )}
      {actions.onAddTask && (
        <IconAction
          inline
          variant="ghost"
          label={t("log.addTask")}
          icon={<ListPlus aria-hidden="true" />}
          onClick={actions.onAddTask}
        />
      )}
    </span>
  );
}

/**
 * One deal, as a card on the board.
 *
 * NO TAG STRIP. How a deal is filed is a fact about the record, not about the
 * work in front of the reader, and a board is scanned: three coloured chips per
 * card turned five columns into a field of colour with the deal names competing
 * against it. The words are a column in the deals table and a panel on the
 * record, which are the two places a reader goes to ask how something is filed.
 * What stays here is what a reader triages by — who it is with, what it is,
 * what it is worth, how long it has sat, and what is wrong with it.
 */
export function DealCard({
  deal,
  href,
  zone,
  onOpen,
  mailAside,
  actions,
  dragHandlers,
}: Readonly<{
  deal: BoardDeal;
  /**
   * The IANA zone the close date is read in. A close date is a calendar day,
   * and a day formatted as if it were an instant lands on the previous date
   * for every reader west of Greenwich — so the card takes the zone the record
   * lives in, the same way the timeline does, rather than the browser's.
   */
  zone: string;
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
  /**
   * What the mail line reveals on hover: the deal's last few messages, read
   * by the caller from the timeline. Given, the line becomes a flyout's
   * trigger; absent, it stays a line of text. See `DealMailChip`.
   */
  mailAside?: (deal: BoardDeal) => ReactNode;
  /** The verbs on the card's last line. See `DealCardActions`. */
  actions?: DealCardActions;
  dragHandlers?: {
    draggable: true;
    onDragStart: (event: React.DragEvent) => void;
  };
}>) {
  const t = useT();
  const { locale } = useLocale();
  // No `stalled` class: the warning Badge in the foot says it in words, and an
  // edge stripe saying the same thing is one statement drawn twice — the reader
  // who cannot see colour reads the badge, and the reader who can read both.
  const classes = [
    "deal-card",
    deal.staged ? "staged" : "",
    deal.archived ? "archived" : "",
  ]
    .filter(Boolean)
    .join(" ");
  return (
    // A div with a STRETCHED anchor inside it, not one anchor around
    // everything. The card carries a second destination now — the company —
    // and an anchor inside an anchor is invalid markup the browser silently
    // unnests, which is how the inner link stops being clickable at all.
    //
    // The stretched link keeps every property the outer anchor had: the whole
    // card is still the deal's click target, still opens in a new tab, still
    // middle-clicks, and is still one tab stop.
    <div className={classes} data-deal={deal.id} {...dragHandlers}>
      {/* Read in the order a rep asks (composed.css says why): who it is with
          and whose it is; what it is called; what it is worth and when it
          closes; and how it is moving. Staged and archived are about the CARD
          rather than the deal's health, so they alone lead it. */}
      {(deal.staged || deal.archived) && (
        <span className="deal-flags">
          {deal.staged && <Badge tone="ai">{t("deal.staged")}</Badge>}
          {deal.archived && <Badge>{t("deal.archived")}</Badge>}
        </span>
      )}
      <span className="deal-head t-caption">
        <DealCardCompany deal={deal} onOpen={onOpen} />
        {deal.owner && <DealOwner owner={deal.owner} />}
      </span>
      {/* The deal's own door, stretched over the card by CSS. It carries the
          NAME rather than sitting empty, so the accessible name of the link is
          the deal a reader is opening — an empty stretched anchor reads to a
          screen reader as a link with no text. */}
      <a
        className="deal-name deal-open"
        href={href}
        // The press handler rides the ANCHOR, not the div around it: the div
        // would also catch the company link's bubbled click, and the drop
        // guard would then refuse a navigation the reader did ask for.
        onClick={(event) => onOpen?.(deal, event)}
      >
        {deal.name}
      </a>
      <span className="deal-figure">
        {/* Compact, because a card is compared at a glance: the column head
            sums the stage exactly and the deal states its own figure whole. */}
        <span className="deal-value">
          {formatMoneyOrWord(
            deal.valueMinor,
            deal.currency,
            locale,
            MONEY_ABSENT,
            formatMoneyCompact,
          )}
        </span>
        {deal.closeDate ? (
          <DealCloses
            day={deal.closeDate}
            provisional={deal.closeDateProvisional ?? false}
            zone={zone}
          />
        ) : (
          <span className="deal-closes">{t("deal.undated")}</span>
        )}
      </span>
      <DealCardFoot deal={deal} mailAside={mailAside} actions={actions} />
    </div>
  );
}
