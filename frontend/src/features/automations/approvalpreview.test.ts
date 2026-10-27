import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { formatDate, formatMoney } from "../../format/format";
import { LOCALES, translate } from "../../i18n";
import {
  commercialPreviewDisplay,
  editedCommercialPreview,
} from "../../lib/approvalpreview";
import { messageApproval } from "../../testing/message-approval";
import { approvalAcceptable } from "./approvalresponsibility";

type Preview = components["schemas"]["ApprovalPreview"];

it.each(LOCALES)(
  "labels every message recipient in %s without replacing the full body",
  (locale) => {
    const approval = messageApproval();
    if (!approval.preview) throw new Error("message fixture needs a preview");
    const rows = commercialPreviewDisplay(
      approval.preview,
      (key, params) => translate(locale, key, params),
      locale,
    );
    for (const [key, address] of [
      ["compose.to", "buyer@example.test"],
      ["compose.cc", "finance@example.test"],
      ["compose.bcc", "archive@example.test"],
    ] as const) {
      expect(rows.find((row) => row.value === address)?.label).toBe(
        translate(locale, key),
      );
    }
    expect(rows.find((row) => row.field === "body")?.value).toBeNull();
    expect(approvalAcceptable(approval)).toBe(true);
  },
);
const preview: Preview = {
  version: 1,
  state: "ready",
  reviewer_reason: "record_authority",
  changes: [
    {
      field: "owner_id",
      before: { kind: "owner", text: "Ada" },
      after: { kind: "owner", text: "Ben" },
    },
  ],
};

describe("commercial approval preview", () => {
  it.each(LOCALES)(
    "explains an unknown conflict without a summary in %s",
    (locale) => {
      const unknown: Preview = {
        ...preview,
        conflicts: [
          { kind: "future_reason", summary: " ", blocks_approval: true },
        ],
      };
      const rows = commercialPreviewDisplay(unknown, (key, params) =>
        translate(locale, key, params),
      );
      expect(
        rows.find((row) => row.field === "preview_conflict_0")?.value,
      ).toBe(translate(locale, "approval.preview.incompleteEvidence"));
      expect(approvalAcceptable({ preview: unknown })).toBe(false);
    },
  );
  it.each(LOCALES)("labels a structured companion value in %s", (locale) => {
    const value = '{"city":"Berlin"}';
    const rows = commercialPreviewDisplay(
      {
        ...preview,
        changes: [
          {
            field: "address",
            before: { kind: "empty", text: "" },
            after: { kind: "structured", text: value },
          },
        ],
      },
      (key, params) => translate(locale, key, params),
    );
    expect(rows[0].comparison).toEqual({
      from: translate(locale, "approval.preview.empty"),
      to: translate(locale, "approval.preview.structured", { value }),
    });
  });
  it.each(LOCALES)("names the exact owner change in %s", (locale) => {
    const rows = commercialPreviewDisplay(preview, (key, params) =>
      translate(locale, key, params),
    );
    expect(rows[0].comparison).toEqual({ from: "Ada", to: "Ben" });
    expect(rows[0].label).toBe(translate(locale, "history.field.owner_id"));
    expect(rows[1].value).toBe(translate(locale, "approval.preview.authority"));
    expect(approvalAcceptable({ preview })).toBe(true);
  });
  it.each(["stale", "unavailable", "incomplete", "future_state"])(
    "explains %s without showing old changes",
    (state) => {
      const incomplete = { ...preview, state };
      const rows = commercialPreviewDisplay(incomplete, (key, params) =>
        translate("en", key, params),
      );
      expect(rows).toHaveLength(1);
      expect(rows[0].comparison).toBeNull();
      expect(rows[0].value).not.toContain("Ada");
      expect(approvalAcceptable({ preview: incomplete })).toBe(false);
    },
  );
  it("refuses an unsupported preview version", () => {
    const newer = { ...preview, version: 999 };
    expect(approvalAcceptable({ preview: newer })).toBe(false);
    const rows = commercialPreviewDisplay(newer, (key, params) =>
      translate("en", key, params),
    );
    expect(rows[0].value).toBe(translate("en", "approval.preview.unavailable"));
  });
});

