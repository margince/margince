SET LOCAL lock_timeout = '3s';

ALTER TABLE raw_capture VALIDATE CONSTRAINT raw_capture_payload_object_shape;
