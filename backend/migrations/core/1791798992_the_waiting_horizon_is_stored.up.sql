-- The waiting horizon a worker pass measured, so a read answers it instead of
-- measuring a year of inbound mail on the request path.
--
-- One row for the installation, like activity_own_sent_mail_repair_cutoff: no
-- workspace column and no policy. No row means nothing was measured yet, and
-- readers use the compiled horizon.
SET LOCAL lock_timeout = '3s';

CREATE TABLE activity_waiting_horizon (
    singleton boolean DEFAULT true NOT NULL,
    horizon_days integer NOT NULL,
    measured_at timestamptz NOT NULL,
    CONSTRAINT activity_waiting_horizon_pkey PRIMARY KEY (singleton),
    CONSTRAINT activity_waiting_horizon_singleton_check CHECK (singleton),
    CONSTRAINT activity_waiting_horizon_days_check CHECK (horizon_days > 0)
);
