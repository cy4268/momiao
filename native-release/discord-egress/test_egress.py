"""Run in an unprivileged private mount+network namespace; no external network."""
import configparser, json, os, pathlib, shlex, socket, ssl, stat, subprocess, sys, tempfile, threading, time

UNITS = pathlib.Path(__file__).resolve().parent
SOCAT = os.environ.get('SOCAT_TEST_BINARY', '/usr/bin/socat')

def command(unit, sock):
    cfg=configparser.ConfigParser(interpolation=None)
    assert cfg.read(UNITS/unit), 'Fixed egress unit missing: '+unit
    cmd=shlex.split(cfg['Service']['ExecStart'])
    assert cmd[0]=='/usr/bin/socat'
    assert cfg['Service']['NoNewPrivileges']=='yes'
    assert cfg['Service']['CapabilityBoundingSet']==''
    cmd[0]=SOCAT
    return [x.replace('/run/momiao-discord/upstream.sock',sock) for x in cmd]

def start(cmd, log):
    return subprocess.Popen(cmd,stdout=log,stderr=log,env=os.environ.copy(),start_new_session=True)

def stop(p):
    import signal
    if p.poll() is None:
        os.killpg(p.pid,signal.SIGTERM)
        p.wait(timeout=5)

def request(cafile, hostname='discord.com'):
    context=ssl.create_default_context(cafile=cafile)
    with socket.create_connection(('127.0.0.1',443),timeout=2) as raw:
        with context.wrap_socket(raw,server_hostname=hostname) as conn:
            conn.settimeout(2)
            conn.sendall(b'GET /synthetic HTTP/1.0\r\nHost: discord.com\r\n\r\n')
            return conn.recv(4096)

uid_map=pathlib.Path('/proc/self/uid_map').read_text().split()
assert len(uid_map)==3 and uid_map[0]=='0' and int(uid_map[1])>0 and uid_map[2]=='1', 'Run as an unprivileged user mapped to root in a private user/mount/network namespace'

if '--client' in sys.argv:
    cert,sock,out=sys.argv[2:5]
    subprocess.run(['ip','link','set','lo','up'],check=True)
    # Docker's measured namespace threshold is zero; reproduce it without host changes.
    pathlib.Path('/proc/sys/net/ipv4/ip_unprivileged_port_start').write_text('0')
    with open(out+'.bridge.log','w') as log:
        p=start(['setpriv','--bounding-set=-all','--inh-caps=-all','--ambient-caps=-all',*command('momiao-discord-bridge.service',sock)],log)
        try:
            for _ in range(60):
                try:
                    response=request(cert)
                    if b'fixed-destination-ok' in response:break
                except OSError:time.sleep(.05)
            else:raise RuntimeError('End-to-end TLS positive failed')
            caps=[x for x in pathlib.Path(f'/proc/{p.pid}/status').read_text().splitlines() if x.startswith(('CapEff:','CapBnd:','CapAmb:'))]
            assert all(int(x.split()[1],16)==0 for x in caps),caps
            checks={'verified_discord_tls':True,'zero_capabilities':True,'namespace_low_port_threshold':0}
            for name,ca,host in [('wrong_hostname',cert,'wrong.invalid'),('untrusted_certificate',None,'discord.com')]:
                try:request(ca,host)
                except ssl.SSLCertVerificationError:checks[name+'_rejected']=True
                else:raise AssertionError(name+' unexpectedly accepted')
            # There is no CONNECT parser: these bytes reach only the TLS fixture and fail.
            with socket.create_connection(('127.0.0.1',443),timeout=2) as client:
                client.settimeout(2)
                client.sendall(b'CONNECT forbidden.invalid:8443 HTTP/1.0\r\n\r\n')
                try:reply=client.recv(512)
                except ConnectionResetError:reply=b''
                assert b'200' not in reply
            checks['connect_target_not_interpreted']=True
            pathlib.Path(out+'.ready').write_text('ready')
            for _ in range(100):
                if pathlib.Path(out+'.stop-upstream').exists():break
                time.sleep(.05)
            else:raise RuntimeError('Upstream-stop coordination failed')
            try:request(cert)
            except (OSError,ssl.SSLError):checks['upstream_absent_closed']=True
            else:raise AssertionError('Missing upstream accepted')
            pathlib.Path(out).write_text(json.dumps(checks))
        finally:stop(p)
    sys.exit(0)

