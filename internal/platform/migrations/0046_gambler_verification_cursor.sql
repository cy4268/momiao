-- Bounded frozen-checkpoint scans and explicit continuation retain sealed data.
ALTER TABLE economy.gambler_campaigns ADD COLUMN verification_phase text NOT NULL DEFAULT '', ADD COLUMN verification_cursor bigint NOT NULL DEFAULT 0 CHECK(verification_cursor>=0), ADD COLUMN reverify_pending boolean NOT NULL DEFAULT false;
