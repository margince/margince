// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The requester's composer: pick the route, say why, and write the note the
// colleague can forward.
//
// Three pieces of copy, three fields, because they are three different
// messages. The reason and the value are read by the COLLEAGUE; the note is
// the only one a person outside the company ever sees. A drawer that collapsed
// them would put the internal case for the ask in front of the contact.

import { useId, useState } from "react";
import type { components } from "../api/schema";
import { Badge, Button, Field, Modal } from "../design-system/atoms";
import { ChoiceList } from "../design-system/choicelist";
import { useT } from "../i18n";
import { type IntroRequestInput, useCreateIntroRequest } from "./introrequests";
import { RouteLine } from "./personroutes";

type RouteCandidate = components["schemas"]["PersonGraphRouteCandidate"];
type FallbackPolicy = components["schemas"]["IntroFallbackPolicy"];

/**
 * IntroDrawer composes one ask.
 *
 * The route arrives as a prop rather than being chosen in here: the reader
 * picked it on the routes list, and offering the choice twice would let the two
 * disagree about which colleague is being asked.
 */
export function IntroDrawer({
  personId,
  personName,
  route,
  open,
  onClose,
}: Readonly<{
  personId: string;
  personName: string;
  route: RouteCandidate | undefined;
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  const [reason, setReason] = useState("");
  const [value, setValue] = useState("");
  const [note, setNote] = useState("");
  const [nameDrop, setNameDrop] = useState(false);
  const [fallback, setFallback] = useState<FallbackPolicy>("none");

  const create = useCreateIntroRequest(personId);

  // An ask with no reason is a favour with no case behind it, and the server
  // refuses it too. Saying so here means the reader learns it before they send.
  const ready = reason.trim().length > 0 && route !== undefined;

  const submit = () => {
    if (!ready || !route) {
      return;
    }
    const body: IntroRequestInput = {
      introducer_user_id: route.via_user_id,
      route_type: route.route_type,
      ...(route.through_person_id
        ? { through_person_id: route.through_person_id }
        : {}),
      internal_reason: reason.trim(),
      ...(value.trim() ? { value_for_target: value.trim() } : {}),
      ...(note.trim() ? { forwardable_note: note.trim() } : {}),
      // The rep typed it. Nothing here drafts, so claiming a model wrote it
      // would mark honest copy as machine-authored.
      note_generated_by: "human",
      note_ai_generated: false,
      name_drop_allowed: nameDrop,
      fallback_policy: fallback,
    };
    create.mutate(body, { onSuccess: () => onClose() });
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      labelledBy={titleId}
      placement="right"
      size="wide"
    >
      <h2 id={titleId}>{t("person.intro.askTitle", { name: personName })}</h2>

      {route ? (
        <p className="pn-route">
          <RouteLine route={route} />
        </p>
      ) : (
        <p className="pn-route">{t("person.graph.noRoute")}</p>
      )}

      <Field
        label={t("person.intro.reasonLabel")}
        hint={t("person.intro.reasonHint")}
      >
        {(control) => (
          <textarea
            {...control}
            rows={3}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
        )}
      </Field>

      <Field
        label={t("person.intro.valueLabel")}
        hint={t("person.intro.valueHint")}
      >
        {(control) => (
          <textarea
            {...control}
            rows={3}
            value={value}
            onChange={(e) => setValue(e.target.value)}
          />
        )}
      </Field>

      <Field
        label={t("person.intro.noteLabel")}
        hint={t("person.intro.noteHint")}
      >
        {(control) => (
          <textarea
            {...control}
            rows={5}
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
        )}
      </Field>

      <label className="pn-check">
        <input
          type="checkbox"
          checked={nameDrop}
          onChange={(e) => setNameDrop(e.target.checked)}
        />
        {t("person.intro.nameDropAsk")}
      </label>

      <ChoiceList<FallbackPolicy>
        legend={t("person.intro.fallbackLegend")}
        value={fallback}
        onChange={setFallback}
        choices={[
          {
            value: "none",
            label: t("person.intro.fallbackNone"),
            description: t("person.intro.fallbackNoneHelp"),
          },
          {
            value: "name_drop",
            label: t("person.intro.fallbackNameDrop"),
            description: t("person.intro.fallbackNameDropHelp"),
          },
          {
            value: "next_route",
            label: t("person.intro.fallbackNextRoute"),
            description: t("person.intro.fallbackNextRouteHelp"),
          },
        ]}
      />

      {/* Nothing here reaches the contact. The verb says so, because a button
          reading "Send" beside a note addressed to Dana would tell the reader
          the product just wrote to her. */}
      <div className="pn-actions">
        <Button onClick={onClose} variant="ghost">
          {t("person.intro.cancel")}
        </Button>
        <Button onClick={submit} disabled={!ready || create.isPending}>
          {t("person.intro.askAction")}
        </Button>
      </div>
      {create.isError ? (
        <p role="alert">
          <Badge tone="danger">{t("person.intro.askFailed")}</Badge>
        </p>
      ) : null}
    </Modal>
  );
}
