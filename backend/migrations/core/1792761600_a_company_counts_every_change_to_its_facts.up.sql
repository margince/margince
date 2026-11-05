-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- One counter per company, moved by every insert, update and delete of its
-- facts. An Undo of a technical lookup compares it with the value the lookup
-- recorded, so a fact added and removed again after the lookup still counts
-- as later work, whatever clock its writer ran on.
--
-- A table of its own rather than a company column: a company row write bumps
-- company.version, and a background fact write must not refuse a person's
-- open company edit.
SET LOCAL lock_timeout = '3s';

CREATE TABLE company_fact_revision (
    company_id uuid NOT NULL,
    revision bigint DEFAULT 0 NOT NULL,

    CONSTRAINT company_fact_revision_pkey PRIMARY KEY (company_id),
    CONSTRAINT company_fact_revision_js_safe CHECK (revision BETWEEN 0 AND 9007199254740991),
    CONSTRAINT company_fact_revision_company_id_fkey FOREIGN KEY (company_id)
        REFERENCES company(id) ON DELETE CASCADE
);

CREATE FUNCTION company_fact_count_revision() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
  touched uuid[];
BEGIN
  IF TG_OP = 'INSERT' THEN
    touched := ARRAY[NEW.company_id];
  ELSIF TG_OP = 'DELETE' THEN
    touched := ARRAY[OLD.company_id];
  ELSE
    touched := ARRAY[OLD.company_id, NEW.company_id];
  END IF;
  -- A company being deleted takes its facts with it; it has no counter left
  -- to move.
  INSERT INTO company_fact_revision AS r (company_id, revision)
  SELECT c.id, 1 FROM company c WHERE c.id = ANY(touched)
  ON CONFLICT (company_id) DO UPDATE SET revision = r.revision + 1;
  RETURN NULL;
END;
$$;

CREATE TRIGGER trg_company_fact_count_revision
    AFTER INSERT OR UPDATE OR DELETE ON company_fact
    FOR EACH ROW EXECUTE FUNCTION company_fact_count_revision();
