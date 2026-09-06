"""Bounded native-only promotion, adapted from the existing exact-input promotion scripts.

Default is an offline plan. --apply is a separate, explicit production operation.
Rollback restores code/configuration only; no SQL down migration or database restore exists.
"""
import argparse
import copy
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys
import tarfile
from urllib.parse import urlsplit

HERE=Path(__file__).resolve().parent
JOURNAL=None
STACK=Path('/opt/chaldea-sv/runs/sv-20260905-a')
COMPOSE=STACK/'compose.json'
ENVFILE=STACK/'secrets/native.env'
HTTP=Path('/opt/chaldea-preview/sync.py')
QUOTA=Path('/opt/momiao-native-quota/sync.py')
PROJECT='chaldea-sv-20260905-a'
PUBLIC_ORIGIN='https://momiao.win'
OLD_IMAGE='calciumion/new-api@sha256:54a0b10924aa75fa5b5947208b820ced66b6ef4b445b35f122b31d80676aba2b'
NEW_IMAGE='sha256:6d7062ca03efec8fd15cf78c1127f3442ff2c9b2a51558b6804489d082d50af7'
NEW_CONFIG='sha256:ea1ca6e8d4ab54a451da5ea305949f6dde6231181730b70834201860b204a766'
ROOT_SHA='76463aae3a51b54577c000e0c4d62ca4d6cc46d5e7c166868553d7db8d3adb1c'
MIGRATOR_SHA='f2ae036113f5fc794ef9c712623d0f846ee40434df6ce10cc1001275812a502a'
HTTP_SHA='62be67325b2b43f18359676da40d8be51a37b62b77dfc7d7a1fffe873170dbdd'
QUOTA_SHA='37105a39ad6c1e5fd846fa8bb455054620273baae4a3fc2ad8303aa88fab3a38'
EGRESS={
 'momiao-discord-bridge.service':'de542a3eae455ed2aa21571feb18ee9245f10d02c68ab036b0919b953568be3a',
 'momiao-discord-sync.service':'4c81e43efae83cea87399876ed7c50d1da7f4e81c0c5a50b6d78f0a135951264',
 'momiao-discord-sync.timer':'7593f39caed5250cb52f8d82ff4b1f489d59419c51640c62f4c86ee7a21073e2',
 'momiao-discord-upstream.service':'414d94877de57cc26ea1de7ad19dbb342d037c5cff8341cc28844afb261a54c7',
 'sync.py':'658d21c9a1ebbd331f46485151a22538a78140bb8a8b8cbab7c4b0b2bcd1a357',
}
REQUIRED=['expected_hostname','release_id','image_archive','image_archive_sha256','loaded_image_id',
 'migration_dsn_file','admission_config_file','catalog_config_file','approve_secure_cookie_settings',
 'approve_native_schema_increment','ack_database_history_preserved',
 'expected_old.container_id','expected_old.image_reference','expected_old.image_id',
 'expected_old.compose_sha256','expected_old.native_env_sha256','expected_old.http_guard_sha256',
 'expected_old.quota_guard_sha256','expected_old.protected_containers',
 'database_backups.native.path','database_backups.native.sha256',
 'database_backups.platform.path','database_backups.platform.sha256']

def need(ok,message):
    if not ok:raise RuntimeError(message)

def require_admission_origin(cfg):
    need(cfg.get('public_origin')==PUBLIC_ORIGIN and cfg.get('redirect_uri')==PUBLIC_ORIGIN+'/oauth/discord','Admission origin/callback is not this site')

def field(request,key):
    value=request
    for part in key.split('.'):
        if not isinstance(value,dict):return None
        value=value.get(part)
    return value

def plan(request):
    return {'status':'DRY_RUN_NOT_DEPLOYED','executed_steps':[],
      'approval_fields_remaining':[k for k in REQUIRED if not field(request,k) or str(field(request,k)).startswith('REQUIRED')],
      'enable_admission':request.get('enable_admission',False),'enable_catalog':request.get('enable_catalog',False),
      'image':NEW_IMAGE,'database_restore':False,
      'sequence':['verify expected-old and offline artifacts','private configuration backup','native migration',
                  'replace only newapi','HTTP namespace READY','quota and Discord READY'],
      'not_executed':['Docker','systemd','database','OAuth','portal changes']}

