// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Phone } from "lucide-react";
import type { ReactNode } from "react";
import { telUri } from "../format/contacturi";
import { IconAction } from "./iconaction";

type CallActionProps =
  | {
      appearance: "value";
      number: string;
      children?: ReactNode;
      className?: string;
      /** What a refused number wears instead of the link's class. */
      textClassName?: string;
    }
  | {
      appearance: "button";
      number: string | undefined;
      label: string;
    }
  | {
      appearance: "icon";
      number: string | undefined;
      label: string;
      /** Why there is nothing to dial, for a missing or refused number. */
      reason: string;
      word?: string;
      /** The id of a sentence on the page that refuses the call, number or not. */
      reasonId?: string;
    };

/** The one control that dials a number: the number itself (`value`), a labelled
 *  verb (`button`) or the square glyph (`icon`), each `tel:` from `telUri`. */
export function CallAction(props: Readonly<CallActionProps>) {
  // No `target="_blank"`: the operating system takes a `tel:`, and a blank
  // target leaves an empty tab behind in some browsers.
  const href = props.number === undefined ? null : telUri(props.number);
  if (props.appearance === "value") {
    const body = props.children ?? props.number;
    if (!href) {
      return <span className={props.textClassName}>{body}</span>;
    }
    return (
      <a className={props.className ?? "link-button"} href={href}>
        {body}
      </a>
    );
  }
  if (props.appearance === "button") {
    if (!href) {
      return null;
    }
    return linkVerb(href, <Phone aria-hidden="true" />, props.label);
  }
  const icon = <Phone aria-hidden="true" />;
  if (props.reasonId !== undefined || !href) {
    return (
      <IconAction
        label={props.label}
        word={props.word}
        icon={icon}
        reason={props.reasonId === undefined ? props.reason : undefined}
        reasonId={props.reasonId}
      />
    );
  }
  return (
    <IconAction label={props.label} word={props.word} icon={icon} href={href} />
  );
}

// A labelled verb that goes somewhere: `Button`'s ghost look on an anchor, which
// `Button` cannot render.
export function linkVerb(href: string, glyph: ReactNode, label: string) {
  return (
    <a className="btn btn-ghost" href={href}>
      {glyph}
      {label}
    </a>
  );
}
