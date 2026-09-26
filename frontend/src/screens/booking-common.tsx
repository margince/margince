import { useRef } from "react";
import { navigate } from "../app/router";
import { Button, Field } from "../design-system/atoms";
import { Select } from "../design-system/select";
import { UTC_ZONE, viewerZone } from "../format/timezone";
import { useT } from "../i18n";
import { PoweredBy } from "./buyerroomframe";

export function BookingFooter() {
  return (
    <PoweredBy className="book-powered" markClassName="book-powered-mark" />
  );
}
export function BookingZone({
  value,
  onChange,
}: Readonly<{ value: string; onChange: (zone: string) => void }>) {
  const t = useT();
  const zones = Array.from(
    new Set([UTC_ZONE, viewerZone(), ...Intl.supportedValuesOf("timeZone")]),
  );
  return (
    <Field label={t("scheduling.timezone")}>
      {(control) => (
        <Select
          {...control}
          value={value}
          options={zones.map((zone) => ({
            value: zone,
            label: zone.replaceAll("_", " "),
          }))}
          onChange={onChange}
        />
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
