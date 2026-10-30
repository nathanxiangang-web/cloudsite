'use strict';
// Session boundary: no token storage, no admin API inference, no domain mutations.
const el = id => document.getElementById(id);
const sections = {
 overview:['总览','查看用户与下载概况，及时发现需要处理的记录。'],
 users:['用户管理','搜索和查看用户，创建、重命名、重置密码、启停、删除及恢复普通账号。'],
 resources:['资源管理','查找文件、整理分类和展示信息。'],
 downloads:['下载任务','查看下载进度与待处理请求。'],
 storage:['存储连接','查看配置，按需检测连接。'],
 indexcore:['索引状态','手动查看所选空间的索引状态。'],
 shares:['分享管理','查找全站分享、查看文件信息，并撤销不再需要的分享。'],
 audit:['操作记录','查询你执行过的管理操作。'],
};
const state = {session:0, principal:null, workspace:null, workspaceRequest:0, tab:'overview', drawer:false,
 binding:'', parent:null, parents:[], resourceCursor:'', resourceNext:'', resourceRequest:0, resourceScope:'',
 resources:[], resourcesLoaded:false, category:'all', layout:'list', taskCursor:'', taskNext:'', taskRequest:0,
 tasksLoaded:false, detailRequest:0, taskDetailRequest:0, resourceFocus:null, taskFocus:null, controller:new AbortController(),
 logoutStatus:'NONE', logoutRequest:0, logoutController:null, logoutInFlight:false, loginInFlight:false};
