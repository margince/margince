// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Bell } from "lucide-react";
import { Suspense, useCallback, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useNoticeRead } from "../components/taskactions.rowwrites";
import { Badge, Button, EmptyState, PendingBody } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { CardBoundary } from "../design-system/cardboundary";
import { Panel, PanelBody } from "../design-system/panel";
import { usePortalPanelFocus } from "../design-system/portalfocus";
import { LoadMoreButton, QueryStates } from "../design-system/querystates";
import {
  NOTIFICATIONS_KEY,
  useMarkAllNoticesRead,
  useNotifications,
} from "../features/settings/notifications.queries";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { lazy } from "../i18n/lazy";
import { problemMessageOf } from "../lib/problem";
import { worklistKey } from "../lib/workkeys";
import { usePopoverDismiss } from "./popover";
import "./notificationbell.css";
import "./topbar-popover.css";

const NoticeRow = lazy(
  async () => ({
    default: (await import("./notificationrow")).NoticeRow,
  }),
  "complete",
  CardBoundary,
);

// The notification centre, as the chrome carries it: a count somebody sees
// without opening anything, and the history behind it.
//
// It is in the top strip rather than on a screen of its own because a notice is
// true of the SESSION and not of the page under it — the same reason the trail,
// the search and the account chip are up here. The Worklist's notices lane is
// the other door onto the same rows and stays the place work gets done; this is
// where a reader glances.
//
// NO INDIGO on an ordinary row. The tint is a claim that an agent authored
// something, so it is drawn for exactly the notices whose origin says so and
// for none of the rest — a centre washed indigo would tell a reader a machine
// decided every one of them.

export function NotificationBell() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const panelId = useId();
  const notices = useNotifications();
  // BOTH SETTLES OUTLIVE THE PANEL. The centre renders only while open, so a
  // refusal declared inside it is thrown away by the dismissal that follows the
  // press — and a write still in flight loses the listener its invalidation
  // rides on, stranding the badge on a figure the server has already moved past.
  const settleAll = useMarkAllNoticesRead();
  // ONE hook for every row rather than one per row, because one hook is what
  // lets one error surface speak for whichever row failed. The row still sends
  // its OWN id as the variable, and `settle.variables` is what that id comes
  // back as — so the pending mark stays on the button that was pressed instead
  // of spreading to all of them.
  const settle = useNoticeRead([NOTIFICATIONS_KEY, worklistKey]);

  // The count is the FIRST page's, which is the whole unread set: the endpoint
  // answers the reader's total beside every page rather than that page's share,
  // so paging back through history never moves the badge.
  const unread = notices.data?.pages[0]?.unread_count ?? 0;

  const dismiss = useCallback(() => setOpen(false), []);
  // One dismissal for every popover in the chrome (app/popover.ts): Escape from
  // anywhere inside, any outside click, and the opening click deferred past.
  usePopoverDismiss(open, panel, dismiss);
  // Focus, shared with every other portalled panel (design-system/portalfocus.ts):
  // into the panel on the press that opened it, back to the bell if the close
  // dropped it, and Tab measured from where the bell sits in the strip. At the
  // body the panel is no longer the bell's next sibling, so none of the three
  // happens by adjacency. The bell has no hover door, so a press is the only way in.
  const panelFocus = usePortalPanelFocus({
    open,
    openedBy: "press",
    trigger,
    panel,
  });

  return (
    <div className="notifbell">
      {/* Bare, like the sidebar toggle and the narrow search glyph: the strip's
          controls are glyphs, and a boxed one read as a field. */}
      <Button
        variant="bare"
        iconOnly
        ref={trigger}
        // The count joins the NAME, not only the box. `aria-label` replaces the
        // button's contents for name computation, so a figure left to the badge
        // alone would reach a screen reader from nowhere — and how many things
        // are waiting is the entire reason somebody presses this.
        aria-label={
          unread > 0
            ? plural("notifications.bellWaiting", unread, {
                count: formatNumber(unread, locale),
              })
            : t("notifications.bell")
        }
        // NO `aria-haspopup`, which is deliberate and follows the agent rail
        // rather than the account menu. `aria-haspopup` names WHAT opens, and
        // the two house popovers answer it differently for a reason: the
        // account menu declares "menu" because it renders a real menu of
        // menuitems, and the rail's portalled panel declares nothing because it
        // renders a region. This is the rail's shape — a panel of prose, links
        // and verbs — and it does not manage focus the way a dialog must, so
        // claiming "dialog" would swap one false announcement for another.
        // `aria-expanded` and `aria-controls` say the true thing: this button
        // expands that region.
        aria-expanded={open}
        aria-controls={open ? panelId : undefined}
        onClick={() => setOpen((current) => !current)}
      >
        <Bell aria-hidden />
        {/* A ZERO IS NOT A COUNT WORTH DRAWING: a badge reading "0" is a mark
            the eye stops on to learn there is nothing, which an unmarked bell
            already says. */}
        {unread > 0 && (
          <span className="notifbell-count">
            <Badge variant="primary" tone="accent">
              {formatNumber(unread, locale)}
            </Badge>
          </span>
        )}
      </Button>
      {/* Portalled to the body: the strip clips what hangs below it, and a
          panel drawn inside the bar would be cut off at its own first row. */}
      {open &&
        createPortal(
          <div
            className="notifbell-loose topbar-popover"
            ref={panel}
            id={panelId}
            {...panelFocus}
          >
            <NotificationCentre
              query={notices}
              unread={unread}
              settleAll={settleAll}
              settle={settle}
              onActivate={dismiss}
            />
          </div>,
          document.body,
        )}
    </div>
  );
}

