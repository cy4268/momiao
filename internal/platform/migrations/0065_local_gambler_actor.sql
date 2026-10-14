-- Local roles authorize new commands; immutable campaign/account history stays.
ALTER TABLE economy.gambler_campaigns
 DROP CONSTRAINT gambler_campaigns_accepted_by_fkey,
 ADD CONSTRAINT gambler_campaigns_accepted_by_fkey
 FOREIGN KEY (accepted_by) REFERENCES identity.users(id);
