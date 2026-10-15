import base64,importlib.util,json,time,unittest
from pathlib import Path
DEPLOY=Path(__file__).resolve().parents[1]/'deploy/cloudsite-alpha'
spec=importlib.util.spec_from_file_location('refresh_storage',DEPLOY/'refresh-storage-token.py');refresh=importlib.util.module_from_spec(spec);spec.loader.exec_module(refresh)
class DeploymentSafety(unittest.TestCase):
 def test_token_expiry_and_scope_are_checked(self):
  now=100000;payload=base64.urlsafe_b64encode(json.dumps({'exp':now+7200}).encode()).decode().rstrip('=');token='header.'+payload+'.signature'
  self.assertTrue(refresh.valid_token(token,now));self.assertFalse(refresh.valid_token(token,now+7200))
  for bad in ['',token+'\n','a.b.c','secret','a.'+base64.urlsafe_b64encode(b'{"exp":true}').decode()+'.b']:self.assertFalse(refresh.valid_token(bad,now))
  me={'id':5,'base_path':'/115网盘/CloudSite','permission':0,'disabled':False};self.assertTrue(refresh.validate_identity(me))
  for change in [{'id':2},{'base_path':'/'},{'permission':65535},{'disabled':True}]:self.assertFalse(refresh.validate_identity(dict(me,**change)))
 def test_private_deployment_has_no_host_ports_and_defaults_route_off(self):
  raw=(DEPLOY/'cloudsiteV1.0.0alpha.yml').read_text()
  self.assertNotIn('ports:',raw);self.assertIn('${CLOUDSITE_TRAEFIK_ENABLED:-false}',raw);self.assertEqual(raw.count('network_mode: service:edge'),2)
  self.assertIn('PANTA_ACQUISITION_WORKER_ENABLED: "false"',raw);self.assertIn('data: {internal: true}',raw)
  for name in ['catalog-entrypoint.py','refresh-storage-token.py']:compile((DEPLOY/name).read_text(),name,'exec')
