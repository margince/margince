// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronRight } from "lucide-react";
import { IconAction } from "./iconaction";

type RowOpenPress = Readonly<{
  label: string;
  refusal?: string;
  opensDialog?: boolean;
  onOpen?: () => void;
  href?: undefined;
}>;

// An address this row opens is a link, so it can be copied, middle-clicked and
// opened in a tab. A refused link is no link, as on `IconAction`, so this half
// takes no refusal: a row that may refuse uses the press half.
type RowOpenLink = Readonly<{
  label: string;
  href: string;
  refusal?: undefined;
  opensDialog?: undefined;
  onOpen?: undefined;
}>;

// The keyboard path to what a press on the row opens. A refusal rides the tip
// and the description, not a visible line that would widen the row.
export function RowOpen(props: RowOpenPress | RowOpenLink) {
  const icon = <ChevronRight aria-hidden />;
  if (props.href !== undefined) {
    return (
      <IconAction
        variant="ghost"
        icon={icon}
        label={props.label}
        href={props.href}
      />
    );
  }
  return (
    <IconAction
      variant="ghost"
      icon={icon}
      label={props.label}
      hint={props.refusal}
      disabled={props.refusal !== undefined}
      opensDialog={props.opensDialog}
      onClick={props.onOpen}
    />
  );
}