def sha(path):
    digest=hashlib.sha256()
    with Path(path).open('rb') as handle:
        for block in iter(lambda:handle.read(1024*1024),b''):digest.update(block)
    return digest.hexdigest()

def regular(path):
    path=Path(path)
    need(path.is_absolute() and path.resolve()==path and stat.S_ISREG(path.lstat().st_mode),'Expected a regular absolute non-symlink file')
    return path

def hashcheck(path,expected):
    need(re.fullmatch('[0-9a-f]{64}',expected or '') and sha(regular(path))==expected,'File hash differs from approved input')

def restricted(path,limit=8192):
    path=regular(path);info=path.stat()
    need(stat.S_IMODE(info.st_mode)==0o600 and info.st_uid in (0,65534) and info.st_size<=limit,'Private input ownership/mode/size rejected')
    return path.read_bytes()

def run(*args,check=True,timeout=120,env=None):
    p=subprocess.run(args,capture_output=True,text=True,timeout=timeout,env=env)
    if check:need(p.returncode==0,'Local command failed: '+Path(args[0]).name)
    return p

def docker(*args,**kwargs):
    return run('docker','--host','unix:///var/run/docker.sock',*args,**kwargs)

def one(service,missing_ok=False):
    ids=docker('ps','-aq','--no-trunc','--filter','label=com.docker.compose.project='+PROJECT,
               '--filter','label=com.docker.compose.service='+service).stdout.split()
    if missing_ok and not ids:return None
    need(len(ids)==1,'Expected exactly one native stack service')
    return json.loads(docker('inspect',ids[0]).stdout)[0]

def snapshot():
    result={}
    for name in ('netns','postgres','redis'):
        item=one(name)
        need(item['State']['Running'],'Protected container is not running')
        result[name]={'id':item['Id'],'started_at':item['State']['StartedAt']}
    return result

def native(expected_image,expected_id=None):
    app=one('newapi');owner=one('netns');pg=one('postgres')
    env=dict(x.split('=',1) for x in app['Config']['Env'] if '=' in x)
    need(app['Config']['Image']==expected_image and (expected_id is None or app['Id']==expected_id),'Native image/container differs from expected-old')
    need(app['State'].get('Health',{}).get('Status')=='healthy' and pg['State'].get('Health',{}).get('Status')=='healthy','Native/PostgreSQL health gate closed')
    need(owner['HostConfig']['NetworkMode']=='none' and app['HostConfig']['NetworkMode']=='container:'+owner['Id'] and pg['HostConfig']['NetworkMode']=='container:'+owner['Id'],'Private namespace topology differs')
    need(app['Config']['User']=='65534:65534' and app['HostConfig']['CapDrop']==['ALL'] and not app['HostConfig']['CapAdd'],'Native capability or UID drift')
    need(env.get('REDIS_CONN_STRING','')=='' and env.get('BATCH_UPDATE_ENABLED')=='false','DB-only mode differs')
    need(not app['HostConfig'].get('PortBindings') and not any(env.get(k) for k in ('HTTP_PROXY','HTTPS_PROXY','ALL_PROXY','http_proxy','https_proxy','all_proxy')),'Native public port/proxy drift')
    return app,env

def atomic(path,data,mode=0o600,uid=0,gid=0):
    path=Path(path);temporary=path.with_name(path.name+'.native-next')
    with temporary.open('xb') as handle:
        os.fchmod(handle.fileno(),mode);os.fchown(handle.fileno(),uid,gid)
        handle.write(data);handle.flush();os.fsync(handle.fileno())
    os.replace(temporary,path)

def patch_guard(source,new_image):
    need(new_image in (NEW_IMAGE,NEW_CONFIG) and source.count(OLD_IMAGE)==1,'Quota guard replacement is not exactly one reviewed image literal')
    return source.replace(OLD_IMAGE,new_image,1)