upstream=command('momiao-discord-upstream.service','FIXTURE_SOCKET')
assert upstream[-1]=='TCP4:discord.com:443,connect-timeout=10',upstream
bridge=command('momiao-discord-bridge.service','FIXTURE_SOCKET')
assert bridge[-1]=='UNIX-CONNECT:FIXTURE_SOCKET'
assert bridge[-2]=='TCP4-LISTEN:443,bind=127.0.0.1,reuseaddr,fork,max-children=16'

# Reconcile the real script at its OS boundary; no systemctl command is executed.
from types import SimpleNamespace
from unittest.mock import patch
source=(UNITS/'sync.py').read_text()
guard_cases=[]
for ready,drift in [(True,False),(False,False),(True,True)]:
    calls=[]
    state=json.dumps({'state':'READY' if ready else 'CLOSED','namespace_path':'/var/run/docker/netns/test','netns_inode':42})
    def fake_run(args,**kwargs):
        calls.append(args)
        return SimpleNamespace(stdout='77' if args[1]=='show' else '')
    def fake_stat(path,*args,**kwargs):
        return SimpleNamespace(st_ino=43 if drift and str(path).startswith('/proc/77/') else 42)
    with patch('pathlib.Path.read_text',return_value=state),patch('os.stat',side_effect=fake_stat),patch('subprocess.run',side_effect=fake_run):
        try:exec(compile(source,'sync.py','exec'),{})
        except SystemExit:closed=True
        else:closed=False
    assert closed==(not ready or drift)
    assert calls[-1][1]==('stop' if closed else 'show')
    guard_cases.append('closed' if closed else 'ready')

with tempfile.TemporaryDirectory(prefix='native-discord-') as temp:
    root=pathlib.Path(temp);sock=str(root/'upstream.sock');out=str(root/'result.json')
    cert=str(root/'cert.pem');key=str(root/'key.pem')
    subprocess.run(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-keyout',key,'-out',cert,'-days','1','-subj','/CN=discord.com','-addext','subjectAltName=DNS:discord.com'],check=True,capture_output=True)
    hosts=root/'hosts';hosts.write_text('127.0.0.1 localhost discord.com\n')
    subprocess.run(['mount','--bind',str(hosts),'/etc/hosts'],check=True)
    subprocess.run(['ip','link','set','lo','up'],check=True)
    tls=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER);tls.load_cert_chain(cert,key)
    snis=[];tls.set_servername_callback(lambda sock,name,ctx:snis.append(name))
    server=socket.socket();server.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);server.bind(('127.0.0.1',443));server.listen();server.settimeout(.1)
    done=threading.Event();served=[]
    def serve():
        while not done.is_set():
            try:raw,_=server.accept()
            except socket.timeout:continue
            except OSError:return
            try:
                with tls.wrap_socket(raw,server_side=True) as client:
                    client.settimeout(2);data=client.recv(8192);served.append(data)
                    client.sendall(b'HTTP/1.0 200 OK\r\nContent-Length: 20\r\n\r\nfixed-destination-ok')
            except (OSError,ssl.SSLError):raw.close()
    thread=threading.Thread(target=serve,daemon=True);thread.start()
    with open(root/'upstream.log','w') as log:
        upstream_process=start(command('momiao-discord-upstream.service',sock),log)
        child=None
        try:
            for _ in range(60):
                if pathlib.Path(sock).exists():break
                time.sleep(.05)
            assert stat.S_IMODE(os.stat(sock).st_mode)==0o600
            child=subprocess.Popen(['unshare','--net','--fork',sys.executable,__file__,'--client',cert,sock,out],stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
            for _ in range(160):
                if pathlib.Path(out+'.ready').exists():break
                if child.poll() is not None:raise RuntimeError(child.communicate())
                time.sleep(.05)
            else:raise RuntimeError('Client readiness deadline')
            stop(upstream_process)
            pathlib.Path(out+'.stop-upstream').write_text('stop')
            stdout,stderr=child.communicate(timeout=10)
            assert child.returncode==0,(stdout,stderr)
            checks=json.loads(pathlib.Path(out).read_text())
            assert 'discord.com' in snis
            assert len(served)==1,served
            checks.update(unix_mode='0600',fixed_target='discord.com:443',external_network=False,real_oauth_calls=0,client_server_separate_network_namespaces=True,sni_preserved=True,application_requests_reached_only_fixed_target=len(served),namespace_guard_synthetic_cases=guard_cases)
            print(json.dumps(checks,indent=2))
        finally:
            stop(upstream_process)
            if child and child.poll() is None:child.terminate();child.wait(timeout=5)
            done.set();server.close();thread.join(timeout=2)
