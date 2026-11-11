// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { RefObject } from "react";
import { Avatar } from "./atoms";
import { useTooltip } from "./tooltip";

/** The colleague who owns a deal, with their picture when they have one. */
export type DealOwnerSeat = Readonly<{
  id: string;
  name: string;
  photo?: string | null;
}>;

/**
 * Who owns the deal, as a mark at the edge of the company line. Initials alone
 * need decoding, so the full name is its label and its tip. It takes no tab
 * stop: the card's link already holds one.
 */
export function DealOwner({
  owner: { id, name, photo },
  dealLink,
}: Readonly<{
  owner: DealOwnerSeat;
  dealLink: RefObject<HTMLAnchorElement | null>;
}>) {
  const { ref, trigger, tip } = useTooltip<HTMLSpanElement>(name);
  const press = handOnTo(dealLink);
  return (
    <span // NOSONAR: forwards a pointer press to the deal's link; the mark itself is not a control
      className="deal-owner"
      role="img"
      aria-label={name}
      ref={ref}
      {...trigger}
      {...press}
    >
      <Avatar name={name} identity={id} photo={photo} />
      {tip}
    </span>
  );
}

/**
 * A mark that sits above a card's stretched link, so its tip can open, hands a
 * press on to that link with the button and keys held: the card stays one door.
 */
export function handOnTo(dealLink: RefObject<HTMLAnchorElement | null>) {
  return {
    onClick: (event: React.MouseEvent) =>
      dealLink.current?.dispatchEvent(
        new MouseEvent("click", {
          bubbles: true,
          cancelable: true,
          button: event.button,
          ctrlKey: event.ctrlKey,
          metaKey: event.metaKey,
          shiftKey: event.shiftKey,
          altKey: event.altKey,
        }),
      ),
    // The middle button opens the deal in a new tab from this real press: a
    // dispatched button-1 click can leave that tab blank under load.
    onAuxClick: (event: React.MouseEvent) => {
      if (event.button === 1 && dealLink.current) {
        window.open(dealLink.current.href, "_blank", "noopener");
      }
    },
  };
}
