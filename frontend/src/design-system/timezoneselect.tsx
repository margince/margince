// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMemo, useState } from "react";
import {
  currentZoneName,
  timezoneOption,
  timezoneOptions,
} from "../format/timezoneoptions";
import { useLocale } from "../i18n";
import { Select, type SelectProps } from "./select";

export function TimezoneSelect(props: Omit<SelectProps, "options">) {
  const { locale } = useLocale();
  const [at] = useState(() => new Date());
  const options = useMemo(() => timezoneOptions(locale, "", at), [locale, at]);
  // A zone saved under its former name takes the place of its current entry.
  // The list then shows it once, and a reader who changes nothing saves nothing.
  const named = currentZoneName(props.value);
  const choices = options.some((option) => option.value === props.value)
    ? options
    : options.some((option) => option.value === named)
      ? options.map((option) =>
          option.value === named
            ? timezoneOption(locale, props.value, at)
            : option,
        )
      : [...options, timezoneOption(locale, props.value, at)];
  return <Select {...props} options={choices} />;
}
