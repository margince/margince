SET LOCAL lock_timeout = '3s';
CREATE TABLE report_projection_fence (
 singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
 generation bigint NOT NULL DEFAULT 0
);
INSERT INTO report_projection_fence(singleton) VALUES(true);
ALTER TABLE report_edition ADD COLUMN expired_at timestamptz;
