SET LOCAL lock_timeout = '3s';

-- Two things a contract could not hold before: how long the customer has to
-- pay, and whatever else this installation needs to record about one.

-- Net terms in days. An integer rather than a picklist because the number is
-- the fact — Net 14, 30 and 60 are shortcuts a form offers, not the vocabulary.
-- Zero is a real value and means due on receipt, which is why the CHECK admits
-- it and the column stays nullable for "not specified": a contract nobody has
-- recorded terms for is not a contract due immediately.
ALTER TABLE contract ADD COLUMN payment_term_days integer;

ALTER TABLE contract
    ADD CONSTRAINT contract_payment_term_days_nonnegative
    CHECK (payment_term_days IS NULL OR payment_term_days >= 0);

-- Contract joins the custom-field targets.
--
-- The CHECK is widened and NOTHING else: datasource.EntityType stays as it is.
-- Declaring a member there obliges native provider routing, agent record-shape
-- generation and the embedding and provenance consumers that enumerate it, and
-- a contract needs none of that to carry a typed extra field. The bounded
-- vocabulary in shared/ports/fieldcatalog is what this constraint mirrors.
--
-- field_provenance is deliberately NOT widened here. Provenance records where a
-- value CAME FROM — a connector, an import, a crawl — and nothing writes
-- contract values from any of those. Widening it would declare a capability no
-- writer has, and the slice that adds one can widen it then.
ALTER TABLE custom_field DROP CONSTRAINT custom_field_object_check;
ALTER TABLE custom_field
    ADD CONSTRAINT custom_field_object_check
    CHECK (object = ANY (ARRAY[
        'contact', 'company', 'deal', 'lead',
        'activity', 'project', 'relationship', 'partner',
        'contract'
    ]::text[]));