it.each(LOCALES)("translates the win reason in %s", (locale) => {
  const rows = commercialPreviewDisplay(
    {
      ...preview,
      changes: [
        {
          field: "won_without_contract_reason",
          before: { kind: "empty", text: "" },
          after: { kind: "text", text: "verbal" },
        },
      ],
    },
    (key, params) => translate(locale, key, params),
  );
  expect(rows[0].label).toBe(translate(locale, "deals.winReason"));
  expect(rows[0].comparison).toEqual({
    from: translate(locale, "approval.preview.empty"),
    to: translate(locale, "deals.winReasonVerbal"),
  });
});

it.each(LOCALES)("keeps blocking conflict context visible in %s", (locale) => {
  const conflict = {
    kind: "active_engagement",
    summary: "A separate support agreement remains active.",
    blocks_approval: true,
  };
  const blocked = { ...preview, state: "incomplete", conflicts: [conflict] };
  const rows = commercialPreviewDisplay(blocked, (key, params) =>
    translate(locale, key, params),
  );
  expect(rows.find((row) => row.value === conflict.summary)?.label).toBe(
    translate(locale, "approval.preview.conflict"),
  );
  expect(
    approvalAcceptable({ preview: { ...preview, conflicts: [conflict] } }),
  ).toBe(false);
  expect(
    approvalAcceptable({
      preview: {
        ...preview,
        conflicts: [{ ...conflict, blocks_approval: false }],
      },
    }),
  ).toBe(true);
});

it.each(LOCALES)("states the deal outcome in %s", (locale) => {
  const rows = commercialPreviewDisplay(
    {
      ...preview,
      changes: [
        {
          field: "status",
          before: { kind: "text", text: "open" },
          after: { kind: "text", text: "won" },
        },
      ],
    },
    (key, params) => translate(locale, key, params),
  );
  expect(rows[0].comparison).toEqual({
    from: translate(locale, "deals.statusOpen"),
    to: translate(locale, "deals.statusWon"),
  });
});

it.each(LOCALES)("does not promise execution-time values in %s", (locale) => {
  const rows = commercialPreviewDisplay(
    {
      ...preview,
      changes: [
        {
          field: "closed_at",
          before: { kind: "empty", text: "" },
          after: { kind: "execution", text: "" },
        },
      ],
    },
    (key, params) => translate(locale, key, params),
  );
  expect(rows[0].comparison).toEqual({
    from: translate(locale, "approval.preview.empty"),
    to: translate(locale, "approval.preview.execution"),
  });
});

it.each(LOCALES)("names the additional effects of a win in %s", (locale) => {
  const rows = commercialPreviewDisplay(
    {
      ...preview,
      effects: ["classify_correspondence", "start_project_delivery"],
    },
    (key, params) => translate(locale, key, params),
  );
  expect(
    rows.find((row) => row.field === "effect_classify_correspondence")?.value,
  ).toBe(translate(locale, "approval.preview.correspondence"));
  expect(
    rows.find((row) => row.field === "effect_start_project_delivery")?.value,
  ).toBe(translate(locale, "approval.preview.delivery"));
});

it.each(LOCALES)(
  "formats money with its own currency and locale in %s",
  (locale) => {
    for (const currency of ["EUR", "JPY", "VND"]) {
      const rows = commercialPreviewDisplay(
        {
          ...preview,
          changes: [
            {
              field: "amount_minor",
              before: { kind: "money", text: "250000", currency: "EUR" },
              after: { kind: "money", text: "300000", currency },
            },
          ],
        },
        (key, params) => translate(locale, key, params),
        locale,
      );
      expect(rows[0].comparison).toEqual({
        from: formatMoney(250000, "EUR", locale),
        to: formatMoney(300000, currency, locale),
      });
    }
  },
);

it("keeps an unsafe integer exact in its labelled minor units", () => {
  const rows = commercialPreviewDisplay(
    {
      ...preview,
      changes: [
        {
          field: "amount_minor",
          before: { kind: "empty", text: "" },
          after: {
            kind: "money",
            text: "12345678901234567890",
            currency: "EUR",
          },
        },
      ],
    },
    (key, params) => translate("en", key, params),
  );
  expect(rows[0].comparison?.to).toBe("12345678901234567890 EUR minor units");
});

