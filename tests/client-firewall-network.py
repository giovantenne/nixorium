"""Exercise the production client rules inside an unprivileged network namespace.

Run via unshare --user --map-root-user --net; never in the host namespace.
The caller supplies the evaluated nft rules and absolute nft/Python paths.
"""
import os
import pathlib
import subprocess
import sys
import time

rules, nft, python = sys.argv[1:4]
internet_rules = sys.argv[4] if len(sys.argv) == 5 else None
if str(pathlib.Path('/proc/self/ns/net').readlink()) == os.environ['NIXORIUM_TEST_PARENT_NETNS']:
    raise SystemExit('Refusing to test in the host network namespace')
if str(pathlib.Path('/proc/self/ns/user').readlink()) == os.environ['NIXORIUM_TEST_PARENT_USERNS']:
    raise SystemExit('Run in a dedicated unprivileged user namespace')
uid_map = [int(n) for n in pathlib.Path('/proc/self/uid_map').read_text().split()]
if len(uid_map) != 3 or uid_map[0] != 0 or uid_map[1] == 0 or uid_map[2] != 1:
    raise SystemExit('Expected a single unprivileged user mapped to namespace root')
children = []

def run(*args, **kwargs):
    result = subprocess.run(args, text=True, capture_output=True, **kwargs)
    if result.returncode:
        raise RuntimeError(f"{args}: {result.stderr or result.stdout}")
    return result.stdout

def spawn(*args, **kwargs):
    p = subprocess.Popen(args, **kwargs)
    children.append(p)
    return p

def ns(pid, *args):
    return ('nsenter', '-t', str(pid), '-n', *args)

def new_namespace():
    p = spawn('unshare', '--net', 'sleep', '300')
    for _ in range(100):
        if pathlib.Path(f'/proc/{p.pid}/ns/net').readlink() != pathlib.Path('/proc/self/ns/net').readlink():
            return p.pid
        time.sleep(.02)
    raise RuntimeError('network namespace did not start')

def connect(pid, address, port, allowed):
    code = '''import socket,sys
try:
 s=socket.create_connection((sys.argv[1],int(sys.argv[2])),.8)
 s.close()
 ok=True
except OSError:
 ok=False
assert ok == (sys.argv[3]=='yes'), (sys.argv[1:],ok)
'''
    run(*ns(pid, python, '-c', code, address, str(port), 'yes' if allowed else 'no'))

