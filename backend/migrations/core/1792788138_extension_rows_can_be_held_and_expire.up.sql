-- An extension row can be held and can expire.
--
-- `extension_legal_hold` is the hold on rows of tables an extension owns. Those
-- tables carry no core column, so the hold lives here, keyed by table and row
-- id. The reason rides the audit row, as it does for the five core tables.
--
-- `extension_expired_row` marks a row whose personal columns the sweep cleared.
-- The row stays, so without the mark the next pass would select it again.
--
-- `expire` is the retention action for those rows: delete the row, or clear its
-- personal columns when the table declares any. The widened CHECK is added NOT
-- VALID and validated in the next migration.
SET LOCAL lock_timeout = '3s';

CREATE TABLE extension_legal_hold (
    table_name text NOT NULL,
    row_id uuid NOT NULL,
    placed_at timestamptz DEFAULT now() NOT NULL,
    PRIMARY KEY (table_name, row_id)
);

CREATE TABLE extension_expired_row (
    table_name text NOT NULL,
    row_id uuid NOT NULL,
    expired_at timestamptz DEFAULT now() NOT NULL,
    PRIMARY KEY (table_name, row_id)
);

ALTER TABLE retention_policy DROP CONSTRAINT retention_policy_action_check;

ALTER TABLE retention_policy ADD CONSTRAINT retention_policy_action_check
    CHECK (action IN ('archive', 'anonymize', 'erase', 'expire')) NOT VALID;
