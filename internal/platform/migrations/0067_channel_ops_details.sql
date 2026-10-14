ALTER TABLE relay.channels
 ADD COLUMN remark text NOT NULL DEFAULT '',
 ADD COLUMN test_model text NOT NULL DEFAULT '',
 ADD COLUMN test_at timestamptz,
 ADD COLUMN response_ms bigint NOT NULL DEFAULT 0;
