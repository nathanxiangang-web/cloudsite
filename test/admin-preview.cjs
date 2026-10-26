'use strict';
// Isolated browser fixture, never embedded or used as a product fallback.
// Run: node test/admin-preview.cjs. Loopback only, ephemeral port, no secrets/DB.
const http=require('node:http'),fs=require('node:fs'),path=require('node:path');
const {principal,descriptor,web}=require('./admin-dom.cjs');
const binding=descriptor.bindings[0].binding_id;
const file={binding_id:binding,resource_id:'fixture-resource-001',name:'部署说明.md',path:'/文档/部署说明.md',directory:false,size_bytes:7530,presence:'PRESENT',copy_id:'fixture-copy-001',copy_availability:'PRESENT',asset_id:null,release_id:null,variant_id:null};
const task={manifest_id:'97000000-0000-4000-8000-000000000005',job_id:'97000000-0000-4000-8000-000000000006',manifest_state:'AWAITING_CANONICAL',job_state:'RETRY_WAIT',result_copy_id:null,claim_attempts:3,failure_count:0,created_at:'2026-10-21T00:00:00Z',updated_at:'2026-10-21T00:01:00Z'};
const users=[{user_id:principal.user_id,login:'fixture-admin',role:'ADMIN',enabled:true,created_at:task.created_at,updated_at:task.updated_at},{user_id:'98000000-0000-4000-8000-000000000002',login:'fixture-user',role:'USER',enabled:false,created_at:task.created_at,updated_at:task.updated_at}];
// Explicit test-only mode for Round1 logout review. One synthetic503, then204.
const logoutReview=process.argv.includes('--logout-review');let sessionValid=true,logoutCalls=0;
const shareReview=process.argv.includes('--share-review'),shareUncertain=process.argv.includes('--share-uncertain');
const shareDTO=require('./fixtures/admin-share-directory.json'),shareRevoked=new Set(),shareActions=new Map();
const fixtureUUID=/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;let shareWrites=0;
const server=http.createServer((req,res)=>{
 res.setHeader('Cache-Control','no-store');res.setHeader('X-CloudSite-Test-Fixture','1');
 res.setHeader('Content-Security-Policy',"default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'self'; frame-ancestors 'none'; object-src 'none'");
 const url=new URL(req.url,'http://127.0.0.1');
 const json=value=>{res.setHeader('Content-Type','application/json');res.end(JSON.stringify(value));};
 if(url.pathname==='/favicon.ico'){res.statusCode=204;return res.end();}
 const revoke=url.pathname.match(/^\/api\/v1\/admin\/shares\/([0-9a-f-]+)\/revoke$/);
 if(shareReview&&req.method==='POST'&&revoke){
  if(req.headers.origin!=='http://127.0.0.1:'+server.address().port){res.statusCode=403;return json({code:'ORIGIN_REJECTED'});}
  let body='';req.on('data',chunk=>{body+=chunk;if(body.length>4096)req.destroy();});req.on('end',()=>{
   let value;try{value=JSON.parse(body);}catch{res.statusCode=400;return json({code:'INVALID_REQUEST'});}
   if(Object.keys(value).length!==1||!fixtureUUID.test(value.request_id)||!fixtureUUID.test(revoke[1])){res.statusCode=400;return json({code:'INVALID_REQUEST'});}
   const stored=shareActions.get(value.request_id);if(stored){if(stored.target_id!==revoke[1]){res.statusCode=409;return json({code:'ACTION_CONFLICT'});}return json(stored);}
   const exists=shareDTO.items.some(item=>item.share_id===revoke[1]),already=shareRevoked.has(revoke[1]);
   const outcome={request_id:value.request_id,target_id:revoke[1],action:'REVOKE_SHARE',status:exists&&!already?'SUCCEEDED':'REJECTED',code:!exists?'SHARE_NOT_FOUND':already?'SHARE_ALREADY_REVOKED':'SHARE_REVOKED',recorded_at:new Date().toISOString()};
   if(exists)shareRevoked.add(revoke[1]);shareActions.set(value.request_id,outcome);shareWrites++;console.log('Fixture share revoke POST #'+shareWrites);
   if(shareUncertain&&shareWrites===1){res.statusCode=503;return json({code:'ACTION_UNCONFIRMED'});}json(outcome);
  });return;
 }
 if(logoutReview&&req.method==='POST'&&url.pathname==='/api/v1/auth/logout'){
  logoutCalls++;console.log('Fixture logout POST #'+logoutCalls);
  if(logoutCalls===1){res.statusCode=503;return json({code:'AUTH_UNAVAILABLE'});}
  sessionValid=false;res.statusCode=204;return res.end();
 }
 if(req.method!=='GET'){res.statusCode=405;return json({code:'FIXTURE_READ_ONLY'});}
 if(url.pathname==='/api/v1/auth/me'){
  if(logoutReview)console.log('Fixture auth/me GET');
  if(!sessionValid){res.statusCode=401;return json({code:'AUTH_REQUIRED'});}
  return json({...principal,login:'fixture-admin'});
 }
 if(url.pathname==='/api/v1/admin/workspace')return json({...descriptor,schema_version:shareReview?18:descriptor.schema_version,admin_reads_enabled:true,bindings:[{binding_id:binding,label:'隔离浏览器测试数据'}]});
 if(shareReview&&url.pathname==='/api/v1/admin/shares'){
  const owner=url.searchParams.get('owner'),status=url.searchParams.get('status')||'ALL',cursor=url.searchParams.get('cursor')||'',limit=Number(url.searchParams.get('limit')||20);
  const items=shareDTO.items.map(item=>({...item,active:item.active&&!shareRevoked.has(item.share_id)})).filter(item=>(!owner||item.owner_id===owner)&&(status==='ALL'||status==='ACTIVE'&&item.active||status==='REVOKED'&&shareRevoked.has(item.share_id)||status==='EXPIRED'&&!item.active&&!shareRevoked.has(item.share_id)));
  const visible=items.filter(item=>item.share_id>cursor).slice(0,limit+1),more=visible.length>limit;
  if(more)visible.pop();return json({items:visible,total:items.length,next_cursor:more?visible.at(-1).share_id:'',read_at:new Date().toISOString(),scope:'FILTERED_PRODUCT_SHARES'});
 }
 if(shareReview&&url.pathname.startsWith('/api/v1/admin/actions/')){const value=shareActions.get(url.pathname.split('/').at(-1));if(!value){res.statusCode=404;return json({code:'ACTION_NOT_FOUND'});}return json(value);}
 if(url.pathname==='/api/v1/admin/overview')return json(require('./fixtures/admin-overview.json'));
 if(url.pathname==='/api/v1/admin/users'){const limit=Number(url.searchParams.get('limit')||20);return json({items:users.slice(0,limit),next_cursor:limit<users.length?users[limit-1].user_id:'',total:users.length,read_at:'2026-10-26T03:00:00Z',scope:'FILTERED_PRODUCT_USERS'});}
 if(url.pathname==='/api/v1/admin/storage')return json({items:[{binding_id:binding,display_label:'隔离配置示例',connection_id:'99000000-0000-4000-8000-000000000002',provider_type:'115',binding_config_status:'ACTIVE',connection_config_status:'DISABLED',binding_updated_at:task.updated_at,connection_updated_at:task.updated_at,indexcore_root_configured:true}],next_cursor:''});
 if(url.pathname==='/api/v1/admin/indexcore')return json({binding_id:binding,root_lifecycle_state:'ACTIVE',current_generation:7,last_applied_admission_seq:null,observed_at:new Date().toISOString(),evidence_source:'INDEXCORE_Q9_ROOT_STATUS'});
 if(url.pathname==='/api/v1/admin/downloads'){
  const all=[{...task,owner_user_id:principal.user_id,binding_id:binding},{...task,manifest_id:'98000000-0000-4000-8000-000000000007',job_id:'98000000-0000-4000-8000-000000000008',owner_user_id:users[1].user_id,binding_id:binding,manifest_state:'RECOVERY_REQUIRED',job_state:'RECOVERY_REQUIRED',claim_attempts:9,failure_count:2}];
  return json({items:all.filter(item=>(!url.searchParams.get('user_id')||item.owner_user_id===url.searchParams.get('user_id'))&&(!url.searchParams.get('status')||url.searchParams.get('status')==='ALL'||item.manifest_state===url.searchParams.get('status'))),next_cursor:''});
 }
 if(url.pathname.startsWith('/api/v1/admin/downloads/')){
  const id=url.pathname.split('/').at(-1),fixture=require('./fixtures/admin-download-detail.json');
  if(id===task.manifest_id)return json({...fixture,...task,owner_user_id:principal.user_id,binding_id:binding,next_attempt_at:task.updated_at,job_created_at:task.created_at,job_updated_at:task.updated_at,started_at:task.created_at});
  if(id==='98000000-0000-4000-8000-000000000007')return json({...fixture,manifest_id:id,job_id:'98000000-0000-4000-8000-000000000008',owner_user_id:users[1].user_id});
  res.statusCode=404;return json({code:'DOWNLOAD_NOT_FOUND'});
 }
 if(url.pathname==='/api/v1/resources/resolve')return json({...file,path:url.searchParams.get('path')});
 if(url.pathname==='/api/v1/resources')return json({items:[{...file,resource_id:'fixture-folder',name:'文档',path:'/文档',directory:true,size_bytes:null,copy_id:null},file,{...file,resource_id:'fixture-video',name:'示例视频.mp4',path:'/示例视频.mp4',size_bytes:2147483648,copy_id:null}],next_cursor:''});
 if(url.pathname==='/api/v1/tasks')return json({items:[task],next_cursor:''});
 if(url.pathname==='/api/v1/tasks/'+task.manifest_id)return json(task);
 const assets={'/admin/':['index.html','text/html'],'/assets/app.css':['app.css','text/css'],'/assets/app.js':['app.js','text/javascript'],'/assets/admin-lists.js':['admin-lists.js','text/javascript'],'/assets/admin-observe.js':['admin-observe.js','text/javascript'],'/assets/admin-operations.js':['admin-operations.js','text/javascript'],'/assets/resource-display.js':['resource-display.js','text/javascript']};
 assets['/assets/admin-shares.js']=['admin-shares.js','text/javascript'];
 assets['/assets/admin-overview.js']=['admin-overview.js','text/javascript'];
 assets['/assets/admin-download-detail.js']=['admin-download-detail.js','text/javascript'];
 const asset=assets[url.pathname];if(!asset){res.statusCode=404;return res.end();}
 res.setHeader('Content-Type',asset[1]+'; charset=utf-8');let data=fs.readFileSync(path.join(web,asset[0]),'utf8');
 if(asset[0]==='index.html')data=data.replace('<body>','<body><aside class="scope-banner" aria-label="测试数据标记">ISOLATED UI FIXTURE · 所有记录为测试数据 · 不连接真实网盘、数据库或生产服务</aside>');
 res.end(data);
});
server.listen(0,'127.0.0.1',()=>console.log('Admin UI fixture: http://127.0.0.1:'+server.address().port+'/admin/'));
for(const signal of ['SIGINT','SIGTERM'])process.on(signal,()=>server.close());
