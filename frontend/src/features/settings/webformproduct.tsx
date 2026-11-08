// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  searchProductCandidates,
  useProductName,
} from "../../components/productsearch";
import { Button, Field } from "../../design-system/atoms";
import { RecordPicker } from "../../design-system/recordpicker";
import { useT } from "../../i18n";

/**
 * The product every lead a form captures is interested in. Product reports
 * count those leads under it. Empty captures leads with no product.
 */
export function ProductField({
  productId,
  error,
  onChange,
}: Readonly<{
  productId: string;
  error?: string;
  onChange: (productId: string) => void;
}>) {
  const t = useT();
  const name = useProductName(productId || null);
  const selected = productId
    ? { id: productId, name: name.data ?? t("webForms.field.productLoading") }
    : null;
  return (
    <div className="form-stack webform-part">
      <Field
        label={t("webForms.field.product")}
        hint={t("webForms.field.productHint")}
        error={error}
      >
        {(field) => (
          <RecordPicker
            id={field.id}
            aria-describedby={field["aria-describedby"]}
            aria-invalid={field["aria-invalid"]}
            searchTargets={searchProductCandidates}
            selected={selected}
            onPick={(candidate) => onChange(candidate.id)}
          />
        )}
      </Field>
      {productId && (
        <Button variant="link" onClick={() => onChange("")}>
          {t("webForms.field.productClear")}
        </Button>
      )}
    </div>
  );
}