server = '''import socket,threading,time

def handle(c):
 try:
  while data:=c.recv(128): c.sendall(data)
 except OSError: pass
 finally: c.close()

def serve(port):
 s=socket.socket(socket.AF_INET6);s.setsockopt(socket.IPPROTO_IPV6,socket.IPV6_V6ONLY,0)
 s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);s.bind(('::',port));s.listen(32)
 while True:
  c,_=s.accept();threading.Thread(target=handle,args=(c,),daemon=True).start()
for port in (22,11100,5900): threading.Thread(target=serve,args=(port,),daemon=True).start()
time.sleep(300)
'''
try:
    run('ip', 'link', 'set', 'lo', 'up')
    run('ip', 'link', 'add', 'br-test', 'type', 'bridge')
    run('ip', 'addr', 'add', '10.77.0.99/24', 'dev', 'br-test')
    run('ip', '-6', 'addr', 'add', 'fd77::99/64', 'dev', 'br-test', 'nodad')
    run('ip', 'link', 'set', 'br-test', 'up')
    client, peer = new_namespace(), new_namespace()
    for pid, suffix, offset in ((client, 'client', 1), (peer, 'peer', 2)):
        outer, inner = f'v-{suffix}', f'i-{suffix}'
        run('ip', 'link', 'add', outer, 'type', 'veth', 'peer', 'name', inner)
        run('ip', 'link', 'set', outer, 'master', 'br-test')
        run('ip', 'link', 'set', outer, 'up')
        run('ip', 'link', 'set', inner, 'netns', str(pid))
        run(*ns(pid, 'ip', 'link', 'set', inner, 'name', 'lab0'))
        run(*ns(pid, 'ip', 'addr', 'add', f'10.77.0.{offset}/24', 'dev', 'lab0'))
        run(*ns(pid, 'ip', '-6', 'addr', 'add', f'fd77::{offset}/64', 'dev', 'lab0', 'nodad'))
        run(*ns(pid, 'ip', 'link', 'set', 'lo', 'up'))
        run(*ns(pid, 'ip', 'link', 'set', 'lab0', 'up'))
    spawn(*ns(client, python, '-c', server))
    time.sleep(.3)
    # Prove all listeners and both IP families work before applying policy.
    for port in (22, 11100, 5900):
        connect(peer, '10.77.0.1', port, True)
        connect(peer, 'fd77::1', port, True)
    old = spawn(*ns(peer, python, '-u', '-c', '''import socket,sys
s=socket.create_connection(('10.77.0.1',11100),1)
print('connected',flush=True)
sys.stdin.readline();s.settimeout(.8)
try:
 s.sendall(b'test');data=s.recv(4)
except OSError: data=b''
assert data!=b'test', 'pre-existing peer connection bypassed the source guard'
'''), stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True)
    assert old.stdout.readline().strip() == 'connected'
    run(*ns(client, nft, '-c', '-f', rules))
    run(*ns(client, nft, '-f', rules))
    old.stdin.write('check\n');old.stdin.flush()
    assert old.wait(timeout=3) == 0
    for port in (22, 11100):
        connect(os.getpid(), '10.77.0.1', port, True)
        connect(peer, '10.77.0.1', port, False)
        connect(os.getpid(), 'fd77::1', port, False)
        connect(peer, 'fd77::1', port, False)
        connect(client, '127.0.0.1', port, True)
    connect(os.getpid(), '10.77.0.1', 5900, False)
    connect(peer, '10.77.0.1', 5900, False)
    connect(client, '127.0.0.1', 5900, True)
    print('PASS: master IPv4 allowed; peer IPv4 and all remote IPv6 denied; old peer connection revoked; loopback retained; external VNC denied')
    if internet_rules:
        # Public-address endpoints live only in this isolated test namespace.
        run('ip', 'addr', 'add', '203.0.113.99/32', 'dev', 'br-test')
        run('ip', '-6', 'addr', 'add', '2001:db8::99/128', 'dev', 'br-test', 'nodad')
        run(*ns(client, 'ip', 'route', 'add', 'default', 'via', '10.77.0.99'))
        run(*ns(client, 'ip', '-6', 'route', 'add', 'default', 'via', 'fd77::99'))
        spawn(python, '-c', server.replace('(22,11100,5900)', '(443,)'))
        time.sleep(.3)
        for address in ('203.0.113.99', '2001:db8::99'):
            connect(client, address, 443, True)
        existing = spawn(*ns(client, python, '-u', '-c', '''import socket,sys
s=socket.create_connection(('203.0.113.99',443),1)
s.sendall(b'ping');assert s.recv(4)==b'ping'
print('connected',flush=True);sys.stdin.readline();s.settimeout(.8)
try:
 s.sendall(b'test');data=s.recv(4)
except OSError: data=b''
assert data!=b'test', 'existing Internet connection bypassed the block'
'''), stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True)
        assert existing.stdout.readline().strip() == 'connected'
        run(*ns(client, nft, '-c', '-f', internet_rules))
        run(*ns(client, nft, '-f', internet_rules))
        existing.stdin.write('check\n');existing.stdin.flush()
        assert existing.wait(timeout=3) == 0
        for address in ('203.0.113.99', '2001:db8::99'):
            connect(client, address, 443, False)
        connect(client, '10.77.0.99', 443, True)
        for port in (22, 11100):
            connect(os.getpid(), '10.77.0.1', port, True)
        # A normal firewall reload must retain the independently owned block.
        reload_rules = ('delete table inet base_firewall\n'
                        'delete table inet nixorium_client_access\n' + pathlib.Path(rules).read_text())
        run(*ns(client, nft, '-f', '-'), input=reload_rules)
        connect(client, '203.0.113.99', 443, False)
        connect(os.getpid(), '10.77.0.1', 11100, True)
        run(*ns(client, nft, 'delete', 'table', 'inet', 'nixorium_internet'))
        for address in ('203.0.113.99', '2001:db8::99'):
            connect(client, address, 443, True)
        connect(peer, '10.77.0.1', 11100, False)
        print('PASS: Internet IPv4/IPv6 and established traffic blocked; LAN/SSH/Veyon retained; firewall reload retains block; unblock restores Internet only')
finally:
    for p in reversed(children):
        if p.poll() is None:
            p.terminate()
    for p in reversed(children):
        try: p.wait(timeout=2)
        except subprocess.TimeoutExpired: p.kill();p.wait()
