-- Retain historical jobs/publications; operators no longer require a Master wallet.
ALTER TABLE rankings.rebuild_jobs DROP CONSTRAINT rebuild_jobs_actor_user_id_fkey;
ALTER TABLE rankings.rebuild_jobs ADD CONSTRAINT rebuild_jobs_actor_user_id_fkey
 FOREIGN KEY(actor_user_id) REFERENCES identity.users(id) ON DELETE RESTRICT;
ALTER TABLE rankings.publication_events DROP CONSTRAINT publication_events_actor_user_id_fkey;
ALTER TABLE rankings.publication_events ADD CONSTRAINT publication_events_actor_user_id_fkey
 FOREIGN KEY(actor_user_id) REFERENCES identity.users(id) ON DELETE RESTRICT;