const messages = {AUTH_REQUIRED:'登录已失效，请重新登录。',AUTH_CHANGED:'账号或权限已变化，请重新登录。',ADMIN_REQUIRED:'此入口仅供管理员使用，请返回前台文件库。',INVALID_REQUEST:'输入不符合要求，请检查后再试。',ORIGIN_REJECTED:'当前页面来源未获允许。',AUTH_UNAVAILABLE:'登录服务暂时不可用。',AUTH_BUSY:'登录请求过多，请稍候一分钟。',NOT_FOUND:'未找到可见资源或属于你的任务。',RESOURCE_AMBIGUOUS:'资源存在多个匹配，系统不会猜选。',READ_UNAVAILABLE:'查询服务暂时不可用，未确认任何成功结果。',SERVICE_STOPPING:'服务正在安全关闭。'};
function errorText(error){return error.name==='AbortError'?'请求已取消。':(messages[error.code]||'连接失败，请检查服务状态后重试。');}
function showError(id,error){el(id).textContent=errorText(error);el(id).hidden=false;}
function clearError(id){el(id).textContent='';el(id).hidden=true;}
function stale(){const e=new Error('Obsolete request');e.name='AbortError';return e;}
function current(generation){if(generation!==state.session)throw stale();}
async function api(path,method='GET',body,generation=state.session,signal=state.controller.signal){
 const response=await fetch(path,{method,credentials:'same-origin',cache:'no-store',signal,headers:body?{'Content-Type':'application/json'}:{},body:body?JSON.stringify(body):undefined});
 if(signal.aborted)throw stale();
 current(generation);
 if(response.status===204)return null;
 // Authorization invalidates private views even when a proxy returns non-JSON.
 if(response.status===401||(response.status===403&&path.startsWith('/api/v1/admin/'))){
  const error=new Error('Authorization ended');error.code=response.status===401?'AUTH_REQUIRED':'ADMIN_REQUIRED';
  if(method==='GET'){signedOut();showError('login-error',error);}throw error;
 }
 const data=await response.json();current(generation);
 if(!response.ok){const error=new Error('Request refused');error.code=data.code;error.status=response.status;if(path.endsWith('/cancel-pending')||/\/admin\/copies\/[^/]+\/presentation$/.test(path))error.receipt=data;throw error;}
 return data;
}
function clearResources(){
 state.resources=[];state.resourcesLoaded=false;state.resourceNext='';state.detailRequest++;
 el('resource-rows').replaceChildren();el('resource-cards').replaceChildren();closeResourceDetail();
 el('resources-empty').hidden=true;el('more-resources').disabled=true;el('resource-count').textContent='—';
 for(const key of ['all','software','video','document','other'])el('count-'+key).textContent='—';
}
function clearTasks(){
 state.tasksLoaded=false;state.taskNext='';state.taskDetailRequest++;el('task-rows').replaceChildren();
 el('task-facts').replaceChildren();el('task-detail').hidden=true;el('tasks-empty').hidden=true;el('more-tasks').disabled=true;
}
function signedOut(){
 state.logoutRequest++;state.logoutController?.abort();state.logoutController=null;state.logoutInFlight=false;state.loginInFlight=false;
 if(state.logoutStatus==='LOGOUT_PENDING')setLogoutStatus('LOGOUT_UNCONFIRMED','注销未确认。服务器会话可能仍然有效，请重试退出登录。');
 state.session++;state.controller.abort();state.controller=new AbortController();state.resourceRequest++;state.taskRequest++;state.workspaceRequest++;
 state.principal=null;state.workspace=null;clearResources();clearTasks();adminLists.reset();adminDownloadDetail.reset();adminPendingCancel.reset();adminObservation.reset();adminOperations.reset();adminShares.reset();adminOverview.reset();adminAudit.reset();adminResourceEditor.reset();setDrawer(false,false);
 for(const id of ['user-login','user-role','avatar','schema','binding-select','workspace-facts','identity-facts','overview-binding-list'])el(id).replaceChildren();
 for(const id of ['overview-schema','overview-bindings','overview-users-total'])el(id).textContent='—';
 el('workspace-observed').textContent='尚未读取';el('workspace-status').textContent='等待查询';
 el('workspace-view').hidden=true;el('login-view').hidden=false;el('login-password').value='';el('resource-search').value='';el('resolve-path').value='/';
 el('directory-label').textContent='根层级';el('directory-back').disabled=true;
 el('refresh-resources').disabled=false;el('refresh-tasks').disabled=false;el('refresh-workspace').disabled=false;
 el('login-submit').disabled=false;el('logout-retry').disabled=false;
 el('resources-footer').textContent='等待查询';el('tasks-footer').textContent='等待查询';
 state.binding='';state.parent=null;state.parents=[];state.resourceCursor='';state.resourceScope='';state.taskCursor='';state.category='all';state.layout='list';
 state.resourceFocus=null;state.taskFocus=null;state.tab='overview';clearError('workspace-error');
}
// A non-secret URL fragment retains only logout intent across normal reloads.
// It never represents revocation, a session identifier, identity or permission.
function logoutIntent(){return window.location?.hash==='#logout';}
function rememberLogoutIntent(){window.history?.replaceState(null,'','#logout');}
function clearLogoutIntent(){if(logoutIntent())window.history?.replaceState(null,'',window.location.pathname+window.location.search);}
function setLogoutStatus(status,message=''){
 state.logoutStatus=status;el('logout-feedback').hidden=status==='NONE';el('logout-message').textContent=message;
 el('logout-feedback').className='logout-notice'+(status==='LOGOUT_CONFIRMED'?' confirmed':'');
 el('logout-retry').hidden=status!=='LOGOUT_PENDING'&&status!=='LOGOUT_UNCONFIRMED';el('logout-retry').disabled=status==='LOGOUT_PENDING';
}
async function revokeLogout(signal){
 const options={credentials:'same-origin',cache:'no-store',signal};
 const response=await fetch('/api/v1/auth/logout',{...options,method:'POST'});
 if(signal.aborted)throw stale();
 if(response.status===204)return 'REVOKED';
 if(response.status===401){
  // POST denial alone is not proof of revocation. At most one read-only check;
  // do not use api(), whose generic401 handler changes session generation.
  const check=await fetch('/api/v1/auth/me',{...options,method:'GET'});
  if(signal.aborted)throw stale();
  if(check.status===401)return 'ALREADY_INVALID';
 }
 throw new Error('Logout not confirmed');
}
async function logout(){
 if(state.logoutInFlight||state.loginInFlight)return; // One explicit attempt, no automatic retry.
 signedOut();rememberLogoutIntent();clearError('login-error');
 const generation=state.session,request=++state.logoutRequest,controller=new AbortController();
 state.logoutController=controller;state.logoutInFlight=true;el('login-submit').disabled=true;
 setLogoutStatus('LOGOUT_PENDING','正在确认服务器注销…本地管理数据已清空，尚未确认退出。');
 let timer;
 try{
  const timeout=new Promise((_,reject)=>{timer=setTimeout(()=>{controller.abort();reject(stale());},15000);});
  const result=await Promise.race([revokeLogout(controller.signal),timeout]);
  if(generation!==state.session||request!==state.logoutRequest)return;
  clearLogoutIntent();setLogoutStatus('LOGOUT_CONFIRMED',result==='REVOKED'?'已退出登录。服务器已确认注销。':'当前会话已失效，服务器已确认无法访问；无需再次退出。');
 }catch(error){
  if(generation===state.session&&request===state.logoutRequest)setLogoutStatus('LOGOUT_UNCONFIRMED','注销未确认。服务器会话可能仍然有效，请重试退出登录。重新登录不会替代原会话注销。');
 }finally{
  clearTimeout(timer);
  if(generation===state.session&&request===state.logoutRequest){state.logoutController=null;state.logoutInFlight=false;el('login-submit').disabled=false;el('logout-retry').disabled=false;}
 }
}
function facts(id,rows){
 const list=el(id);list.replaceChildren();
 for(const [label,value] of rows){const row=document.createElement('div'),dt=document.createElement('dt'),dd=document.createElement('dd');dt.textContent=label;dd.textContent=value===null||value===undefined?'未提供':String(value);row.append(dt,dd);list.append(row);}
}
function renderWorkspace(workspace,principal){
	adminObservation.configure(workspace.bindings);adminOperations.configure(workspace.bindings);adminShares.configure();
 state.workspace=workspace;adminResourceEditor.controls();el('user-login').textContent=principal.login;el('user-role').textContent='ADMIN · 管理中心';
 el('avatar').textContent=principal.login.charAt(0).toUpperCase();el('schema').textContent='Schema v'+workspace.schema_version;
 el('overview-schema').textContent='v'+workspace.schema_version;el('overview-bindings').textContent=String(workspace.bindings.length);
 el('workspace-status').textContent='已读取配置 · 非健康探测';el('workspace-status').className='badge neutral';
 const flag=value=>value===true?'配置启用 · 未验证运行结果':value===false?'配置未启用':'未提供';
 facts('workspace-facts',[['产品模式',workspace.mode],['云下载能力',flag(workspace.cloud_download_enabled)],['来源检查能力',flag(workspace.cloud_download_preview_enabled)],['文件访问能力',flag(workspace.content_access_enabled)],['分享能力',flag(workspace.sharing_enabled)],['覆盖范围','当前产品进程的配置能力描述；非全站业务统计']]);
 facts('identity-facts',[['登录账号',principal.login],['产品 User ID',principal.user_id],['当前角色',principal.role]]);
 el('workspace-observed').textContent='客户端读取于 '+new Date().toLocaleString()+' · source: admin/workspace + auth/me';
 const bindings=el('overview-binding-list');bindings.replaceChildren();el('binding-select').replaceChildren();
 for(const binding of workspace.bindings){
  const option=document.createElement('option');option.value=binding.binding_id;option.textContent=binding.label;el('binding-select').append(option);
  const item=document.createElement('div'),name=document.createElement('strong'),id=document.createElement('small');name.textContent=binding.label;id.textContent=binding.binding_id;item.append(name,id);bindings.append(item);
 }
 if(!workspace.bindings.some(b=>b.binding_id===state.binding)){
  state.binding=workspace.bindings[0]?.binding_id||'';state.parent=null;state.parents=[];state.resourceCursor='';state.resourceScope='';state.resourceRequest++;clearResources();
  el('directory-label').textContent='根层级';el('directory-back').disabled=true;
 }
 el('binding-select').value=state.binding;el('resolve-submit').disabled=!state.binding;
}
// Existing RoleAdmin-protected descriptor is checked before ordinary read APIs.
// /me also detects a different current principal; never adopt another session's data.
async function verifyAdmin(generation=state.session){
 const workspace=await api('/api/v1/admin/workspace','GET',undefined,generation);
 const principal=await api('/api/v1/auth/me','GET',undefined,generation);current(generation);
 if(principal.role!=='ADMIN'||(state.principal&&state.principal.user_id!==principal.user_id)){
  const error=new Error('Identity changed');error.code='AUTH_CHANGED';signedOut();showError('login-error',error);throw error;
 }
 if(state.principal){const previous=state.workspace;state.workspace=workspace;
  const scope=w=>JSON.stringify([w?.schema_version,w?.admin_pending_cancellation_enabled,w?.admin_audit_enabled,w?.admin_copy_presentation_enabled,(w?.bindings||[]).map(b=>b.binding_id).sort()]);
  if(scope(previous)!==scope(workspace)){adminPendingCancel.invalidate();adminAudit.reset();adminResourceEditor.invalidate();}
 }
 return {workspace,principal};
}
async function adminRead(path,generation=state.session,signal){
 await verifyAdmin(generation);current(generation);if(signal?.aborted)throw stale();return api(path,'GET',undefined,generation,signal);
}
async function signedIn(principal,generation=state.session){
 current(generation);
 const workspace=await api('/api/v1/admin/workspace','GET',undefined,generation);current(generation);
 if(principal.role!=='ADMIN'){const error=new Error('Admin required');error.code='ADMIN_REQUIRED';signedOut();throw error;}
 state.logoutRequest++;state.logoutController?.abort();state.logoutController=null;state.logoutInFlight=false;setLogoutStatus('NONE');clearLogoutIntent();
 state.principal=principal;renderWorkspace(workspace,principal);el('login-view').hidden=true;el('workspace-view').hidden=false;
 clearError('login-error');window.scrollTo(0,0);selectTab('overview',false);
}
async function refreshWorkspace(){
 if(!state.principal)return;const generation=state.session,request=++state.workspaceRequest;
 const summary=state.tab==='overview'?adminOverview.load():Promise.resolve();
 el('refresh-workspace').disabled=true;el('workspace-status').textContent='正在读取配置…';
 try{
  const {workspace,principal}=await verifyAdmin(generation);
  if(generation!==state.session||request!==state.workspaceRequest)return;
  renderWorkspace(workspace,principal);clearError('workspace-error');
 }
 catch(error){if(generation===state.session&&request===state.workspaceRequest){el('workspace-status').textContent='STALE · 配置读取失败';el('workspace-status').className='badge warning';showError('workspace-error',error);}}
 finally{await summary;if(generation===state.session&&request===state.workspaceRequest)el('refresh-workspace').disabled=false;}
}

