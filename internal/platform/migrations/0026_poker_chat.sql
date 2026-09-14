-- Durable, table-local Poker chat. Runtime grants are applied separately by
-- the deployment owner; this forward migration never widens PUBLIC access.
ALTER TABLE poker.tables ADD COLUMN chat_sequence bigint NOT NULL DEFAULT 0
 CHECK(chat_sequence>=0);

-- Public projections use member_id. newapi_user_id is retained only for
-- authorization, rate limits and owner-only moderation targets.
CREATE TABLE poker.chat_members (
 table_id uuid NOT NULL REFERENCES poker.tables(table_id),
 newapi_user_id bigint NOT NULL REFERENCES identity.account_refs(newapi_user_id),
 member_id uuid NOT NULL,
 display_name_snapshot text NOT NULL CHECK(octet_length(display_name_snapshot) BETWEEN 1 AND 4096),
 avatar_id_snapshot text NOT NULL CHECK(octet_length(avatar_id_snapshot) BETWEEN 1 AND 512),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(table_id,newapi_user_id),
 UNIQUE(table_id,member_id)
);

CREATE TABLE poker.chat_messages (
 table_id uuid NOT NULL REFERENCES poker.tables(table_id),
 message_sequence bigint NOT NULL CHECK(message_sequence>0),
 message_id uuid NOT NULL,
 sender_member_id uuid,
 kind text NOT NULL CHECK(kind IN('USER_TEXT','SYSTEM')),
 body text NOT NULL CHECK(octet_length(body) BETWEEN 1 AND 2048),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(table_id,message_sequence),
 UNIQUE(table_id,message_id),
 FOREIGN KEY(table_id,sender_member_id) REFERENCES poker.chat_members(table_id,member_id),
 CHECK((kind='USER_TEXT')=(sender_member_id IS NOT NULL))
);
CREATE INDEX poker_chat_sender_time
 ON poker.chat_messages(table_id,sender_member_id,created_at DESC);

CREATE TABLE poker.chat_mutes (
 table_id uuid NOT NULL REFERENCES poker.tables(table_id),
 target_newapi_user_id bigint NOT NULL REFERENCES identity.account_refs(newapi_user_id),
 muted_by_newapi_user_id bigint NOT NULL REFERENCES identity.account_refs(newapi_user_id),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(table_id,target_newapi_user_id)
);

CREATE TRIGGER poker_chat_members_immutable BEFORE UPDATE OR DELETE OR TRUNCATE
 ON poker.chat_members FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE TRIGGER poker_chat_messages_immutable BEFORE UPDATE OR DELETE OR TRUNCATE
 ON poker.chat_messages FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
CREATE TRIGGER poker_chat_mutes_immutable BEFORE UPDATE OR DELETE OR TRUNCATE
 ON poker.chat_mutes FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();

REVOKE ALL ON poker.chat_members,poker.chat_messages,poker.chat_mutes FROM PUBLIC;
