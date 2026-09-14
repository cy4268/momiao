-- User-approved 2026-09-06 Poker rules; no historical incomplete ruleset is activated.
INSERT INTO poker.ruleset_versions VALUES('poker-cash-v1-20260906','NO_ANTE','WAIT_FOR_BB','poker-initial-button-v1','poker-holdem-high-v1','poker-pot-after-call-whole-chip-v1','poker-deck-v1','poker-deal-v1',true);
INSERT INTO poker.blind_preset_versions(version,small_blind_units,big_blind_units,minimum_buyin_bb,maximum_buyin_bb) VALUES
 ('5-10',2500000,5000000,40,100),('10-20',5000000,10000000,40,100),('25-50',12500000,25000000,40,100),
 ('50-100',25000000,50000000,40,100),('100-200',50000000,100000000,40,100),('500-1000',250000000,500000000,40,100);
