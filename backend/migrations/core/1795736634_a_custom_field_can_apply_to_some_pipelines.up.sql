-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A deal field may apply to some pipelines only, so a field that matters to a
-- partner pipeline does not clutter the sales pipeline. NULL means every
-- pipeline, which is what every field that exists applies to today. Only a deal
-- field has a pipeline to narrow by. An array cannot carry a foreign key, so the
-- engine checks each id against the live pipelines when it writes the column,
-- and a pipeline archived later matches nothing. NOT VALID skips the row scan;
-- every row is NULL.
SET LOCAL lock_timeout = '3s';

ALTER TABLE custom_field ADD COLUMN pipeline_ids uuid[];
ALTER TABLE custom_field ADD CONSTRAINT custom_field_pipeline_ids_check
    CHECK (pipeline_ids IS NULL OR (object = 'deal' AND cardinality(pipeline_ids) > 0)) NOT VALID;
