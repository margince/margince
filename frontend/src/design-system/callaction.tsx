// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Phone } from "lucide-react";
import type { MouseEvent, ReactNode } from "react";
import {
  type MessengerUris,
  messengerUris,
  telUri,
} from "../format/contacturi";
import { useT } from "../i18n";
import { MESSENGER_NAME, useCallMessengers } from "../lib/callmessengers";
import { type CallRecord, useCallStart } from "../lib/callstart";
import "./callaction.css";
import { opensElsewhere } from "./filechip";
import { IconAction } from "./iconaction";
import { OffsiteLink } from "./offsitelink";

type CallActionProps = { record: CallRecord } & (
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
    }
);

/** The one control that dials a number: the number itself (`value`), a labelled
 *  verb (`button`) or the square glyph (`icon`), each `tel:` from `telUri`.
 *  `value` adds a chat link per messenger the installation turns on. */
export function CallAction(props: Readonly<CallActionProps>) {
  const start = useCallStart();
  // Never `preventDefault`: the operating system still places the call. Safari
  // leaves a pressed link unfocused, and the log form hands focus back to it.
  const onClick = start
    ? (event: MouseEvent<HTMLAnchorElement>) => {
        if (opensElsewhere(event)) return;
        event.currentTarget.focus({ preventScroll: true });
        start(props.record);
      }
    : undefined;
  // No `target="_blank"`: the operating system takes a `tel:`, and a blank
  // target leaves an empty tab behind in some browsers.
  const href = props.number === undefined ? null : telUri(props.number);
  if (props.appearance === "value") {
    const body = props.children ?? props.number;
    if (!href) {
      return <span className={props.textClassName}>{body}</span>;
    }
    return withChats(
      props.number,
      <a
        className={`${props.className ?? "link-button"} callaction-number`}
        href={href}
        onClick={onClick}
      >
        {body}
      </a>,
    );
  }
  if (props.appearance === "button") {
    if (!href) {
      return null;
    }
    return linkVerb(href, <Phone aria-hidden="true" />, props.label, onClick);
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
    <IconAction
      label={props.label}
      word={props.word}
      icon={icon}
      href={href}
      onClick={onClick}
    />
  );
}

// A labelled verb that goes somewhere: `Button`'s ghost look on an anchor, which
// `Button` cannot render.
export function linkVerb(
  href: string,
  glyph: ReactNode,
  label: string,
  onClick?: (event: MouseEvent<HTMLAnchorElement>) => void,
) {
  return (
    <a className="btn btn-ghost" href={href} onClick={onClick}>
      {glyph}
      {label}
    </a>
  );
}

function withChats(number: string, dial: ReactNode) {
  const chats = messengerUris(number);
  return chats ? <WithMessengers dial={dial} chats={chats} /> : dial;
}

// Its own component so only a number with a country code reads the settings.
// A fragment, so the number is the first child a host's layout measures.
function WithMessengers({
  dial,
  chats,
}: Readonly<{ dial: ReactNode; chats: MessengerUris }>) {
  const t = useT();
  const messengers = useCallMessengers();
  return (
    <>
      {dial}
      {messengers.length > 0 && (
        <span className="callaction-chats">
          {messengers.map((messenger) => (
            <OffsiteLink
              key={messenger}
              href={chats[messenger]}
              label={t("callAction.openChat", {
                app: MESSENGER_NAME[messenger],
              })}
            >
              {MESSENGER_NAME[messenger]}
            </OffsiteLink>
          ))}
        </span>
      )}
    </>
  );
}
