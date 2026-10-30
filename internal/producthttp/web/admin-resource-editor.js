'use strict';
// Product presentation only. Persistent recovery contains opaque IDs/version,
// never labels, credentials or an executable retry payload.
const AdminResourceEditor={
 categories:{AUTO:'自动（沿用规范展示）',SOFTWARE:'软件',VIDEO:'视频',DOCUMENT:'文档',IMAGE:'图片',AUDIO:'音频',ARCHIVE:'压缩包',OTHER:'其他'},
 uuid:v=>typeof v==='string'&&/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(v),
 name(v){if(typeof v!=='string'||v.trim()!==v||[...v].length>160||/[\p{Cc}\p{Cf}<>\uFFFD]/u.test(v))return false;let bytes=0;for(const c of v){const n=c.codePointAt(0);if(n>=0xd800&&n<=0xdfff)return false;bytes+=n<128?1:n<2048?2:n<65536?3:4;}return bytes<=640;},
 create({present,el,read,command,generation,principal,isActive,bindingAllowed,capable,dateText,onError,newID,now=()=>Date.now()}){
  const uuid=AdminResourceEditor.uuid,max=Number.MAX_SAFE_INTEGER,mem=new Map();
  let last=null,target=null,doc=null,confirmation=null,focus=null,seq=0,opSeq=0,busy=false,controller,confirmTimer;
  const actor=()=>principal()?.role==='ADMIN'?principal().user_id:'';
  const context=()=>({actor:actor(),gen:generation()});
  const same=c=>c.actor&&c.actor===actor()&&c.gen===generation()&&isActive();
  const base=id=>'/api/v1/admin/copies/'+id+'/presentation';
  const stamp=v=>AdminDownloadDetail.time(v)&&/T\d{2}:\d{2}:\d{2}(?:\.\d{1,6})?(?:Z|[+-]\d{2}:\d{2})$/.test(v);
  const keys=(v,allowed)=>v&&typeof v==='object'&&!Array.isArray(v)&&Object.keys(v).every(k=>allowed.includes(k));
  const key=id=>'cloudsite-copy-presentation:'+id;
  function pending(){
   const id=actor();if(!id)return null;if(mem.has(id))return mem.get(id);
   try{const raw=window.sessionStorage?.getItem(key(id));if(!raw)return null;const p=JSON.parse(raw);
    if(!keys(p,['request_id','target_id','binding_id','expected_version'])||!uuid(p.request_id)||!uuid(p.target_id)||!uuid(p.binding_id)||!Number.isSafeInteger(p.expected_version)||p.expected_version<0||p.expected_version>=max)throw Error('Invalid recovery reference');mem.set(id,p);return p;
   }catch{const p={corrupt:true};mem.set(id,p);return p;}
  }
  function save(id,p){try{window.sessionStorage?.setItem(key(id),JSON.stringify(p));if(!window.sessionStorage)throw Error('Recovery storage unavailable');mem.set(id,p);return true;}catch{return false;}}
  function finish(id){mem.delete(id);try{window.sessionStorage?.removeItem(key(id));}catch{}}
  function receiptControls(){
   const p=pending(),visible=p||(last?.actor===actor()&&bindingAllowed(last.binding_id)?last:null);el('resource-edit-recovery').hidden=!visible;
   el('resource-edit-request').textContent=visible?.request_id||'—';el('resource-edit-target').textContent=visible?.target_id||'—';
   el('resource-edit-lookup').disabled=busy||!p||p.corrupt||!isActive()||!bindingAllowed(p.binding_id);
   if(p?.corrupt)el('resource-edit-result').textContent='恢复标识不可用 · 请通过审计记录核实历史请求，暂不开放新提交。';
  }
  function controls(){
   const permitted=capable()&&isActive()&&doc?.can_edit===true&&target&&bindingAllowed(target.binding_id)&&!pending();
   for(const id of ['resource-edit-name','resource-edit-category','resource-edit-hidden','resource-edit-prepare'])el(id).disabled=busy||!permitted;
   el('resource-edit-confirm').disabled=busy||!confirmation;el('resource-edit-refresh').disabled=busy||!target||!capable();receiptControls();
  }
  function dismiss(restore=true){clearTimeout(confirmTimer);confirmation=null;el('resource-edit-dialog').hidden=true;document.body.classList.toggle('resource-edit-confirm-open',false);if(restore&&focus?.isConnected)focus.focus();controls();}
  function clear(){doc=null;el('resource-edit-form').hidden=true;el('resource-edit-name').value='';el('resource-edit-category').value='AUTO';el('resource-edit-hidden').checked=false;el('resource-edit-facts').replaceChildren();}
  function invalidate(){seq++;opSeq++;controller?.abort();busy=false;dismiss(false);target=null;clear();el('resource-edit-status').textContent='选择一个实际资源后读取产品展示信息。';controls();}
  function reset(){last=null;invalidate();el('resource-edit-result').textContent='';receiptControls();}
  function valid(d,t){
   const e=d?.evidence,p=d?.presentation;
   if(!keys(d,['evidence','presentation','source','visibility_effect','empty_name_fallback','can_edit','read_at'])||d.source!=='PANTA_COPY_PROJECTION'||d.visibility_effect!=='PRESENTATION_ONLY_NO_ACCESS_CONTROL'||d.empty_name_fallback!=='CURRENT_CANONICAL_DISPLAY'||!stamp(d.read_at)||typeof d.can_edit!=='boolean')throw Error('Invalid presentation source');
   if(!keys(e,['copy_id','binding_id','indexcore_root_id','indexcore_resource_id','availability','projection_cursor','copy_updated_at'])||e.copy_id!==t.copy_id||e.binding_id!==t.binding_id||e.indexcore_resource_id!==t.resource_id||typeof e.indexcore_root_id!=='string'||!e.indexcore_root_id||e.indexcore_root_id.length>1024||!['PRESENT','REMOVED'].includes(e.availability)||!Number.isSafeInteger(e.projection_cursor)||e.projection_cursor<1||!stamp(e.copy_updated_at))throw Error('Invalid Copy evidence');
   if(!keys(p,['display_name','category','hidden','version','updated_at','updated_by'])||!AdminResourceEditor.name(p.display_name)||!Object.hasOwn(AdminResourceEditor.categories,p.category)||typeof p.hidden!=='boolean'||!Number.isSafeInteger(p.version)||p.version<0||p.version>max)throw Error('Invalid overlay');
   if(p.version===0?(p.display_name!==''||p.category!=='AUTO'||p.hidden||p.updated_at!==null||p.updated_by!==null):(!stamp(p.updated_at)||!uuid(p.updated_by)))throw Error('Invalid persisted version');
   if(d.can_edit&&(e.availability!=='PRESENT'||p.version===max))throw Error('Invalid edit permission');return d;
  }
  function draw(d){
   doc=d;el('resource-edit-form').hidden=false;el('resource-edit-name').value=d.presentation.display_name;el('resource-edit-category').value=d.presentation.category;el('resource-edit-hidden').checked=d.presentation.hidden;
   el('resource-edit-facts').replaceChildren();
   for(const [label,value] of [['保存的产品展示名称',d.presentation.display_name||'未设置 · 沿用当前规范展示'],['展示类别',AdminResourceEditor.categories[d.presentation.category]],['展示隐藏标记',d.presentation.hidden?'已保存 · 不限制分享与访问':'未标记'],['展示版本',String(d.presentation.version)],['Copy 状态',d.evidence.availability],['服务端读取时间',dateText(d.read_at)]]){const row=document.createElement('div'),dt=document.createElement('dt'),dd=document.createElement('dd');dt.textContent=label;dd.textContent=value;row.append(dt,dd);el('resource-edit-facts').append(row);}
   el('resource-edit-status').textContent=d.can_edit?'已读取持久展示信息 · 修改后需确认保存':'只读 · Copy 已移除、版本已达上限或当前不允许编辑';controls();
  }
  async function deadline(work,c){let timer;try{return await Promise.race([work,new Promise((_,reject)=>{timer=setTimeout(()=>{c.abort();reject(Error('Presentation deadline'));},15000);})]);}finally{clearTimeout(timer);}}
  async function fetchExact(t,c,ctx){return valid(await deadline(read(base(t.copy_id)+'?binding_id='+encodeURIComponent(t.binding_id),ctx.gen,c.signal),c),t);}
  async function load(){
   if(!target||!capable()||!bindingAllowed(target.binding_id)||!isActive())return;
   const t={...target},ctx=context(),n=++seq;controller?.abort();controller=new AbortController();const c=controller;busy=true;dismiss(false);clear();el('resource-edit-status').textContent='LOADING · 正在读取精确 Copy 展示信息';controls();
   try{const d=await fetchExact(t,c,ctx);if(n===seq&&same(ctx)&&target?.copy_id===t.copy_id&&capable()&&bindingAllowed(t.binding_id)&&!c.signal.aborted)draw(d);}
   catch(e){if(n===seq&&same(ctx)){clear();el('resource-edit-status').textContent=e.status===404?'NOT_FOUND · Copy 不存在或不在当前范围':'UNAVAILABLE · 未取得可信展示信息，编辑已关闭';onError(e);}}
   finally{if(n===seq&&same(ctx)){busy=false;controls();}}
  }
  async function open(item,focusEditor=false){
   invalidate();if(!item)return;el('resource-edit-panel').hidden=false;
   if(!capable()){el('resource-edit-status').textContent='当前环境未开放资源编辑 · 需要 Schema 20 及实际读写能力';return;}
   if(!uuid(item.copy_id)||!uuid(item.binding_id)||!bindingAllowed(item.binding_id)||typeof item.resource_id!=='string'||!item.resource_id){el('resource-edit-status').textContent='只读 · 当前资源没有可信的 Copy 投影，不能编辑；不会创建或猜测 Copy';return;}
   target={copy_id:item.copy_id,binding_id:item.binding_id,resource_id:item.resource_id};await load();if(focusEditor&&doc&&isActive()&&target?.copy_id===item.copy_id){el('resource-edit-panel').scrollIntoView({block:'start'});if(!el('resource-edit-name').disabled)el('resource-edit-name').focus();}
  }
  async function prepare(event){
   event?.preventDefault();if(busy||!doc?.can_edit||pending()||!capable()||!target)return;
   const returnFocus=event?.submitter||document.activeElement;
   const fields={display_name:el('resource-edit-name').value,category:el('resource-edit-category').value,hidden:el('resource-edit-hidden').checked};
   if(!AdminResourceEditor.name(fields.display_name)||!Object.hasOwn(AdminResourceEditor.categories,fields.category)||typeof fields.hidden!=='boolean'){el('resource-edit-status').textContent='输入未提交 · 展示名称最多 160 个字符，不接受 HTML、控制字符或首尾空白';return;}
   const t={...target},ctx=context(),version=doc.presentation.version,n=++seq;controller?.abort();controller=new AbortController();const c=controller;busy=true;controls();
   try{const d=await fetchExact(t,c,ctx);if(n!==seq||!same(ctx)||!capable()||!bindingAllowed(t.binding_id)||c.signal.aborted)return;
    if(!d.can_edit||d.presentation.version!==version){draw(d);el('resource-edit-status').textContent='资源或展示版本已变化 · 已刷新，请检查后重新编辑';return;}
    doc=d;confirmation={...ctx,...t,...fields,expected_version:version,at:now()};focus=returnFocus;el('resource-edit-summary').textContent='产品展示名称：'+(fields.display_name||'清除并沿用当前规范展示')+'；类别：'+AdminResourceEditor.categories[fields.category]+'；展示隐藏标记：'+(fields.hidden?'标记':'不标记')+'。不修改网盘文件名、索引或当前分享权限。';
    el('resource-edit-dialog').hidden=false;document.body.classList.toggle('resource-edit-confirm-open',true);el('resource-edit-dismiss').focus();confirmTimer=setTimeout(()=>dismiss(),30000);
   }catch(e){if(n===seq&&same(ctx)){clear();el('resource-edit-status').textContent='无法确认最新版本 · 未提交修改';onError(e);}}
   finally{if(n===seq&&same(ctx)){busy=false;controls();}}
  }
  function receipt(r,p){
   if(!keys(r,['presentation_version','request_id','action','target_id','status','code','recorded_at'])||r.request_id!==p.request_id||r.target_id!==p.target_id||r.action!=='SET_COPY_PRESENTATION'||!stamp(r.recorded_at))throw Error('Invalid presentation receipt');
   if(r.status==='UNKNOWN'){if(r.code!=='RESERVED_NO_CONFIRMED_RESULT'||r.presentation_version!==undefined)throw Error('Invalid reservation');return false;}
   if(r.status==='SUCCEEDED'){if(r.code!=='COPY_PRESENTATION_SET'||r.presentation_version!==p.expected_version+1)throw Error('Invalid success version');}
   else if(r.status!=='REJECTED'||!['COPY_NOT_FOUND','COPY_UNAVAILABLE','VERSION_CONFLICT'].includes(r.code)||r.presentation_version!==undefined)throw Error('Invalid rejection');
   return true;
  }
  async function operation(lookup=false){
   if(busy||!actor()||!isActive())return;let p=pending(),body,receiptView;
   function showReceipt(message,evidence){receiptView={message,evidence};present(el('resource-edit-result'),message,evidence,'原始回执');}
   if(lookup){if(!p||p.corrupt||!bindingAllowed(p.binding_id))return;}
   else{
    const v=confirmation;if(p||!v||!same(v)||!capable()||!bindingAllowed(v.binding_id)||target?.copy_id!==v.copy_id||doc?.presentation.version!==v.expected_version||now()-v.at>=30000){dismiss();return;}
    let request_id;try{request_id=newID();}catch{dismiss();el('resource-edit-status').textContent='无法生成安全请求标识 · 未发送修改';return;}if(!uuid(request_id)){dismiss();el('resource-edit-status').textContent='无法生成安全请求标识 · 未发送修改';return;}
    p={request_id,target_id:v.copy_id,binding_id:v.binding_id,expected_version:v.expected_version};
    if(!save(v.actor,p)){dismiss();el('resource-edit-status').textContent='无法保存原请求恢复标识 · 未发送修改，请检查浏览器会话存储';return;}
    body={request_id,expected_version:v.expected_version,binding_id:v.binding_id,display_name:v.display_name,category:v.category,hidden:v.hidden};dismiss(false);
   }
   const ctx=context(),original={...p},n=++opSeq;controller?.abort();controller=new AbortController();const c=controller;let permit=true,terminal=false,conflict=false;busy=true;clear();el('resource-edit-status').textContent='结果未确认 · 编辑已锁定，请查询原审计回执';
   const current=()=>n===opSeq&&same(ctx)&&pending()?.request_id===original.request_id&&bindingAllowed(original.binding_id);
   el('resource-edit-result').textContent=lookup?'正在查询原审计回执…':'保存中 · 尚未确认结果';controls();el('resource-edit-recovery').focus();el('resource-edit-recovery').scrollIntoView({block:'nearest'});
   try{
    const work=lookup?read('/api/v1/admin/actions/'+original.request_id,ctx.gen,c.signal):command(base(original.target_id),body,ctx.gen,c.signal,()=>permit&&current()&&capable());
    const r=await deadline(work,c);if(!current()||c.signal.aborted)return;terminal=receipt(r,original);
    showReceipt(r.status==='SUCCEEDED'?'保存已记录 · 版本 '+r.presentation_version+' · 正在重新读取服务器展示信息':terminal?'修改被拒绝，请检查最新信息':'结果未确认，请查询原请求，不要重复保存',r.status+' / '+r.code);
   }catch(e){if(current()){
    if(!lookup&&[409,404].includes(e.status)){conflict=true;try{terminal=receipt(e.receipt,original);showReceipt('修改未完成，已请求刷新',e.receipt.code);}catch{el('resource-edit-result').textContent='冲突或目标变化 · 回执未确认，只能查询原请求';}}
    else present(el('resource-edit-result'),'结果未确认，只能查询原审计回执，不会重复发送修改','UNKNOWN','原始状态');onError(e);
   }}finally{permit=false;if(n===opSeq&&ctx.actor===actor()){busy=false;controls();}}
   if(!current())return;
   if(terminal){last={...original,actor:ctx.actor};finish(ctx.actor);controls();}
   if((terminal||conflict)&&target&&capable())await load();
   if(terminal&&doc&&receiptView&&n===opSeq&&same(ctx))showReceipt(receiptView.message.replace('正在重新读取服务器展示信息','已重新读取当前展示信息'),receiptView.evidence);
  }
  function keyHandler(event){
   if(el('resource-edit-dialog').hidden)return false;
   if(event.key==='Escape'){event.preventDefault?.();dismiss();return true;}
   if(event.key==='Tab'){const first=el('resource-edit-dismiss'),last=el('resource-edit-confirm');if(event.shiftKey&&document.activeElement===first){event.preventDefault();last.focus();}else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first.focus();}return true;}return false;
  }
  for(const [value,label] of Object.entries(AdminResourceEditor.categories)){const o=document.createElement('option');o.value=value;o.textContent=label;el('resource-edit-category').append(o);}
  el('resource-edit-form').addEventListener('submit',prepare);el('resource-edit-dismiss').addEventListener('click',()=>dismiss());el('resource-edit-confirm').addEventListener('click',()=>operation(false));el('resource-edit-lookup').addEventListener('click',()=>operation(true));el('resource-edit-refresh').addEventListener('click',load);
  return {open,invalidate,reset,controls,key:keyHandler,prepare};
 }
};
