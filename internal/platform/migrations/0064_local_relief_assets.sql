-- Historical Native observations remain; new claims observe only local assets.
ALTER TABLE rewards.relief_claims ALTER COLUMN native_observed_at DROP NOT NULL;