it.each(LOCALES)(
  "does not infer review authority from an unknown reason in %s",
  (locale) => {
    const rows = commercialPreviewDisplay(
      { ...preview, reviewer_reason: "future_reason" },
      (key, params) => translate(locale, key, params),
    );
    expect(rows.at(-1)?.value).toBe(
      translate(locale, "approval.preview.authorityUnknown"),
    );
  },
);
it.each(LOCALES)(
  "labels a withdrawn owner name without exposing it in %s",
  (locale) => {
    const rows = commercialPreviewDisplay(
      {
        ...preview,
        changes: [
          {
            field: "owner_id",
            before: { kind: "unavailable_name", text: "" },
            after: { kind: "owner", text: "Ben" },
          },
        ],
      },
      (key, params) => translate(locale, key, params),
    );
    expect(rows[0].comparison).toEqual({
      from: translate(locale, "approval.preview.nameUnavailable"),
      to: "Ben",
    });
  },
);

it.each(LOCALES)(
  "formats preview dates with the card's record clock in %s",
  (locale) => {
    const day = "2026-09-15";
    const rows = commercialPreviewDisplay(
      {
        ...preview,
        changes: [
          {
            field: "expected_close_date",
            before: { kind: "empty", text: "" },
            after: { kind: "date", text: day },
          },
        ],
      },
      (key, params) => translate(locale, key, params),
      locale,
      (value) => formatDate(value, locale, "Europe/Berlin"),
    );
    expect(rows[0].comparison?.to).toBe(
      formatDate(day, locale, "Europe/Berlin"),
    );
  },
);

it("shows edited words and lifecycle choices without changing reference labels", () => {
  const original = {
    ...preview,
    changes: [
      ...preview.changes,
      {
        field: "lost_reason",
        before: { kind: "empty", text: "" },
        after: { kind: "text", text: "Old reason" },
      },
      {
        field: "lifecycle",
        before: { kind: "lifecycle", text: "customer" },
        after: { kind: "lifecycle", text: "former_customer" },
      },
    ],
  };
  const edited = editedCommercialPreview(original, {
    owner_id: "unresolved ID",
    lost_reason: "Corrected reason",
    proposed_lifecycle: "prospect",
  });
  expect(edited?.changes[0]).toEqual(original.changes[0]);
  expect(edited?.changes[1].after.text).toBe("Corrected reason");
  expect(edited?.changes[2].after.text).toBe("prospect");
  expect(original.changes[1].after.text).toBe("Old reason");
});

it.each(LOCALES)("describes a project close in %s", (locale) => {
  const t = (key: Parameters<typeof translate>[1]) => translate(locale, key);
  const rows = commercialPreviewDisplay(
    {
      ...preview,
      changes: [
        {
          field: "phase",
          before: { kind: "text", text: "delivering" },
          after: { kind: "text", text: "closed" },
        },
        {
          field: "closed_reason",
          before: { kind: "empty", text: "" },
          after: { kind: "text", text: "Completed" },
        },
      ],
    },
    (key, params) => translate(locale, key, params),
    locale,
  );
  expect(rows[0].label).toBe(t("project.phaseLabel"));
  expect(rows[0].comparison).toEqual({
    from: t("project.phase.delivering"),
    to: t("project.phase.closed"),
  });
  expect(rows[1].label).toBe(t("project.advance.reason"));
  expect(rows[1].comparison?.to).toBe("Completed");
});

it.each(LOCALES)("explains message authorization in %s", (locale) => {
  const rows = commercialPreviewDisplay(
    { ...preview, version: 2, effects: ["authorize_message_send"] },
    (key, params) => translate(locale, key, params),
    locale,
  );
  expect(
    rows.find((row) => row.field === "effect_authorize_message_send")?.value,
  ).toBe(translate(locale, "approval.preview.authorizeMessage"));
});

