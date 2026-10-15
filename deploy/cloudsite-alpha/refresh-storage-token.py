"""Renew only the existing restricted storage APPLICATION login, not 115."""
import base64,ipaddress,json,os,stat,subprocess,sys,time,urllib.request
from pathlib import Path
ROOT=Path('/opt/my-servers/cloudsiteV1.0.0alpha/private')
def validate_identity(value,expected=None):
 if expected is None:expected={'id':5,'base_path':'/115网盘/CloudSite','permission':0}
 return str(value.get('id'))==str(expected.get('id')) and value.get('base_path')==expected.get('base_path') and value.get('permission')==expected.get('permission') and not value.get('disabled',False)
def valid_token(value,now):
 try:
  if not isinstance(value,str) or not value or len(value)>8192 or '\r' in value or '\n' in value:return False
  parts=value.split('.')
  if len(parts)!=3:return False
  expiry=json.loads(base64.urlsafe_b64decode(parts[1]+'==='))['exp']
  return type(expiry) is int and expiry>now+3600
 except Exception:return False
def run():
 os.umask(0o077)
 path=ROOT/'storage-reader.json';info=path.lstat()
 if not stat.S_ISREG(info.st_mode) or info.st_uid!=0 or info.st_mode&0o077:raise ValueError()
 credential=json.loads(path.read_text())
 container=credential.get('container_name','alist')
 if container not in ['alist','openlist']:raise ValueError()
 raw=subprocess.check_output(['docker','inspect',container,'--format','{{json .NetworkSettings.Networks}}'])
 ip=json.loads(raw)['app-net']['IPAddress']
 if not ipaddress.ip_address(ip).is_private:raise ValueError()
 base='http://'+ip+':5244'
 body=json.dumps({'username':credential['username'],'password':credential['password']}).encode()
 request=urllib.request.Request(base+'/api/auth/login',data=body,headers={'Content-Type':'application/json'})
 with urllib.request.urlopen(request,timeout=20) as response:result=json.load(response)
 if result['code']!=200:raise ValueError()
 token=result['data']['token']
 if not valid_token(token,time.time()):raise ValueError()
 with urllib.request.urlopen(urllib.request.Request(base+'/api/me',headers={'Authorization':token}),timeout=20) as response:me=json.load(response)
 if me['code']!=200 or not validate_identity(me['data'],credential.get('expected_identity')):raise ValueError()
 target=ROOT/'product/content.token';parent=target.parent.stat()
 if parent.st_uid!=10001 or stat.S_IMODE(parent.st_mode)!=0o700:raise ValueError()
 staged=target.parent/('token-'+str(os.getpid())+'.staged')
 with staged.open('x') as f:f.write(token);f.flush();os.fsync(f.fileno())
 os.chown(staged,10001,10001);os.replace(staged,target)
 print('cloudsite restricted storage application token renewed')
if __name__=='__main__':
 try:run()
 except Exception:
  sys.stderr.write('cloudsite storage application token renewal failed; existing token retained\n');sys.exit(1)
