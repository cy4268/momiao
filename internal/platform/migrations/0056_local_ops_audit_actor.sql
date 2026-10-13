-- Local administrators do not need a platform Master/account_ref to edit Ops.
-- Keep the audit rows; bind the actor to the actual local identity authority.
ALTER TABLE ops.audit_log DROP CONSTRAINT audit_log_actor_user_id_fkey;
ALTER TABLE ops.audit_log ADD CONSTRAINT audit_log_actor_user_id_fkey
 FOREIGN KEY(actor_user_id) REFERENCES identity.users(id);