// Canonical resource presentation reuses the consumer-compatible pure helper.
// Pure DOM formatting port; controllers keep their transport and state contracts.
function present(node,primary,evidence='',caption='技术详情'){
 node.replaceChildren();if(primary){const text=document.createElement('span');text.textContent=primary;node.append(text);}
 if(evidence){const disclosure=document.createElement('details'),summary=document.createElement('summary'),body=document.createElement('div');disclosure.className='technical-details';summary.textContent=caption;body.className='technical-evidence';body.textContent=evidence;disclosure.append(summary,body);node.append(disclosure);}
}
function friendlyInfo(value){
 const text=String(value),prefix=text.split(' · ')[0];
 const states={SUCCEEDED:'操作已完成',REJECTED:'操作未执行，请查看记录原因',UNKNOWN:'结果未确认，请查询原操作结果，不要重复提交'};
 if(states[prefix])return states[prefix];
 return text.replace(/\bLOADING\b/g,'正在查询').replace(/\bUNAVAILABLE\b/g,'暂不可用').replace(/\bSTALE\b/g,'上次结果').replace(/\bNOT_CHECKED\b/g,'尚未检测').replace(/\bNOT_TRACKED\b/g,'暂无观测').replace(/\b[0-9a-f]{8}-[0-9a-f-]{27}\b/g,'见技术详情');
}
function rowLabels(row,labels){Array.from(row.children).forEach((node,i)=>node.setAttribute('data-label',labels[i]||''));}
function resourceState(value){return {PRESENT:'已索引',REMOVED:'已移除'}[value]||'未提供';}
function badge(value,kind=''){const node=document.createElement('span');node.className='badge'+(kind?' '+kind:'');node.textContent=value;return node;}
function cell(row,text){const node=document.createElement('td');if(text!==undefined)node.textContent=text;row.append(node);return node;}
function size(value){return ResourceDisplay.size(value);}
function resourceName(item){return ResourceDisplay.name(item);}
function closeResourceDetail(returnFocus=false){adminResourceEditor.invalidate();el('file-detail').hidden=true;el('resource-identities').replaceChildren();for(const id of ['file-title','file-type','file-path','file-size','file-presence','file-copy','file-id','file-note','file-glyph'])el(id).textContent='';if(returnFocus)state.resourceFocus?.focus();}
function resourceDetail(item,edit=false){
 const type=ResourceDisplay.classify(item);el('file-title').textContent=resourceName(item);el('file-type').textContent=type.label+(type.extension?' · .'+type.extension:'');el('file-path').textContent=item.path||'路径未提供';el('file-size').textContent=size(item.size_bytes);el('file-presence').textContent=resourceState(item.presence);el('file-copy').textContent=item.copy_id?(item.copy_availability||'状态未提供')+' · '+item.copy_id:'当前产品库未投影';el('file-id').textContent=item.resource_id;el('file-note').textContent=ResourceDisplay.accessNote(item);el('file-glyph').textContent=type.glyph;el('file-glyph').className='file-hero '+type.key;
 facts('resource-identities',[['Binding ID',item.binding_id],['Root ID','当前 Resource DTO 未提供'],['Asset ID',item.asset_id],['Release ID',item.release_id],['Variant ID',item.variant_id],['来源 / 覆盖','产品 Q4 当前页或 Q5 精确定位；不推测物理删除']]);
 el('file-detail').hidden=false;adminResourceEditor.open(item,edit);el('file-detail').focus?.();el('file-detail').scrollIntoView({block:'nearest'});
}
function openResource(item,edit=false){
 if(item.directory===true){state.parents.push({parent:state.parent,label:el('directory-label').textContent});state.parent=item.resource_id;state.resourceCursor='';el('directory-label').textContent=item.path||resourceName(item);el('directory-back').disabled=false;loadResources();}
 else{state.detailRequest++;state.resourceFocus=document.activeElement;resourceDetail(item,edit);}
}
function drawResources(){
 if(!state.resourcesLoaded)return;
 const counts=ResourceDisplay.counts(state.resources);for(const key of Object.keys(counts)){el('count-'+key).textContent=String(counts[key]);el('category-'+key).setAttribute('aria-pressed',String(state.category===key));}
 const items=ResourceDisplay.filter(state.resources,state.category,el('resource-search').value);
 el('resource-rows').replaceChildren();el('resource-cards').replaceChildren();
 for(const item of items){
  const type=ResourceDisplay.classify(item);const row=document.createElement('tr');const name=cell(row);const title=document.createElement('button');title.type='button';title.className='file-button';title.textContent=resourceName(item);title.addEventListener('click',()=>openResource(item));const icon=document.createElement('span');icon.className='file-icon '+type.key;icon.textContent=type.glyph;name.append(icon,title);if(item.path){const path=document.createElement('small');path.textContent=item.path;name.append(path);}
  cell(row,type.label);cell(row,size(item.size_bytes));cell(row).append(badge(resourceState(item.presence)));cell(row).append(badge(item.copy_id?'已关联':'暂无记录',item.copy_id?'':'neutral'));if(state.workspace?.schema_version>=20&&state.workspace?.admin_copy_presentation_enabled===true&&AdminResourceEditor.uuid(item.copy_id)&&item.copy_availability==='PRESENT'&&state.workspace.bindings.some(b=>b.binding_id===item.binding_id)&&item.directory!==true){const edit=document.createElement('button');edit.type='button';edit.className='quiet';edit.textContent='编辑展示信息';edit.addEventListener('click',()=>openResource(item,true));name.append(edit);}
 rowLabels(row,['文件','类型','大小','索引状态','管理记录']);el('resource-rows').append(row);
  const card=document.createElement('button');card.type='button';card.className='resource-card';card.addEventListener('click',()=>openResource(item));const art=document.createElement('span');art.className='card-art '+type.key;art.textContent=type.glyph;const label=document.createElement('strong');label.textContent=resourceName(item);const meta=document.createElement('small');meta.textContent=type.label+' · '+size(item.size_bytes);const path=document.createElement('small');path.className='card-path';path.textContent=item.path||'路径未提供';card.append(art,label,meta,path,badge(resourceState(item.presence)),badge(item.copy_id?'已关联':'暂无记录','neutral'));el('resource-cards').append(card);
 }
 el('resource-table').hidden=state.layout!=='list';el('resource-cards').hidden=state.layout!=='cards';el('view-list').setAttribute('aria-pressed',String(state.layout==='list'));el('view-cards').setAttribute('aria-pressed',String(state.layout==='cards'));el('resources-empty').hidden=items.length!==0;
 el('empty-title').textContent=state.resources.length?'当前页没有匹配的文件':'这个层级当前没有资源';el('empty-description').textContent=state.resources.length?'分类和搜索只筛选本页；其他分页或目录不在此结果中。':'规范查询返回空结果，不代表整个网盘为空；页面不会编造文件或触发扫描。';el('resources-footer').textContent='本页显示 '+items.length+' / '+state.resources.length+' 项 · 不含其他分页';
}
function renderResources(page){
 state.resources=page.items;state.resourcesLoaded=true;state.resourceNext=page.next_cursor||'';el('more-resources').disabled=!state.resourceNext;el('resource-count').textContent=String(page.items.length);drawResources();
}

