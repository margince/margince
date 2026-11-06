import { Avatar, Badge } from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { CellStrip } from "../../design-system/listtable";
import { StatusBadge, type StatusKind } from "../../design-system/statusbadge";
import { formatDateAbbrev, formatDateTime } from "../../format/format";
import { formatRelativeTime } from "../../format/relativetime";
import { useLocale, useT } from "../../i18n";
import { useViewerZone } from "../../lib/viewerzone";
import { memberAnchorId, type User } from "./users-members";

export function MemberIdentity({ member }: Readonly<{ member: User }>) {
  const t = useT();
  return (
    // Focusable by script only: the deactivate confirm hands focus back here.
    <span id={memberAnchorId(member.id)} className="users-member" tabIndex={-1}>
      <Avatar
        name={member.display_name}
        identity={member.id}
        photo={member.avatar_url}
      />
      <CellStack>
        <span className="users-member-name">
          <span className="t-name">{member.display_name}</span>
          {/* The agent seat owns records, so it is listed; the badge keeps it
              from passing for a colleague. */}
          {member.service_extension ? (
            <Badge>{t("users.serviceMember")}</Badge>
          ) : (
            member.is_agent && <Badge tone="ai">{t("users.agentSeat")}</Badge>
          )}
          <MemberStatus
            status={member.status}
            mailFailed={member.invite_email_state === "failed"}
          />
        </span>
        <span className="t-caption">
          {member.service_extension
            ? t("users.serviceMemberFor", {
                extension: member.service_extension,
              })
            : member.email}
        </span>
      </CellStack>
    </span>
  );
}

export function MemberTeams({
  ids,
  names,
}: Readonly<{
  ids: string[] | undefined;
  names: ReadonlyMap<string, string>;
}>) {
  const known = (ids ?? []).flatMap((id) => names.get(id) ?? []);
  if (known.length === 0) {
    return null;
  }
  return (
    <span className="users-teams" title={known.join(", ")}>
      <CellStrip>
        {known.map((name) => (
          <Badge key={name}>{name}</Badge>
        ))}
      </CellStrip>
    </span>
  );
}

// Active is every seat's ordinary state, so only the exceptions are marked.
// Invited and deactivated are settled facts; a suspension asks someone to act.
const STATUS_KIND = {
  invited: "neutral",
  suspended: "attention",
  deactivated: "neutral",
} as const satisfies Readonly<Record<string, StatusKind>>;

// An invitation whose mail the relay refused reaches nobody until an admin
// resends it, so it asks for action where a plain invitation does not.
function MemberStatus({
  status,
  mailFailed,
}: Readonly<{ status: string; mailFailed: boolean }>) {
  const t = useT();
  if (status === "invited" && mailFailed) {
    return (
      <StatusBadge
        kind="attention"
        label={t("users.status.inviteMailFailed")}
      />
    );
  }
  if (
    status !== "invited" &&
    status !== "suspended" &&
    status !== "deactivated"
  ) {
    return null;
  }
  return (
    <StatusBadge
      kind={STATUS_KIND[status]}
      label={t(`users.status.${status}`)}
    />
  );
}

export function MemberActivity({
  member,
  management,
}: Readonly<{ member: User; management: boolean }>) {
  const readerZone = useViewerZone();
  const t = useT();
  const { locale } = useLocale();
  const added = member.created_at;
  return (
    <CellStack>
      {management && <LastActive at={member.last_active_at} />}
      {added && (
        <time className="t-caption users-nowrap" dateTime={added}>
          {t("users.addedOn", {
            date: formatDateAbbrev(added, locale, readerZone),
          })}
        </time>
      )}
    </CellStack>
  );
}

// Null means never signed in, or withheld from this reader: a delegated admin
// sees none for a member they do not outrank. The cell claims neither.
function LastActive({ at }: Readonly<{ at: string | null | undefined }>) {
  const readerZone = useViewerZone();
  const t = useT();
  const { locale } = useLocale();
  if (!at) {
    return <span className="sr-only">{t("users.lastActiveUnknown")}</span>;
  }
  return (
    <time
      className="users-nowrap"
      dateTime={at}
      title={formatDateTime(at, locale, readerZone)}
    >
      {formatRelativeTime(at, locale)}
    </time>
  );
}
