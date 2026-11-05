import { useEffect, useRef } from "react";
import { Button, Field } from "../../design-system/atoms";
import { TimezoneSelect } from "../../design-system/timezoneselect";
import { useT } from "../../i18n";
import { navigate } from "../../lib/router";

// A guest's tab names the booking they are on, never the product. The shell's
// "Margince" title comes back when the screen leaves. null leaves the title
// alone, for the host's own signed-in views of the same screens.
export function useBookingTitle(title: string | null) {
  useEffect(() => {
    if (title === null) return;
    const previous = document.title;
    document.title = title;
    return () => {
      document.title = previous;
    };
  }, [title]);
}

export function BookingZone({
  value,
  onChange,
}: Readonly<{ value: string; onChange: (zone: string) => void }>) {
  const t = useT();
  return (
    <Field label={t("scheduling.timezone")}>
      {(control) => (
        <TimezoneSelect {...control} value={value} onChange={onChange} />
      )}
    </Field>
  );
}
export function useBookingIntent() {
  const intent = useRef<{ payload: string; key: string } | null>(null);
  return (payload: unknown) => {
    const encoded = JSON.stringify(payload);
    if (intent.current?.payload !== encoded)
      intent.current = { payload: encoded, key: crypto.randomUUID() };
    return intent.current.key;
  };
}

export function BookingBack() {
  const t = useT();
  return (
    <div className="book-actions">
      <Button onClick={() => navigate({ screen: "home" })}>
        {t("scheduling.back")}
      </Button>
    </div>
  );
}
