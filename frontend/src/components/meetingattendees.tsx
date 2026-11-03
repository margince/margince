// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Who was in a logged meeting or call: the contacts on their side and the
// colleagues on ours. Every contact named becomes a link, so the entry reaches
// each attendee's History and, through their employer, their company's. Every
// colleague named is recorded as a participant, so it counts toward their
// relationship with those contacts the way a captured calendar meeting does.

import { useCallback } from "react";
import { api } from "../api/client";
import { Field } from "../design-system/atoms";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../design-system/recordpicker";
import { Select } from "../design-system/select";
import { TokenList } from "../design-system/tokeninput";
import { useT } from "../i18n";
import type { MeetingAttendees } from "../lib/activitybody";
import type { EntityKind } from "../lib/entity";
import { unwrap } from "../lib/problem";
import { useMe } from "../lib/session";
import { useAssignableUserOptions } from "./assigneepicker";
import {
  RosterFailedLine,
  useRosterPartial,
  useRosterPartialHint,
} from "./entityref";

const SEARCH_LIMIT = 20;

// The server's cap on one activity's links and on its named colleagues
// (`maxActivityLinks`, `maxItems` of `attendee_user_ids` in crm.yaml).
export const MAX_MEETING_NAMES = 25;

async function searchContacts(
  q: string,
  companyID?: string,
): Promise<RecordPickerCandidate[]> {
  const data = unwrap(
    await api.GET("/contacts", {
      params: {
        query: {
          q,
          limit: SEARCH_LIMIT,
          ...(companyID ? { company_id: companyID } : {}),
        },
      },
    }),
  );
  return data.data.map((contact) => ({
    id: contact.id,
    // The employer tells two contacts of one name apart once the search
    // crosses companies.
    name: contact.employer
      ? `${contact.full_name} · ${contact.employer.company_name}`
      : contact.full_name,
  }));
}

// The company's own contacts, for a verb that acts on one of them.
export function searchCompanyContacts(
  companyID: string,
  q: string,
): Promise<RecordPickerCandidate[]> {
  return searchContacts(q, companyID);
}

// Every contact the reader may see, with the open company's own contacts first:
// on a company page the likeliest attendees work there. The open contact and
// the ones already picked are left out, because each is linked already.
function useAttendeeSearch(
  entityType: EntityKind,
  entityId: string,
  picked: readonly RecordPickerCandidate[],
) {
  return useCallback(
    async (q: string) => {
      const [own, everyone] = await Promise.all([
        entityType === "company" ? searchContacts(q, entityId) : [],
        searchContacts(q),
      ]);
      const taken = new Set(picked.map((contact) => contact.id));
      if (entityType === "contact") {
        taken.add(entityId);
      }
      const out: RecordPickerCandidate[] = [];
      for (const contact of [...own, ...everyone]) {
        if (!taken.has(contact.id)) {
          taken.add(contact.id);
          out.push(contact);
        }
      }
      return out.slice(0, SEARCH_LIMIT);
    },
    [entityType, entityId, picked],
  );
}

/**
 * The attendee and colleague fields for a meeting or call. On a company page
 * the contacts named here replace the company link, which the server refuses.
 * Anywhere else they join the open record.
 */
export function MeetingAttendeeFields({
  entityType,
  entityId,
  value,
  onChange,
  disabled,
}: Readonly<{
  entityType: EntityKind;
  entityId: string;
  value: MeetingAttendees;
  onChange: (next: MeetingAttendees) => void;
  disabled?: boolean;
}>) {
  const t = useT();
  const search = useAttendeeSearch(entityType, entityId, value.contacts);
  // Off a company page the open record holds one of the links.
  const contactsFull =
    value.contacts.length >=
    MAX_MEETING_NAMES - (entityType === "company" ? 0 : 1);
  return (
    <>
      <Field
        label={
          entityType === "company" ? t("log.attendee") : t("log.otherAttendees")
        }
      >
        {(control) => (
          <RecordPicker
            id={control.id}
            aria-describedby={control["aria-describedby"]}
            searchTargets={search}
            onPick={(contact) =>
              onChange({ ...value, contacts: [...value.contacts, contact] })
            }
            disabled={disabled || contactsFull}
          />
        )}
      </Field>
      {value.contacts.length > 0 && (
        <TokenList
          items={value.contacts.map((contact) => ({
            id: contact.id,
            label: contact.name,
          }))}
          removeLabel={(item) => t("log.removeAttendee", { name: item.label })}
          disabled={disabled}
          onRemove={(id) =>
            onChange({
              ...value,
              contacts: value.contacts.filter((contact) => contact.id !== id),
            })
          }
        />
      )}
      <ColleaguesField value={value} onChange={onChange} disabled={disabled} />
    </>
  );
}

// The reader is on the meeting already as whoever logged it, so they are not
// offered; neither is a colleague already named.
function ColleaguesField({
  value,
  onChange,
  disabled,
}: Readonly<{
  value: MeetingAttendees;
  onChange: (next: MeetingAttendees) => void;
  disabled?: boolean;
}>) {
  const t = useT();
  const me = useMe();
  const { options, roster } = useAssignableUserOptions();
  const partialHint = useRosterPartialHint(useRosterPartial("user", true));
  const labelOf = new Map(
    options.map((option) => [option.value, option.label]),
  );
  const offered = options.filter(
    (option) =>
      option.value !== me.data?.user?.id &&
      !value.colleagueIds.includes(option.value),
  );
  return (
    <>
      <Field label={t("log.colleagues")} hint={partialHint}>
        {(control) => (
          <Select
            {...control}
            options={offered}
            value=""
            placeholder={t("log.addColleague")}
            disabled={
              disabled ||
              offered.length === 0 ||
              value.colleagueIds.length >= MAX_MEETING_NAMES
            }
            onChange={(id) =>
              id &&
              onChange({ ...value, colleagueIds: [...value.colleagueIds, id] })
            }
          />
        )}
      </Field>
      <RosterFailedLine kind="user" roster={roster} />
      {value.colleagueIds.length > 0 && (
        <TokenList
          items={value.colleagueIds.map((id) => ({
            id,
            label: labelOf.get(id) ?? id,
          }))}
          removeLabel={(item) => t("log.removeAttendee", { name: item.label })}
          disabled={disabled}
          onRemove={(id) =>
            onChange({
              ...value,
              colleagueIds: value.colleagueIds.filter((other) => other !== id),
            })
          }
        />
      )}
    </>
  );
}