it.each(LOCALES)("explains lead retirement in %s", (locale) => {
  const rows = commercialPreviewDisplay(
    {
      ...preview,
      effects: ["archive_lead_remove_tags"],
      changes: [
        {
          field: "lead_status",
          before: { kind: "text", text: "new" },
          after: { kind: "text", text: "disqualified" },
        },
      ],
    },
    (key, params) => translate(locale, key, params),
    locale,
  );
  expect(rows.find((row) => row.field === "lead_status")?.comparison).toEqual({
    from: translate(locale, "lead.statusNew"),
    to: translate(locale, "lead.statusDisqualified"),
  });
  expect(
    rows.find((row) => row.field === "effect_archive_lead_remove_tags")?.value,
  ).toBe(translate(locale, "approval.preview.archiveLead"));
});

it.each(LOCALES)("describes website read scopes in %s", (locale) => {
  for (const [depth, key] of [
    ["page", "approval.preview.readPage"],
    ["site", "approval.preview.readSite"],
    ["technical", "approval.preview.readTechnical"],
  ] as const) {
    const rows = commercialPreviewDisplay(
      {
        ...preview,
        version: 2,
        changes: [],
        effects: ["authorize_website_read"],
        details: [
          { field: "enrichment_depth", value: { kind: "text", text: depth } },
          {
            field: "target_url",
            value: { kind: "text", text: "https://example.com" },
          },
        ],
      },
      (key, params) => translate(locale, key, params),
      locale,
    );
    expect(
      rows.find(
        (row) => row.label === translate(locale, "approval.preview.readScope"),
      )?.value,
    ).toBe(translate(locale, key));
    expect(
      rows.find((row) => row.field === "effect_authorize_website_read")?.value,
    ).toBe(translate(locale, "approval.preview.authorizeWebsite"));
  }
});

it.each(LOCALES)(
  "distinguishes a recorded meeting from an invitation in %s",
  (locale) => {
    const rows = commercialPreviewDisplay(
      {
        ...preview,
        version: 2,
        changes: [],
        effects: ["record_meeting_only"],
        details: [
          {
            field: "meeting_start",
            value: { kind: "text", text: "2027-01-12T09:00:00+07:00" },
          },
        ],
      },
      (key, params) => translate(locale, key, params),
      locale,
    );
    expect(
      rows.find(
        (row) =>
          row.label === translate(locale, "approval.preview.meetingStart"),
      )?.value,
    ).toBe("2027-01-12T09:00:00+07:00");
    expect(
      rows.find((row) => row.field === "effect_record_meeting_only")?.value,
    ).toBe(translate(locale, "approval.preview.recordMeeting"));
  },
);

it.each(LOCALES)("explains preserved booking consent in %s", (locale) => {
  const rows = commercialPreviewDisplay(
    {
      ...preview,
      version: 2,
      changes: [],
      details: [
        { field: "consent_result", value: { kind: "text", text: "withdrawn" } },
        {
          field: "consent_wording",
          value: { kind: "text", text: "Contact me about this meeting." },
        },
      ],
      effects: [
        "record_meeting_with_consent",
        "preserve_existing_consent",
        "marketing_confirmation_unavailable",
      ],
    },
    (key, params) => translate(locale, key, params),
    locale,
  );
  expect(
    rows.find(
      (row) =>
        row.label === translate(locale, "approval.preview.consentResult"),
    )?.value,
  ).toBe(translate(locale, "approval.preview.consentWithdrawn"));
  expect(
    rows.find(
      (row) =>
        row.label === translate(locale, "approval.preview.consentWording"),
    )?.value,
  ).toBe("Contact me about this meeting.");
  for (const [effect, key] of [
    ["record_meeting_with_consent", "approval.preview.recordMeetingConsent"],
    ["preserve_existing_consent", "approval.preview.preserveConsent"],
    [
      "marketing_confirmation_unavailable",
      "approval.preview.confirmationUnavailable",
    ],
  ] as const) {
    expect(rows.find((row) => row.field === `effect_${effect}`)?.value).toBe(
      translate(locale, key),
    );
  }
});

