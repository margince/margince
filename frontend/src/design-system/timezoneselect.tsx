// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMemo, useState } from "react";
import { timezoneOption, timezoneOptions } from "../format/timezone";
import { useLocale } from "../i18n";
import { Select, type SelectProps } from "./select";

export function TimezoneSelect(props: Omit<SelectProps, "options">) {
  const { locale } = useLocale();
  const [at] = useState(() => new Date());
  const options = useMemo(() => timezoneOptions(locale, "", at), [locale, at]);
  const choices = options.some((option) => option.value === props.value)
    ? options
    : [...options, timezoneOption(locale, props.value, at)];
  return <Select {...props} options={choices} />;
}
