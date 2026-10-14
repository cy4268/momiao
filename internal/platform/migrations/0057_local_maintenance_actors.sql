-- Maintenance actor fields use local users too; existing rows are retained.
ALTER TABLE ops.maintenance_windows
 DROP CONSTRAINT maintenance_windows_created_by_fkey,
 DROP CONSTRAINT maintenance_windows_activated_by_fkey,
 DROP CONSTRAINT maintenance_windows_ended_by_fkey;
ALTER TABLE ops.maintenance_windows
 ADD CONSTRAINT maintenance_windows_created_by_fkey FOREIGN KEY(created_by) REFERENCES identity.users(id) ON DELETE RESTRICT,
 ADD CONSTRAINT maintenance_windows_activated_by_fkey FOREIGN KEY(activated_by) REFERENCES identity.users(id) ON DELETE RESTRICT,
 ADD CONSTRAINT maintenance_windows_ended_by_fkey FOREIGN KEY(ended_by) REFERENCES identity.users(id) ON DELETE RESTRICT;