function scopeKey(){return [state.binding,state.parent||'',state.resourceCursor].join('|');}
async function loadResources(){
 if(!state.binding){clearResources();el('resources-footer').textContent='未配置展示空间';return;}
 const generation=state.session,request=++state.resourceRequest,key=scopeKey();const previous=state.resourceScope===key&&state.resourcesLoaded;
 if(!previous)clearResources();else{state.detailRequest++;closeResourceDetail();}
 state.resourceScope=key;el('resources-empty').hidden=true;clearError('workspace-error');el('refresh-resources').disabled=true;el('more-resources').disabled=true;el('resources-footer').textContent=previous?'正在刷新… · 旧页暂留':'LOADING · 正在查询…';
 const query=new URLSearchParams({binding_id:state.binding,limit:'20'});if(state.parent)query.set('parent_resource_id',state.parent);if(state.resourceCursor)query.set('cursor',state.resourceCursor);
 try{const page=await adminRead('/api/v1/resources?'+query,generation);if(generation===state.session&&request===state.resourceRequest)renderResources(page);}
 catch(error){if(generation===state.session&&request===state.resourceRequest){el('resources-empty').hidden=true;el('resources-footer').textContent=previous?'STALE · 查询失败，保留上次同页结果（未确认最新）':'查询未完成 · 不以空结果代替失败';showError('workspace-error',error);}}
 finally{if(generation===state.session&&request===state.resourceRequest)el('refresh-resources').disabled=false;}
}
const manifestLabels={PENDING:'等待激活',ACTIVE:'执行中',AWAITING_VISIBILITY:'等待观察',AWAITING_CANONICAL:'等待规范确认',READY:'READY',FAILED:'失败',RECOVERY_REQUIRED:'需人工恢复',CANCELED:'已取消'};
function dateText(value){const d=new Date(value);return value&&Number.isFinite(d.getTime())?d.toLocaleString():'未提供';}
const adminLists=AdminLists.create({present,friendlyInfo,rowLabels,el,read:adminRead,generation:()=>state.session,principal:()=>state.principal,onError:error=>showError('workspace-error',error),dateText,cell,badge,downloadSelected:id=>adminDownloadDetail.load(id),downloadsChanged:()=>adminDownloadDetail.close(),bindingAllowed:id=>state.workspace?.bindings.some(b=>b.binding_id===id)===true,userSelected:item=>adminOperations.selectUser(item),ownerSelected:id=>{el('admin-user-filter').value=id;adminLists.downloads.owner=id;adminLists.downloads.cursor='';selectTab('downloads');}});
const adminOverview=AdminOverview.create({present,friendlyInfo,rowLabels,el,read:adminRead,generation:()=>state.session,principal:()=>state.principal,isActive:()=>state.tab==='overview',dateText});
const adminResourceEditor=AdminResourceEditor.create({present,el,read:adminRead,generation:()=>state.session,principal:()=>state.principal,isActive:()=>state.tab==='resources',bindingAllowed:id=>state.workspace?.bindings.some(b=>b.binding_id===id)===true,capable:()=>state.workspace?.schema_version>=20&&state.workspace?.admin_copy_presentation_enabled===true,dateText,newID:()=>window.crypto.randomUUID(),onError:error=>{if(['AUTH_REQUIRED','ADMIN_REQUIRED','AUTH_CHANGED'].includes(error.code)){signedOut();showError('login-error',error);}},command:async(path,body,gen,signal,guard)=>{const {workspace}=await verifyAdmin(gen);current(gen);if(signal.aborted||!guard()||workspace.schema_version<20||workspace.admin_copy_presentation_enabled!==true||!workspace.bindings.some(b=>b.binding_id===body.binding_id))throw stale();return api(path,'PUT',body,gen,signal);}});
const adminDownloadDetail=AdminDownloadDetail.create({onChange:d=>adminPendingCancel.observe(d),el,read:adminRead,generation:()=>state.session,principal:()=>state.principal,isActive:()=>state.tab==='downloads',dateText,bindingAllowed:id=>state.workspace?.bindings.some(b=>b.binding_id===id)===true});
const adminPendingCancel=AdminPendingCancel.create({present,el,read:adminRead,readExact:id=>adminDownloadDetail.load(id,true),generation:()=>state.session,principal:()=>state.principal,isActive:()=>state.tab==='downloads',bindingAllowed:id=>state.workspace?.bindings.some(b=>b.binding_id===id)===true,capable:()=>state.workspace?.schema_version>=19&&state.workspace?.admin_pending_cancellation_enabled===true,newID:()=>window.crypto.randomUUID(),onError:error=>{signedOut();showError('login-error',error);},onMissing:()=>adminDownloadDetail.close(),onTerminal:async(id,missing)=>{const gen=state.session;await Promise.all([adminLists.downloads.load(),adminOverview.load(true)]);if(gen===state.session&&state.tab==='downloads'&&!missing)await adminDownloadDetail.load(id,true);},command:async(path,body,gen,signal,guard)=>{const {workspace}=await verifyAdmin(gen);current(gen);if(!guard()||signal.aborted||workspace.schema_version<19||workspace.admin_pending_cancellation_enabled!==true||!workspace.bindings.some(b=>b.binding_id===body.expected_binding_id)){throw stale();}return api(path,'POST',body,gen,signal);}});
const adminAudit=AdminAudit.create({present,friendlyInfo,rowLabels,el,read:adminRead,generation:()=>state.session,principal:()=>state.principal,isActive:()=>state.tab==='audit',capable:()=>state.workspace?.schema_version>=16&&state.workspace?.admin_audit_enabled===true,dateText,cell,badge,onError:error=>showError('workspace-error',error)});
const adminObservation=AdminObserve.create({present,friendlyInfo,rowLabels,el,read:adminRead,generation:()=>state.session,principal:()=>state.principal,onError:error=>showError('workspace-error',error),dateText,cell,badge});
const adminOperations=AdminOperations.create({present,friendlyInfo,rowLabels,el,read:adminRead,command:async(path,method,body,gen)=>{await verifyAdmin(gen);current(gen);return api(path,method,body,gen);},generation:()=>state.session,principal:()=>state.principal,dateText,onError:error=>{if(error.code==='AUTH_REQUIRED'||error.code==='ADMIN_REQUIRED')signedOut();showError('workspace-error',error);},onUsersChanged:()=>adminLists.users.load(),newID:()=>window.crypto.randomUUID()});
const adminShares=AdminShares.create({present,friendlyInfo,rowLabels,el,read:adminRead,command:async(path,method,body,gen)=>{await verifyAdmin(gen);current(gen);return api(path,method,body,gen);},generation:()=>state.session,principal:()=>state.principal,isActive:()=>state.tab==='shares',dateText,cell,badge,onStart:()=>clearError('workspace-error'),onError:error=>{if(error.code==='AUTH_REQUIRED'||error.code==='ADMIN_REQUIRED')signedOut();showError('workspace-error',error);},newID:()=>window.crypto.randomUUID()});
Object.assign(messages,{NOT_CONNECTED:'Q9 尚未连接，未执行在线检测。',ROOT_NOT_READY:'Root 状态尚未就绪；没有成功观测。',ROOT_NOT_FOUND:'未取得该 Root 的状态，不能推断文件或网盘已删除。',OBSERVATION_UNKNOWN:'本次观测失败，实际运行状态未知。',OBSERVATION_BUSY:'观测频率或并发已达上限，请稍后手动查询。'});
function renderTasks(page){
 el('task-rows').replaceChildren();
 for(const item of page.items){const row=document.createElement('tr'),button=document.createElement('button');button.type='button';button.className='file-button';button.textContent=item.manifest_id.slice(0,8)+'…';button.title=item.manifest_id;button.addEventListener('click',()=>taskDetail(item.manifest_id));cell(row).append(button);
 cell(row).append(badge(manifestLabels[item.manifest_state]||item.manifest_state,item.manifest_state==='RECOVERY_REQUIRED'?'warning':'neutral'));cell(row,item.job_state||'未创建 Job');cell(row,item.claim_attempts+' / '+item.failure_count);cell(row,dateText(item.updated_at));el('task-rows').append(row);}
 state.tasksLoaded=true;state.taskNext=page.next_cursor||'';el('more-tasks').disabled=!state.taskNext;el('tasks-empty').hidden=page.items.length!==0;el('tasks-footer').textContent='本页 '+page.items.length+' 项 · 仅当前登录者，不是全站任务';
}
async function loadTasks(){
 const generation=state.session,request=++state.taskRequest,previous=state.tasksLoaded;closeTaskDetail();
 clearError('workspace-error');el('refresh-tasks').disabled=true;el('more-tasks').disabled=true;el('tasks-empty').hidden=true;el('tasks-footer').textContent=previous?'正在刷新… · 旧页暂留':'LOADING · 正在查询…';
 const query=new URLSearchParams({limit:'20'});if(state.taskCursor)query.set('cursor',state.taskCursor);
 try{const page=await adminRead('/api/v1/tasks?'+query,generation);if(generation===state.session&&request===state.taskRequest)renderTasks(page);}
 catch(error){if(generation===state.session&&request===state.taskRequest){el('tasks-empty').hidden=true;el('tasks-footer').textContent=previous?'STALE · 查询失败，保留上次同页结果（未确认最新）':'查询未完成 · 不以空结果代替失败';showError('workspace-error',error);}}
 finally{if(generation===state.session&&request===state.taskRequest)el('refresh-tasks').disabled=false;}
}
function closeTaskDetail(returnFocus=false){state.taskDetailRequest++;el('task-detail').hidden=true;el('task-facts').replaceChildren();if(returnFocus)state.taskFocus?.focus();}
async function taskDetail(id){
 const generation=state.session;closeTaskDetail();const request=state.taskDetailRequest;state.taskFocus=document.activeElement;
 try{const item=await adminRead('/api/v1/tasks/'+encodeURIComponent(id),generation);if(generation!==state.session||request!==state.taskDetailRequest)return;
 facts('task-facts',[['Manifest ID',item.manifest_id],['Job ID',item.job_id],['Manifest 状态',manifestLabels[item.manifest_state]||item.manifest_state],['Job 状态',item.job_state],['结果 Copy ID',item.result_copy_id],['领取次数',item.claim_attempts],['失败次数',item.failure_count],['创建时间',dateText(item.created_at)],['更新时间',dateText(item.updated_at)]]);
 el('task-detail').hidden=false;el('task-detail').focus();el('task-detail').scrollIntoView({block:'nearest'});
 }catch(error){if(generation===state.session&&request===state.taskDetailRequest)showError('workspace-error',error);}
}

