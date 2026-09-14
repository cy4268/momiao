"""Build the frozen first-three-games SQL seed with an independent stdlib oracle.

No Go imports, database access, credentials or secret seeds. Run from repository
root; changes to this published output require a NEW migration after release.
"""
import hashlib
import itertools
import json
import pathlib
import struct
from fractions import Fraction


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def lp(value):
    raw = value.encode()
    return struct.pack(">H", len(raw)) + raw


def quote(value):
    return "'" + value.replace("'", "''") + "'"


def digest_hex(raw):
    return "decode('" + hashlib.sha256(raw).hexdigest() + "','hex')"


policy_id = "01993200-0000-7000-8000-000000000100"
policy = dict(version_number=1, minimum_wager_units=5000000, maximum_mode="NONE", input_step_units=500000,
              quick_amount_units=[5000000, 50000000, 250000000, 500000000])
statements = ["-- Frozen IS-06 seed, independently enumerated/weighted by scripts/games_seed.py.",
              "INSERT INTO games.wager_policy_versions VALUES(" + quote(policy_id) + ",1,5000000,'NONE',500000,ARRAY[5000000,50000000,250000000,500000000]::bigint[]," + digest_hex(canonical(policy).encode()) + ");",
              "INSERT INTO games.runtime_gate(singleton,active_wager_policy_version_id) VALUES(TRUE," + quote(policy_id) + ");"]
tables = {
    "scratch": [("LOSS", 0, 54000), ("BREAK_EVEN", 1, 19500), ("T2", 2, 18500), ("T3", 3, 5000), ("T5", 5, 2000), ("T10", 10, 800), ("T25", 25, 180), ("TOP", 100, 20)],
    "summon": [("T0", 0, 59850), ("T1", 1, 25000), ("T2", 2, 10000), ("T3", 5, 4000), ("T4", 20, 1050), ("T5", 100, 100)],
}
for index, (game, title) in enumerate([("dice", "命运骰盅"), ("scratch", "星纹刮刮卡"), ("summon", "圣晶召唤")], 1):
    version = f"01993200-0000-7000-8000-{index:012d}"
    artifact = f"01993200-0000-7000-8000-{200 + index:012d}"
    impl = f"direct.{game}.v1"
    algorithm = f"{game}-map-v1"
    schema = f"{game}-config-v1"
    rules = f"{game}-rules-v1"
    payload = dict(config_version=version, ruleset_version=rules)
    resources = {}
    if game == "dice":
        counts = {"SMALL": 0, "BIG": 0, "TRIPLE": 0}
        for a, b, c in itertools.product(range(1, 7), repeat=3):
            counts["TRIPLE" if a == b == c else "SMALL" if a+b+c <= 10 else "BIG"] += 1
        assert counts == dict(SMALL=105, BIG=105, TRIPLE=6)
        payload.update(allowed_choices=["BIG", "SMALL"], big_range=[11, 17], small_range=[4, 10], dice_count=3,
                       faces_per_die=6, triple_rule="BOTH_LOSE", win_total_payout_multiplier=2)
        math = dict(break_even="0", loss=str(Fraction(111, 216)), rtp=str(Fraction(210, 216)), top="0", win=str(Fraction(105, 216)))
    else:
        table = tables[game]
        assert sum(weight for _, _, weight in table) == 100000
        prizes = [dict(tier=tier, multiplier=multiplier, weight=weight) for tier, multiplier, weight in table]
        payload.update(prize_table_version=f"{game}-prize-v1", prizes=prizes)
        resources["prize_table_version"] = f"{game}-prize-v1"
        if game == "summon":
            payload.update(pool_id="SUMMON_MAIN_V1", draw_counts=[1, 10])
            resources["pool_id"] = "SUMMON_MAIN_V1"
        math = dict(break_even=str(Fraction(sum(w for _, m, w in table if m == 1), 100000)),
                    loss=str(Fraction(sum(w for _, m, w in table if m == 0), 100000)),
                    rtp=str(Fraction(sum(m*w for _, m, w in table), 100000)),
                    top=str(Fraction(table[-1][2], 100000)),
                    win=str(Fraction(sum(w for _, m, w in table if m > 1), 100000)))
    raw = canonical(payload)
    hash_sql = digest_hex(b"CHALDEA-GAME-CONFIG-V1\0" + lp(game) + lp(schema) + lp(algorithm) + raw.encode())
    print(canonical(dict(game=game, config_version=version, config_hash=hash_sql.split("'")[1],
                         algorithm=algorithm, exact_math=math,
                         math_sha256=hashlib.sha256(canonical(math).encode()).hexdigest())))
    statements.append("INSERT INTO games.game_registry(game_slug,title,sort_order,publication_state,configured_runtime_state,implementation_key) VALUES(" + ",".join([quote(game), quote(title), str(index), "'PUBLISHED'", "'AVAILABLE'", quote(impl)]) + ");")
    statements.append("INSERT INTO games.game_config_versions(config_version_id,game_slug,config_schema_version,ruleset_version,algorithm_version,config_payload,canonical_payload,config_hash,resource_versions) VALUES(" + ",".join([quote(version), quote(game), quote(schema), quote(rules), quote(algorithm), quote(raw) + "::jsonb", "convert_to(" + quote(raw) + ",'UTF8')", hash_sql, quote(canonical(resources)) + "::jsonb"]) + ");")
    statements.append("INSERT INTO games.game_validation_artifacts(validation_artifact_id,game_slug,artifact_type,implementation_key,ruleset_version,algorithm_version,config_version_id,config_hash,validator_version,validation_build,result_summary,artifact_sha256,status,verified_at) VALUES(" + ",".join([quote(artifact), quote(game), "'EXACT_MATH'", quote(impl), quote(rules), quote(algorithm), quote(version), hash_sql, "'direct-validate-v1'", "'direct-games-v1'", quote(canonical(math)) + "::jsonb", digest_hex(canonical(math).encode()), "'VERIFIED'", "now()"]) + ");")
    statements.append("UPDATE games.game_registry SET active_config_version_id=" + quote(version) + " WHERE game_slug=" + quote(game) + ";")
for index, game, title in [(4, "slot", "月光回响"), (5, "blackjack", "二十一点"), (6, "texas-holdem", "德州扑克")]:
    statements.append(f"INSERT INTO games.game_registry(game_slug,title,sort_order,publication_state,configured_runtime_state,implementation_key) VALUES('{game}','{title}',{index},'COMING_SOON','UNAVAILABLE','{'direct' if index<6 else 'poker'}.{game}.v1');")
pathlib.Path("internal/platform/migrations/0011_games_seed.sql").write_text("\n".join(statements) + "\n", encoding="utf-8")
print("Generated 0011: 216 Dice outcomes and complete Scratch/Summon prize weights independently validated.")
