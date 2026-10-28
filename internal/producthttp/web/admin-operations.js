'use strict';
// Specific commands only. Unknown outcomes are reconciled by GET, never POSTed
// again. Storage contains opaque request IDs only, scoped to the verified actor.
const AdminOperations={create({el,command,read,generation,principal,dateText,onError,onUsersChanged,newID}){
 let busy=false,unconfirmed=false,selected=null,owner='',request=0,currentID='',pendingAction=null,detail=0;
 // Strict action×status×code triples. Each action lists the only codes the
 // backend can return for SUCCEEDED and for REJECTED; UNKNOWN is reservation only.
 const ACTION_OUTCOMES={
  CREATE_USER:{SUCCEEDED:['USER_CREATED'],REJECTED:['CONFLICT','LOGIN_CONFLICT','TARGET_FORBIDDEN','INVALID_REQUEST']},
  SET_USER_ENABLED:{SUCCEEDED:['USER_STATE_CHANGED'],REJECTED:['TARGET_FORBIDDEN','VERSION_CONFLICT','USER_STATE_CONFLICT','INVALID_REQUEST']},
  RENAME_USER:{SUCCEEDED:['USER_RENAMED'],REJECTED:['TARGET_FORBIDDEN','VERSION_CONFLICT','USER_STATE_CONFLICT','LOGIN_CONFLICT','INVALID_REQUEST']},
  RESET_USER_PASSWORD:{SUCCEEDED:['USER_PASSWORD_RESET'],REJECTED:['TARGET_FORBIDDEN','VERSION_CONFLICT','USER_STATE_CONFLICT','INVALID_REQUEST']},
  DELETE_USER:{SUCCEEDED:['USER_DELETED'],REJECTED:['TARGET_FORBIDDEN','VERSION_CONFLICT','USER_STATE_CONFLICT','ACTIVE_DOWNLOADS','DOWNLOAD_STATE_CONFLICT','INVALID_REQUEST']},
  RESTORE_USER:{SUCCEEDED:['USER_RESTORED'],REJECTED:['TARGET_FORBIDDEN','VERSION_CONFLICT','USER_STATE_CONFLICT','PASSWORD_UNCHANGED','INVALID_REQUEST']},
  PROBE_115:{SUCCEEDED:['PROBE_RECORDED'],REJECTED:['NOT_CONNECTED','SCOPE_REJECTED','TARGET_FORBIDDEN','RATE_LIMITED','INVALID_REQUEST']}
 };
 const UNKNOWN_CODES=['RESERVED_NO_CONFIRMED_RESULT'];
 const uuid=/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
 function remember(){try{if(owner&&currentID)window.sessionStorage.setItem('cloudsite-admin-action:'+owner,currentID);}catch{}}
 function controls(){const has=!!selected&&Number.isSafeInteger(selected.version);const del=!!selected&&Boolean(selected.deleted||selected.deleted_at);const canTarget=busy||unconfirmed||!has;el('create-user-submit').disabled=busy||unconfirmed;el('change-user-submit').disabled=canTarget||del;el('rename-user-submit').disabled=canTarget||del;el('reset-user-submit').disabled=canTarget||del;el('delete-user-submit').disabled=canTarget||del;el('restore-user-submit').disabled=canTarget||!del;el('user-detail-submit').disabled=canTarget||del;el('probe-account').disabled=busy||unconfirmed||!el('probe-binding').value;el('probe-binding').disabled=busy;el('lookup-action').disabled=busy;}
 function note(text){el('action-status').textContent=text;}
 function clearCapacity(){el('account-capacity').replaceChildren();el('account-session-status').textContent='NOT_CHECKED';el('account-shared-capacity').textContent='NOT_CHECKED';}
 function result(value){
  if(!value||value.request_id!==currentID||!['SUCCEEDED','REJECTED','UNKNOWN'].includes(value.status)||!Number.isFinite(new Date(value.recorded_at).getTime()))throw Error('Invalid action evidence');
  const outcomes=ACTION_OUTCOMES[value.action];if(!outcomes)throw Error('Invalid action evidence');
  const codes=value.status==='UNKNOWN'?UNKNOWN_CODES:outcomes[value.status];
  if(!codes||typeof value.code!=='string'||!codes.includes(value.code))throw Error('Invalid action evidence');
  if(pendingAction&&value.action!==pendingAction)throw Error('Invalid action evidence');
  if(value.status==='SUCCEEDED'&&value.action!=='PROBE_115'&&(!value.user||value.user.user_id!==value.target_id||!uuid.test(value.target_id)||!Number.isSafeInteger(value.user.version)||value.user.version<0||typeof value.user.enabled!=='boolean'||typeof value.user.deleted!=='boolean'))throw Error('Invalid user receipt');
  if(value.capacity&&(!uuid.test(value.target_id)||value.action!=='PROBE_115'||value.capacity.source!=='115_GET_INFO'||!Number.isFinite(new Date(value.capacity.probed_at).getTime())))throw Error('Invalid probe receipt');
  unconfirmed=value.status==='UNKNOWN';note(value.status+' · '+value.code+' · 服务端记录于 '+dateText(value.recorded_at)+' · 请求 '+value.request_id);
  if(value.capacity&&value.target_id!==el('probe-binding').value){clearCapacity();const text=document.createElement('p');text.textContent='历史检测属于 Binding '+value.target_id+'，不作为当前选择的账号状态。';el('account-capacity').append(text);}
  else if(value.capacity){
   clearCapacity();const c=value.capacity,status=['AUTHENTICATED','NOT_AUTHENTICATED','NOT_CONNECTED','UNKNOWN'].includes(c.status)?c.status:'UNKNOWN';
   el('account-session-status').textContent=status;el('account-shared-capacity').textContent=status==='AUTHENTICATED'?'见下方容量明细':'未取得容量';
   const text=document.createElement('p');text.textContent=status+' · '+(c.source==='115_GET_INFO'?'115_GET_INFO':'UNKNOWN')+' · 检测于 '+dateText(c.probed_at)+' · 仅代表该次检测，不是持续在线保证';el('account-capacity').append(text);
   if(status==='AUTHENTICATED'){
    const values=[c.total_bytes,c.used_bytes,c.remaining_bytes];if(values.some(v=>typeof v!=='string'||!/^\d+$/.test(v)))throw Error('Invalid byte counts');
    const [total,used,remaining]=values.map(BigInt);if(total<=0n||used>total||remaining!==total-used)throw Error('Invalid capacity');
    for(const [name,v]of [['共享账号总容量',c.total_bytes],['共享账号已用',c.used_bytes],['共享账号剩余',c.remaining_bytes]]){const row=document.createElement('p');row.textContent=name+'：'+v+' bytes';el('account-capacity').append(row);}
   }
  }
  if(value.status==='SUCCEEDED'&&value.user){selected=null;el('selected-user').textContent='操作已提交（'+value.code+'），请重新读取用户列表';el('disable-confirmation').hidden=true;el('confirm-user-disable').checked=false;el('rename-user-form').hidden=true;el('reset-user-password-form').hidden=true;el('delete-user-form').hidden=true;el('restore-user-form').hidden=true;onUsersChanged();}
 }
 async function execute(path,method,body,action){
  if(busy||unconfirmed||!principal())return;
  const gen=generation(),seq=++request;let id;try{id=newID();if(!uuid.test(id))throw Error('No secure UUID');}catch{note('无法生成安全请求 ID，未提交');return;}
  currentID=id;pendingAction=action||null;el('action-request-id').value=id;remember();busy=true;unconfirmed=true;clearCapacity();controls();note('提交中 · 尚未确认结果 · 请求 '+id);
  try{const value=await command(path,method,{...body,request_id:id},gen);if(gen===generation()&&seq===request)result(value);}
  catch(error){if(gen===generation()&&seq===request){unconfirmed=true;note('UNKNOWN · 未确认操作结果；请查询操作记录，不要重复提交 · 请求 '+id);onError(error);}}
  finally{if(gen===generation()&&seq===request){busy=false;controls();}}
 }
 async function lookup(){
  if(busy||!principal())return;const id=el('action-request-id').value.trim();if(!uuid.test(id)){note('请输入操作请求 UUID');return;}
  currentID=id;pendingAction=null;remember();busy=true;const seq=++request,gen=generation();controls();
  try{const value=await read('/api/v1/admin/actions/'+id,gen);if(gen===generation()&&seq===request)result(value);}
  catch(error){if(gen===generation()&&seq===request){unconfirmed=true;note('UNKNOWN · 未取得已确认结果，不重发操作 · 请求 '+id);onError(error);}}
  finally{if(gen===generation()&&seq===request){busy=false;controls();}}
 }
 function detailFacts(value){const list=el('user-detail-facts');list.replaceChildren();const rows=[['下载任务总数',value.download_count],['进行中下载',value.active_downloads],['账号 version',value.version],['最近更新',value.updated_at===undefined||value.updated_at===null?'未获取':dateText(value.updated_at)]];for(const [label,raw] of rows){const row=document.createElement('div'),dt=document.createElement('dt'),dd=document.createElement('dd');dt.textContent=label;dd.textContent=raw===undefined||raw===null||raw===''?'未获取':String(raw);row.append(dt,dd);list.append(row);}}
 async function loadUserDetail(){if(!selected||selected.role!=='USER'||!Number.isSafeInteger(selected.version))return;const id=selected.user_id,pending=++detail,gen=generation();try{const value=await read('/api/v1/admin/users/'+id,gen);if(gen!==generation()||pending!==detail||!selected||selected.user_id!==id)return;if(!value||value.user_id!==id||value.role!=='USER')throw Error('Invalid user detail identity');detailFacts(value);}catch(error){if(gen!==generation()||pending!==detail||!selected||selected.user_id!==id)return;el('user-detail-facts').textContent='详情读取失败；不据此推断用户状态';onError(error);}}
 function reset(){request++;detail++;busy=false;unconfirmed=false;selected=null;owner='';currentID='';el('new-user-password').value='';el('new-user-login').value='';el('action-request-id').value='';el('probe-binding').replaceChildren();el('probe-binding').value='';el('selected-user').textContent='请先在用户列表选择一个 USER';el('confirm-user-disable').checked=false;el('rename-user-login').value='';el('reset-user-password').value='';el('confirm-user-reset').checked=false;el('confirm-user-delete').checked=false;el('restore-user-password').value='';pendingAction=null;note('尚未操作 · 所有结果以服务器为准');clearCapacity();el('user-detail-facts').replaceChildren();controls();}
 function configure(bindings){
  const next=principal()?.user_id||'';if(owner!==next){reset();owner=next;try{const id=window.sessionStorage.getItem('cloudsite-admin-action:'+owner);if(uuid.test(id)){currentID=id;el('action-request-id').value=id;unconfirmed=true;note('存在上次操作记录，请先查询服务端结果 · '+id);}}catch{}}
  const previous=el('probe-binding').value;el('probe-binding').replaceChildren();for(const b of bindings||[]){const option=document.createElement('option');option.value=b.binding_id;option.textContent=b.label;el('probe-binding').append(option);}el('probe-binding').value=(bindings||[]).some(b=>b.binding_id===previous)?previous:bindings?.[0]?.binding_id||'';controls();
 }
 function focusUserFeedback(id){const target=el(id);target.scrollIntoView({block:'start'});target.focus({preventScroll:true});}
 function selectUser(user){if(busy||unconfirmed){note((busy?'操作正在提交，请等待结果。':'存在未确认的操作，请先查询服务端操作记录，不要重复提交。')+(currentID?' 请求 '+currentID:''));focusUserFeedback('action-status');return;}if(!user||user.role!=='USER'||!Number.isSafeInteger(user.version)||user.version<0){note('用户信息不完整或不是普通 USER，请刷新用户列表后重新选择。');focusUserFeedback('action-status');return;}selected={...user};detail++;el('user-detail-facts').replaceChildren();const del=Boolean(user.deleted||user.deleted_at);el('selected-user').textContent=user.login+' · '+user.user_id+' · version '+user.version+' · '+(del?'已逻辑删除（可恢复）':(user.enabled?'启用':'停用'));el('change-user-submit').textContent=user.enabled?'停用此 USER':'启用此 USER';el('confirm-user-disable').checked=false;el('disable-confirmation').hidden=del||!user.enabled;el('rename-user-form').hidden=del;el('reset-user-password-form').hidden=del;el('delete-user-form').hidden=del;el('restore-user-form').hidden=!del;el('rename-user-login').value=user.login||'';el('confirm-user-reset').checked=false;el('confirm-user-delete').checked=false;controls();focusUserFeedback('selected-user');}
 el('create-user-form').addEventListener('submit',event=>{event.preventDefault();const login=el('new-user-login').value.trim().toLowerCase(),password=el('new-user-password').value;el('new-user-password').value='';return execute('/api/v1/admin/users','POST',{login,password},'CREATE_USER');});
 el('change-user-submit').addEventListener('click',()=>{if(!selected||selected.enabled&&!el('confirm-user-disable').checked){note('请明确确认停用后将撤销该用户全部登录会话');return;}return execute('/api/v1/admin/users/'+selected.user_id,'PATCH',{enabled:!selected.enabled,expected_version:selected.version},'SET_USER_ENABLED');});
 el('probe-account').addEventListener('click',()=>execute('/api/v1/admin/storage/'+el('probe-binding').value+'/probe','POST',{},'PROBE_115'));
 el('probe-binding').addEventListener('change',()=>clearCapacity());
 el('lookup-action').addEventListener('click',lookup);
 el('rename-user-form').addEventListener('submit',event=>{event.preventDefault();if(!selected){note('请先选择一个 USER');return;}const login=el('rename-user-login').value.trim().toLowerCase();return execute('/api/v1/admin/users/'+selected.user_id+'/rename','POST',{login,expected_version:selected.version},'RENAME_USER');});
 el('reset-user-password-form').addEventListener('submit',event=>{event.preventDefault();if(!selected){note('请先选择一个 USER');return;}if(!el('confirm-user-reset').checked){note('请确认重置密码将撤销该用户全部网站会话');return;}const password=el('reset-user-password').value;el('reset-user-password').value='';return execute('/api/v1/admin/users/'+selected.user_id+'/password','POST',{password,expected_version:selected.version},'RESET_USER_PASSWORD');});
 el('delete-user-submit').addEventListener('click',()=>{if(!selected){note('请先选择一个 USER');return;}if(!el('confirm-user-delete').checked){note('请确认逻辑删除保留历史任务/分享且可恢复');return;}return execute('/api/v1/admin/users/'+selected.user_id,'DELETE',{expected_version:selected.version},'DELETE_USER');});
 el('restore-user-form').addEventListener('submit',event=>{event.preventDefault();if(!selected){note('请先选择一个 USER');return;}const password=el('restore-user-password').value;el('restore-user-password').value='';return execute('/api/v1/admin/users/'+selected.user_id+'/restore','POST',{password,expected_version:selected.version},'RESTORE_USER');});
 return {reset,configure,selectUser,lookup,loadUserDetail};
}};
