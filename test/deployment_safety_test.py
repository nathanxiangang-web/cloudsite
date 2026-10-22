import base64,importlib.util,json,time,unittest,io,subprocess,tarfile
from pathlib import Path
DEPLOY=Path(__file__).resolve().parents[1]/'deploy/cloudsite-alpha'
spec=importlib.util.spec_from_file_location('refresh_storage',DEPLOY/'refresh-storage-token.py');refresh=importlib.util.module_from_spec(spec);spec.loader.exec_module(refresh)
class DeploymentSafety(unittest.TestCase):
 def test_windows_archive_preserves_published_migration_bytes(self):
  root=DEPLOY.parents[1]
  raw=subprocess.check_output(['git','-c','core.autocrlf=true','archive','--format=tar','HEAD','migrations'],cwd=root,timeout=15)
  with tarfile.open(fileobj=io.BytesIO(raw),mode='r:') as archive:
   files=[item for item in archive.getmembers() if item.name.endswith('.sql')]
   self.assertEqual(len(files),16)  # UI-3A appends migration16; retain byte/LF checks for ALL history.
   self.assertEqual(sorted(item.name for item in files)[-1],'migrations/0016_product_admin_actions.sql')
   for item in files:
    published=subprocess.check_output(['git','cat-file','blob','HEAD:'+item.name],cwd=root,timeout=5)
    self.assertNotIn(b'\r\n',published)
    self.assertEqual(archive.extractfile(item).read(),published,item.name)
 def test_token_expiry_and_scope_are_checked(self):
  now=100000;payload=base64.urlsafe_b64encode(json.dumps({'exp':now+7200}).encode()).decode().rstrip('=');token='header.'+payload+'.signature'
  self.assertTrue(refresh.valid_token(token,now));self.assertFalse(refresh.valid_token(token,now+7200))
  for bad in ['',token+'\n','a.b.c','secret','a.'+base64.urlsafe_b64encode(b'{"exp":true}').decode()+'.b']:self.assertFalse(refresh.valid_token(bad,now))
  me={'id':5,'base_path':'/115网盘/CloudSite','permission':0,'disabled':False};self.assertTrue(refresh.validate_identity(me))
  for change in [{'id':2},{'base_path':'/'},{'permission':65535},{'disabled':True}]:self.assertFalse(refresh.validate_identity(dict(me,**change)))
  local={'id':3,'base_path':'/115open/CloudSite','permission':65341,'disabled':False}
  expected={k:local[k] for k in ['id','base_path','permission']}
  self.assertTrue(refresh.validate_identity(local,expected));self.assertFalse(refresh.validate_identity(local));self.assertFalse(refresh.validate_identity(dict(local,base_path='/'),expected))
 def test_private_deployment_has_no_host_ports_and_defaults_route_off(self):
  raw=(DEPLOY/'cloudsiteV1.0.0alpha.yml').read_text()
  self.assertNotIn('ports:',raw);self.assertIn('${CLOUDSITE_TRAEFIK_ENABLED:-false}',raw);self.assertEqual(raw.count('network_mode: service:edge'),2)
  self.assertIn('PANTA_ACQUISITION_WORKER_ENABLED: "false"',raw);self.assertIn('data: {internal: true}',raw)
  self.assertIn('tmpfs: ["/tmp:size=64m,mode=1777"]',raw)
  for name in ['catalog-entrypoint.py','refresh-storage-token.py']:compile((DEPLOY/name).read_text(),name,'exec')
