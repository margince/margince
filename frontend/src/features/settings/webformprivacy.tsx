// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Field, Textarea, TextInput } from "../../design-system/atoms";
import { useT } from "../../i18n";

// The notice the form shows under it at the point of collection, and the
// policy it links to. Both are optional: an enquiry needs a notice, not consent.
export function PrivacyNoticeFields({
  notice,
  policy,
  errors,
  onChange,
}: Readonly<{
  notice: string;
  policy: string;
  errors: ReadonlyMap<string, string>;
  onChange: (patch: {
    privacyNotice?: string;
    privacyNoticeUrl?: string;
  }) => void;
}>) {
  const t = useT();
  return (
    <div className="form-stack webform-part">
      <Field
        label={t("webForms.field.privacyNotice")}
        hint={t("webForms.field.privacyNoticeHint")}
        error={errors.get("privacy_notice")}
      >
        {(field) => (
          <Textarea
            {...field}
            rows={2}
            value={notice}
            onChange={(event) =>
              onChange({ privacyNotice: event.target.value })
            }
          />
        )}
      </Field>
      <Field
        label={t("webForms.field.privacyNoticeUrl")}
        hint={t("webForms.field.privacyNoticeUrlHint")}
        error={errors.get("privacy_notice_url")}
      >
        {(field) => (
          <TextInput
            {...field}
            type="url"
            value={policy}
            placeholder={t("webForms.field.privacyNoticeUrlPlaceholder")}
            onChange={(event) =>
              onChange({ privacyNoticeUrl: event.target.value })
            }
          />
        )}
      </Field>
    </div>
  );
}
