'use strict';
// Homepage read model. No commands, provider calls or sampled task counts.
const AdminOverview={create({present,friendlyInfo,el,read,generation,principal,isActive,dateText}){
 const manifestFields=['pending','active','awaiting_visibility','awaiting_canonical','ready','failed','canceled','recovery_required'];
 const jobFields=['queued','running','retry_wait','succeeded','failed','canceled','recovery_required'];
 let request=0,controller;
 const stamp=value=>typeof value==='string'&&/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(value)&&!value.startsWith('0000-')&&Number.isFinite(Date.parse(value))&&Date.parse(value)!==Date.parse('0001-01-01T00:00:00Z')&&new Date(value.slice(0,10)+'T00:00:00Z').toISOString().slice(0,10)===value.slice(0,10);
 const count=value=>Number.isSafeInteger(value)&&value>=0;
 function sum(values){let total=0;for(const value of values){if(!count(value)||value>Number.MAX_SAFE_INTEGER-total)throw Error('Invalid count');total+=value;}return total;}
 function validate(dto){
  if(!dto||dto.source!=='CLOUDSITE_POSTGRES_MANIFESTS_AND_JOBS'||dto.scope!=='PRODUCT_ACQUISITIONS_IN_CONFIGURED_BINDINGS'||dto.owner_user_id!==undefined&&dto.owner_user_id!==''||!stamp(dto.read_at))throw Error('Invalid overview');
  const m={},j={};for(const key of ['total','incomplete',...manifestFields]){if(!count(dto.manifests?.[key]))throw Error('Invalid manifest');m[key]=dto.manifests[key];}
  for(const key of ['total',...jobFields]){if(!count(dto.jobs?.[key]))throw Error('Invalid job');j[key]=dto.jobs[key];}
  m.canceled_without_job=Object.hasOwn(dto.manifests,'canceled_without_job')?dto.manifests.canceled_without_job:0;
  if(!count(m.canceled_without_job)||m.canceled_without_job>m.canceled)throw Error('Invalid pre-activation cancellation count');
  if(sum(manifestFields.map(key=>m[key]))!==m.total||sum(jobFields.map(key=>j[key]))!==j.total||sum([m.pending,m.active,m.awaiting_visibility,m.awaiting_canonical])!==m.incomplete||j.total!==m.total-m.pending-m.canceled_without_job||j.queued>m.active||j.succeeded!==m.ready||j.canceled!==m.canceled-m.canceled_without_job||['failed','recovery_required'].some(key=>j[key]!==m[key]))throw Error('Inconsistent overview');
  return {manifests:m,jobs:j,read_at:dto.read_at};
 }
 function clearTasks(){for(const key of ['total','incomplete','canceled_without_job',...manifestFields])el('overview-manifests-'+key).textContent='—';for(const key of ['total',...jobFields])el('overview-jobs-'+key).textContent='—';el('overview-read-at').textContent='尚未取得有效数据库读取时间';}
 function clearUsers(){el('overview-users-total').textContent='—';el('overview-users-read-at').textContent='尚未取得有效数据库读取时间';}
 function status(id,text,kind='neutral'){present(el(id),friendlyInfo(text),text!==friendlyInfo(text)?text:'');el(id).className='badge '+kind;}
 function invalidate(){request++;controller?.abort();controller=null;clearTasks();clearUsers();status('overview-status','等待查询');status('overview-users-status','等待查询');el('overview-panel').setAttribute('aria-busy','false');}
 async function load(background=false){
  if(principal()?.role!=='ADMIN'||!background&&!isActive())return;
  invalidate();controller=new AbortController();const signal=controller.signal,seq=request,gen=generation(),user=principal().user_id;
  const current=()=>!signal.aborted&&seq===request&&gen===generation()&&(background||isActive())&&principal()?.role==='ADMIN'&&principal()?.user_id===user;
  clearTasks();clearUsers();status('overview-status','LOADING · 正在读取');status('overview-users-status','LOADING · 正在读取');el('overview-panel').setAttribute('aria-busy','true');
  await Promise.all([
   (async()=>{try{
    const value=await read('/api/v1/admin/overview',gen,signal);if(!current())return;const dto=validate(value);
    for(const [kind,fields] of [['manifests',dto.manifests],['jobs',dto.jobs]])for(const [key,value] of Object.entries(fields))el('overview-'+kind+'-'+key).textContent=String(value);
    el('overview-read-at').textContent='数据库读取于 '+dateText(dto.read_at);
    status('overview-status',dto.manifests.recovery_required?'已读取 · 有任务需要人工处理':dto.manifests.failed?'已读取 · 存在失败记录':'已更新',dto.manifests.recovery_required||dto.manifests.failed?'warning':'neutral');
   }catch{if(current()){clearTasks();status('overview-status','UNAVAILABLE · 任务统计暂不可用','warning');}}})(),
   (async()=>{try{
    const page=await read('/api/v1/admin/users?limit=1',gen,signal);if(!current())return;
    if(!page||page.scope!=='FILTERED_PRODUCT_USERS'||!stamp(page.read_at)||!count(page.total)||!Array.isArray(page.items)||page.items.length>1||page.total<page.items.length||typeof page.next_cursor!=='string'||page.next_cursor!==''&&!/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(page.next_cursor))throw Error('Invalid user aggregate');
    el('overview-users-total').textContent=String(page.total);el('overview-users-read-at').textContent='数据库读取于 '+dateText(page.read_at);status('overview-users-status','已读取 · 产品账号总数');
   }catch{if(current()){clearUsers();status('overview-users-status','UNAVAILABLE · 用户统计暂不可用','warning');}}})()
  ]);
  if(current())el('overview-panel').setAttribute('aria-busy','false');
 }
 return {load,invalidate,reset:invalidate};
}};
