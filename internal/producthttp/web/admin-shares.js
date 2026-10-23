'use strict';
// Share presentation and operation state only. Auth/HTTP are injected ports;
// neither user lifecycle commands nor provider/IndexCore operations live here.
const AdminShares={create({el,read,command,generation,principal,isActive,dateText,cell,badge,onError,onStart,newID}){
 const uuid=/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
 const validID=value=>typeof value==='string'&&uuid.test(value);
 const state={owner:'',status:'ALL',cursor:'',next:'',read:0,operation:0,actor:'',loaded:false,key:'',items:[],selected:null,busy:false,unknown:false,intent:null};
 const storageKey=actor=>'cloudsite-admin-share-action:'+actor;
 const validTime=value=>typeof value==='string'&&Number.isFinite(new Date(value).getTime());
 function persist(){try{window.sessionStorage?.setItem(storageKey(state.actor),JSON.stringify(state.intent));}catch{}}
 function forget(){try{window.sessionStorage?.removeItem(storageKey(state.actor));}catch{}}
 function controls(){
  el('revoke-share').disabled=state.busy||state.unknown||!state.selected?.active;
  el('share-revoke-confirm').disabled=state.busy||state.unknown||!state.selected?.active;
  el('lookup-share-action').disabled=state.busy||!state.intent;
  el('more-shares').disabled=!state.next;
 }
 function clearDetail(){state.selected=null;el('share-detail').hidden=true;el('share-detail-facts').replaceChildren();el('share-revoke-confirm').checked=false;controls();}
 function clearPage(){state.loaded=false;state.items=[];state.next='';el('shares-rows').replaceChildren();el('shares-empty').hidden=true;el('shares-total').textContent='—';clearDetail();controls();}
 function reset(){
  state.read++;state.operation++;state.actor='';state.owner='';state.status='ALL';state.cursor='';state.key='';state.busy=false;state.unknown=false;state.intent=null;
  clearPage();el('shares-owner').value='';el('shares-status').value='ALL';el('shares-footer').textContent='等待查询';el('refresh-shares').disabled=false;
  el('share-action-status').textContent='尚未操作';el('share-action-request').textContent='—';el('share-action-target').textContent='—';controls();
 }
 function configure(){
  const p=principal(),actor=p?.role==='ADMIN'?p.user_id:'';
  if(!validID(actor)){reset();return;}
  if(actor===state.actor)return;
  reset();state.actor=actor;
  try{
   const saved=JSON.parse(window.sessionStorage?.getItem(storageKey(actor))||'null');
   if(saved&&validID(saved.request_id)&&validID(saved.target_id)){
    state.intent={request_id:saved.request_id,target_id:saved.target_id};state.unknown=true;
    el('share-action-request').textContent=saved.request_id;el('share-action-target').textContent=saved.target_id;
    el('share-action-status').textContent='上次撤销未确认，请查询原操作结果；不会自动重发。';
   }
  }catch{}
  controls();
 }
 async function timed(work){
  let timer;
  try{return await Promise.race([work,new Promise((_,reject)=>{timer=setTimeout(()=>reject(Error('Share request deadline')),15000);})]);}
  finally{clearTimeout(timer);}
 }
 function safeItem(item){
  if(!item||![item.share_id,item.owner_id,item.binding_id].every(validID)||typeof item.file_name!=='string'||!item.file_name.trim()||typeof item.canonical_path!=='string'||!item.canonical_path.startsWith('/')||/[\0\r\n]/.test(item.file_name+item.canonical_path)||!Number.isSafeInteger(item.size_bytes)||item.size_bytes<0||typeof item.active!=='boolean'||!validTime(item.created_at)||!validTime(item.expires_at)||new Date(item.expires_at)<new Date(item.created_at))throw Error('Invalid share item');
  return {share_id:item.share_id,owner_id:item.owner_id,binding_id:item.binding_id,file_name:item.file_name,canonical_path:item.canonical_path,size_bytes:item.size_bytes,active:item.active,created_at:item.created_at,expires_at:item.expires_at};
 }
 function pageItems(page){
  if(!page||!Array.isArray(page.items)||page.items.length>20||!Number.isSafeInteger(page.total)||page.total<page.items.length||page.total<0||page.scope!=='FILTERED_PRODUCT_SHARES'||!validTime(page.read_at)||typeof page.next_cursor!=='string')throw Error('Invalid share page');
  let previous=state.cursor;
  const items=page.items.map(raw=>{
   const item=safeItem(raw);
   if(item.share_id<=previous||state.owner&&item.owner_id!==state.owner||state.status==='ACTIVE'&&!item.active||['REVOKED','EXPIRED'].includes(state.status)&&item.active)throw Error('Invalid share scope');
   previous=item.share_id;return item;
  });
  if(page.next_cursor!==''&&(items.length!==20||page.next_cursor!==previous))throw Error('Invalid share cursor');
  return items;
 }
 function select(item,focus=true){
  if(state.busy)return;
  state.selected=item;el('share-revoke-confirm').checked=false;el('share-detail-facts').replaceChildren();
  for(const [label,value] of [['文件名',item.file_name],['分享 ID',item.share_id],['所属用户',item.owner_id],['存储空间',item.binding_id],['文件路径',item.canonical_path],['文件大小',item.size_bytes+' bytes'],['创建时间',dateText(item.created_at)],['到期时间',dateText(item.expires_at)],['分享状态',item.active?'未撤销且未到期':'不可用（已撤销或到期）']]){
   const row=document.createElement('div'),dt=document.createElement('dt'),dd=document.createElement('dd');dt.textContent=label;dd.textContent=value;row.append(dt,dd);el('share-detail-facts').append(row);
  }
  el('share-detail').hidden=false;controls();if(focus)el('share-detail').focus({preventScroll:true});
 }
 async function load(){
  if(principal()?.role!=='ADMIN'||!isActive())return;
  onStart();
  configure();const gen=generation(),seq=++state.read,key=[state.owner,state.status,state.cursor].join('|'),previous=state.loaded&&state.key===key;
  if(!previous)clearPage();state.key=key;el('shares-empty').hidden=true;el('refresh-shares').disabled=true;el('more-shares').disabled=true;
  el('shares-footer').textContent=previous?'正在刷新，暂留上次结果…':'正在读取分享…';
  const query=new URLSearchParams({limit:'20',status:state.status});if(state.owner)query.set('owner',state.owner);if(state.cursor)query.set('cursor',state.cursor);
  try{
   const page=await timed(read('/api/v1/admin/shares?'+query,gen));
   if(gen!==generation()||seq!==state.read||!isActive())return;
   const items=pageItems(page);el('shares-rows').replaceChildren();state.items=items;state.next=page.next_cursor;state.loaded=true;
   for(const item of items){
    const row=document.createElement('tr');cell(row,item.file_name);cell(row,item.owner_id);cell(row).append(badge(item.active?'未撤销且未到期':'已撤销或到期',item.active?'':'neutral'));cell(row,dateText(item.created_at));cell(row,dateText(item.expires_at));
    const button=document.createElement('button');button.type='button';button.className='quiet';button.textContent='查看详情';button.setAttribute('aria-label','查看分享 '+item.file_name);button.addEventListener('click',()=>select(item));cell(row).append(button);el('shares-rows').append(row);
   }
   if(state.selected){const updated=items.find(item=>item.share_id===state.selected.share_id);if(updated&&!state.busy)select(updated,false);else if(!state.busy)clearDetail();}
   el('shares-empty').hidden=items.length!==0;el('shares-total').textContent=String(page.total);el('shares-footer').textContent='本页 '+items.length+' 项 · 匹配总数 '+page.total+' · 数据库读取于 '+dateText(page.read_at);controls();
  }catch(error){if(gen===generation()&&seq===state.read&&isActive()){el('shares-footer').textContent=previous?'查询失败，保留上次结果（未确认最新）':'分享查询未完成，不以空列表代替失败';onError(error);}}
  finally{if(gen===generation()&&seq===state.read){el('refresh-shares').disabled=false;controls();}}
 }
 function receipt(value){
  const intent=state.intent,codes={SUCCEEDED:['SHARE_REVOKED'],REJECTED:['SHARE_ALREADY_REVOKED','SHARE_NOT_FOUND','TARGET_FORBIDDEN'],UNKNOWN:['RESERVED_NO_CONFIRMED_RESULT']};
  if(!intent||!value||value.request_id!==intent.request_id||value.target_id!==intent.target_id||value.action!=='REVOKE_SHARE'||!codes[value.status]?.includes(value.code)||!validTime(value.recorded_at))throw Error('Invalid share receipt');
  state.unknown=value.status==='UNKNOWN';
  el('share-action-status').textContent=({SHARE_REVOKED:'分享已撤销',SHARE_ALREADY_REVOKED:'分享此前已撤销',SHARE_NOT_FOUND:'未找到该分享',TARGET_FORBIDDEN:'不允许操作此分享',RESERVED_NO_CONFIRMED_RESULT:'撤销仍未确认，请查询原操作结果；不要重复提交。'}[value.code])+' · '+value.status+' / '+value.code+' · 服务端记录于 '+dateText(value.recorded_at);
  if(!state.unknown)forget();
  return !state.unknown;
 }
 async function operation(lookup){
  if(principal()?.role!=='ADMIN'||state.busy)return;
  configure();
  if(lookup){if(!state.intent)return;}
  else{
   if(state.unknown||!state.selected?.active)return;
   if(!el('share-revoke-confirm').checked){el('share-action-status').textContent='请先确认撤销此分享。';return;}
   const id=newID();if(!validID(id))return;
   state.intent={request_id:id,target_id:state.selected.share_id};state.unknown=true;persist();
  }
  onStart();
  const gen=generation(),seq=++state.operation,intent={...state.intent};state.busy=true;
  el('share-action-request').textContent=intent.request_id;el('share-action-target').textContent=intent.target_id;el('share-revoke-confirm').checked=false;
  el('share-action-status').textContent=lookup?'正在查询原操作结果…':'正在撤销分享…';controls();let refresh=false;
  try{
   const value=await timed(lookup?read('/api/v1/admin/actions/'+intent.request_id,gen):command('/api/v1/admin/shares/'+intent.target_id+'/revoke','POST',{request_id:intent.request_id},gen));
   if(gen!==generation()||seq!==state.operation)return;
   refresh=receipt(value);
  }catch(error){if(gen===generation()&&seq===state.operation){state.unknown=true;el('share-action-status').textContent='撤销未确认，请查询原操作结果；不会自动重发。';onError(error);}}
  finally{if(gen===generation()&&seq===state.operation){state.busy=false;controls();}}
  if(refresh&&isActive())await load();
 }
 el('shares-filter-form').addEventListener('submit',event=>{
  event.preventDefault();const owner=el('shares-owner').value.trim(),status=el('shares-status').value;
  if(owner&&!uuid.test(owner)||!['ALL','ACTIVE','REVOKED','EXPIRED'].includes(status)){onError({code:'INVALID_REQUEST'});return;}
  state.owner=owner;state.status=status;state.cursor='';return load();
 });
 el('clear-shares-filter').addEventListener('click',()=>{state.owner='';state.status='ALL';state.cursor='';el('shares-owner').value='';el('shares-status').value='ALL';return load();});
 el('refresh-shares').addEventListener('click',()=>{state.cursor='';return load();});
 el('more-shares').addEventListener('click',()=>{if(state.next){state.cursor=state.next;return load();}});
 el('revoke-share').addEventListener('click',()=>operation(false));el('lookup-share-action').addEventListener('click',()=>operation(true));
 return {reset,configure,load,invalidate(){state.read++;el('refresh-shares').disabled=false;controls();}};
}};
