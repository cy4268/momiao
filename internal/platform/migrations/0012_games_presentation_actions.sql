-- Durable action identities for the Scratch presentation gate. Other games'
-- financial actions are introduced by their own forward integration migration.
CREATE TABLE games.round_actions (
 action_id UUID PRIMARY KEY,
 round_id UUID NOT NULL REFERENCES games.game_rounds,
 newapi_user_id BIGINT NOT NULL REFERENCES identity.account_refs,
 action_sequence BIGINT NOT NULL CHECK (action_sequence>0),
 action_type TEXT NOT NULL CHECK (action_type='SCRATCH_REVEAL_COMPLETE'),
 additional_stake_units BIGINT NOT NULL DEFAULT 0 CHECK (additional_stake_units=0),
 system_action BOOLEAN NOT NULL DEFAULT FALSE CHECK (NOT system_action),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(round_id,action_sequence)
);
CREATE TRIGGER immutable_history BEFORE UPDATE OR DELETE OR TRUNCATE ON games.round_actions FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
