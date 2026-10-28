'use strict';
// One audited intent at a time. No provider ports, command retry or disk storage.
const AdminPendingCancel={create({present,el,read,command,readExact,generation,principal,isActive,bindingAllowed,capable,onTerminal,onMissing,onError,newID,now=()=>Date.now()}){
 let selected=null,observed=0,confirmation=null,intent=null,busy=false,preparing=false,seq=0,prepareSeq=0,expiry,controller;
 const context=()=>({gen:generation(),actor:principal()?.user_id});
 let session=context();
 const same=c=>c.gen===generation()&&c.actor===principal()?.user_id&&principal()?.role==='ADMIN';
 const active=c=>same(c)&&isActive();
 const uuid=AdminDownloadDetail.uuid;
 const fresh=d=>d&&AdminDownloadDetail.valid(d)&&d.manifest_state==='PENDING'&&d.job_id===null&&d.max_attempts===0&&d.copy===null&&!d.error_recorded&&d.error_code==='NONE'&&bindingAllowed(d.binding_id)&&AdminDownloadDetail.time(d.updated_at)&&/T\d{2}:\d{2}:\d{2}(?:\.\d{1,6})?(?:Z|[+-]\d{2}:\d{2})$/.test(d.updated_at);
 const unresolved=()=>intent&&!intent.terminal;
 function controls(){
  const allowed=active(session)&&capable()&&fresh(selected)&&now()-observed<30000;
  el('pending-cancel-open').hidden=!allowed;
  el('pending-cancel-open').disabled=!allowed||busy||preparing||Boolean(unresolved());
  el('pending-cancel-confirm').disabled=busy||!confirmation;
  el('pending-cancel-lookup').disabled=busy||!unresolved()||!active(session)||!bindingAllowed(intent?.binding);
  el('pending-cancel-action').hidden=!intent;
 }
 function dismiss(restore=true){
  confirmation=null;el('pending-cancel-dialog').hidden=true;document.body.classList.toggle('pending-confirm-open',false);
  if(restore&&!el('pending-cancel-open').hidden)el('pending-cancel-open').focus();
  controls();
 }
 function reset(){
  seq++;prepareSeq++;controller?.abort();clearTimeout(expiry);selected=null;confirmation=null;intent=null;busy=false;preparing=false;session=context();
  dismiss(false);el('pending-cancel-status').textContent='';el('pending-cancel-request').textContent='—';el('pending-cancel-target').textContent='—';controls();
 }
 function boundary(){if(!same(session))reset();}
 function observe(d){
  boundary();if(busy){seq++;controller?.abort();busy=false;if(unresolved())el('pending-cancel-status').textContent='结果未确认 · 请查询原审计回执，不会重复发送取消命令。';}
  dismiss(false);clearTimeout(expiry);selected=d?{...d}:null;observed=now();
  if(d)expiry=setTimeout(()=>{selected=null;dismiss(false);controls();},30000);
  controls();
 }
 function invalidate(){
  boundary();seq++;prepareSeq++;controller?.abort();busy=false;preparing=false;observe(null);
  if(unresolved())el('pending-cancel-status').textContent='结果未确认 · 请查询原审计回执，不会重复发送取消命令。';
 }
 async function open(){
  boundary();controls();if(el('pending-cancel-open').disabled||el('pending-cancel-open').hidden)return;
  const c=context(),target=selected.manifest_id,request=++prepareSeq;preparing=true;controls();
  try{
   // Never derive a command tuple from a list, stale detail or browser clock.
   const d=await readExact(target);
   if(request!==prepareSeq||!active(c)||!capable()||!fresh(d)||selected?.manifest_id!==target)return;
   confirmation={...c,target,owner:d.owner_user_id,binding:d.binding_id,updated:d.updated_at,at:now()};
   el('pending-cancel-confirm-target').textContent=target;el('pending-cancel-dialog').hidden=false;
   document.body.classList.toggle('pending-confirm-open',true);el('pending-cancel-dismiss').focus();
  }finally{if(request===prepareSeq){preparing=false;controls();}}
 }
 function validReceipt(r){
  const codes={SUCCEEDED:['PENDING_CANCELED'],REJECTED:['PENDING_CHANGED','MANIFEST_NOT_FOUND','OWNER_UNAVAILABLE','PENDING_NOT_ISOLATED','MANIFEST_UNAVAILABLE'],UNKNOWN:['RESERVED_NO_CONFIRMED_RESULT']};
  if(!r||r.action!=='CANCEL_PENDING_MANIFEST'||r.request_id!==intent?.request_id||r.target_id!==intent?.target_id||!codes[r.status]?.includes(r.code)||!AdminDownloadDetail.time(r.recorded_at))throw Error('Invalid cancellation receipt');
  if(r.manifest&&(!Object.hasOwn(AdminDownloadDetail.labels,r.manifest.state)||r.manifest.job_id!==null&&!uuid(r.manifest.job_id)||!AdminDownloadDetail.time(r.manifest.updated_at)))throw Error('Invalid manifest receipt');
  if(r.status==='SUCCEEDED'&&(!r.manifest||r.manifest.state!=='CANCELED'||r.manifest.job_id!==null))throw Error('Invalid cancellation success');
  return r;
 }
 function receipt(r){
  validReceipt(r);intent.terminal=r.status!=='UNKNOWN';
  const messages={PENDING_CANCELED:'取消已记录；任务详情以服务器重新读取为准',PENDING_CHANGED:'请求已变化，未执行此次取消',MANIFEST_NOT_FOUND:'任务不存在或不在管理范围',OWNER_UNAVAILABLE:'所属用户不可用，未执行取消',PENDING_NOT_ISOLATED:'已有执行或准入证据，不能取消',MANIFEST_UNAVAILABLE:'任务关联异常，不能取消',RESERVED_NO_CONFIRMED_RESULT:'结果未确认，请查询原审计回执，不要重复提交'};
  present(el('pending-cancel-status'),messages[r.code],r.status+' / '+r.code,'原始回执');
  return r.status!=='UNKNOWN';
 }
 async function operation(lookup=false){
  boundary();if(!active(session)||busy)return;
  if(lookup){if(!unresolved())return;}
  else{
   const c=confirmation;
   if(unresolved()||!c||!active(c)||!capable()||!fresh(selected)||selected.manifest_id!==c.target||selected.updated_at!==c.updated||selected.owner_user_id!==c.owner||selected.binding_id!==c.binding||now()-c.at>=30000){dismiss();return;}
   const request_id=newID();if(!uuid(request_id)){dismiss();return;}
   intent={request_id,target_id:c.target,owner:c.owner,binding:c.binding,updated:c.updated,terminal:false};dismiss(false);
  }
  const c=context(),request=++seq,original={...intent};busy=true;controller=new AbortController();const requestController=controller,signal=requestController.signal;
  let permit=true,timer,terminal=false,missing=false,conflict=false;
  const current=()=>request===seq&&active(c)&&intent?.request_id===original.request_id&&bindingAllowed(original.binding);
  const guard=()=>permit&&current()&&capable();
  el('pending-cancel-request').textContent=original.request_id;el('pending-cancel-target').textContent=original.target_id;
  el('pending-cancel-status').textContent=lookup?'正在查询原审计回执…':'正在提交取消 · 结果尚未确认';controls();el('pending-cancel-action').focus();el('pending-cancel-action').scrollIntoView({block:'nearest'});
  try{
   const work=lookup?read('/api/v1/admin/actions/'+original.request_id,c.gen,signal):command('/api/v1/admin/downloads/'+original.target_id+'/cancel-pending',{request_id:original.request_id,expected_owner_user_id:original.owner,expected_binding_id:original.binding,expected_updated_at:original.updated},c.gen,signal,guard);
   const r=await Promise.race([work,new Promise((_,reject)=>{timer=setTimeout(()=>{requestController.abort();reject(Error('Cancellation deadline'));},15000);})]);
   if(!current())return;terminal=receipt(r);missing=r.code==='MANIFEST_NOT_FOUND';
  }catch(error){
   if(current()){
    if(!lookup&&[409,404].includes(error.status)){
     conflict=true;
     try{terminal=receipt(error.receipt);missing=error.status===404;}catch{el('pending-cancel-status').textContent='冲突或范围已变化 · 结果未确认，只能查询原审计回执。';missing=error.status===404;}
    }else el('pending-cancel-status').textContent='结果未确认 · 请查询原审计回执，不会重复发送取消命令。';
    if(error.code==='AUTH_REQUIRED'||error.code==='ADMIN_REQUIRED'||error.code==='AUTH_CHANGED')onError(error);
   }
  }finally{permit=false;clearTimeout(timer);if(request===seq&&same(c)){busy=false;controls();}}
  if(!current())return;
  if(missing)onMissing();
  if(terminal||conflict)await onTerminal(original.target_id,missing);
 }
 function key(event){
  if(el('pending-cancel-dialog').hidden)return false;
  if(event.key==='Escape'){event.preventDefault?.();dismiss();return true;}
  if(event.key==='Tab'){
   const first=el('pending-cancel-dismiss'),last=el('pending-cancel-confirm');
   if(event.shiftKey&&document.activeElement===first){event.preventDefault();last.focus();}
   else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first.focus();}
   return true;
  }
  return false;
 }
 el('pending-cancel-open').addEventListener('click',open);
 el('pending-cancel-dismiss').addEventListener('click',()=>dismiss());
 el('pending-cancel-confirm').addEventListener('click',()=>operation(false));
 el('pending-cancel-lookup').addEventListener('click',()=>operation(true));
 return {observe,invalidate,reset,open,key,controls};
}};
