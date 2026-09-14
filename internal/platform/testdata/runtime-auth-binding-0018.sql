GRANT USAGE ON SCHEMA identity TO :"runtime_role";
GRANT SELECT(newapi_user_id,security_epoch,security_epoch_changed_at) ON identity.account_refs TO :"runtime_role";
GRANT SELECT ON identity.native_session_bindings TO :"runtime_role";
GRANT INSERT(session_id_hash,newapi_user_id,session_version,native_auth_version,security_epoch_snapshot,native_created_at) ON identity.native_session_bindings TO :"runtime_role";
GRANT UPDATE(session_version,native_auth_version) ON identity.native_session_bindings TO :"runtime_role";
