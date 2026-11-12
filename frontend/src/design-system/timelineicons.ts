import {
  CalendarClock,
  CheckSquare,
  Mail,
  MessageCircle,
  PencilLine,
  Phone,
  Puzzle,
  StickyNote,
  Zap,
} from "lucide-react";

// One icon per kind of timeline entry.
export const TIMELINE_ICON = {
  email: Mail,
  meeting: CalendarClock,
  note: StickyNote,
  call: Phone,
  task: CheckSquare,
  message: MessageCircle,
  change: PencilLine,
  extension: Puzzle,
  signal: Zap,
} as const;
