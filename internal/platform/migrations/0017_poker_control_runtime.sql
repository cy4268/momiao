-- Forward-only control fencing, audit and restart isolation. No wallet changes.
-- The frozen table-name bound is 40 extended grapheme clusters, not 128 bytes.
-- UAX29 is enforced in the typed service; PG retains a nonempty text invariant.
ALTER TABLE poker.tables DROP CONSTRAINT tables_name_check;
ALTER TABLE poker.tables ADD CONSTRAINT tables_name_check CHECK(char_length(name)>0);
CREATE TABLE poker.audit_events (
 event_id uuid PRIMARY KEY,
 table_id uuid NOT NULL REFERENCES poker.tables,
 session_id uuid REFERENCES poker.sessions,
 actor_user_id bigint REFERENCES identity.account_refs,
 actor_kind text NOT NULL CHECK(actor_kind IN('USER','SYSTEM','ADMIN')),
 event_type text NOT NULL,
 request_hash bytea NOT NULL CHECK(octet_length(request_hash)=32),
 details jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX poker_audit_table_time ON poker.audit_events(table_id,created_at,event_id);
CREATE TRIGGER poker_audit_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON poker.audit_events
 FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
ALTER TABLE poker.recovery_state DROP CONSTRAINT recovery_state_state_check;
ALTER TABLE poker.recovery_state ADD CONSTRAINT recovery_state_state_check
 CHECK(state IN('NORMAL','RECOVERING','GRACE','RESUMED','NEEDS_REVIEW'));
ALTER TABLE poker.recovery_state ALTER COLUMN started_at DROP NOT NULL;
ALTER TABLE poker.recovery_state ALTER COLUMN grace_until DROP NOT NULL;
ALTER TABLE poker.recovery_state ADD COLUMN previous_table_state text;
ALTER TABLE poker.recovery_state ADD COLUMN reason_code text;
ALTER TABLE poker.recovery_state ADD COLUMN detected_at timestamptz;
UPDATE poker.recovery_state SET reason_code='LEGACY_CONTEXT_NOT_RECORDED';
CREATE INDEX poker_session_owner_started ON poker.sessions(newapi_user_id,started_at,session_id);
CREATE INDEX poker_participant_session ON poker.hand_participants(session_id,hand_id);
REVOKE ALL ON poker.audit_events FROM PUBLIC;
