'use strict';
const el = id => document.getElementById(id);
const state = {session:0, tab:'resources', binding:'', parent:null, parents:[], resourceCursor:'', resourceNext:'', taskCursor:'', taskNext:'', resourceRequest:0, taskRequest:0, detailRequest:0, resources:[], resourcesLoaded:false, category:'all', layout:'list', controller:new AbortController()};
const messages = {AUTH_REQUIRED:'登录已失效，请重新登录。', INVALID_REQUEST:'输入不符合要求，请检查后再试。', ORIGIN_REJECTED:'当前页面来源未获允许。', AUTH_UNAVAILABLE:'登录服务暂时不可用。', AUTH_BUSY:'登录请求过多，请稍候一分钟。', NOT_FOUND:'未找到可见资源或属于你的任务。', RESOURCE_AMBIGUOUS:'资源存在多个匹配，系统不会猜选。', READ_UNAVAILABLE:'查询服务暂时不可用，未确认任何成功结果。', SERVICE_STOPPING:'服务正在安全关闭。'};
function errorText(error){return error.code==='ADMIN_REQUIRED'?'此入口仅供管理员使用，请返回前台文件库。':error.name==='AbortError'?'请求已取消。':(messages[error.code]||'连接失败，请检查服务状态后重试。');}
function showError(id,error){const node=el(id);node.textContent=errorText(error);node.hidden=false;}
function clearError(id){el(id).textContent='';el(id).hidden=true;}
async function api(path,method='GET',body){
 const response=await fetch(path,{method,credentials:'same-origin',cache:'no-store',signal:state.controller.signal,headers:body?{'Content-Type':'application/json'}:{},body:body?JSON.stringify(body):undefined});
 if(response.status===204)return null;
 const data=await response.json();
 if(!response.ok){const error=new Error('Request refused');error.code=data.code;if(response.status===401&&method==='GET')signedOut();throw error;}
 return data;
}
function clearResources(){state.resources=[];state.resourcesLoaded=false;state.resourceNext='';state.detailRequest++;el('resource-rows').replaceChildren();el('resource-cards').replaceChildren();closeResourceDetail();el('more-resources').disabled=true;el('resource-count').textContent='—';for(const key of ['all','software','video','document','other'])el('count-'+key).textContent='—';}
function signedOut(){
 state.session++;state.controller.abort();state.controller=new AbortController();state.resourceRequest++;state.taskRequest++;
 clearResources();for(const id of ['task-rows','task-detail','user-login','user-role','schema','binding-select'])el(id).replaceChildren();
 el('task-detail').hidden=true;el('workspace-view').hidden=true;el('login-view').hidden=false;el('login-password').value='';el('resource-search').value='';el('resolve-path').value='/';el('directory-label').textContent='根层级';el('directory-back').disabled=true;state.binding='';state.parent=null;state.parents=[];state.resourceCursor='';state.taskCursor='';state.taskNext='';state.category='all';state.layout='list';clearError('workspace-error');
}
async function signedIn(principal){
 state.session++;const generation=state.session;
 const workspace=await api('/api/v1/admin/workspace');if(generation!==state.session)return;
 el('user-login').textContent=principal.login;el('user-role').textContent=principal.role==='ADMIN'?'管理员 · 只读':'产品用户 · 只读';el('avatar').textContent=principal.login.charAt(0).toUpperCase();el('schema').textContent='Schema v'+workspace.schema_version;
 el('binding-select').replaceChildren();
 for(const binding of workspace.bindings){const option=document.createElement('option');option.value=binding.binding_id;option.textContent=binding.label;el('binding-select').append(option);}
 state.binding=workspace.bindings[0].binding_id;state.parent=null;state.parents=[];state.resourceCursor='';state.taskCursor='';
 el('login-view').hidden=true;el('workspace-view').hidden=false;window.scrollTo(0,0);clearError('login-error');selectTab('resources');
}
function badge(value,kind=''){const node=document.createElement('span');node.className='badge'+(kind?' '+kind:'');node.textContent=value;return node;}
function cell(row,text){const node=document.createElement('td');if(text!==undefined)node.textContent=text;row.append(node);return node;}
function size(value){return ResourceDisplay.size(value);}
function resourceName(item){return ResourceDisplay.name(item);}
function closeResourceDetail(){el('file-detail').hidden=true;for(const id of ['file-title','file-type','file-path','file-size','file-presence','file-copy','file-id','file-note','file-glyph'])el(id).textContent='';}
function resourceDetail(item){
 const type=ResourceDisplay.classify(item);el('file-title').textContent=resourceName(item);el('file-type').textContent=type.label+(type.extension?' · .'+type.extension:'');el('file-path').textContent=item.path||'路径未提供';el('file-size').textContent=size(item.size_bytes);el('file-presence').textContent=item.presence;el('file-copy').textContent=item.copy_id?(item.copy_availability||'状态未提供')+' · '+item.copy_id:'当前产品库未投影';el('file-id').textContent=item.resource_id;el('file-note').textContent=ResourceDisplay.accessNote(item);el('file-glyph').textContent=type.glyph;el('file-glyph').className='file-hero '+type.key;el('file-primary-action').textContent=type.action+' · 待接入';el('file-detail').hidden=false;el('file-detail').scrollIntoView({block:'nearest'});
}
function openResource(item){
 if(item.directory===true){state.parents.push({parent:state.parent,label:el('directory-label').textContent});state.parent=item.resource_id;state.resourceCursor='';el('directory-label').textContent=item.path||resourceName(item);el('directory-back').disabled=false;loadResources();}
 else{state.detailRequest++;resourceDetail(item);}
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
async function loadResources(){
 if(!state.binding)return;const generation=state.session;const request=++state.resourceRequest;clearResources();el('resources-empty').hidden=true;clearError('workspace-error');el('refresh-resources').disabled=true;el('more-resources').disabled=true;el('resources-footer').textContent='正在查询…';el('resource-count').textContent='—';
 const query=new URLSearchParams({binding_id:state.binding,limit:'20'});if(state.parent)query.set('parent_resource_id',state.parent);if(state.resourceCursor)query.set('cursor',state.resourceCursor);
 try{const page=await api('/api/v1/resources?'+query);if(generation===state.session&&request===state.resourceRequest)renderResources(page);}
 catch(error){if(generation===state.session&&request===state.resourceRequest){el('resource-rows').replaceChildren();el('resources-empty').hidden=true;el('resources-footer').textContent='查询未完成';showError('workspace-error',error);}}
 finally{if(generation===state.session&&request===state.resourceRequest)el('refresh-resources').disabled=false;}
}
const manifestLabels={PENDING:'等待激活',ACTIVE:'执行中',AWAITING_VISIBILITY:'等待观察',AWAITING_CANONICAL:'等待规范确认',READY:'READY',FAILED:'失败',RECOVERY_REQUIRED:'需人工恢复',CANCELED:'已取消'};
function renderTasks(page){
 el('task-rows').replaceChildren();
 for(const item of page.items){const row=document.createElement('tr');const button=document.createElement('button');button.type='button';button.className='file-button';button.textContent=item.manifest_id.slice(0,8)+'…';button.title=item.manifest_id;button.addEventListener('click',()=>taskDetail(item.manifest_id));cell(row).append(button);
  cell(row).append(badge(manifestLabels[item.manifest_state]||item.manifest_state,item.manifest_state==='READY'?'':item.manifest_state==='RECOVERY_REQUIRED'?'warning':'neutral'));cell(row,item.job_state||'未创建 Job');cell(row,item.claim_attempts+' / '+item.failure_count);cell(row,new Date(item.updated_at).toLocaleString());el('task-rows').append(row);
 }
 state.taskNext=page.next_cursor||'';el('more-tasks').disabled=!state.taskNext;el('tasks-empty').hidden=page.items.length!==0;el('tasks-footer').textContent='本页 '+page.items.length+' 项 · 仅当前产品用户';
}
async function loadTasks(){
 const generation=state.session;const request=++state.taskRequest;clearError('workspace-error');el('refresh-tasks').disabled=true;el('more-tasks').disabled=true;el('tasks-footer').textContent='正在查询…';const query=new URLSearchParams({limit:'20'});if(state.taskCursor)query.set('cursor',state.taskCursor);
 try{const page=await api('/api/v1/tasks?'+query);if(generation===state.session&&request===state.taskRequest)renderTasks(page);}
 catch(error){if(generation===state.session&&request===state.taskRequest){el('task-rows').replaceChildren();el('tasks-empty').hidden=true;el('tasks-footer').textContent='查询未完成';showError('workspace-error',error);}}
 finally{if(generation===state.session&&request===state.taskRequest)el('refresh-tasks').disabled=false;}
}
async function taskDetail(id){const generation=state.session;try{const item=await api('/api/v1/tasks/'+encodeURIComponent(id));if(generation!==state.session)return;el('task-detail').textContent=JSON.stringify(item,null,2);el('task-detail').hidden=false;}catch(error){if(generation===state.session)showError('workspace-error',error);}}
function selectTab(tab){state.detailRequest++;closeResourceDetail();state.tab=tab;el('resources-panel').hidden=tab!=='resources';el('tasks-panel').hidden=tab!=='tasks';el('nav-resources').classList.toggle('selected',tab==='resources');el('nav-tasks').classList.toggle('selected',tab==='tasks');el('page-title').textContent=tab==='resources'?'资源浏览':'我的任务';el('page-caption').textContent=tab==='resources'?'资源工作空间 / 浏览':'资源工作空间 / 任务';el('page-subtitle').textContent=tab==='resources'?'软件、视频与文档 · 来自已配置目录的真实规范索引。':'查看自己的持久任务与结果，不以下载成功代替 READY。';clearError('workspace-error');if(tab==='resources')loadResources();else loadTasks();}
el('login-form').addEventListener('submit',async event=>{event.preventDefault();clearError('login-error');el('login-submit').disabled=true;try{const p=await api('/api/v1/auth/login','POST',{login:el('login-name').value,password:el('login-password').value});await signedIn(p);}catch(error){showError('login-error',error);if(error.code==='AUTH_REQUIRED')el('login-error').textContent='账号或密码不正确。';}finally{el('login-password').value='';el('login-submit').disabled=false;}});
el('logout').addEventListener('click',async()=>{el('logout').disabled=true;try{await api('/api/v1/auth/logout','POST');signedOut();}catch(error){showError('workspace-error',error);}finally{el('logout').disabled=false;}});
el('nav-resources').addEventListener('click',()=>selectTab('resources'));el('nav-tasks').addEventListener('click',()=>selectTab('tasks'));
el('binding-select').addEventListener('change',()=>{state.binding=el('binding-select').value;state.parent=null;state.parents=[];state.resourceCursor='';el('resource-search').value='';el('directory-label').textContent='根层级';el('directory-back').disabled=true;loadResources();});
el('directory-back').addEventListener('click',()=>{const previous=state.parents.pop();if(!previous)return;state.parent=previous.parent;state.resourceCursor='';el('directory-label').textContent=previous.label;el('directory-back').disabled=state.parents.length===0;loadResources();});
el('refresh-resources').addEventListener('click',()=>{state.resourceCursor='';loadResources();});el('more-resources').addEventListener('click',()=>{if(state.resourceNext){state.resourceCursor=state.resourceNext;loadResources();}});
el('refresh-tasks').addEventListener('click',()=>{state.taskCursor='';loadTasks();});el('more-tasks').addEventListener('click',()=>{if(state.taskNext){state.taskCursor=state.taskNext;loadTasks();}});
el('resolve-form').addEventListener('submit',async event=>{event.preventDefault();const generation=state.session;const request=++state.detailRequest;clearError('workspace-error');closeResourceDetail();try{const query=new URLSearchParams({binding_id:state.binding,path:el('resolve-path').value});const item=await api('/api/v1/resources/resolve?'+query);if(generation!==state.session||request!==state.detailRequest)return;resourceDetail(item);}catch(error){if(generation===state.session&&request===state.detailRequest)showError('workspace-error',error);}});
for(const category of ['all','software','video','document','other'])el('category-'+category).addEventListener('click',()=>{state.category=category;drawResources();});
el('resource-search').addEventListener('input',()=>drawResources());
for(const layout of ['list','cards'])el('view-'+layout).addEventListener('click',()=>{state.layout=layout;drawResources();});
el('file-close').addEventListener('click',()=>{state.detailRequest++;closeResourceDetail();});
(async()=>{try{await signedIn(await api('/api/v1/auth/me'));}catch(error){if(error.code!=='AUTH_REQUIRED')showError('login-error',error);}})();
