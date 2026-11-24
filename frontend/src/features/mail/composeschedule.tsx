import { ChevronUp, Clock } from "lucide-react";
import {
  type ComponentProps,
  Suspense,
  useEffect,
  useRef,
  useState,
} from "react";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { Popover } from "../../design-system/popover";
import { calendarDay, isRealCalendarDay } from "../../format/calendarday";
import { formatDateTime } from "../../format/format";
import {
  browserZone,
  instantAtWallClock,
  startOfDayInZone,
  zoneNameAndOffset,
} from "../../format/timezone";
import { type Locale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { lazy } from "../../i18n/lazy";
import type { ScheduleDialog } from "./composescheduledialog";

// When a message goes: the presets, and the wall-clock arithmetic that keeps one
// meaning what it says in the reader's own zone. The dialog loads on first open.

export function schedulePresets(
  now: Date,
  zone = browserZone(),
): readonly {
  key: MessageKey;
  at: Date;
}[] {
  const today = calendarDay(now, zone);
  const at = (days: number, hour: number) => {
    const day = calendarDay(
      new Date(startOfDayInZone(today, zone, days)),
      zone,
    );
    return new Date(instantAtWallClock(day, hour * 3_600_000, zone));
  };
  // The weekday of the zone's own date: arithmetic on a named day, no clock read.
  const index = new Date(`${today}T12:00:00Z`).getUTCDay();
  const untilMonday = (8 - index) % 7 || 7;
  return [
    { key: "compose.scheduleTomorrow", at: at(1, 8) },
    { key: "compose.scheduleAfternoon", at: at(1, 13) },
    { key: "compose.scheduleMonday", at: at(untilMonday, 8) },
  ];
}

type ScheduleSource = Pick<
  components["schemas"]["SendEmailRequest"],
  "scheduled_at" | "scheduled_tz" | "scheduled_intent"
>;
export type ScheduleFields = {
  [Key in keyof ScheduleSource]?: NonNullable<ScheduleSource[Key]>;
};

// A saved legacy instant remains an instant until the reader chooses a time.
export function scheduleFields(
  local: string,
  saved?: ScheduleFields,
  timezone = browserZone(),
): ScheduleFields {
  if (local === "") return {};
  if (saved !== undefined) return saved;
  const at = scheduleInstant(local, timezone);
  return {
    scheduled_at: at.toISOString(),
    scheduled_tz: timezone,
    scheduled_intent: { wall_clock: local, timezone },
  };
}

function scheduleInstant(local: string, timezone: string): Date {
  const parts =
    /^([0-9]{4}-[0-9]{2}-[0-9]{2})T([0-9]{2}):([0-9]{2})(?::([0-9]{2}(?:\.[0-9]{1,6})?))?$/.exec(
      local,
    );
  if (!parts || !isRealCalendarDay(parts[1] ?? ""))
    throw new RangeError("Invalid scheduled time");
  const day = parts[1] ?? "";
  const hour = Number(parts[2]);
  const minute = Number(parts[3]);
  const second = Number(parts[4] ?? 0);
  if (hour > 23 || minute > 59 || second >= 60)
    throw new RangeError("Invalid scheduled time");
  const milliseconds = (hour * 3600 + minute * 60 + Math.floor(second)) * 1000;
  let instant: number;
  try {
    instant = instantAtWallClock(day, milliseconds, timezone);
  } catch (error) {
    if (!(error instanceof RangeError) || Number(day.substring(0, 4)) < 100)
      throw error;
    instant = Date.parse(startOfDayInZone(day, timezone, 1));
  }
  return new Date(instant);
}

export function scheduleSourceOf(saved: ScheduleSource): ScheduleFields {
  return {
    scheduled_at: saved.scheduled_at ?? undefined,
    scheduled_tz: saved.scheduled_tz ?? undefined,
    scheduled_intent: saved.scheduled_intent ?? undefined,
  };
}

export function sameScheduleSource(
  a?: ScheduleFields,
  b?: ScheduleFields,
): boolean {
  return (
    a?.scheduled_at === b?.scheduled_at &&
    a?.scheduled_tz === b?.scheduled_tz &&
    a?.scheduled_intent?.wall_clock === b?.scheduled_intent?.wall_clock &&
    a?.scheduled_intent?.timezone === b?.scheduled_intent?.timezone
  );
}

export function momentOf(local: string, zone = browserZone()): Date | null {
  if (!local) return null;
  try {
    return scheduleInstant(local, zone);
  } catch (error) {
    if (error instanceof RangeError) return null;
    throw error;
  }
}

/** A scheduled moment, spelled the way every line in this drawer spells one. */
export function momentLabel(at: Date, locale: Locale, zone: string): string {
  return `${formatDateTime(at.toISOString(), locale, zone)} · ${zoneNameAndOffset(locale, at, zone)}`;
}

export function scheduledSendLabel(
  local: string,
  locale: Locale,
  zone: string,
): string | null {
  const at = momentOf(local, zone);
  return at ? momentLabel(at, locale, zone) : null;
}

// The other way to send, behind the confirm button's own caret.
//
// A moment is a choice ABOUT the send rather than a field of the message, so
// it belongs with the send control and not in the form above it — it was a
// datetime field between the consent purpose and the recipient warnings, which
// is where a rep reads the message rather than where they decide to release
// it.
export function ScheduleMenu({
  onOpen,
  reasonId,
}: Readonly<{
  onOpen: () => void;
  /** The sentence that holds Send holds this menu too. */
  reasonId?: string;
}>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  // The dialog opens after the closed panel hands focus back to its trigger.
  // So the dialog returns focus to the trigger, not to the page.
  const opening = useRef(false);
  useEffect(() => {
    if (open || !opening.current) return;
    opening.current = false;
    onOpen();
  }, [open, onOpen]);
  return (
    <Popover
      variant="primary"
      reasonId={reasonId}
      className="compose-sendmenu"
      open={open}
      onOpenChange={setOpen}
      label={
        <>
          <ChevronUp aria-hidden="true" size={16} />
          <span className="sr-only">{t("compose.sendOptions")}</span>
        </>
      }
    >
      <Button
        variant="ghost"
        onClick={() => {
          opening.current = true;
          setOpen(false);
        }}
      >
        <Clock aria-hidden="true" />
        {t("compose.scheduleSend")}
      </Button>
    </Popover>
  );
}

const LazyScheduleDialog = lazy(async () => ({
  default: (await import("./composescheduledialog")).ScheduleDialog,
}));

/** The schedule dialog, whose code loads the first time `open` turns true. */
export function ScheduleDialogOnDemand({
  open,
  ...dialog
}: Readonly<
  Omit<ComponentProps<typeof ScheduleDialog>, "open"> & { open: boolean | null }
>) {
  if (open === null) return null;
  return (
    <Suspense fallback={null}>
      <LazyScheduleDialog open={open} {...dialog} />
    </Suspense>
  );
}
