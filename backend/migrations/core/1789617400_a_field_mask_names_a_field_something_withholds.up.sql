-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Bounded: the ALTER below takes ACCESS EXCLUSIVE on a table this migration did
-- not create, and a writer holding a row lock would otherwise queue every
-- reader behind it. Two seconds is the tree's own number.
SET LOCAL lock_timeout = '2s';

-- A configured mask that withholds nothing is worse than no mask at all.
--
-- deals/fieldmask.go says the shape out loud: a mask naming a column it has no
-- withhold func for "is inert", and applyTo "drops rather than reports" it. The
-- deal is also the only object any reader masks — the helper in platform/auth
-- is generic, and nothing else calls it. So a row saying
-- ('rep', 'contact', 'phone', 'always') is accepted, stored, loaded onto every
-- principal holding the role, and then withholds nothing. An administrator
-- reads the configuration back unchanged and believes a number is hidden while
-- it is printed on every screen that shows it.
--
-- Silence is the one direction a guardrail must not fail in, so the refusal
-- moves to where the configuration is WRITTEN. There is no API writer for
-- field_mask; it is written by migration or by an operator's SQL, and both
-- reach the database. Refusing in loadFieldMasks instead would take out every
-- seat holding the role at sign-in, which is a much larger blast radius for a
-- mistake that is a typo.
CREATE TABLE maskable_field (
    object text NOT NULL,
    field text NOT NULL,
    CONSTRAINT maskable_field_pkey PRIMARY KEY (object, field)
);

-- SELECT only, like currency_minor_digits: this is what the BUILD can withhold,
-- not an installation's configuration, and it changes when the enforcing code
-- does. The application role having no DELETE is also what makes the workspace
-- reset's sweep fail loudly rather than quietly empty the catalog.
-- The default privileges in this schema grant the application role every verb
-- on a new table, so SELECT-only is a REVOKE and not merely a narrow GRANT —
-- exactly as currency_minor_digits had to do it.
REVOKE INSERT, UPDATE, DELETE ON TABLE maskable_field FROM margince_app;
GRANT SELECT ON TABLE maskable_field TO margince_app;

-- Exactly the keys of deals.dealMaskableFields, which is the code that does the
-- withholding. The pairing is held in both directions by the fixture in
-- migrations/testdata/maskable_fields.txt: a pair listed here and enforced
-- nowhere fails, and so does a pair the code withholds and this table does not
-- offer.
INSERT INTO maskable_field (object, field) VALUES
    ('deal', 'amount_minor'),
    ('deal', 'expected_arr_minor'),
    ('deal', 'currency'),
    ('deal', 'company_id'),
    ('deal', 'project_id'),
    ('deal', 'partner_company_id');

-- Rows naming a pair nothing withholds are removed rather than carried past the
-- constraint. They withheld nothing on the day they were written, so nothing a
-- reader could see changes here; what changes is that the configuration stops
-- claiming otherwise. Carrying them behind a NOT VALID constraint would leave
-- exactly the false belief this migration exists to end.
DELETE FROM field_mask fm
WHERE NOT EXISTS (
    SELECT 1 FROM maskable_field mf
    WHERE mf.object = fm.object AND mf.field = fm.field
);

ALTER TABLE field_mask
    ADD CONSTRAINT field_mask_maskable_fkey
    FOREIGN KEY (object, field) REFERENCES maskable_field (object, field);
