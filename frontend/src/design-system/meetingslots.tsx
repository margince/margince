import { Button, EmptyState } from "./atoms";
import "./meetingslots.css";

export function MeetingSlots({
  slots,
  selected,
  selectedMany,
  onSelect,
  empty,
  disabled = false,
}: Readonly<{
  slots: readonly { start: string; end: string; label: string }[];
  selected?: string;
  selectedMany?: readonly string[];
  onSelect: (slot: { start: string; end: string }) => void;
  empty: string;
  disabled?: boolean;
}>) {
  if (slots.length === 0) return <EmptyState>{empty}</EmptyState>;
  return (
    <div className="meeting-slots">
      {slots.map((slot) => (
        <Button
          key={slot.start}
          variant={
            selected === slot.start ||
            selectedMany?.includes(slot.start) === true
              ? "primary"
              : "ghost"
          }
          aria-pressed={
            selected === slot.start ||
            selectedMany?.includes(slot.start) === true
          }
          disabled={disabled}
          onClick={() => onSelect({ start: slot.start, end: slot.end })}
        >
          {slot.label}
        </Button>
      ))}
    </div>
  );
}

export type MeetingDay = Readonly<{
  key: string;
  weekday: string;
  date: string;
  slots: readonly { start: string; end: string; label: string; time: string }[];
}>;

// A week of free times read as columns, one per day, so a host compares days
// at a glance instead of scanning one long list of full date strings. `label`
// is the slot's accessible name; `time` is the short face drawn in the column.
// Without `onSelect` the week is a preview of times somebody else will choose
// from: the same columns at the same size, the times drawn but not pressable.
export function MeetingWeek({
  days,
  selected,
  onSelect,
  emptyDay,
  disabled = false,
}: Readonly<{
  days: readonly MeetingDay[];
  selected: readonly string[];
  onSelect?: (slot: { start: string; end: string }) => void;
  emptyDay: string;
  disabled?: boolean;
}>) {
  return (
    <div className="meeting-week">
      {days.map((day) => (
        <fieldset key={day.key} className="meeting-week-day">
          <legend className="sr-only">{`${day.weekday} ${day.date}`}</legend>
          <div className="meeting-week-head" aria-hidden="true">
            <span className="t-caption">{day.weekday}</span>
            <span className="t-name">{day.date}</span>
          </div>
          {day.slots.length === 0 && (
            <p className="t-caption meeting-week-empty">{emptyDay}</p>
          )}
          {day.slots.map((slot) => {
            if (!onSelect)
              return (
                <span key={slot.start} className="meeting-week-time t-num">
                  <span aria-hidden="true">{slot.time}</span>
                  <span className="sr-only">{slot.label}</span>
                </span>
              );
            const on = selected.includes(slot.start);
            return (
              <Button
                key={slot.start}
                variant={on ? "primary" : "ghost"}
                aria-pressed={on}
                aria-label={slot.label}
                disabled={disabled}
                onClick={() => onSelect({ start: slot.start, end: slot.end })}
              >
                {slot.time}
              </Button>
            );
          })}
        </fieldset>
      ))}
    </div>
  );
}
