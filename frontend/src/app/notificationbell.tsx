// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Bell } from "lucide-react";
import { useCallback, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Badge, Button, EmptyState } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { LoadMoreButton, QueryStates } from "../screens/common";
import {
  NOTIFICATIONS_KEY,
  type NotificationItem,
  useMarkAllNoticesRead,
  useNotifications,
} from "../screens/notifications.queries";
import { useNoticeRead } from "../screens/taskactions";
import { worklistKey } from "../screens/worklist.queries";
import { recordRoute } from "./entity";
import { usePopoverDismiss } from "./popover";
import { routeHash } from "./router";
import "./notificationbell.css";

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
  const { locale } = useLocale();
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const panelId = useId();
  const notices = useNotifications();

  // The count is the FIRST page's, which is the whole unread set: the endpoint
  // answers the reader's total beside every page rather than that page's share,
  // so paging back through history never moves the badge.
  const unread = notices.data?.pages[0]?.unread_count ?? 0;

  /**
   * Close, and put focus back where it can be used.
   *
   * Only when the panel actually HELD it — an outside click usually lands on
   * something focusable of its own, and pulling focus onto the bell afterwards
   * would undo what the click just did. The same rule the account menu keeps,
   * because they are two instances of one popover.
   */
  const dismiss = useCallback(() => {
    const held = panel.current?.contains(document.activeElement) ?? false;
    setOpen(false);
    if (held) {
      trigger.current?.focus();
    }
  }, []);
  // One dismissal for every popover in the chrome (app/popover.ts): Escape from
  // anywhere inside, any outside click, and the opening click deferred past.
  usePopoverDismiss(open, panel, dismiss);

  return (
    <div className="notifbell">
      <Button
        iconOnly
        ref={trigger}
        // The count joins the NAME, not only the box. `aria-label` replaces the
        // button's contents for name computation, so a figure left to the badge
        // alone would reach a screen reader from nowhere — and how many things
        // are waiting is the entire reason somebody presses this.
        aria-label={
          unread > 0
            ? t("notifications.bellWaiting", {
                count: formatNumber(unread, locale),
              })
            : t("notifications.bell")
        }
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls={open ? panelId : undefined}
        onClick={() => setOpen((current) => !current)}
      >
        <Bell aria-hidden />
        {/* A ZERO IS NOT A COUNT WORTH DRAWING: a badge reading "0" is a mark
            the eye stops on to learn there is nothing, which an unmarked bell
            already says. The spelling is the rail's, so the two counts in the
            chrome are one badge. */}
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
          <div className="notifbell-loose" ref={panel} id={panelId}>
            <NotificationCentre query={notices} />
          </div>,
          document.body,
        )}
    </div>
  );
}

function NotificationCentre({
  query,
}: Readonly<{ query: ReturnType<typeof useNotifications> }>) {
  const t = useT();
  const settleAll = useMarkAllNoticesRead();
  const rows = query.data?.pages.flatMap((page) => page.items) ?? [];
  return (
    <Panel
      title={t("notifications.centre")}
      titleAction={
        <Button
          small
          pending={settleAll.isPending}
          onClick={() => settleAll.mutate()}
        >
          {/* No number, deliberately. The server settles more than this panel
              ever showed — a reader's own stage-move notices among them — so
              any figure quoted here would be about a set they never saw. */}
          {t("notifications.markAllRead")}
        </Button>
      }
    >
      <PanelBody>
        <QueryStates query={query} pendingLabel={t("notifications.centre")}>
          {rows.length === 0 ? (
            <EmptyState>{t("notifications.empty")}</EmptyState>
          ) : (
            <>
              <ul className="notifbell-list">
                {rows.map((notice) => (
                  <NoticeRow key={notice.id} notice={notice} />
                ))}
              </ul>
              <LoadMoreButton query={query} />
            </>
          )}
        </QueryStates>
      </PanelBody>
    </Panel>
  );
}

/**
 * Whether an agent authored the change behind this notice.
 *
 * The origin names the ORIGINAL actor rather than the automation that delivered
 * the message, which is the distinction that makes the tint honest: a system
 * job running a rule nobody inferred is not a model deciding, and drawing it in
 * the AI colour would tell a reader one did.
 */
function agentAuthored(notice: NotificationItem): boolean {
  return notice.origin?.actor_type === "agent";
}

function NoticeRow({ notice }: Readonly<{ notice: NotificationItem }>) {
  const t = useT();
  // The centre and the Worklist's lane count the same rows, so settling one
  // here has to reach both or the two doors disagree about what is left.
  const settle = useNoticeRead([NOTIFICATIONS_KEY, worklistKey]);
  const route = recordRoute(notice.target?.type, notice.target?.id);
  const settled = notice.read_at !== undefined;
  return (
    <li className={settled ? "notifrow notifrow-settled" : "notifrow"}>
      <div className="notifrow-head">
        {route ? (
          <a className="notifrow-subject" href={routeHash(route)}>
            {notice.subject}
          </a>
        ) : (
          // A kind this app has no page for is NAMED and not linked. A link
          // into nothing is worse than no link: it promises a page the product
          // cannot reach.
          <span className="notifrow-subject">{notice.subject}</span>
        )}
        {!settled && <Badge tone="accent">{t("notifications.new")}</Badge>}
        {agentAuthored(notice) && (
          <Badge tone="ai">{t("notifications.byAgent")}</Badge>
        )}
      </div>
      {notice.body !== undefined && (
        <p className="notifrow-body t-sub">{notice.body}</p>
      )}
      {/* The verb is its own control rather than something the link does on its
          way out: a mutation fired as the reader navigates away races the
          unmount, and a reader who opened a record in a new tab would find the
          notice settled by a visit they had not made yet. */}
      {!settled && (
        <div className="notifrow-verb">
          <Button
            small
            pending={settle.isPending}
            onClick={() => settle.mutate(notice.id)}
          >
            {t("notifications.markRead")}
          </Button>
        </div>
      )}
    </li>
  );
}