it.each(LOCALES)(
  "distinguishes invitation delivery from guest acceptance in %s",
  (locale) => {
    const rows = commercialPreviewDisplay(
      {
        ...preview,
        version: 2,
        changes: [],
        details: [
          { field: "video_call", value: { kind: "boolean", text: "false" } },
          { field: "email_reminder", value: { kind: "boolean", text: "true" } },
          {
            field: "invitation_description",
            value: { kind: "text", text: "Discuss delivery." },
          },
        ],
        effects: ["authorize_calendar_invitation"],
      },
      (key, params) => translate(locale, key, params),
      locale,
    );
    expect(
      rows.find(
        (row) => row.label === translate(locale, "scheduling.videoGeneric"),
      )?.value,
    ).toBe(translate(locale, "field.no"));
    expect(
      rows.find(
        (row) => row.label === translate(locale, "scheduling.emailReminder"),
      )?.value,
    ).toBe(translate(locale, "field.yes"));
    expect(
      rows.find(
        (row) => row.label === translate(locale, "history.field.description"),
      )?.value,
    ).toBe("Discuss delivery.");
    expect(
      rows.find((row) => row.field === "effect_authorize_calendar_invitation")
        ?.value,
    ).toBe(translate(locale, "approval.preview.authorizeInvitation"));
  },
);

it.each(LOCALES)(
  "shows a sender claim separately from automatic resolution in %s",
  (locale) => {
    const t = (key: Parameters<typeof translate>[1]) => translate(locale, key);
    const rows = commercialPreviewDisplay(
      {
        ...preview,
        version: 2,
        changes: [],
        details: [
          {
            field: "communication_context",
            value: { kind: "text", text: "requested_followup" },
          },
          {
            field: "operator_reason",
            value: { kind: "text", text: "Buyer requested a plan." },
          },
          {
            field: "contract_id",
            value: { kind: "reference", text: "Delivery agreement" },
          },
        ],
      },
      t,
      locale,
    );
    expect(
      rows.find((row) => row.label === t("approval.preview.claimedContext"))
        ?.value,
    ).toBe(t("compose.why.requestedFollowup"));
    expect(
      rows.find((row) => row.label === t("approval.preview.operatorReason"))
        ?.value,
    ).toBe("Buyer requested a plan.");
    expect(
      rows.find((row) => row.label === t("approval.preview.evidenceContract"))
        ?.value,
    ).toBe("Delivery agreement");
    const automatic = commercialPreviewDisplay(
      {
        ...preview,
        changes: [],
        details: [
          {
            field: "communication_context",
            value: { kind: "empty", text: "" },
          },
        ],
      },
      t,
      locale,
    );
    expect(automatic[0].value).toBe(t("approval.preview.contextAutomatic"));
  },
);

it.each(LOCALES)(
  "distinguishes contact archival from preservation in %s",
  (locale) => {
    const t = (key: Parameters<typeof translate>[1]) => translate(locale, key);
    const archive = commercialPreviewDisplay(
      { ...preview, version: 2, effects: ["restore_lead_archive_contact"] },
      t,
      locale,
    );
    const preserve = commercialPreviewDisplay(
      { ...preview, version: 2, effects: ["restore_lead_preserve_contact"] },
      t,
      locale,
    );
    const archiveText = archive.find(
      (row) => row.field === "effect_restore_lead_archive_contact",
    )?.value;
    const preserveText = preserve.find(
      (row) => row.field === "effect_restore_lead_preserve_contact",
    )?.value;
    expect(archiveText).toBe(t("approval.preview.demoteArchive"));
    expect(preserveText).toBe(t("approval.preview.demotePreserve"));
    expect(archiveText).not.toBe(preserveText);
  },
);

