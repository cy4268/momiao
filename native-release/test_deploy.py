"""Offline contract checks only: never invoke Docker, systemd, PostgreSQL or SSH."""
import copy
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile

sys.dont_write_bytecode=True
SCRIPT=Path(__file__).with_name('deploy.py')
assert SCRIPT.exists(), 'Deployment entry is not implemented'
spec=importlib.util.spec_from_file_location('native_deploy',SCRIPT)
module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)

request={'expected_old':{},'enable_admission':False,'enable_catalog':False}
plan=module.plan(request)
assert plan['executed_steps']==[] and plan['database_restore'] is False
assert plan['enable_admission'] is False and plan['enable_catalog'] is False
assert 'image_archive_sha256' in plan['approval_fields_remaining']
assert plan['sequence'].index('native migration')<plan['sequence'].index('replace only newapi')
assert plan['sequence'].index('HTTP namespace READY')<plan['sequence'].index('quota and Discord READY')

module.require_admission_origin({'public_origin':'https://momiao.win','redirect_uri':'https://momiao.win/oauth/discord'})
try:module.require_admission_origin({'public_origin':'https://other.invalid','redirect_uri':'https://other.invalid/oauth/discord'})
except RuntimeError:pass
else:raise AssertionError('Another site was accepted for this stack')

source="assert app['Config']['Image']=='"+module.OLD_IMAGE+"'\nassert private_namespace\n"
patched=module.patch_guard(source,module.NEW_IMAGE)
assert patched==source.replace(module.OLD_IMAGE,module.NEW_IMAGE)
try:module.patch_guard(source+source,module.NEW_IMAGE)
except RuntimeError:pass
else:raise AssertionError('Duplicate guard replacement was accepted')

original={'services':{'newapi':{'image':module.OLD_IMAGE,'network_mode':'service:netns','cap_drop':['ALL'],'volumes':['/native/data:/data'],'env_file':['/private/native.env']},'netns':{'network_mode':'none'},'postgres':{'image':'unchanged'},'redis':{'image':'unchanged'}}}
before=copy.deepcopy(original)
changed=module.compose_candidate(original,module.NEW_IMAGE,'synthetic-release','/opt/momiao-native/releases/synthetic',False,False,'https://admission.test','')
assert original==before
assert all(changed['services'][key]==original['services'][key] for key in ('netns','postgres','redis'))
app=changed['services']['newapi']
assert app['network_mode']=='service:netns' and app['cap_drop']==['ALL'] and not app.get('cap_add')
assert app['environment']['MOMIAO_ADMISSION_ENABLED']=='false'
assert app['environment']['REDIS_CONN_STRING']=='' and app['environment']['BATCH_UPDATE_ENABLED']=='false'
assert app['extra_hosts']=={'discord.com':'127.0.0.1'}

with tempfile.TemporaryDirectory(prefix='native-deploy-dry-') as temp:
    path=Path(temp)/'request.json';path.write_text(json.dumps(request))
    p=subprocess.run([sys.executable,str(SCRIPT),str(path)],capture_output=True,text=True)
    assert p.returncode==0,p.stderr
    output=json.loads(p.stdout)
    assert output['status']=='DRY_RUN_NOT_DEPLOYED' and output['executed_steps']==[]
    p=subprocess.run([sys.executable,str(SCRIPT),str(path),'--rollback'],capture_output=True,text=True)
    assert p.returncode==0 and json.loads(p.stdout)['status']=='DRY_RUN_NOT_ROLLED_BACK'
    assert sorted(x.name for x in Path(temp).iterdir())==['request.json']
print(json.dumps({'offline_contract':'passed','docker_calls':0,'systemd_calls':0,'database_calls':0,'remote_calls':0,'deployment_executed':False}))