def mapping(value):
    return dict(x.split('=',1) for x in value) if isinstance(value,list) else dict(value or {})

def compose_candidate(original,image,release_id,directory,admission,catalog,origin,old_origins):
    result=copy.deepcopy(original);app=result['services']['newapi']
    need(app['network_mode']=='service:netns' and result['services']['netns']['network_mode']=='none','Compose namespace topology differs')
    need(app.get('cap_drop')==['ALL'] and not app.get('cap_add') and not app.get('ports'),'Compose capability/public port drift')
    app['image']=image
    app['environment']=mapping(app.get('environment'))
    app['environment'].update(REDIS_CONN_STRING='',BATCH_UPDATE_ENABLED='false',MOMIAO_ADMISSION_ENABLED=str(admission).lower(),
        MOMIAO_ADMISSION_CONFIG_FILE='/run/secrets/momiao-admission.json',MOMIAO_CATALOG_CONFIG_FILE='/run/secrets/momiao-catalog.conf',
        SESSION_COOKIE_SECURE='true',SESSION_COOKIE_TRUSTED_URL=','.join(dict.fromkeys([x.strip() for x in old_origins.split(',') if x.strip()]+[origin])))
    app['labels']=mapping(app.get('labels'));app['labels']['win.momiao.native-release']=release_id
    hosts=app.get('extra_hosts',{})
    if isinstance(hosts,list):hosts=dict(x.split(':',1) for x in hosts)
    need('discord.com' not in hosts,'Existing Discord hosts mapping needs separate review')
    app['extra_hosts']={**hosts,'discord.com':'127.0.0.1'}
    volumes=app.setdefault('volumes',[])
    targets=[x.get('target') if isinstance(x,dict) else x.split(':')[1] for x in volumes]
    for name in ('admission.json','catalog.conf'):
        target='/run/secrets/momiao-'+name
        need(target not in targets,'Existing feature configuration mount needs separate review')
        volumes.append({'type':'bind','source':directory+'/secrets/'+name,'target':target,'read_only':True,'bind':{'create_host_path':False}})
    return result

def write_state(state):
    global JOURNAL
    path=Path(state['directory'])/'rollback/state.json'
    atomic(path,(json.dumps(state,indent=2)+'\n').encode())
    JOURNAL=path

def state_ready(path):
    need(json.loads(Path(path).read_text())['state']=='READY','Native transport guard is not READY')

def synchronize():
    run('systemctl','start','chaldea-preview-sync.service')
    state_ready('/run/chaldea-preview/state.json')
    run('systemctl','start','momiao-native-quota-sync.service')
    state_ready('/opt/momiao-native-quota/state.json')

def recreate():
    docker('compose','-p',PROJECT,'-f',str(COMPOSE),'up','-d','--no-deps','--force-recreate','--pull','never','--no-build','--wait','--wait-timeout','80','newapi',timeout=110)

def acquire_lock():
    import fcntl
    lock=os.open('/run/momiao-native-release.lock',os.O_CREAT|os.O_RDWR|os.O_NOFOLLOW,0o600)
    fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
    return lock

