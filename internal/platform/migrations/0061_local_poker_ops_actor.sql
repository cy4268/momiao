-- Hosts and administrators use local identity; preserve existing moderation history.
ALTER TABLE poker.chat_moderation_events DROP CONSTRAINT chat_moderation_events_actor_user_id_fkey;
ALTER TABLE poker.chat_moderation_events ADD CONSTRAINT chat_moderation_events_actor_user_id_fkey
 FOREIGN KEY(actor_user_id) REFERENCES identity.users(id);