// Local navigation does not invent routes or request future backend endpoints.
function selectTab(tab,probe=true){
 const own=tab==='tasks',section=own?'downloads':tab;if(!sections[section])return;
 state.resourceRequest++;state.taskRequest++;state.detailRequest++;state.workspaceRequest++;adminOverview.invalidate();adminAudit.invalidate();adminDownloadDetail.invalidate();adminPendingCancel.invalidate();adminLists.invalidate();adminObservation.invalidate();adminShares.invalidate();closeResourceDetail();closeTaskDetail();state.tab=tab;setDrawer(false,false);el('refresh-workspace').disabled=false;
 for(const id of ['overview','users','resources','downloads','storage','indexcore','shares','audit'])el(id+'-panel').hidden=section!==id;
 el('user-commands-panel').hidden=section!=='users';el('account-probe-panel').hidden=section!=='storage';el('admin-action-panel').hidden=!['users','storage'].includes(section);
 if(['storage','indexcore','audit'].includes(section))el('diagnostics-nav').open=true;
 for(const name of Object.keys(sections)){el('nav-'+name).classList.toggle('selected',section===name);el('nav-'+name).setAttribute('aria-current',section===name?'page':'false');}
 el('tasks-panel').hidden=!own;el('global-tasks-placeholder').hidden=own;el('nav-tasks').setAttribute('aria-pressed',String(own));el('scope-global').setAttribute('aria-pressed',String(!own));
 el('page-title').textContent=own?'我的任务':sections[section][0];el('page-caption').textContent=own?'云下载与任务 / 我的任务':sections[section][0];el('page-subtitle').textContent=sections[section][1];
 clearError('workspace-error');if(probe){el('main-content').focus({preventScroll:true});window.scrollTo(0,0);}
 if(tab==='overview'){if(probe)refreshWorkspace();else adminOverview.load();}else if(tab==='audit')adminAudit.load();else if(tab==='shares')adminShares.load();else if(tab==='storage')adminObservation.loadStorage();else if(tab==='indexcore'){if(probe)refreshWorkspace();}else if(tab==='users')adminLists.users.load();else if(tab==='downloads')adminLists.downloads.load();else if(tab==='resources')loadResources();else if(own)loadTasks();else if(probe)refreshWorkspace();
}
function setDrawer(open,returnFocus=true){
 state.drawer=open;el('sidebar').classList.toggle('drawer-open',open);el('menu-backdrop').hidden=!open;
 el('menu-open').setAttribute('aria-expanded',String(open));el('sidebar').setAttribute('role',open?'dialog':'complementary');el('sidebar').setAttribute('aria-modal',String(open));
 el('main-shell').inert=open;document.body?.classList.toggle('drawer-lock',open);
 if(open)el('menu-close').focus();else if(returnFocus)el('menu-open').focus();
}
function keyNavigation(event){
 if(adminResourceEditor.key(event)||adminPendingCancel.key(event))return;
 if(event.key==='Escape'&&!el('audit-detail').hidden){adminAudit.close(true);return;}
 if(event.key==='Escape'){if(state.drawer)setDrawer(false);else if(!el('admin-download-detail').hidden)adminDownloadDetail.close(true);else if(!el('task-detail').hidden)closeTaskDetail(true);else if(!el('file-detail').hidden){state.detailRequest++;closeResourceDetail(true);}return;}
 if(event.key==='Tab'&&state.drawer){const buttons=Array.from(el('sidebar').querySelectorAll('a[href],summary,button:not([disabled])')).filter(n=>!n.closest?.('details:not([open])')||n.tagName==='SUMMARY');const first=buttons[0],last=buttons[buttons.length-1];
 if(event.shiftKey&&document.activeElement===first){event.preventDefault();last.focus();}else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first.focus();}}
}
el('login-form').addEventListener('submit',async event=>{
 event.preventDefault();if(state.logoutInFlight||state.loginInFlight)return;
 const loginPassword=el('login-password').value;signedOut();clearError('login-error');const generation=state.session;state.loginInFlight=true;el('login-submit').disabled=true;el('logout-retry').disabled=true;
 // Capture password before clearing the form at the session boundary.
 try{const p=await api('/api/v1/auth/login','POST',{login:el('login-name').value,password:loginPassword},generation);await signedIn(p,generation);}
 catch(error){if(generation===state.session){showError('login-error',error);if(error.code==='AUTH_REQUIRED')el('login-error').textContent='账号或密码不正确。';}}
 finally{if(generation===state.session){state.loginInFlight=false;el('login-password').value='';el('login-submit').disabled=false;el('logout-retry').disabled=false;}}
});
el('logout').addEventListener('click',logout);el('logout-retry').addEventListener('click',logout);
for(const tab of Object.keys(sections))el('nav-'+tab).addEventListener('click',()=>selectTab(tab));
el('nav-tasks').addEventListener('click',()=>selectTab('tasks'));el('scope-global').addEventListener('click',()=>selectTab('downloads'));
el('open-resources').addEventListener('click',()=>selectTab('resources'));el('open-my-tasks').addEventListener('click',()=>selectTab('tasks'));
for(const [id,tab] of [['open-users','users'],['open-admin-downloads','downloads'],['open-shares','shares'],['open-diagnostics','indexcore'],['open-storage','storage']])el(id).addEventListener('click',()=>selectTab(tab));
el('refresh-workspace').addEventListener('click',()=>refreshWorkspace());
el('refresh-storage').addEventListener('click',()=>{clearError('workspace-error');return adminObservation.refreshStorage();});
el('more-storage').addEventListener('click',()=>adminObservation.moreStorage());
el('root-binding').addEventListener('change',()=>{clearError('workspace-error');adminObservation.selectBinding();});
el('query-root').addEventListener('click',()=>{clearError('workspace-error');return adminObservation.queryRoot();});
el('refresh-users').addEventListener('click',()=>{clearError('workspace-error');return adminLists.users.refresh();});
el('users-apply-filter').addEventListener('click',()=>{clearError('workspace-error');return adminLists.users.filter(el('users-search').value,el('users-status').value);});
el('user-detail-submit').addEventListener('click',()=>{clearError('workspace-error');return adminOperations.loadUserDetail();});
el('more-users').addEventListener('click',()=>adminLists.users.more());
el('refresh-admin-downloads').addEventListener('click',()=>{clearError('workspace-error');return adminLists.downloads.refresh();});
el('more-admin-downloads').addEventListener('click',()=>adminLists.downloads.more());
el('admin-filter-form').addEventListener('submit',event=>{event.preventDefault();clearError('workspace-error');return adminLists.downloads.filter(el('admin-user-filter').value.trim(),el('admin-download-status').value);});
el('admin-filter-clear').addEventListener('click',()=>{el('admin-user-filter').value='';el('admin-download-status').value='ALL';clearError('workspace-error');return adminLists.downloads.filter('','ALL');});
el('admin-download-lookup').addEventListener('submit',event=>{event.preventDefault();return adminDownloadDetail.load(el('admin-download-id').value.trim());});
el('admin-download-detail-refresh').addEventListener('click',()=>adminDownloadDetail.refresh());
el('admin-download-detail-close').addEventListener('click',()=>adminDownloadDetail.close(true));
el('binding-select').addEventListener('change',()=>{
 const id=el('binding-select').value;if(!state.workspace?.bindings.some(b=>b.binding_id===id))return;
 state.binding=id;state.parent=null;state.parents=[];state.resourceCursor='';el('resource-search').value='';el('directory-label').textContent='根层级';el('directory-back').disabled=true;loadResources();
});
el('directory-back').addEventListener('click',()=>{const previous=state.parents.pop();if(!previous)return;state.parent=previous.parent;state.resourceCursor='';el('directory-label').textContent=previous.label;el('directory-back').disabled=state.parents.length===0;loadResources();});
el('refresh-resources').addEventListener('click',()=>{state.resourceCursor='';loadResources();});
el('more-resources').addEventListener('click',()=>{if(state.resourceNext){state.resourceCursor=state.resourceNext;loadResources();}});
el('refresh-tasks').addEventListener('click',()=>{state.taskCursor='';clearTasks();loadTasks();});
el('more-tasks').addEventListener('click',()=>{if(state.taskNext){state.taskCursor=state.taskNext;clearTasks();loadTasks();}});
el('resolve-form').addEventListener('submit',async event=>{
 event.preventDefault();if(!state.binding)return;const generation=state.session,request=++state.detailRequest;clearError('workspace-error');closeResourceDetail();state.resourceFocus=document.activeElement;
 try{const query=new URLSearchParams({binding_id:state.binding,path:el('resolve-path').value});const item=await adminRead('/api/v1/resources/resolve?'+query,generation);if(generation!==state.session||request!==state.detailRequest)return;resourceDetail(item);}
 catch(error){if(generation===state.session&&request===state.detailRequest)showError('workspace-error',error);}
});
for(const category of ['all','software','video','document','other'])el('category-'+category).addEventListener('click',()=>{state.category=category;drawResources();});
el('resource-search').addEventListener('input',()=>drawResources());
for(const layout of ['list','cards'])el('view-'+layout).addEventListener('click',()=>{state.layout=layout;drawResources();});
el('file-close').addEventListener('click',()=>{state.detailRequest++;closeResourceDetail(true);});
el('task-close').addEventListener('click',()=>closeTaskDetail(true));
el('menu-open').addEventListener('click',()=>setDrawer(true));el('menu-close').addEventListener('click',()=>setDrawer(false));el('menu-backdrop').addEventListener('click',()=>setDrawer(false));
document.addEventListener?.('keydown',keyNavigation);
document.addEventListener?.('visibilitychange',()=>{if(document.visibilityState==='visible')refreshWorkspace();});
window.addEventListener?.('focus',()=>refreshWorkspace());
window.addEventListener?.('hashchange',()=>{if(state.logoutStatus==='LOGOUT_PENDING'||state.logoutStatus==='LOGOUT_UNCONFIRMED')rememberLogoutIntent();});
window.addEventListener?.('resize',()=>{if(state.drawer&&window.matchMedia('(min-width:768px)').matches)setDrawer(false);});
(async()=>{
 if(logoutIntent()){signedOut();setLogoutStatus('LOGOUT_UNCONFIRMED','退出状态尚未确认。服务器会话可能仍然有效，请重试退出登录；本页不会自动恢复管理界面。');return;}
 const generation=state.session;try{await signedIn(await api('/api/v1/auth/me'),generation);}catch(error){if(generation===state.session&&error.code!=='AUTH_REQUIRED')showError('login-error',error);}
})();
