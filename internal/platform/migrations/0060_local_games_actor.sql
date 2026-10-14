-- Configuration authors are local users; no legacy Master is required.
ALTER TABLE games.game_config_versions DROP CONSTRAINT game_config_versions_created_by_fkey;
ALTER TABLE games.game_config_versions ADD CONSTRAINT game_config_versions_created_by_fkey
 FOREIGN KEY(created_by) REFERENCES identity.users(id) ON DELETE RESTRICT;
