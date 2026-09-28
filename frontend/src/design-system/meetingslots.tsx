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
