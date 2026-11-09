import { Checkbox } from "../design-system/atoms";
import { useT } from "../i18n";
export function TaskCompletionChoice({
  enabled,
  checked,
  onChange,
  pending,
}: Readonly<{
  enabled: boolean;
  checked: boolean;
  onChange: (checked: boolean) => void;
  pending: boolean;
}>) {
  const t = useT();
  return enabled ? (
    <Checkbox
      label={t("tasks.log.markDone")}
      checked={checked}
      onChange={(event) => onChange(event.target.checked)}
      disabled={pending}
    />
  ) : null;
}
