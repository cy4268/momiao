"""Bind controller-accepted G2 math to the compiled config; never resample games.

Only use while preparing an unpublished migration. Published SQL is immutable.
Inputs are public configuration/artifact metadata, never secrets or DB connections.
"""
import argparse, hashlib, json, pathlib, re
p=argparse.ArgumentParser()
p.add_argument('--configs',required=True)
p.add_argument('--game',choices=['slot','blackjack'],required=True)
p.add_argument('--output',required=True)
a=p.parse_args()
root=pathlib.Path(__file__).resolve().parent.parent
c=json.loads(pathlib.Path(a.configs).read_text(encoding='utf-8-sig'))[a.game]
source=(root/'internal/games/extra_validation_data.go').read_text(encoding='utf-8')
match=re.search(r'"'+a.game+r'":\s*("(?:\\.|[^"\\])*")',source)
summary=json.loads(match[1]);meta=json.loads(summary)
assert meta['config_hash']==c['config_hash'] and meta['config_version']==c['config_version']
def q(v):return "'"+v.replace("'","''")+"'"
def canon(v):return json.dumps(v,ensure_ascii=False,separators=(',',':'),sort_keys=True)
def digest(v):return "decode("+q(v)+",'hex')"
raw=canon(c['canonical_payload'])
assert hashlib.sha256(raw.encode()).hexdigest()==c['canonical_payload_sha256']
resources=({k:c['canonical_payload'][k] for k in ['reel_strip_version','payline_version','paytable_version']}
           if a.game=='slot' else {'shuffle_algorithm_version':'blackjack-fy-v1'})
values=[q(c['config_version']),q(a.game),q(c['schema']),q(c['ruleset']),q(c['algorithm']),q(raw)+'::jsonb',"convert_to("+q(raw)+",'UTF8')",digest(c['config_hash']),q(canon(resources))+'::jsonb']
lines=['INSERT INTO games.game_config_versions(config_version_id,game_slug,config_schema_version,ruleset_version,algorithm_version,config_payload,canonical_payload,config_hash,resource_versions) VALUES('+','.join(values)+');']
artifact='01993200-0000-7000-8000-00000000020'+('4' if a.game=='slot' else '5')
summaryhash=hashlib.sha256(summary.encode()).hexdigest()
values=[q(artifact),q(a.game),q(meta['artifact_type']),q(c['implementation']),q(c['ruleset']),q(c['algorithm']),q(c['config_version']),digest(c['config_hash']),q(meta['validator_version']),q(meta['validation_build']),q(summary)+'::jsonb',digest(summaryhash),"'VERIFIED'",'now()']
lines+=['INSERT INTO games.game_validation_artifacts(validation_artifact_id,game_slug,artifact_type,implementation_key,ruleset_version,algorithm_version,config_version_id,config_hash,validator_version,validation_build,result_summary,artifact_sha256,status,verified_at) VALUES('+','.join(values)+');',
        "UPDATE games.game_registry SET publication_state='PUBLISHED',configured_runtime_state='AVAILABLE',active_config_version_id="+q(c['config_version'])+' WHERE game_slug='+q(a.game)+';']
out=pathlib.Path(a.output)
marker='-- BEGIN FROZEN CONFIG SEED (scripts/games_extra_seed.py)\n'
prefix=out.read_text(encoding='utf-8').split(marker)[0]
out.write_text(prefix+marker+'\n'.join(lines)+'\n',encoding='utf-8')
print(a.game,c['config_version'],'config_hash='+c['config_hash'],'summary_sha256='+summaryhash)