function NotificationCentre({
  query,
  unread,
  settleAll,
  settle,
  onActivate,
}: Readonly<{
  query: ReturnType<typeof useNotifications>;
  unread: number;
  settleAll: ReturnType<typeof useMarkAllNoticesRead>;
  settle: ReturnType<typeof useNoticeRead>;
  /** Dismiss the panel, for a row that both acts and closes. */
  onActivate: () => void;
}>) {
  const t = useT();
  const rows = query.data?.pages.flatMap((page) => page.items) ?? [];
  return (
    <Panel
      title={t("notifications.centre")}
      // OFFERED ONLY WHILE SOMETHING IS UNREAD. The verb sits above the states
      // below it, so a centre that has never held anything would otherwise hand
      // the reader an enabled button whose whole effect is a write with nothing
      // to settle.
      titleAction={
        unread > 0 ? (
          <Button
            pending={settleAll.isPending}
            onClick={() => settleAll.mutate()}
          >
            {/* No number, deliberately. The server settles more than this panel
                ever showed — a reader's own stage-move notices among them — so
                any figure quoted here would be about a set they never saw. */}
            {t("notifications.markAllRead")}
          </Button>
        ) : undefined
      }
    >
      <PanelBody>
        {/* A REFUSED SETTLE HAS TO BE SAID. Both verbs here leave the same
            rendering behind when they fail as a click that did nothing would —
            the mark stops turning, the badge does not move — so without this
            the reader is left to guess whether they missed the button.

            A Callout rather than the toast the Worklist's own acknowledge
            raises, because this surface is portalled and dismissible: a toast
            fires at the app's toast root, outside the panel, and would still be
            standing after an outside click had taken away the row it is about.
            The Callout sits where the reader is already looking and leaves with
            the panel. It is also what the preferences page in this feature
            does, so one feature has one way of reporting a refused write. */}
        {settleAll.isError && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("notifications.markAllFailed")}
          >
            {problemMessageOf(settleAll.error, t)}
          </Callout>
        )}
        {settle.isError && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t("notifications.markReadFailed")}
          >
            {problemMessageOf(settle.error, t)}
          </Callout>
        )}
        <QueryStates query={query} pendingLabel={t("notifications.centre")}>
          {rows.length === 0 ? (
            <EmptyState>{t("notifications.empty")}</EmptyState>
          ) : (
            <>
              <Suspense
                fallback={<PendingBody label={t("notifications.centre")} />}
              >
                <ul className="notifbell-list">
                  {rows.map((notice) => (
                    <NoticeRow
                      key={notice.id}
                      notice={notice}
                      onSettle={settle.mutate}
                      onActivate={onActivate}
                      settling={
                        settle.isPending && settle.variables === notice.id
                      }
                    />
                  ))}
                </ul>
              </Suspense>
              <LoadMoreButton query={query} />
            </>
          )}
        </QueryStates>
      </PanelBody>
    </Panel>
  );
}
