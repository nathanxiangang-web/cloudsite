'use strict';
// Specific commands only. Unknown outcomes are reconciled by GET, never POSTed
// again. Storage contains opaque request IDs only, scoped to the verified actor.
const AdminOperations={create({el,command,read,generation,principal,dateText,onError,onUsersChanged,newID}){
 let busy=false,unconfirmed=false,selected=null,owner='',request=0,currentID='';
 const uuid=/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
 function remember(){try{if(owner&&currentID)window.sessionStorage.setItem('cloudsite-admin-action:'+owner,currentID);}catch{}}
 function controls(){el('create-user-submit').disabled=busy||unconfirmed;el('change-user-submit').disabled=busy||unconfirmed||!selected||!Number.isSafeInteger(selected.version);el('probe-account').disabled=busy||unconfirmed||!el('probe-binding').value;el('probe-binding').disabled=busy;el('lookup-action').disabled=busy;}
 function note(text){el('action-status').textContent=text;}
 function clearCapacity(){el('account-capacity').replaceChildren();el('account-session-status').textContent='NOT_CHECKED';el('account-shared-capacity').textContent='NOT_CHECKED';}
 function result(value){
  if(!value||value.request_id!==currentID||!['CREATE_USER','SET_USER_ENABLED','PROBE_115'].includes(value.action)||!['SUCCEEDED','REJECTED','UNKNOWN'].includes(value.status)||!['USER_CREATED','USER_STATE_CHANGED','PROBE_RECORDED','CONFLICT','VERSION_CONFLICT','TARGET_FORBIDDEN','NOT_FOUND','SCOPE_REJECTED','NOT_CONNECTED','RATE_LIMITED','RESERVED_NO_CONFIRMED_RESULT'].includes(value.code)||!Number.isFinite(new Date(value.recorded_at).getTime()))throw Error('Invalid action evidence');
  if(value.status==='SUCCEEDED'&&value.action!=='PROBE_115'&&(!value.user||value.user.user_id!==value.target_id||!uuid.test(value.target_id)||!Number.isSafeInteger(value.user.version)||value.user.version<0||typeof value.user.enabled!=='boolean'))throw Error('Invalid user receipt');
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
  if(value.status==='SUCCEEDED'&&value.user){selected=null;el('selected-user').textContent='操作已提交，重新读取用户列表';el('disable-confirmation').hidden=true;el('confirm-user-disable').checked=false;onUsersChanged();}
 }
 async function execute(path,method,body){
  if(busy||unconfirmed||!principal())return;
  const gen=generation(),seq=++request;let id;try{id=newID();if(!uuid.test(id))throw Error('No secure UUID');}catch{note('无法生成安全请求 ID，未提交');return;}
  currentID=id;el('action-request-id').value=id;remember();busy=true;unconfirmed=true;clearCapacity();controls();note('提交中 · 尚未确认结果 · 请求 '+id);
  try{const value=await command(path,method,{...body,request_id:id},gen);if(gen===generation()&&seq===request)result(value);}
  catch(error){if(gen===generation()&&seq===request){unconfirmed=true;note('UNKNOWN · 未确认操作结果；请查询操作记录，不要重复提交 · 请求 '+id);onError(error);}}
  finally{if(gen===generation()&&seq===request){busy=false;controls();}}
 }
 async function lookup(){
  if(busy||!principal())return;const id=el('action-request-id').value.trim();if(!uuid.test(id)){note('请输入操作请求 UUID');return;}
  currentID=id;remember();busy=true;const seq=++request,gen=generation();controls();
  try{const value=await read('/api/v1/admin/actions/'+id,gen);if(gen===generation()&&seq===request)result(value);}
  catch(error){if(gen===generation()&&seq===request){unconfirmed=true;note('UNKNOWN · 未取得已确认结果，不重发操作 · 请求 '+id);onError(error);}}
  finally{if(gen===generation()&&seq===request){busy=false;controls();}}
 }
 function reset(){request++;busy=false;unconfirmed=false;selected=null;owner='';currentID='';el('new-user-password').value='';el('new-user-login').value='';el('action-request-id').value='';el('probe-binding').replaceChildren();el('probe-binding').value='';el('selected-user').textContent='请先在用户列表选择一个 USER';el('confirm-user-disable').checked=false;note('尚未操作 · 所有结果以服务器为准');clearCapacity();controls();}
 function configure(bindings){
  const next=principal()?.user_id||'';if(owner!==next){reset();owner=next;try{const id=window.sessionStorage.getItem('cloudsite-admin-action:'+owner);if(uuid.test(id)){currentID=id;el('action-request-id').value=id;unconfirmed=true;note('存在上次操作记录，请先查询服务端结果 · '+id);}}catch{}}
  const previous=el('probe-binding').value;el('probe-binding').replaceChildren();for(const b of bindings||[]){const option=document.createElement('option');option.value=b.binding_id;option.textContent=b.label;el('probe-binding').append(option);}el('probe-binding').value=(bindings||[]).some(b=>b.binding_id===previous)?previous:bindings?.[0]?.binding_id||'';controls();
 }
 function selectUser(user){if(busy||unconfirmed||user.role!=='USER'||!Number.isSafeInteger(user.version)||user.version<0)return;selected={...user};el('selected-user').textContent=user.login+' · '+user.user_id+' · version '+user.version+' · 当前'+(user.enabled?'启用':'停用');el('change-user-submit').textContent=user.enabled?'停用此 USER':'启用此 USER';el('confirm-user-disable').checked=false;el('disable-confirmation').hidden=!user.enabled;controls();}
 el('create-user-form').addEventListener('submit',event=>{event.preventDefault();const login=el('new-user-login').value.trim().toLowerCase(),password=el('new-user-password').value;el('new-user-password').value='';return execute('/api/v1/admin/users','POST',{login,password});});
 el('change-user-submit').addEventListener('click',()=>{if(!selected||selected.enabled&&!el('confirm-user-disable').checked){note('请明确确认停用后将撤销该用户全部登录会话');return;}return execute('/api/v1/admin/users/'+selected.user_id,'PATCH',{enabled:!selected.enabled,expected_version:selected.version});});
 el('probe-account').addEventListener('click',()=>execute('/api/v1/admin/storage/'+el('probe-binding').value+'/probe','POST',{}));
 el('probe-binding').addEventListener('change',()=>clearCapacity());
 el('lookup-action').addEventListener('click',lookup);
 return {reset,configure,selectUser,lookup};
}};
