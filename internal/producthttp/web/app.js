'use strict';
const el = id => document.getElementById(id);
const state = {session:0, tab:'resources', binding:'', parent:null, parents:[], resourceCursor:'', resourceNext:'', taskCursor:'', taskNext:'', resourceRequest:0, taskRequest:0, controller:new AbortController()};
const messages = {AUTH_REQUIRED:'登录已失效，请重新登录。', INVALID_REQUEST:'输入不符合要求，请检查后再试。', ORIGIN_REJECTED:'当前页面来源未获允许。', AUTH_UNAVAILABLE:'登录服务暂时不可用。', AUTH_BUSY:'登录请求过多，请稍候一分钟。', NOT_FOUND:'未找到可见资源或属于你的任务。', RESOURCE_AMBIGUOUS:'资源存在多个匹配，系统不会猜选。', READ_UNAVAILABLE:'查询服务暂时不可用，未确认任何成功结果。', SERVICE_STOPPING:'服务正在安全关闭。'};
function errorText(error){return error.name==='AbortError'?'请求已取消。':(messages[error.code]||'连接失败，请检查服务状态后重试。');}
function showError(id,error){const node=el(id);node.textContent=errorText(error);node.hidden=false;}
function clearError(id){el(id).textContent='';el(id).hidden=true;}
async function api(path,method='GET',body){
 const response=await fetch(path,{method,credentials:'same-origin',cache:'no-store',signal:state.controller.signal,headers:body?{'Content-Type':'application/json'}:{},body:body?JSON.stringify(body):undefined});
 if(response.status===204)return null;
 const data=await response.json();
 if(!response.ok){const error=new Error('Request refused');error.code=data.code;if(response.status===401&&method==='GET')signedOut();throw error;}
 return data;
}
function signedOut(){
 state.session++;state.controller.abort();state.controller=new AbortController();state.resourceRequest++;state.taskRequest++;
 for(const id of ['resource-rows','task-rows','resolved-detail','task-detail','user-login','user-role','schema','binding-select'])el(id).replaceChildren();
 el('resolved-detail').hidden=true;el('task-detail').hidden=true;el('workspace-view').hidden=true;el('login-view').hidden=false;el('login-password').value='';state.binding='';state.parent=null;state.parents=[];clearError('workspace-error');
}
async function signedIn(principal){
 state.session++;const generation=state.session;
 const workspace=await api('/api/v1/workspace');if(generation!==state.session)return;
 el('user-login').textContent=principal.login;el('user-role').textContent=principal.role==='ADMIN'?'管理员 · 只读':'产品用户 · 只读';el('avatar').textContent=principal.login.charAt(0).toUpperCase();el('schema').textContent='Schema v'+workspace.schema_version;
 el('binding-select').replaceChildren();
 for(const binding of workspace.bindings){const option=document.createElement('option');option.value=binding.binding_id;option.textContent=binding.label;el('binding-select').append(option);}
 state.binding=workspace.bindings[0].binding_id;state.parent=null;state.parents=[];state.resourceCursor='';state.taskCursor='';
 el('login-view').hidden=true;el('workspace-view').hidden=false;clearError('login-error');selectTab('resources');
}
function badge(value,kind=''){const node=document.createElement('span');node.className='badge'+(kind?' '+kind:'');node.textContent=value;return node;}
function cell(row,text){const node=document.createElement('td');if(text!==undefined)node.textContent=text;row.append(node);return node;}
function size(value){if(value==null)return '—';if(value<1024)return value+' B';if(value<1048576)return (value/1024).toFixed(1)+' KB';return (value/1048576).toFixed(1)+' MB';}
function resourceName(item){return item.name||item.path||item.resource_id;}
function renderResources(page){
 el('resource-rows').replaceChildren();
 for(const item of page.items){const row=document.createElement('tr');const name=cell(row);let title;
  if(item.directory){title=document.createElement('button');title.type='button';title.className='file-button';title.textContent='▱ '+resourceName(item);title.addEventListener('click',()=>{state.parents.push({parent:state.parent,label:el('directory-label').textContent});state.parent=item.resource_id;state.resourceCursor='';el('directory-label').textContent=item.path||resourceName(item);el('directory-back').disabled=false;loadResources();});}
  else{title=document.createElement('span');title.className='file-name';title.textContent='▧ '+resourceName(item);}name.append(title);if(item.path){const path=document.createElement('small');path.textContent=item.path;name.append(path);}
  cell(row,item.directory?'目录':(item.directory===false?'文件':'未提供'));cell(row,size(item.size_bytes));cell(row).append(badge(item.presence));cell(row).append(badge(item.copy_id?item.copy_availability:'未投影',item.copy_id?'':'neutral'));el('resource-rows').append(row);
 }
 state.resourceNext=page.next_cursor||'';el('more-resources').disabled=!state.resourceNext;el('resources-empty').hidden=page.items.length!==0;el('resource-count').textContent=String(page.items.length);el('resources-footer').textContent='本页 '+page.items.length+' 项 · 规范查询结果';
}
async function loadResources(){
 if(!state.binding)return;const generation=state.session;const request=++state.resourceRequest;clearError('workspace-error');el('refresh-resources').disabled=true;el('more-resources').disabled=true;el('resources-footer').textContent='正在查询…';el('resource-count').textContent='—';
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
function selectTab(tab){state.tab=tab;el('resources-panel').hidden=tab!=='resources';el('tasks-panel').hidden=tab!=='tasks';el('nav-resources').classList.toggle('selected',tab==='resources');el('nav-tasks').classList.toggle('selected',tab==='tasks');el('page-title').textContent=tab==='resources'?'资源浏览':'我的任务';el('page-caption').textContent=tab==='resources'?'资源工作空间 / 浏览':'资源工作空间 / 任务';el('page-subtitle').textContent=tab==='resources'?'查看指定存储空间内的规范资源，不推测物理状态。':'查看自己的持久任务与结果，不以下载成功代替 READY。';clearError('workspace-error');if(tab==='resources')loadResources();else loadTasks();}
el('login-form').addEventListener('submit',async event=>{event.preventDefault();clearError('login-error');el('login-submit').disabled=true;try{const p=await api('/api/v1/auth/login','POST',{login:el('login-name').value,password:el('login-password').value});await signedIn(p);}catch(error){showError('login-error',error);}finally{el('login-password').value='';el('login-submit').disabled=false;}});
el('logout').addEventListener('click',async()=>{el('logout').disabled=true;try{await api('/api/v1/auth/logout','POST');signedOut();}catch(error){showError('workspace-error',error);}finally{el('logout').disabled=false;}});
el('nav-resources').addEventListener('click',()=>selectTab('resources'));el('nav-tasks').addEventListener('click',()=>selectTab('tasks'));
el('binding-select').addEventListener('change',()=>{state.binding=el('binding-select').value;state.parent=null;state.parents=[];state.resourceCursor='';el('directory-label').textContent='根层级';el('directory-back').disabled=true;el('resolved-detail').hidden=true;loadResources();});
el('directory-back').addEventListener('click',()=>{const previous=state.parents.pop();if(!previous)return;state.parent=previous.parent;state.resourceCursor='';el('directory-label').textContent=previous.label;el('directory-back').disabled=state.parents.length===0;loadResources();});
el('refresh-resources').addEventListener('click',()=>{state.resourceCursor='';loadResources();});el('more-resources').addEventListener('click',()=>{if(state.resourceNext){state.resourceCursor=state.resourceNext;loadResources();}});
el('refresh-tasks').addEventListener('click',()=>{state.taskCursor='';loadTasks();});el('more-tasks').addEventListener('click',()=>{if(state.taskNext){state.taskCursor=state.taskNext;loadTasks();}});
el('resolve-form').addEventListener('submit',async event=>{event.preventDefault();const generation=state.session;clearError('workspace-error');el('resolved-detail').hidden=true;try{const query=new URLSearchParams({binding_id:state.binding,path:el('resolve-path').value});const item=await api('/api/v1/resources/resolve?'+query);if(generation!==state.session)return;el('resolved-detail').textContent='精确匹配：'+resourceName(item)+'\n路径：'+(item.path||'—')+'\n规范状态：'+item.presence+' · Copy：'+(item.copy_id||'尚未投影，不能作为可访问结果');el('resolved-detail').hidden=false;}catch(error){if(generation===state.session)showError('workspace-error',error);}});
(async()=>{try{await signedIn(await api('/api/v1/auth/me'));}catch(error){if(error.code!=='AUTH_REQUIRED')showError('login-error',error);}})();
