-- Native history has no local request ID. Retry attempts share one logical ID.
ALTER TABLE relay.logs ADD COLUMN request_id text;
ALTER TABLE relay.logs ADD COLUMN attempt integer NOT NULL DEFAULT 1;
CREATE INDEX logs_request ON relay.logs(request_id) WHERE request_id IS NOT NULL;
