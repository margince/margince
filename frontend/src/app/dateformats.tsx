import { type ReactNode, useLayoutEffect } from "react";
import {
  type DateTimePreferences,
  setDateTimePreferences,
} from "../format/preferences";
import { useInstallationSettings } from "./uploadlimit";

// Mounted at the authenticated boundary: a settings change updates the whole
// presentation, while signing out releases this installation's preferences.
export function DateFormatsProvider({
  children,
}: Readonly<{ children: ReactNode }>) {
  // The authenticated boundary owns the request. Retrying on this observer
  // mount would turn a failed settings read into a splash/remount loop.
  const { data } = useInstallationSettings(false);
  const dateFormat = data?.date_format ?? "locale";
  const timeFormat = data?.time_format ?? "locale";
  useLayoutEffect(() => {
    const next: DateTimePreferences = { dateFormat, timeFormat };
    setDateTimePreferences(next);
    return () =>
      setDateTimePreferences({ dateFormat: "locale", timeFormat: "locale" });
  }, [dateFormat, timeFormat]);
  return children;
}
