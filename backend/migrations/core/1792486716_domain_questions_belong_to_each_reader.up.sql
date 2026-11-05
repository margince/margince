SET LOCAL lock_timeout = '3s';

CREATE TABLE company_domain_question_reader (
 domain text NOT NULL REFERENCES company_domain_disposition(domain) ON DELETE CASCADE,
 reader_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
 evidence_at timestamptz,
 PRIMARY KEY (domain, reader_id)
);
CREATE INDEX company_domain_question_reader_reader_idx ON company_domain_question_reader(reader_id, domain);
INSERT INTO company_domain_question_reader (domain, reader_id, evidence_at)
 SELECT domain, owner_id, last_evidence_at FROM company_domain_disposition WHERE owner_id IS NOT NULL AND status = 'pending';
