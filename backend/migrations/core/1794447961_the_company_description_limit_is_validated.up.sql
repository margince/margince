SET LOCAL lock_timeout = '2s';
ALTER TABLE company VALIDATE CONSTRAINT company_description_length;