it.each(LOCALES)("explains promotion scope in %s", (locale) => {
  const rows = commercialPreviewDisplay(
    {
      version: 2,
      state: "ready",
      reviewer_reason: "record_authority",
      changes: [],
      details: [
        {
          field: "promotion_trigger",
          value: { kind: "text", text: "human_qualify" },
        },
        {
          field: "promotion_contact_name",
          value: { kind: "text", text: "Dana Buyer" },
        },
        {
          field: "promotion_deal_amount",
          value: { kind: "money", text: "250000", currency: "USD" },
        },
        {
          field: "promotion_restriction",
          value: { kind: "text", text: "marketing_objection" },
        },
        {
          field: "promotion_restriction_purpose",
          value: { kind: "empty", text: "" },
        },
      ],
      effects: ["promote_create_contact", "promote_open_deal_seat_contact"],
    },
    (key, params) => translate(locale, key, params),
    locale,
  );
  expect(
    rows.find(
      (row) =>
        row.label === translate(locale, "approval.preview.promotionTrigger"),
    )?.value,
  ).toBe(translate(locale, "lead.trigger.humanQualify"));
  expect(
    rows.find(
      (row) =>
        row.label ===
        translate(locale, "approval.preview.promotionContactName"),
    )?.value,
  ).toBe("Dana Buyer");
  expect(
    rows.find(
      (row) =>
        row.label ===
        translate(locale, "approval.preview.promotionRestriction"),
    )?.value,
  ).toBe(translate(locale, "sendPermission.reason.objected"));
  expect(
    rows.find(
      (row) =>
        row.label ===
        translate(locale, "approval.preview.promotionRestrictionPurpose"),
    )?.value,
  ).toBe(translate(locale, "approval.preview.promotionAllMarketing"));
  expect(
    rows.find((row) => row.field === "effect_promote_open_deal_seat_contact")
      ?.value,
  ).toBe(translate(locale, "approval.preview.promotionDeal"));
  expect(rows.some((row) => row.value === "promote_create_contact")).toBe(
    false,
  );
});

it.each(LOCALES)("shows each carried link and basis in %s", (locale) => {
  const issued = "2026-09-15T08:00:00Z";
  const rows = commercialPreviewDisplay(
    {
      version: 2,
      state: "ready",
      reviewer_reason: "record_authority",
      changes: [],
      details: [
        {
          field: "promotion_withdrawal_address",
          value: { kind: "text", text: "buyer@example.test" },
        },
        {
          field: "promotion_link_issued",
          value: { kind: "date", text: issued },
        },
        {
          field: "promotion_record_revoked",
          value: { kind: "boolean", text: "true" },
        },
        {
          field: "promotion_basis_kind",
          value: { kind: "text", text: "subject_initiated_correspondence" },
        },
      ],
    },
    (key, params) => translate(locale, key, params),
    locale,
    (value) => formatDate(value, locale, "Europe/Berlin"),
  );
  expect(
    rows.find(
      (row) =>
        row.label === translate(locale, "approval.preview.withdrawalAddress"),
    )?.value,
  ).toBe("buyer@example.test");
  expect(
    rows.find(
      (row) => row.label === translate(locale, "approval.preview.linkIssued"),
    )?.value,
  ).toBe(formatDate(issued, locale, "Europe/Berlin"));
  expect(
    rows.find(
      (row) =>
        row.label === translate(locale, "approval.preview.recordRevoked"),
    )?.value,
  ).toBe(translate(locale, "field.yes"));
  expect(
    rows.find(
      (row) => row.label === translate(locale, "approval.preview.basisKind"),
    )?.value,
  ).toBe(translate(locale, "approval.preview.basisCorrespondence"));
});

it.each(LOCALES)(
  "explains relink participants and retention in %s",
  (locale) => {
    const rows = commercialPreviewDisplay(
      {
        version: 2,
        state: "ready",
        reviewer_reason: "record_authority",
        changes: [],
        details: [
          {
            field: "relink_destination",
            value: { kind: "text", text: "Delivery project" },
          },
          {
            field: "relink_activity",
            value: { kind: "text", text: "Buyer discussion" },
          },
          {
            field: "relink_participant_move",
            value: { kind: "text", text: "Original buyer" },
          },
          {
            field: "relink_participant_role",
            value: { kind: "text", text: "bcc" },
          },
        ],
        effects: ["relink_preserve_other_links", "relink_project_retention"],
      },
      (key, params) => translate(locale, key, params),
      locale,
    );
    expect(
      rows.find(
        (row) =>
          row.label === translate(locale, "approval.preview.relinkDestination"),
      )?.value,
    ).toBe("Delivery project");
    expect(
      rows.find(
        (row) => row.label === translate(locale, "approval.preview.relinkRole"),
      )?.value,
    ).toBe(translate(locale, "compose.bcc"));
    expect(
      rows.find((row) => row.field === "effect_relink_project_retention")
        ?.value,
    ).toBe(translate(locale, "approval.preview.relinkRetention"));
    expect(
      rows.find(
        (row) =>
          row.label ===
          translate(locale, "approval.preview.relinkParticipantMove"),
      )?.value,
    ).toBe("Original buyer");
  },
);

