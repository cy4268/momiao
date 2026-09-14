-- Keep the established Ops permission vocabulary while making the durable
-- operation row shape agree with the application permission catalog.
ALTER TABLE ops.admin_operations
 DROP CONSTRAINT admin_operation_typed_shape,
 ADD CONSTRAINT admin_operation_typed_shape CHECK(operation_type IS NULL OR (
  actor_kind='ADMIN' AND newapi_user_id IS NOT NULL AND newapi_user_id>0 AND
  operation_type ~ '^[A-Z][A-Z0-9_]{0,127}$' AND action=operation_type AND
  actor_role_snapshot IN ('SUPER_ADMIN','OPERATOR','AUDITOR') AND
  actor_scopes_snapshot IS NOT NULL AND actor_authz_epoch_snapshot IS NOT NULL AND
  actor_authz_epoch_snapshot>0 AND required_permission IS NOT NULL AND
  required_permission ~ '^[a-z][a-z0-9_.-]{0,127}$' AND
  risk_level IN ('LEVEL_1_ROUTINE','LEVEL_2_IMPACTFUL','LEVEL_3_CRITICAL') AND
  target_type IS NOT NULL AND octet_length(target_type) BETWEEN 1 AND 128 AND
  target_id IS NOT NULL AND octet_length(target_id) BETWEEN 1 AND 512 AND
  target_version_snapshot IS NOT NULL AND octet_length(target_version_snapshot) BETWEEN 1 AND 128 AND
  input_schema_version IS NOT NULL AND input_payload IS NOT NULL AND jsonb_typeof(input_payload)='object' AND
  input_hash IS NOT NULL AND octet_length(input_hash)=32 AND
  environment IN ('DEVELOPMENT','STAGING','PRODUCTION') AND
  impact_schema_version IS NOT NULL AND impact_preview IS NOT NULL AND jsonb_typeof(impact_preview)='object' AND
  impact_hash IS NOT NULL AND octet_length(impact_hash)=32 AND
  confirmation_mode IN ('NONE','EXPLICIT','TYPED') AND requires_fresh_auth IS NOT NULL AND
  (risk_level<>'LEVEL_1_ROUTINE' OR requires_fresh_auth=false) AND
  (risk_level<>'LEVEL_3_CRITICAL' OR requires_fresh_auth=true)));
