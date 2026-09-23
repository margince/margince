// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation } from "@tanstack/react-query";
import { Mail } from "lucide-react";
import { type FormEvent, useState } from "react";
import { api } from "../api/client";
import {
  Avatar,
  Badge,
  Button,
  Field,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ErrorLine } from "../design-system/errorline";
import { Eyebrow } from "../design-system/eyebrow";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody } from "../design-system/panel";
import { formatDateAbbrev } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { Wordmark } from "./auth";
import { throwProblem } from "./common";

// The buyer's page around its documents: the ground and the product's mark,
// the screens a dead or lapsed link lands on, the hero the seller wrote, and
// the contact beside the board. Everything here is drawn from the room's own
// row and the wire; the session and the board are `buyerroom.tsx`'s.

export function BuyerFrame({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <div className="buyer-page">
      <div className="buyer-column">
        {children}
        <PoweredBy />
      </div>
    </div>
  );
}

// The one thing on the buyer's page that is ours rather than the seller's:
// the product's mark, closing the column, saying what is serving the room.
function PoweredBy() {
  const t = useT();
  return (
    <span className="buyer-powered">
      <span className="t-caption" aria-hidden>
        {t("buyer.poweredBy")}
      </span>
      <Wordmark
        alt={t("buyer.poweredByMargince")}
        className="buyer-powered-mark"
      />
    </span>
  );
}

// The link no longer admits anyone. Whatever the reason — used, lapsed,
// retired, never valid — the page says the same thing and offers the one
// recovery a buyer has: a fresh link to the address they were invited at.
export function DeadLink({ message }: Readonly<{ message: string }>) {
  const t = useT();
  return (
    <Panel title={t("buyer.deadTitle")}>
      <PanelBody>
        <p>{message}</p>
      </PanelBody>
      <PanelBody>
        <LinkRequest />
      </PanelBody>
    </Panel>
  );
}

export function LinkRequest() {
  const t = useT();
  const [email, setEmail] = useState("");
  const request = useMutation({
    mutationKey: ["buyer-room-link-request"],
    mutationFn: async (address: string) => {
      const { error } = await api.POST("/public/rooms/link-request", {
        body: { email: address },
      });
      if (error) {
        throwProblem(error, t);
      }
    },
  });
  if (request.isSuccess) {
    return (
      <Callout
        tone="success"
        kind="outcome"
        title={t("buyer.linkRequestedTitle")}
      >
        {t("buyer.linkRequested")}
      </Callout>
    );
  }
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    request.mutate(email.trim());
  };
  return (
    <form className="buyer-linkrequest" onSubmit={submit}>
      <Field label={t("buyer.emailLabel")} hint={t("buyer.emailHint")}>
        {(field) => (
          <TextInput
            {...field}
            type="email"
            required
            value={email}
            onChange={(event) => setEmail(event.target.value)}
          />
        )}
      </Field>
      <Button
        type="submit"
        variant="primary"
        pending={request.isPending}
        disabled={email.trim() === ""}
      >
        <Mail aria-hidden />
        {t("buyer.requestLink")}
      </Button>
      <ErrorLine error={request.error} />
    </form>
  );
}

// The two access states this page draws a hero for, and what the pill says
// about each. `BuyerRoomAccess` is a plain wire string rather than a closed
// union, so this names the states it can speak for and stays silent about the
// rest: a build that has not heard of a state has no claim to make about it.
const ACCESS_PILL: Record<
  string,
  { label: MessageKey; tone?: "success"; live?: boolean }
> = {
  live: { label: "room.state.live", tone: "success", live: true },
  closed: { label: "room.state.closed" },
};

// The buyer's first screenful, and the only part of this page that is the
// SELLER's rather than the product's: what the room is called, what they wrote
// to open it, and whether it still takes answers. A hero rather than a header
// because this page is the one thing a client ever sees of Margince, and a
// form with a heading on it would be the wrong first impression of the deal
// it carries.
export function BuyerHero({
  title,
  welcome,
  access,
  closedAt,
}: Readonly<{
  title: string;
  welcome: string;
  access: string;
  closedAt: string | null | undefined;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const pill = ACCESS_PILL[access];
  return (
    <header className="buyer-hero">
      <div className="buyer-hero-top">
        <Eyebrow as="span">{t("buyer.eyebrow")}</Eyebrow>
        {pill ? (
          <span className="buyer-hero-standing">
            <Badge tone={pill.tone} live={pill.live}>
              {t(pill.label)}
            </Badge>
            {/* The day the record was fixed, beside the word that says it is
                one: "closed" alone leaves a buyer wondering whether they
                missed something last week or last year. */}
            {access === "closed" && closedAt ? (
              <span className="t-caption">
                {t("buyer.closedOn", {
                  date: formatDateAbbrev(closedAt, locale, viewerZone()),
                })}
              </span>
            ) : null}
          </span>
        ) : null}
      </div>
      <Heading size="xlarge" className="t-display">
        {title}
      </Heading>
      {welcome ? <p className="buyer-welcome">{welcome}</p> : null}
    </header>
  );
}

// Who is on the other end, as a card beside the documents rather than a line
// under the title: the one face on a page that is otherwise paper, and the
// answer to the question a buyer has after reading — whom do I ask.
//
// The mark is the steward's own, drawn from their name the way every contact
// in the product is drawn. A room whose steward's seat is gone gets no mark
// and no name: a monogram of the words "your contact" draws a contact who does
// not exist, so the card keeps only the sentence.
export function ContactCard({
  stewardName,
  access,
}: Readonly<{ stewardName: string | null | undefined; access: string }>) {
  const t = useT();
  const steward = stewardLabel(stewardName, t);
  return (
    <Panel>
      <PanelBody>
        <div className="buyer-contact">
          {stewardName ? <Avatar name={stewardName} size="md" /> : null}
          <div className="buyer-contact-id">
            <Eyebrow as="span">{t("buyer.contactEyebrow")}</Eyebrow>
            {stewardName ? <p className="t-h3">{stewardName}</p> : null}
          </div>
        </div>
        <p className="t-caption buyer-meta">
          {t("buyer.contact", { steward })}
          {access === "closed" ? ` ${t("buyer.closedNote")}` : ""}
        </p>
        <p className="buyer-contact-body">
          {t("buyer.contactBody", { steward })}
        </p>
      </PanelBody>
    </Panel>
  );
}

// Whom to ask, as a buyer reads it: the seller's own name while their seat
// stands, and the product's word for "somebody there" once it is gone. Both
// screens that name a steward say it through this, so a room cannot address a
// buyer to a contact on one and to nobody on the next.
export function stewardLabel(
  name: string | null | undefined,
  t: ReturnType<typeof useT>,
): string {
  return name ?? t("buyer.stewardUnknown");
}