def apply(request):
    need(not plan(request)['approval_fields_remaining'],'Explicit deployment fields/approvals are incomplete')
    need(sys.platform=='linux' and os.geteuid()==0 and run('hostname').stdout.strip()==request['expected_hostname'],'Wrong execution host/user')
    need(all(request[k] is True for k in ('approve_secure_cookie_settings','approve_native_schema_increment','ack_database_history_preserved')),'Explicit bounded approvals are required')
    need(all(type(request.get(k,False)) is bool for k in ('enable_admission','enable_catalog')),'Feature switches must be explicit booleans')
    need(re.fullmatch(r'[0-9]{8}T[0-9]{6}Z-native6d7062',request['release_id']) is not None,'Release directory identifier rejected')
    expected=request['expected_old'];image=request['loaded_image_id']
    need(image in (NEW_IMAGE,NEW_CONFIG) and expected['image_reference']==OLD_IMAGE,'Unreviewed image identity')
    need(expected['http_guard_sha256']==HTTP_SHA and expected['quota_guard_sha256']==QUOTA_SHA,'Unreviewed guard baseline')
    lock=acquire_lock()
    for path,key in ((COMPOSE,'compose_sha256'),(ENVFILE,'native_env_sha256'),(HTTP,'http_guard_sha256'),(QUOTA,'quota_guard_sha256')):hashcheck(path,expected[key])
    before,env=native(OLD_IMAGE,expected['container_id'])
    need(before['Image']==expected['image_id'] and env.get('SESSION_SECRET'),'Image identity or stable session secret missing')
    need(json.loads(docker('image','inspect',OLD_IMAGE).stdout)[0]['Id']==expected['image_id'],'Prior image is not available for offline rollback')
    need(snapshot()==expected['protected_containers'],'Protected container baseline differs')
    state_ready('/run/chaldea-preview/state.json');state_ready('/opt/momiao-native-quota/state.json')
    need(run('nsenter','--net=/run/chaldea-preview/netns','cat','/proc/sys/net/ipv4/ip_unprivileged_port_start').stdout.strip()=='0','Zero-capability low-port prerequisite differs')
    need(':443 ' not in run('nsenter','--net=/run/chaldea-preview/netns','ss','-lntH').stdout,'Native loopback443 is occupied')
    hashcheck(request['image_archive'],request['image_archive_sha256'])
    with tarfile.open(request['image_archive'],'r:*') as archive:
        manifest=json.load(archive.extractfile('manifest.json'))
        need(len(manifest)==1 and manifest[0].get('RepoTags') in (None,[],['momiao-native:release-prep-20260906']),'Offline archive contains unexpected image tags')
        need(hashlib.sha256(archive.extractfile(manifest[0]['Config']).read()).hexdigest()==NEW_CONFIG[7:],'Offline image config is not the reviewed candidate')
    need(set(request['database_backups'])=={'native','platform'},'Require exactly the approved native/platform backup references')
    for backup in request['database_backups'].values():hashcheck(backup['path'],backup['sha256'])
    cfg=json.loads(restricted(request['admission_config_file']))
    keys={'client_id','client_secret','guild_id','role_id','public_origin','redirect_uri','policy_version','reader_key'}
    need(set(cfg)==keys and all(isinstance(x,str) and x for x in cfg.values()),'Admission configuration shape rejected')
    require_admission_origin(cfg)
    catalog={}
    for line in restricted(request['catalog_config_file']).decode().splitlines():
        if not line.strip() or line.lstrip().startswith('#'):continue
        key,value=line.strip().split('=',1);need(key not in catalog,'Duplicate catalog key');catalog[key]=value
    need(set(catalog)<={'enabled','reader_secret','public_group'} and catalog.get('enabled') in ('true','false') and catalog.get('public_group','default')=='default','Catalog configuration shape rejected')
    need(re.fullmatch('[0-9a-fA-F]{64}',catalog.get('reader_secret','')) is not None and 32<=len(cfg['reader_key'])<=256 and cfg['reader_key']!=catalog['reader_secret'],'Independent reader credentials rejected')
    dsn=restricted(request['migration_dsn_file']).decode().strip();db=urlsplit(dsn);live_db=urlsplit(env.get('SQL_DSN',''))
    need(db.scheme in ('postgres','postgresql') and db.hostname=='127.0.0.1' and (db.port or 5432)==5432 and db.path=='/sv_native' and db.username==live_db.username and db.path==live_db.path,'Migration DSN must use the existing native database owner/target')
    install={name:(Path('/opt/momiao-discord/sync.py') if name=='sync.py' else Path('/etc/systemd/system')/name) for name in EGRESS}
    need(not Path('/opt/momiao-discord').exists() and all(not p.exists() and not p.is_symlink() for p in install.values()),'Discord transport already exists; separate update review required')
    run('id','-u','momiao-egress');run('socat','-V')
    for name,digest in EGRESS.items():hashcheck(HERE/'discord-egress'/name,digest)
    directory=Path('/opt/momiao-native/releases')/request['release_id']
    need(not directory.exists() and directory.resolve()==directory,'Release directory must be new and non-symlinked')
    directory.mkdir(parents=True,mode=0o755);(directory/'rollback').mkdir(mode=0o700);(directory/'secrets').mkdir(mode=0o700)
    os.chown(directory/'secrets',65534,65534)
    state={'schema':'native-release-receipt.v1','directory':str(directory),'release_id':request['release_id'],'expected_hostname':request['expected_hostname'],
           'old_image':OLD_IMAGE,'new_image':image,'old_container_id':before['Id'],'new_container_id':None,'protected_containers':snapshot(),
           'database_backups':request['database_backups'],'database_restore':False,'migration':'not_started','stage':'configuration_backup','original_files':{},'new_files':{}}
    for path in (COMPOSE,ENVFILE,HTTP,QUOTA):
        info=path.stat();backup=directory/'rollback'/(path.name+'.'+sha(path)[:8])
        atomic(backup,path.read_bytes())
        state['original_files'][str(path)]={'backup':str(backup),'sha256':sha(path),'mode':stat.S_IMODE(info.st_mode),'uid':info.st_uid,'gid':info.st_gid}
    write_state(state)
    if docker('image','inspect',image,check=False).returncode:docker('load','--input',request['image_archive'],timeout=300)
    loaded=json.loads(docker('image','inspect',image).stdout)[0]
    need(loaded['Id']==image and loaded['Os']=='linux' and loaded['Architecture']=='amd64','Loaded engine image identity differs')
    hashes=docker('run','--rm','--network','none','--entrypoint','/usr/bin/sha256sum',image,'/new-api','/usr/local/bin/momiao-admission-migrate').stdout
    need(hashes.splitlines()==[ROOT_SHA+'  /new-api',MIGRATOR_SHA+'  /usr/local/bin/momiao-admission-migrate'],'Image payload hashes differ')
    admission=request.get('enable_admission',False);catalog_on=request.get('enable_catalog',False)
    atomic(directory/'secrets/admission.json',json.dumps(cfg).encode(),uid=65534,gid=65534)
    catalog['enabled']=str(catalog_on).lower()
    atomic(directory/'secrets/catalog.conf',('\n'.join(k+'='+v for k,v in catalog.items())+'\n').encode(),uid=65534,gid=65534)
    candidate=compose_candidate(json.loads(COMPOSE.read_text()),image,request['release_id'],str(directory),admission,catalog_on,cfg['public_origin'],env.get('SESSION_COOKIE_TRUSTED_URL',''))
    candidate_bytes=(json.dumps(candidate,indent=2)+'\n').encode();guard_bytes=patch_guard(QUOTA.read_text(),image).encode()
    state['installed_hashes']={str(COMPOSE):hashlib.sha256(candidate_bytes).hexdigest(),str(QUOTA):hashlib.sha256(guard_bytes).hexdigest()}
    state['stage']='native_migration';state['migration']='attempted_outcome_pending';write_state(state)
    name='momiao-native-migrate-'+request['release_id'].lower();owner=one('netns')['Id']
    def migrate(apply_increment=False):
        args=['run','--rm','--name',name,'--label','win.momiao.native-release='+request['release_id'],'--network','container:'+owner,
              '--read-only','--env','SQL_DSN','--entrypoint','/usr/local/bin/momiao-admission-migrate',image]
        if apply_increment:args+=['--apply']
        try:return docker(*args,check=False,env={**os.environ,'SQL_DSN':dsn})
        finally:
            remaining=docker('inspect',name,check=False)
            if remaining.returncode==0:
                row=json.loads(remaining.stdout)[0]
                need(row['Config']['Labels'].get('win.momiao.native-release')==request['release_id'],'Migration container ownership differs')
                docker('rm','-f',row['Id'])
    if migrate().returncode:need(migrate(True).returncode==0,'Native schema increment failed; preserve DB and inspect before retry')
    need(migrate().returncode==0,'Native schema readiness check failed')
    state['migration']='ready_increment_retained_on_rollback';state['stage']='replace_native';write_state(state)
    for path,data in ((COMPOSE,candidate_bytes),(QUOTA,guard_bytes)):
        metadata=state['original_files'][str(path)];hashcheck(path,metadata['sha256'])
        atomic(path,data,metadata['mode'],metadata['uid'],metadata['gid'])
    recreate()
    app,new_env=native(image);need(app['Image']==image and new_env.get('SESSION_SECRET')==env['SESSION_SECRET'],'New image/session-secret verification failed')
    state['new_container_id']=app['Id'];state['stage']='native_healthy';write_state(state)
    need(snapshot()==state['protected_containers'],'Protected container changed')
    hashcheck(HTTP,HTTP_SHA);hashcheck(ENVFILE,expected['native_env_sha256']);synchronize()
    state['stage']='installing_discord'
    state['new_files']={str(path):EGRESS[name] for name,path in install.items()}
    write_state(state)
    Path('/opt/momiao-discord').mkdir(mode=0o755)
    for name,path in install.items():
        need(not path.exists() and not path.is_symlink(),'Discord installation target changed after preflight')
        atomic(path,(HERE/'discord-egress'/name).read_bytes(),0o644)
    run('systemd-analyze','verify',*[str(p) for p in install.values() if p.suffix in ('.service','.timer')])
    run('systemctl','daemon-reload')
    run('systemctl','enable','--now','momiao-discord-upstream.service','momiao-discord-sync.timer')
    run('systemctl','start','momiao-discord-sync.service')
    for unit in ('momiao-discord-upstream.service','momiao-discord-bridge.service','momiao-discord-sync.timer'):
        need(run('systemctl','is-active',unit).stdout.strip()=='active','Discord transport is not active')
    pid=run('systemctl','show','momiao-discord-bridge.service','-p','MainPID','--value').stdout.strip()
    need(os.stat('/proc/'+pid+'/ns/net').st_ino==os.stat('/run/chaldea-preview/netns').st_ino,'Discord namespace identity differs')
    need(all(int(line.split()[1],16)==0 for line in Path('/proc/'+pid+'/status').read_text().splitlines() if line.startswith(('CapEff:','CapBnd:','CapAmb:'))),'Discord bridge gained capabilities')
    need(snapshot()==state['protected_containers'],'Protected containers changed')
    hashcheck(COMPOSE,state['installed_hashes'][str(COMPOSE)]);hashcheck(QUOTA,state['installed_hashes'][str(QUOTA)])
    hashcheck(HTTP,HTTP_SHA);hashcheck(ENVFILE,expected['native_env_sha256'])
    state['stage']='DEPLOYED_NATIVE_ONLY';write_state(state)
    return {'status':state['stage'],'receipt':str(directory/'rollback/state.json'),'database_restore':False,'real_oauth_tested':False}

