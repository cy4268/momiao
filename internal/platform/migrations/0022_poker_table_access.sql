-- Dormant credential storage; this migration does not enable PASSWORD entry.
CREATE TABLE poker.table_access_credentials (
 table_id uuid PRIMARY KEY REFERENCES poker.tables(table_id),
 password_phc text NOT NULL CHECK(octet_length(password_phc) BETWEEN 1 AND 512),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TRIGGER table_access_credentials_immutable BEFORE UPDATE OR DELETE OR TRUNCATE
 ON poker.table_access_credentials FOR EACH STATEMENT EXECUTE FUNCTION economy.reject_history_change();
REVOKE ALL ON poker.table_access_credentials FROM PUBLIC;
