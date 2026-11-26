-- A delivery's body and markup are the text that went on the wire: the author's
-- words, then the sender's sign-off, then the legal disclosures and the
-- unsubscribe footer. Sending the same mail again has to start from the
-- author's words alone, because the send adds those three parts again.
--
-- The three parts only ever come after the author's text, so the delivery
-- records how many characters at the front are the author's. NULL is a delivery sent
-- before this column existed: its author's text cannot be told apart from what
-- followed it, and it is not offered for a second send.
--
-- `send_claims` keeps what the send stated about why it was sent: its
-- communication context, marketing purpose, operator reason, evidence, sending
-- legal entity and the thread it answered. A send that repeats this one asks the
-- consent engine the same question. It is NULL on a delivery sent before the
-- column existed, which is not offered a second send either.
--
-- `resent_from` names the bounced delivery a send repeats. A bounce can be sent
-- again once, and `resent_from` is what says it was.
SET LOCAL lock_timeout = '3s';

ALTER TABLE comms_outbound
    ADD COLUMN IF NOT EXISTS authored_body_len integer,
    ADD COLUMN IF NOT EXISTS authored_html_len integer,
    ADD COLUMN IF NOT EXISTS send_claims jsonb,
    ADD COLUMN IF NOT EXISTS resent_from uuid;

ALTER TABLE comms_outbound
    ADD CONSTRAINT comms_outbound_resent_from_fkey
    FOREIGN KEY (resent_from) REFERENCES comms_outbound (id) ON DELETE SET NULL NOT VALID;