def rollback(state):
    need(state.get('schema')=='native-release-receipt.v1' and state.get('database_restore') is False,'Not a native code/config rollback receipt')
    need(re.fullmatch(r'[0-9]{8}T[0-9]{6}Z-native6d7062',state.get('release_id','')) is not None and Path(state['directory'])==Path('/opt/momiao-native/releases')/state['release_id'],'Rollback directory differs from this release family')
    need(state['old_image']==OLD_IMAGE and state['new_image'] in (NEW_IMAGE,NEW_CONFIG),'Rollback image is not the reviewed pair')
    need(sys.platform=='linux' and os.geteuid()==0 and run('hostname').stdout.strip()==state['expected_hostname'],'Wrong rollback host/user')
    lock=acquire_lock()
    need(snapshot()==state['protected_containers'],'Protected container drift; stop rollback for review')
    current=one('newapi',missing_ok=True)
    old_untouched=current is not None and current['Id']==state['old_container_id'] and current['Config']['Image']==state['old_image']
    own_new=current is not None and current['Config']['Image']==state['new_image'] and current['Config'].get('Labels',{}).get('win.momiao.native-release')==state['release_id']
    creation_failed=current is None and state['stage'] in ('replace_native','native_healthy','installing_discord','DEPLOYED_NATIVE_ONLY')
    need(old_untouched or creation_failed or (own_new and (state['new_container_id'] is None or current['Id']==state['new_container_id'])),'Rollback container is not the recorded release')
    allowed={str(COMPOSE),str(ENVFILE),str(HTTP),str(QUOTA)}
    need(set(state['original_files'])==allowed,'Rollback source-file set differs')
    for target,metadata in state['original_files'].items():
        need(Path(metadata['backup']).parent==Path(state['directory'])/'rollback','Rollback backup is outside the recorded private directory')
        hashcheck(metadata['backup'],metadata['sha256'])
        need(sha(target) in (metadata['sha256'],state.get('installed_hashes',{}).get(target)),'Configuration drift; stop rollback for review')
    for target,digest in state['new_files'].items():
        need(target in [str(Path('/etc/systemd/system')/n) for n in EGRESS if n!='sync.py']+['/opt/momiao-discord/sync.py'],'Unexpected rollback unit path')
        if Path(target).exists():hashcheck(target,digest)
    if state['new_files']:
        run('systemctl','daemon-reload')
        run('systemctl','disable','--now','momiao-discord-sync.timer','momiao-discord-upstream.service',check=False)
        run('systemctl','stop','momiao-discord-bridge.service','momiao-discord-sync.timer','momiao-discord-upstream.service',check=False)
        for unit in ('momiao-discord-bridge.service','momiao-discord-upstream.service'):
            need(run('systemctl','show',unit,'-p','MainPID','--value',check=False).stdout.strip() in ('','0'),'Discord process remains active; stop rollback for review')
        need(run('systemctl','is-active','momiao-discord-sync.timer',check=False).returncode!=0,'Discord timer remains active')
    for target in (str(COMPOSE),str(QUOTA)):
        metadata=state['original_files'][target]
        atomic(target,Path(metadata['backup']).read_bytes(),metadata['mode'],metadata['uid'],metadata['gid'])
    for target in state['new_files']:
        if Path(target).exists():Path(target).unlink()
    if state['new_files']:run('systemctl','daemon-reload')
    if own_new or creation_failed:recreate()
    native(state['old_image']);synchronize()
    need(snapshot()==state['protected_containers'],'Protected container changed during rollback')
    for target,metadata in state['original_files'].items():hashcheck(target,metadata['sha256'])
    state['stage']='ROLLED_BACK_CODE_CONFIG_ONLY';write_state(state)
    return {'status':state['stage'],'native_schema_retained':state['migration'],'database_restore':False,'private_backup_and_unused_release_configs_retained':True}

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('request',type=Path);parser.add_argument('--apply',action='store_true');parser.add_argument('--rollback',action='store_true')
    args=parser.parse_args();request=json.loads(args.request.read_text(encoding='utf-8-sig'))
    if not args.apply:
        result={'status':'DRY_RUN_NOT_ROLLED_BACK','executed_steps':[],'database_restore':False} if args.rollback else plan(request)
    else:
        if args.rollback:
            need(args.request.resolve()==Path(request['directory'])/'rollback/state.json','Rollback must use the recorded private receipt path')
            restricted(args.request.resolve(),1024*1024)
        result=rollback(request) if args.rollback else apply(request)
    print(json.dumps(result,indent=2))

if __name__=='__main__':
    try:main()
    except Exception as error:
        print(json.dumps({'status':'STOPPED_NOT_MARKED_SUCCESS','error_type':type(error).__name__,
                          'reason':str(error) if isinstance(error,RuntimeError) else 'Input or local operation failed; details withheld',
                          'receipt':str(JOURNAL) if JOURNAL else None,
                          'database_restore':False,'next':'Inspect the private receipt, then explicitly request code/config rollback if required'}))
        sys.exit(1)
