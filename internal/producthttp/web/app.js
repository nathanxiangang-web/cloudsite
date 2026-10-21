'use strict';
// Session boundary: no token storage, no admin API inference, no domain mutations.
const el = id => document.getElementById(id);
const sections = {
 overview:['工作台总览','以明确的数据来源，理解已接入能力与尚缺的证据。'],
 users:['用户管理','全站用户只读聚合与账号管理将在后续阶段接入。','用户管理只读接口尚未接入；当前身份不代表全站用户信息。','用户管理 · 尚未接入'],
 resources:['资源中心','查看配置 allowlist 内的规范资源、目录与 Copy；分类和搜索仅作用于当前页。'],
 downloads:['云下载与任务','全站任务尚未接入；“我的任务”只读取当前登录者的持久任务。'],
 storage:['存储与连接','连接运行状态、原生会话与存储映射聚合尚未接入。','总览仅提供配置 Binding 别名，不证明凭据有效或网盘在线。','连接管理 · 尚未接入'],
 indexcore:['IndexCore 观测','物理事实仍由 IndexCore 负责；本页尚无运行诊断聚合接口。','Q4/Q5 资源读取已在资源中心接入；Root 健康、Q8 游标与诊断未接入。','运行诊断 · 尚未接入'],
 shares:['分享管理','全站分享的脱敏概览尚未接入。','不读取或兑换他人的分享 token、保护码或签名地址。前台已有分享功能不受影响。','分享管理 · 尚未接入'],
 audit:['审计与安全','完整持久审计读模型尚未接入。','不会将临时日志当作完整审计，也不会编造历史操作记录。','查看审计 · 尚未接入'],
 settings:['系统设置','系统设置与管理变更接口尚未接入。','当前 schema 与配置能力可在总览查看；没有配置写入、任意命令或 AI 启用入口。','修改设置 · 尚未接入']
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
async function api(path,method='GET',body,generation=state.session){
 const response=await fetch(path,{method,credentials:'same-origin',cache:'no-store',signal:state.controller.signal,headers:body?{'Content-Type':'application/json'}:{},body:body?JSON.stringify(body):undefined});
 current(generation);
 if(response.status===204)return null;
 // Authorization invalidates private views even when a proxy returns non-JSON.
 if(response.status===401||(response.status===403&&path==='/api/v1/admin/workspace')){
  const error=new Error('Authorization ended');error.code=response.status===401?'AUTH_REQUIRED':'ADMIN_REQUIRED';
  if(method==='GET'){signedOut();showError('login-error',error);}throw error;
 }
 const data=await response.json();current(generation);
 if(!response.ok){const error=new Error('Request refused');error.code=data.code;throw error;}
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
 state.principal=null;state.workspace=null;clearResources();clearTasks();setDrawer(false,false);
 for(const id of ['user-login','user-role','avatar','schema','binding-select','workspace-facts','identity-facts','overview-binding-list'])el(id).replaceChildren();
 for(const id of ['overview-schema','overview-bindings'])el(id).textContent='—';
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
 state.workspace=workspace;el('user-login').textContent=principal.login;el('user-role').textContent='ADMIN · 只读';
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
 return {workspace,principal};
}
async function adminRead(path,generation=state.session){
 await verifyAdmin(generation);current(generation);return api(path,'GET',undefined,generation);
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
 el('refresh-workspace').disabled=true;el('workspace-status').textContent='正在读取配置…';
 try{const {workspace,principal}=await verifyAdmin(generation);if(generation===state.session&&request===state.workspaceRequest){renderWorkspace(workspace,principal);clearError('workspace-error');}}
 catch(error){if(generation===state.session&&request===state.workspaceRequest){el('workspace-status').textContent='STALE · 配置读取失败';el('workspace-status').className='badge warning';showError('workspace-error',error);}}
 finally{if(generation===state.session&&request===state.workspaceRequest)el('refresh-workspace').disabled=false;}
}

// Canonical resource presentation reuses the consumer-compatible pure helper.
function badge(value,kind=''){const node=document.createElement('span');node.className='badge'+(kind?' '+kind:'');node.textContent=value;return node;}
function cell(row,text){const node=document.createElement('td');if(text!==undefined)node.textContent=text;row.append(node);return node;}
function size(value){return ResourceDisplay.size(value);}
function resourceName(item){return ResourceDisplay.name(item);}
function closeResourceDetail(returnFocus=false){el('file-detail').hidden=true;el('resource-identities').replaceChildren();for(const id of ['file-title','file-type','file-path','file-size','file-presence','file-copy','file-id','file-note','file-glyph'])el(id).textContent='';if(returnFocus)state.resourceFocus?.focus();}
function resourceDetail(item){
 const type=ResourceDisplay.classify(item);el('file-title').textContent=resourceName(item);el('file-type').textContent=type.label+(type.extension?' · .'+type.extension:'');el('file-path').textContent=item.path||'路径未提供';el('file-size').textContent=size(item.size_bytes);el('file-presence').textContent=item.presence;el('file-copy').textContent=item.copy_id?(item.copy_availability||'状态未提供')+' · '+item.copy_id:'当前产品库未投影';el('file-id').textContent=item.resource_id;el('file-note').textContent=ResourceDisplay.accessNote(item);el('file-glyph').textContent=type.glyph;el('file-glyph').className='file-hero '+type.key;el('file-primary-action').textContent=type.action+' · 待接入';
 facts('resource-identities',[['Binding ID',item.binding_id],['Root ID','当前 Resource DTO 未提供'],['Asset ID',item.asset_id],['Release ID',item.release_id],['Variant ID',item.variant_id],['来源 / 覆盖','产品 Q4 当前页或 Q5 精确定位；不推测物理删除']]);
 el('file-detail').hidden=false;el('file-detail').focus?.();el('file-detail').scrollIntoView({block:'nearest'});
}
function openResource(item){
 if(item.directory===true){state.parents.push({parent:state.parent,label:el('directory-label').textContent});state.parent=item.resource_id;state.resourceCursor='';el('directory-label').textContent=item.path||resourceName(item);el('directory-back').disabled=false;loadResources();}
 else{state.detailRequest++;state.resourceFocus=document.activeElement;resourceDetail(item);}
}
function drawResources(){
 if(!state.resourcesLoaded)return;
 const counts=ResourceDisplay.counts(state.resources);for(const key of Object.keys(counts)){el('count-'+key).textContent=String(counts[key]);el('category-'+key).setAttribute('aria-pressed',String(state.category===key));}
 const items=ResourceDisplay.filter(state.resources,state.category,el('resource-search').value);
 el('resource-rows').replaceChildren();el('resource-cards').replaceChildren();
 for(const item of items){
  const type=ResourceDisplay.classify(item);const row=document.createElement('tr');const name=cell(row);const title=document.createElement('button');title.type='button';title.className='file-button';title.textContent=resourceName(item);title.addEventListener('click',()=>openResource(item));const icon=document.createElement('span');icon.className='file-icon '+type.key;icon.textContent=type.glyph;name.append(icon,title);if(item.path){const path=document.createElement('small');path.textContent=item.path;name.append(path);}
  cell(row,type.label);cell(row,size(item.size_bytes));cell(row).append(badge(item.presence));cell(row).append(badge(item.copy_id?item.copy_availability:'当前库未投影',item.copy_id?'':'neutral'));el('resource-rows').append(row);
  const card=document.createElement('button');card.type='button';card.className='resource-card';card.addEventListener('click',()=>openResource(item));const art=document.createElement('span');art.className='card-art '+type.key;art.textContent=type.glyph;const label=document.createElement('strong');label.textContent=resourceName(item);const meta=document.createElement('small');meta.textContent=type.label+' · '+size(item.size_bytes);const path=document.createElement('small');path.className='card-path';path.textContent=item.path||'路径未提供';card.append(art,label,meta,path,badge(item.presence),badge(item.copy_id?item.copy_availability:'当前库未投影','neutral'));el('resource-cards').append(card);
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
 state.resourceRequest++;state.taskRequest++;state.detailRequest++;closeResourceDetail();closeTaskDetail();state.tab=tab;setDrawer(false,false);
 for(const id of ['overview','resources','downloads'])el(id+'-panel').hidden=section!==id;
 const planned=Boolean(sections[section][2]);el('planned-panel').hidden=!planned;
 for(const name of Object.keys(sections)){el('nav-'+name).classList.toggle('selected',section===name);el('nav-'+name).setAttribute('aria-current',section===name?'page':'false');}
 el('tasks-panel').hidden=!own;el('global-tasks-placeholder').hidden=own;el('nav-tasks').setAttribute('aria-pressed',String(own));el('scope-global').setAttribute('aria-pressed',String(!own));
 el('page-title').textContent=own?'我的任务':sections[section][0];el('page-caption').textContent=own?'云下载与任务 / 我的任务':sections[section][0];el('page-subtitle').textContent=sections[section][1];
 if(planned){el('planned-title').textContent=sections[section][0]+' · 尚未接入';el('planned-description').textContent=sections[section][2];el('planned-action').textContent=sections[section][3];}
 clearError('workspace-error');if(probe){el('main-content').focus({preventScroll:true});window.scrollTo(0,0);}
 if(tab==='resources')loadResources();else if(own)loadTasks();else if(probe)refreshWorkspace();
}
function setDrawer(open,returnFocus=true){
 state.drawer=open;el('sidebar').classList.toggle('drawer-open',open);el('menu-backdrop').hidden=!open;
 el('menu-open').setAttribute('aria-expanded',String(open));el('sidebar').setAttribute('role',open?'dialog':'complementary');el('sidebar').setAttribute('aria-modal',String(open));
 el('main-shell').inert=open;document.body?.classList.toggle('drawer-lock',open);
 if(open)el('menu-close').focus();else if(returnFocus)el('menu-open').focus();
}
function keyNavigation(event){
 if(event.key==='Escape'){if(state.drawer)setDrawer(false);else if(!el('task-detail').hidden)closeTaskDetail(true);else if(!el('file-detail').hidden){state.detailRequest++;closeResourceDetail(true);}return;}
 if(event.key==='Tab'&&state.drawer){const buttons=Array.from(el('sidebar').querySelectorAll('a[href],button:not([disabled])'));const first=buttons[0],last=buttons[buttons.length-1];
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
el('refresh-workspace').addEventListener('click',()=>refreshWorkspace());
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
