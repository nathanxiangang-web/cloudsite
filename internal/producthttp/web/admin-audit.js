'use strict';
// Historical product audit only. This module receives GET, never a command port.
const AdminAudit={
 actions:{SET_COPY_PRESENTATION:'修改资源展示信息',CREATE_USER:'创建用户',SET_USER_ENABLED:'启停用户',RENAME_USER:'重命名用户',RESET_USER_PASSWORD:'重置用户密码',DELETE_USER:'删除用户',RESTORE_USER:'恢复用户',REVOKE_SHARE:'撤销分享',PROBE_115:'记录 115 检测',CANCEL_PENDING_MANIFEST:'取消待提交请求'},
 statuses:{UNKNOWN:'结果未确认',SUCCEEDED:'已记录成功',REJECTED:'已拒绝'},
 codes:{SET_COPY_PRESENTATION:['COPY_PRESENTATION_SET','COPY_NOT_FOUND','COPY_UNAVAILABLE','VERSION_CONFLICT'],CREATE_USER:['USER_CREATED','CONFLICT'],SET_USER_ENABLED:['USER_STATE_CHANGED'],RENAME_USER:['USER_RENAMED','LOGIN_CONFLICT'],RESET_USER_PASSWORD:['USER_PASSWORD_RESET'],DELETE_USER:['USER_DELETED','ACTIVE_DOWNLOADS','DOWNLOAD_STATE_CONFLICT'],RESTORE_USER:['USER_RESTORED','PASSWORD_UNCHANGED'],REVOKE_SHARE:['SHARE_REVOKED','SHARE_ALREADY_REVOKED','SHARE_NOT_FOUND','TARGET_FORBIDDEN'],PROBE_115:['PROBE_RECORDED','SCOPE_REJECTED','NOT_CONNECTED','RATE_LIMITED'],CANCEL_PENDING_MANIFEST:['PENDING_CANCELED','PENDING_CHANGED','MANIFEST_NOT_FOUND','OWNER_UNAVAILABLE','PENDING_NOT_ISOLATED','MANIFEST_UNAVAILABLE']},
 create({present,rowLabels,el,read,generation,principal,isActive,capable,dateText,cell,badge,onError}){
  const source='CLOUDSITE_POSTGRES_ADMIN_ACTION_LEDGER',scope='CURRENT_ADMIN_ACTIONS_IN_CONFIGURED_BINDINGS',uuid=AdminDownloadDetail.uuid;
  const stamp=v=>AdminDownloadDetail.time(v)&&/T\d{2}:\d{2}:\d{2}(?:\.\d{1,6})?(?:Z|[+-]\d{2}:\d{2})$/.test(v);
  const epoch=v=>BigInt(Date.parse(v))*1000n+BigInt(((v.match(/\.(\d+)/)?.[1]||'')+'000000').slice(3,6));
  let seq=0,detailSeq=0,controller,detailController,cursor='',next='',key='',loaded=false,observed='',focus,target='',filter={action:'ALL',status:'ALL',from:'',before:''};
  const actor=()=>principal()?.role==='ADMIN'?principal().user_id:'';
  const current=(g,a,n)=>g===generation()&&actor()===a&&isActive()&&n===seq;
  function item(v,detail=false){
   const allowed=['actor_id','request_id','action','status','code','created_at','recorded_at','target_status','manifest_state',...(detail?['source','scope','read_at']:[])];
   if(!v||Object.keys(v).some(k=>!allowed.includes(k))||v.actor_id!==actor()||!uuid(v.request_id)||!Object.hasOwn(AdminAudit.actions,v.action)||!Object.hasOwn(AdminAudit.statuses,v.status)||!stamp(v.created_at)||!['NOT_RECORDED','REDACTED','DELETED_OR_MISSING'].includes(v.target_status))throw Error('Invalid audit item');
   const common=['TARGET_FORBIDDEN','VERSION_CONFLICT','USER_STATE_CONFLICT'];
   if(v.status==='UNKNOWN'){if(v.recorded_at!==null||v.code!=='RESERVED_NO_CONFIRMED_RESULT'||v.manifest_state!==undefined)throw Error('Invalid reservation');}
   else if(!stamp(v.recorded_at)||epoch(v.recorded_at)<epoch(v.created_at)||!AdminAudit.codes[v.action].includes(v.code)&&!(common.includes(v.code)&&['SET_USER_ENABLED','RENAME_USER','RESET_USER_PASSWORD','DELETE_USER','RESTORE_USER'].includes(v.action)))throw Error('Invalid audit result');
   const success={SET_COPY_PRESENTATION:'COPY_PRESENTATION_SET',CREATE_USER:'USER_CREATED',SET_USER_ENABLED:'USER_STATE_CHANGED',RENAME_USER:'USER_RENAMED',RESET_USER_PASSWORD:'USER_PASSWORD_RESET',DELETE_USER:'USER_DELETED',RESTORE_USER:'USER_RESTORED',REVOKE_SHARE:'SHARE_REVOKED',PROBE_115:'PROBE_RECORDED',CANCEL_PENDING_MANIFEST:'PENDING_CANCELED'};
   if(v.status==='SUCCEEDED'&&v.code!==success[v.action]||v.status==='REJECTED'&&v.code===success[v.action])throw Error('Inconsistent audit result');
   if(v.manifest_state!==undefined&&(v.action!=='CANCEL_PENDING_MANIFEST'||!Object.hasOwn(AdminDownloadDetail.labels,v.manifest_state)))throw Error('Invalid historical state');
   return v;
  }
  const targetText=v=>({NOT_RECORDED:'预留时未记录目标',REDACTED:'目标标识已脱敏',DELETED_OR_MISSING:'用户目标已删除或不存在（历史保留）'}[v]);
  function close(restore=false){detailSeq++;detailController?.abort();target='';el('audit-detail').hidden=true;el('audit-facts').replaceChildren();el('audit-detail-status').textContent='';el('audit-detail-refresh').disabled=false;if(restore)focus?.focus();}
  function clear(){loaded=false;next='';el('audit-rows').replaceChildren();el('audit-empty').hidden=true;el('audit-more').disabled=true;}
  function invalidate(){seq++;controller?.abort();close();el('audit-refresh').disabled=false;el('audit-more').disabled=true;}
  function reset(){invalidate();cursor='';key='';observed='';filter={action:'ALL',status:'ALL',from:'',before:''};clear();for(const k of ['action','status'])el('audit-'+k).value='ALL';for(const k of ['from','before'])el('audit-'+k).value='';el('audit-actor').textContent='—';el('audit-footer').textContent='等待查询';}
  async function get(path,gen,c){let timer;try{return await Promise.race([read(path,gen,c.signal),new Promise((_,reject)=>{timer=setTimeout(()=>{c.abort();reject(Error('Audit read deadline'));},15000);})]);}finally{clearTimeout(timer);}}
  async function load(){
   if(!actor()||!isActive())return;
   close();seq++;controller?.abort();controller=new AbortController();const c=controller,g=generation(),a=actor(),n=seq,k=JSON.stringify([a,filter,cursor]),previous=loaded&&k===key;
   if(!previous)clear();key=k;el('audit-actor').textContent=a;el('audit-empty').hidden=true;el('audit-refresh').disabled=true;el('audit-more').disabled=true;el('audit-footer').textContent=previous?'LOADING · 刷新中，旧页暂留':'LOADING · 正在读取持久管理操作';
   if(!capable()){clear();el('audit-footer').textContent='UNAVAILABLE · 当前环境未配置审计读取能力';el('audit-refresh').disabled=false;return;}
   const q=new URLSearchParams({limit:'20',actor_id:a,action:filter.action,status:filter.status});if(cursor)q.set('cursor',cursor);if(filter.from)q.set('from',filter.from);if(filter.before)q.set('before',filter.before);
   try{
    const p=await get('/api/v1/admin/audit/actions?'+q,g,c);if(!current(g,a,n)||c.signal.aborted)return;
    if(!p||p.source!==source||p.scope!==scope||!stamp(p.read_at)||!Array.isArray(p.items)||p.items.length>20||typeof p.next_cursor!=='string'||p.next_cursor!==''&&!/^[A-Za-z0-9_-]{1,2048}$/.test(p.next_cursor))throw Error('Invalid audit page');
    let previousItem;for(const v of p.items){item(v);if(filter.action!=='ALL'&&v.action!==filter.action||filter.status!=='ALL'&&v.status!==filter.status||filter.from&&epoch(v.created_at)<epoch(filter.from)||filter.before&&epoch(v.created_at)>=epoch(filter.before)||previousItem&&(epoch(v.created_at)>epoch(previousItem.created_at)||epoch(v.created_at)===epoch(previousItem.created_at)&&v.request_id>=previousItem.request_id))throw Error('Invalid audit ordering/filter');previousItem=v;}
    el('audit-rows').replaceChildren();for(const v of p.items){const row=document.createElement('tr'),open=document.createElement('button');open.type='button';open.className='file-button';open.textContent='记录 '+v.request_id.slice(-8);open.setAttribute('aria-label','查看操作记录 '+v.request_id);open.addEventListener('click',()=>detail(v.request_id));const identity=cell(row);identity.append(open);const full=document.createElement('div');present(full,'',v.request_id,'完整操作编号');identity.append(full);cell(row,AdminAudit.actions[v.action]);cell(row).append(badge(AdminAudit.statuses[v.status],v.status==='UNKNOWN'?'warning':'neutral'));present(cell(row),'',v.code,'查看结果码');cell(row,dateText(v.created_at));cell(row,dateText(v.recorded_at));rowLabels(row,['操作编号','操作','结果','技术结果码','请求时间','结果时间']);el('audit-rows').append(row);}
    loaded=true;next=p.next_cursor;observed=dateText(p.read_at);el('audit-empty').hidden=p.items.length!==0;el('audit-more').disabled=!next;present(el('audit-footer'),'本页 '+p.items.length+' 条记录 · 已更新','本页 '+p.items.length+' 条 · 未提供全量总数 · 数据库读取于 '+observed+' · 仅当前管理员可见范围','读取范围与时间');
   }catch(e){if(current(g,a,n)){el('audit-empty').hidden=true;el('audit-footer').textContent=previous?'STALE · 本次读取失败，保留上次同一查询结果 · 数据库读取于 '+observed:'UNAVAILABLE · 审计读取未完成，不以空记录代替失败';onError(e);}}
   finally{if(current(g,a,n))el('audit-refresh').disabled=false;}
  }
  async function detail(id,refresh=false){
   if(!actor()||!isActive()||!capable()||!uuid(id))return;
   if(!refresh)focus=document.activeElement;close();target=id;detailController=new AbortController();const c=detailController,g=generation(),a=actor(),n=++detailSeq;
   const fresh=()=>n===detailSeq&&g===generation()&&actor()===a&&isActive();
   el('audit-detail').hidden=false;el('audit-detail-status').textContent='LOADING · 正在读取精确审计证据';el('audit-detail-refresh').disabled=true;el('audit-detail').focus();el('audit-detail').scrollIntoView({block:'nearest'});
   try{const d=await get('/api/v1/admin/audit/actions/'+a+'/'+id,g,c);if(!fresh()||c.signal.aborted)return;item(d,true);if(d.request_id!==id||d.source!==source||d.scope!==scope||!stamp(d.read_at))throw Error('Invalid audit detail');
    for(const [label,value] of [['请求 ID',d.request_id],['当前管理员 ID',d.actor_id],['操作',AdminAudit.actions[d.action]],['记录结果',AdminAudit.statuses[d.status]],['可信结果码',d.code],['操作预留时间',dateText(d.created_at)],['终态记录时间',d.recorded_at?dateText(d.recorded_at):'无已确认终态'],['历史目标',targetText(d.target_status)],['历史 Manifest 状态',d.manifest_state||'未提供（不猜测）'],['数据库读取时间',dateText(d.read_at)]]){const row=document.createElement('div'),dt=document.createElement('dt'),dd=document.createElement('dd');dt.textContent=label;dd.textContent=value;row.append(dt,dd);el('audit-facts').append(row);}
    el('audit-detail-status').textContent=d.status==='UNKNOWN'?'结果未确认 · 仅有持久预留，没有已确认回执；本页不会执行或重放操作。':'已读取历史管理记录 · 不代表网盘在线或目标当前可用';
   }catch(e){if(fresh()){el('audit-facts').replaceChildren();el('audit-detail-status').textContent=e.code==='AUDIT_NOT_FOUND'?'NOT_FOUND · 记录不存在或不在当前可见范围':'UNAVAILABLE · 精确回执未取得，不推测结果';if(e.code!=='AUDIT_NOT_FOUND')onError(e);}}
   finally{if(fresh())el('audit-detail-refresh').disabled=false;}
  }
  function timeInput(id){const value=el(id).value;if(!value)return '';if(!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(?::\d{2})?$/.test(value))throw Error('Invalid UTC input');const d=new Date(value+'Z');if(!Number.isFinite(d.getTime())||d.toISOString().slice(0,10)!==value.slice(0,10))throw Error('Invalid UTC date');return d.toISOString();}
  el('audit-filter').addEventListener('submit',event=>{event.preventDefault();try{const action=el('audit-action').value,status=el('audit-status').value,from=timeInput('audit-from'),before=timeInput('audit-before');if(action!=='ALL'&&!Object.hasOwn(AdminAudit.actions,action)||status!=='ALL'&&!Object.hasOwn(AdminAudit.statuses,status)||from&&before&&from>=before)throw Error('Invalid audit filter');filter={action,status,from,before};cursor='';return load();}catch{el('audit-footer').textContent='筛选未提交 · 请检查操作、状态与 UTC 时间范围';}});
  el('audit-clear').addEventListener('click',()=>{reset();return load();});el('audit-refresh').addEventListener('click',()=>{cursor='';return load();});el('audit-more').addEventListener('click',()=>{if(next){cursor=next;return load();}});el('audit-detail-refresh').addEventListener('click',()=>target?detail(target,true):undefined);el('audit-detail-close').addEventListener('click',()=>close(true));
  return {load,reset,invalidate,detail,close};
 }
};
