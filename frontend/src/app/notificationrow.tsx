import { AppLink } from "../design-system/applink";
import { Badge, Button } from "../design-system/atoms";
import type { NotificationItem } from "../features/settings/notifications.queries";
import { formatDateTime } from "../format/format";
import { useLocale, useT } from "../i18n";
import { opensElsewhere } from "../lib/linkclick";
import { useSourceOpener } from "../lib/opensource";
import { displayedBody } from "../lib/systemdisplay";
import { useViewerZone } from "../lib/viewerzone";
import { noticeAction } from "./noticeaction";
import { noticeHeadline } from "./noticeheadline";

// Delivery by automation does not make the original author an agent.
function agentAuthored(notice: NotificationItem): boolean {
  return notice.origin?.actor_type === "agent";
}

export function NoticeRow({
  notice,
  onSettle,
  onActivate,
  settling,
}: Readonly<{
  notice: NotificationItem;
  onSettle: (id: string) => void;
  // Following a subject dismisses the panel covering its destination.
  onActivate: () => void;
  settling: boolean;
}>) {
  const readerZone = useViewerZone();
  const t = useT();
  const action = noticeAction(notice.target);
  const href = action?.href;
  const sourceOpener = useSourceOpener();
  const openActivity =
    notice.target?.type === "activity"
      ? sourceOpener({ id: notice.target.id, kind: "note" })
      : undefined;
  const { locale } = useLocale();
  const settled = notice.read_at !== undefined;
  const { headline, detail } = noticeHeadline(notice, t);
  return (
    <li className={settled ? "notifrow notifrow-settled" : "notifrow"}>
      <div className="notifrow-head">
        {href ? (
          <AppLink
            className="entity-link notifrow-subject"
            href={href}
            onClick={onActivate}
          >
            {headline}
          </AppLink>
        ) : (
          <span className="notifrow-subject">{headline}</span>
        )}
        {!settled && <Badge tone="accent">{t("notifications.unread")}</Badge>}
        {agentAuthored(notice) && (
          <Badge tone="ai">{t("notifications.byAgent")}</Badge>
        )}
      </div>
      {detail !== undefined && <p className="notifrow-body t-sub">{detail}</p>}
      {notice.body !== undefined && (
        <p className="notifrow-body t-sub">{displayedBody(notice)}</p>
      )}
      <time className="t-caption" dateTime={notice.created_at}>
        {formatDateTime(notice.created_at, locale, readerZone)}
      </time>
      {/* Reading in a new tab does not settle the notice in this one. */}
      {(action || openActivity || !settled) && (
        <div className="notifrow-verb">
          {action && (
            <AppLink
              className="btn btn-ghost"
              href={action.href}
              onClick={(event) => {
                if (!opensElsewhere(event)) onActivate();
              }}
            >
              {t(action.label)}
            </AppLink>
          )}
          {openActivity && (
            <Button
              variant="ghost"
              onClick={() => {
                onActivate();
                openActivity();
              }}
            >
              {t("tasks.openSource")}
            </Button>
          )}
          {!settled && (
            <Button
              variant="link"
              pending={settling}
              onClick={() => onSettle(notice.id)}
            >
              {t("notifications.markRead")}
            </Button>
          )}
        </div>
      )}
    </li>
  );
}

import "./notificationbell.css";