it.each(LOCALES)("labels company creation details in %s", (locale) => {
  const preview: Preview = {
    version: 2,
    state: "ready",
    reviewer_reason: "record_authority",
    changes: [],
    details: [
      {
        field: "creation_primary_domain",
        value: { kind: "text", text: "buyer.example.test" },
      },
      { field: "address_city", value: { kind: "text", text: "Berlin" } },
    ],
    effects: [
      "create_company_record",
      "create_workspace_visible",
      "geocode_if_enabled",
    ],
  };
  const rows = commercialPreviewDisplay(
    preview,
    (key, params) => translate(locale, key, params),
    locale,
  );
  expect(rows.find((row) => row.value === "buyer.example.test")?.label).toBe(
    translate(locale, "approval.preview.primaryDomain"),
  );
  expect(rows.find((row) => row.value === "Berlin")?.label).toBe(
    translate(locale, "create.city"),
  );
  expect(JSON.stringify(rows)).toContain(
    translate(locale, "approval.preview.geocodeConditional"),
  );
});

it.each(LOCALES)("labels contact source context in %s", (locale) => {
  const preview: Preview = {
    version: 2,
    state: "ready",
    reviewer_reason: "record_authority",
    changes: [],
    details: [
      {
        field: "creation_acquisition",
        value: { kind: "text", text: "unknown_legacy" },
      },
      { field: "creation_email_type", value: { kind: "text", text: "work" } },
      { field: "creation_phone_type", value: { kind: "text", text: "mobile" } },
      {
        field: "creation_source_consent",
        value: { kind: "text", text: "withdrawn" },
      },
    ],
    effects: ["create_contact_record", "record_contact_acquisition"],
  };
  const rows = commercialPreviewDisplay(
    preview,
    (key, params) => translate(locale, key, params),
    locale,
  );
  for (const key of [
    "notice.acq.unknownLegacy",
    "field.emailWork",
    "field.phoneMobile",
    "approval.preview.consentWithdrawn",
  ] as const) {
    expect(JSON.stringify(rows)).toContain(translate(locale, key));
  }
});

it.each(LOCALES)("describes offer creation and full terms in %s", (locale) => {
  const terms = "First paragraph.\n\n" + "Complete terms. ".repeat(40);
  const rows = commercialPreviewDisplay(
    {
      ...preview,
      version: 2,
      changes: [],
      details: [
        { field: "offer_status", value: { kind: "text", text: "draft" } },
        { field: "terms_text", value: { kind: "text", text: terms } },
        {
          field: "arr_minor",
          value: { kind: "money", text: "80000", currency: "EUR" },
        },
        { field: "offer_line_position", value: { kind: "number", text: "1" } },
      ],
      effects: ["create_offer_draft", "assign_offer_number"],
    },
    (key, params) => translate(locale, key, params),
    locale,
  );
  expect(
    rows.find((row) => row.label === translate(locale, "offer.termsText"))
      ?.value,
  ).toBe(terms);
  expect(
    rows.find((row) => row.label === translate(locale, "history.field.status"))
      ?.value,
  ).toBe(translate(locale, "commercial.offer.draft"));
  expect(
    rows.find((row) => row.label === translate(locale, "offer.arr"))?.value,
  ).toBe(formatMoney(80000, "EUR", locale));
  expect(
    rows.find((row) => row.label === translate(locale, "offer.position"))
      ?.value,
  ).toBe("1");
  expect(
    rows.some(
      (row) => row.value === translate(locale, "approval.preview.createOffer"),
    ),
  ).toBe(true);
  expect(
    rows.some(
      (row) =>
        row.value === translate(locale, "approval.preview.assignOfferNumber"),
    ),
  ).toBe(true);
});

it("shows an extension argument named like a markup detail as its text", () => {
  const markup = "<img src=x onerror=alert(1)>";
  const rows = commercialPreviewDisplay(
    {
      ...preview,
      version: 2,
      changes: [],
      details: [
        { field: "argument:html_body", value: { kind: "text", text: markup } },
      ],
    },
    (key, params) => translate("en", key, params),
  );
  const row = rows.find((candidate) => candidate.label === "html_body");
  expect(row?.value).toBe(markup);
  expect(row?.href).toBeUndefined();
});
