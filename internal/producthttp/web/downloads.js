'use strict';
const element=id=>document.getElementById(id);
const state={owner:null,intent:new CloudDownloadModel.Intent(),controller:new AbortController(),reconciling:false,cursor:'',refreshing:false,stopped:false,paginated:false,enabled:false,sessionCheck:false,sessionView:null,sessionLoading:false,canSubmit:false,readyFiles:null,privateFiles:null,privateTrail:[],privateBusy:false,privateContent:false,privateOnly:false};
const invalidSources={
 SOURCE_INVALID:'链接格式无法识别，请检查下载链接是否完整。',
 MAGNET_BTIH_MISSING:'磁力链接缺少有效的 BTIH 资源标识。',
 MAGNET_AMBIGUOUS:'磁力链接的资源标识或文件名互相冲突。',
 MAGNET_UNSUPPORTED:'暂不支持这种磁力链接类型或选定文件下载，请使用标准 BTIH 链接。',
 MAGNET_MALFORMED:'磁力链接参数的编码或格式有误。'
};
const errorText=code=>({ ...invalidSources,PRIVATE_SPACE_REQUIRED:'请先开通私有空间，再提交下载。',PROVIDER_LOGIN_REQUIRED:'115 账号登录已失效，本次新请求未创建下载任务。请管理员恢复网盘登录后继续。',PROVIDER_SESSION_UNKNOWN:'暂时无法确认 115 登录状态，本次新请求未创建下载任务。请稍后检查账号。',AUTH_REQUIRED:'请先登录文件库。',INVALID_REQUEST:'链接或文件名不符合要求，请检查后重试。',ORIGIN_REJECTED:'当前网站入口不可用。',OPERATION_CONFLICT:'同一次提交的内容不一致，请先核实上次结果，不要创建重复任务。',OPERATION_LIMIT:'已达到当前账号的下载次数上限。',CONCURRENCY_LIMIT:'同时进行的任务已满，请等待现有任务完成。',BINDING_NOT_AVAILABLE:'当前下载目标暂不可用。',ADMISSION_UNCERTAIN:'提交结果尚未确认，请核实或使用原内容继续同一次提交。',OPERATION_NOT_FOUND:'暂未查到提交记录。这不能证明之前未提交，请继续核实，或用原内容重试同一次提交。',FROZEN_INTENT:'这次提交的内容已固定，请保持原链接与文件名。',STORAGE_UNAVAILABLE:'浏览器无法保存这次操作标识，请允许本站会话存储后再提交。'})[code]||'连接暂时不可用。若已经点过提交，请先核实上次结果，不要重复新建。';
function message(text){element('download-message').textContent=text;}
function error(code){element('download-error').textContent=errorText(code);element('download-error').hidden=false;}
function clearDownloadError(){element('download-error').hidden=true;element('download-error').textContent='';}
function drawPrivateFolders(folders){
 const list=element('private-folder-list');
 const entries=folders.map(folder=>{
  const entry=document.createElement('li'),button=document.createElement('button');
  button.type='button';button.className='secondary';button.textContent=folder.name;
  button.addEventListener('click',()=>openPrivateFolder([...state.privateTrail,folder]));
  entry.append(button);return entry;
 });
 list.replaceChildren(...entries);
 element('private-folder-location').textContent=state.privateTrail.map(folder=>folder.name).join(' / ')||'私有下载空间';
 element('private-folder-up').hidden=state.privateTrail.length===0;
}
function openPrivateFolder(trail){
 if(!state.privateFiles||state.stopped)return;
 state.privateFiles.clear();state.privateBusy=false;state.privateTrail=trail;
 state.privateFiles=new FileSurface.PrivateFiles(trail.at(-1)?.path||'');
 clearFiles();drawPrivateFolders([]);element('private-files-status').textContent='正在加载当前文件夹…';
 return refreshPrivateFiles();
}
function showPrivateFiles(result){
 if(!result||state.stopped||!state.privateFiles)return;
 library.remote=false;library.cloudScope=result.scope;library.bindings=result.scope?[{binding_id:result.scope.binding_id}]:[];library.wholeRoot=true;
 library.content=state.privateContent;library.sharing=false;library.userID=state.owner;library.onUnauthorized=()=>stop();library.failed=false;
 library.catalog={files:result.items,complete:!result.cursor,limited:!!result.cursor,unknown:0};
 if(library.selected&&!result.items.some(item=>item.binding_id===library.selected.binding_id&&item.resource_id===library.selected.resource_id&&item.path===library.selected.path))closeDetail();
 element('my-shares').hidden=true;drawFiles(true);drawPrivateFolders(result.folders);element('load-more').hidden=true;
 element('private-files-more').hidden=!result.cursor;
 element('collection-status').textContent='当前目录已加载 '+result.items.length+' 个文件';
 element('load-status').textContent=result.cursor?'可继续加载。搜索与分类仅针对已加载文件。':'';
 element('private-files-status').textContent=result.folders.length?'已收录 '+result.folders.length+' 个文件夹，打开后可查看文件。':result.items.length?'':'当前文件夹没有已收录的内容。';
}
async function refreshPrivateFiles(more=false){
 const model=state.privateFiles;if(!model||state.privateBusy||state.stopped)return;
 state.privateBusy=true;element('private-files-refresh').disabled=true;element('private-files-more').disabled=true;
 element('library-error').hidden=true;element('library-error').textContent='';
 try{const result=await model.read(api,more);if(!state.stopped&&state.privateFiles===model&&result)showPrivateFiles(result);}
 catch(failure){if(!state.stopped&&state.privateFiles===model){element('library-error').textContent=failure.code==='AUTH_REQUIRED'?'登录已失效，请重新登录。':'私有文件暂时无法读取，已显示的内容可能不是最新状态。';element('library-error').hidden=false;element('private-files-status').textContent='私有文件刷新失败。';}}
 finally{if(state.privateFiles===model){state.privateBusy=false;if(!state.stopped){element('private-files-refresh').disabled=false;element('private-files-more').disabled=false;element('private-files-more').hidden=!model.cursor;}}}
}
async function mountPrivateFiles(){
 if(state.stopped||typeof FileSurface==='undefined'||typeof CloudFileSurface==='undefined')return;
 if(state.privateFiles)state.privateFiles.clear();
 CloudFileSurface.clear();state.privateFiles=new FileSurface.PrivateFiles();state.privateTrail=[];state.privateBusy=false;
 element('download-files-scope').textContent='你的云下载文件由 IndexCore 收录后显示。点击文件夹查看其中的文件。';
 element('download-files').hidden=false;element('refresh').hidden=true;element('private-folders').hidden=false;element('private-files-refresh').hidden=false;element('private-files-more').hidden=true;
 await refreshPrivateFiles();
}
function mountLegacyFiles(principal,info,onUnauthorized){
 if(state.privateFiles)state.privateFiles.clear();state.privateFiles=null;state.privateTrail=[];state.privateBusy=false;
 element('download-files-scope').textContent='云下载目录中已收录的文件在这里展示，包含子文件夹中的内容；不会混入主文件库。';
 element('refresh').hidden=false;element('private-folders').hidden=true;drawPrivateFolders([]);element('private-files-refresh').hidden=true;element('private-files-more').hidden=true;element('private-files-status').textContent='';
 if(typeof CloudFileSurface!=='undefined')return CloudFileSurface.mount(principal,info,onUnauthorized);
}
function stop(clearPending=false){if(typeof CloudSpace!=='undefined')CloudSpace.reset();if(state.privateFiles)state.privateFiles.clear();state.privateFiles=null;state.privateTrail=[];state.privateBusy=false;state.privateContent=false;if(typeof CloudFileSurface!=='undefined')CloudFileSurface.clear();element('private-folders').hidden=true;drawPrivateFolders([]);element('private-files-refresh').hidden=true;element('private-files-more').hidden=true;element('private-files-status').textContent='';state.stopped=true;state.controller.abort();if(clearPending&&state.owner)CloudDownloadModel.clearPending(state.owner);state.intent.clear();element('download-source').value='';element('download-name').value='';element('download-task-list').replaceChildren();element('download-workspace').hidden=true;element('download-gate').hidden=false;element('download-gate').textContent='请登录后再使用云下载。';element('download-login').hidden=false;element('download-logout').hidden=true;}
async function api(path,method='GET',body){const response=await fetch(path,{method,credentials:'same-origin',cache:'no-store',signal:state.controller.signal,headers:body?{'Content-Type':'application/json'}:{},body:body?JSON.stringify(body):undefined});if(response.status===204)return null;const data=await response.json();if(!response.ok){if(response.status===401)stop();const failure=new Error('REQUEST_FAILED');failure.code=data.code;throw failure;}return data;}
function controls(){
 const authenticated=!state.sessionCheck||state.sessionView?.status==='AUTHENTICATED'&&Date.parse(state.sessionView.valid_until)>Date.now();
 const spaceReady=!state.privateOnly||typeof CloudSpace!=='undefined'&&CloudSpace.state.active;
 state.canSubmit=state.enabled&&authenticated&&spaceReady;
 const frozen=state.intent.busy||state.reconciling||state.intent.source!==null;
 element('download-source').disabled=frozen;
 element('download-name').disabled=frozen;
 const submit=element('download-submit');
 submit.disabled=!state.canSubmit||state.intent.busy||state.reconciling||!element('download-source').value;
 submit.textContent=state.intent.busy?'正在检查并提交…':state.intent.key?'继续同一次提交':'开始下载';
 const reconcileButton=element('download-reconcile');
 reconcileButton.hidden=!state.intent.key||!state.enabled;
 reconcileButton.disabled=state.intent.busy||state.reconciling;
}
function sessionMessage(){
 if(!state.sessionCheck)return;
 const view=state.sessionView;const fresh=view&&Date.parse(view.valid_until)>Date.now();
 const status=fresh?view.status:'UNKNOWN';
 element('download-session-status').textContent=state.sessionLoading&&!view?'正在检查 115 账号登录状态…':status==='AUTHENTICATED'?'115 账号登录有效，可以提交下载。':status==='LOGIN_REQUIRED'?'115 账号登录已失效，请管理员重新登录。新下载已暂停，输入和文件查看不受影响。':'暂时无法确认 115 账号状态，新下载已暂停。请稍后重新检查。';
 controls();
}
async function refreshSession(){
 if(!state.sessionCheck||state.stopped||state.sessionLoading)return;
 state.sessionLoading=true;element('download-session-refresh').disabled=true;sessionMessage();
 try{const view=await api('/api/v1/cloud-downloads/session');if(state.stopped)return;if(!view||!['AUTHENTICATED','LOGIN_REQUIRED','UNKNOWN'].includes(view.status)||!Number.isFinite(Date.parse(view.valid_until)))throw Error('INVALID_SESSION');state.sessionView=view;}
 catch(failure){if(!state.stopped)state.sessionView=null;}
 finally{state.sessionLoading=false;if(!state.stopped){element('download-session-refresh').disabled=false;sessionMessage();}}
}
element('download-session-refresh').addEventListener('click',refreshSession);
element('download-source').addEventListener('input',controls);

function accept(result){
 if(state.intent.accepted(result)){
  CloudDownloadModel.clearPending(state.owner);
  element('download-source').value='';
  element('download-name').value='';
  message('任务已提交。网盘完成下载并通过收录确认后，可在本页的已下载文件中查看。');
 }else{
  message('提交已保存，正在等待激活。请保留原链接和操作。');
 }
 controls();
}

function submissionFailed(failure,wasPending){
 state.intent.failed();
 // Source errors are returned only after the server finds no matching durable operation.
 if(Object.hasOwn(invalidSources,failure.code)||(!wasPending&&failure.code==='INVALID_REQUEST')){
  CloudDownloadModel.clearPending(state.owner);
  state.intent.clear();
  error(failure.code);
  message('请修改输入后重新开始。');
  controls();
  return;
 }
 const loginRequired=failure.code==='PROVIDER_LOGIN_REQUIRED';
 const sessionRejected=loginRequired||failure.code==='PROVIDER_SESSION_UNKNOWN';
 if(state.sessionCheck&&sessionRejected){
  state.sessionView={status:loginRequired?'LOGIN_REQUIRED':'UNKNOWN',valid_until:new Date(Date.now()+15000).toISOString()};
  sessionMessage();
 }
 error(failure.message==='FROZEN_INTENT'?'FROZEN_INTENT':failure.code);
 message(sessionRejected?'链接和操作标识已保留。恢复账号状态后可继续同一次提交。':'请先核实上次提交，或保持原内容继续同一次操作。');
 controls();
}

element('download-form').addEventListener('submit',async event=>{
 event.preventDefault();
 if(state.stopped)return;
 controls();
 if(!state.canSubmit||state.intent.busy||state.reconciling)return;
 clearDownloadError();
 message('');
 const source=element('download-source').value;
 const expected=element('download-name').value;
 const encoder=new TextEncoder();
 if(!source||encoder.encode(source).length>65536||expected!==''&&(encoder.encode(expected).length>512||expected.trim()===''||expected.includes('\0'))){
  error('INVALID_REQUEST');
  return;
 }
 const wasPending=state.intent.key!==null;
 let body;
 try{
  body=state.intent.begin(source,expected,()=> 'web-'+crypto.randomUUID());
  try{
   CloudDownloadModel.savePending(state.owner,body.operation_key);
  }catch(failure){
   if(wasPending)state.intent.failed();else state.intent.clear();
   error('STORAGE_UNAVAILABLE');
   controls();
   return;
  }
  controls();
  const endpoint=typeof CloudSpace!=='undefined'?CloudSpace.endpoint():'/api/v1/cloud-downloads';
  const result=await api(endpoint,'POST',body);
  if(state.stopped)return;
  accept(result);
  await refresh();
 }catch(failure){
  if(!state.stopped)submissionFailed(failure,wasPending);
 }finally{
  if(body)body.source='';
 }
});

async function reconcile(){
 if(!state.enabled||!state.intent.key||state.intent.busy||state.reconciling||state.stopped)return;
 clearDownloadError();
 state.reconciling=true;
 controls();
 try{
  const result=await api('/api/v1/cloud-downloads/operations/'+encodeURIComponent(state.intent.key));
  if(state.stopped)return;
  accept(result);
  await refresh();
 }catch(failure){
  if(!state.stopped)error(failure.code);
 }finally{
  state.reconciling=false;
  if(!state.stopped)controls();
 }
}
element('download-reconcile').addEventListener('click',reconcile);
function drawTask(task){const row=document.createElement('article');row.className='download-task';const content=document.createElement('div');const title=document.createElement('strong');title.textContent='云下载任务';const time=document.createElement('p');const created=new Date(task.created_at);time.textContent=Number.isFinite(created.getTime())?'提交于 '+created.toLocaleString('zh-CN'):'提交时间待核实';content.append(title,time);const status=document.createElement('div');const label=document.createElement('span');label.className='task-state';label.textContent=CloudDownloadModel.taskLabel(task);status.append(label);if(task.manifest_state==='READY'&&task.job_state==='SUCCEEDED'&&task.result_copy_id){const link=document.createElement('a');link.href='#download-files';link.textContent='查看已下载文件';status.append(link);}row.append(content,status);return row;}
async function refresh(more=false){
 if(state.refreshing||state.stopped||!state.owner)return;
 state.refreshing=true;element('download-refresh').disabled=true;
 try{
  const cursor=more?state.cursor:'';const page=await api('/api/v1/tasks?limit=50'+(cursor?'&cursor='+encodeURIComponent(cursor):''));if(state.stopped)return;
  if(!Array.isArray(page.items))throw new Error('INVALID_TASKS');
  if(!more){
   const ready=page.items.filter(t=>t.manifest_state==='READY'&&t.job_state==='SUCCEEDED'&&t.result_copy_id).map(t=>t.result_copy_id).sort().join('|');
   if(state.readyFiles!==null){
    if(state.privateFiles)await refreshPrivateFiles();
    else if(!state.privateOnly&&typeof CloudFileSurface!=='undefined'&&(ready!==state.readyFiles||CloudFileSurface.usesSnapshots?.()!==true))await CloudFileSurface.refresh();
   }
   state.readyFiles=ready;
  }
  if(state.stopped)return;
  if(!more)element('download-task-list').replaceChildren();
  for(const task of page.items)element('download-task-list').append(drawTask(task));
  state.cursor=page.next_cursor||'';element('download-more').hidden=!state.cursor;element('download-tasks-status').textContent=element('download-task-list').children.length?'仅显示自己的任务；状态查询不会再次启动下载。':'还没有云下载任务。';
  if(!state.enabled){element('download-usage').textContent='当前仅检查链接，不启动下载，也不扣次数。';return;}
  const usage=await api('/api/v1/usage');if(!state.stopped)element('download-usage').textContent=usage.unlimited===true?'下载次数不限 · 已提交 '+usage.used+' 次':'已使用 '+usage.used+' / '+usage.operation_limit+' 次';
 }
 catch(failure){if(!state.stopped)element('download-tasks-status').textContent='状态刷新失败，已显示的内容可能不是最新状态。';}
 finally{state.refreshing=false;element('download-refresh').disabled=false;}
}
element('download-refresh').addEventListener('click',()=>{state.paginated=false;refresh();});element('download-more').addEventListener('click',()=>{state.paginated=true;refresh(true);});
element('private-files-refresh').addEventListener('click',()=>refreshPrivateFiles());element('private-files-more').addEventListener('click',()=>refreshPrivateFiles(true));
element('private-folder-up').addEventListener('click',()=>openPrivateFolder(state.privateTrail.slice(0,-1)));
element('download-logout').addEventListener('click',async()=>{try{await api('/api/v1/auth/logout','POST');stop(true);}catch(failure){error(failure.code);}});
document.addEventListener('visibilitychange',()=>{if(!document.hidden&&!state.paginated)refresh();});
window.addEventListener('pagehide',()=>stop());
window.addEventListener('pageshow',event=>{if(event.persisted)location.reload();});
setInterval(()=>{if(!document.hidden){sessionMessage();refreshSession();if(!state.paginated)refresh();}},10000);
(async()=>{try{const principal=await api('/api/v1/auth/me');if(state.stopped)return;const info=await api('/api/v1/workspace');if(state.stopped)return;state.enabled=info.cloud_download_enabled===true;state.privateOnly=info.cloud_download_private_only===true;state.sessionCheck=info.cloud_download_session_check===true;state.canSubmit=state.enabled&&!state.sessionCheck;if(!state.enabled&&info.cloud_download_preview_enabled!==true){element('download-gate').textContent='云下载服务尚未启用，现有文件预览与下载不受影响。';return;}state.owner=principal.user_id;state.privateContent=info.content_access_enabled===true;state.intent=new CloudDownloadModel.Intent(CloudDownloadModel.pending(state.owner));element('download-user').textContent=principal.login;element('download-logout').hidden=false;element('download-gate').hidden=state.enabled;element('download-gate').textContent=info.cloud_download_unavailable_reason==='LOGIN_REQUIRED'?'115 登录已失效。你可以查看已收录的文件，恢复登录后才能开始下载。':'云下载尚未启用。你可以查看已收录的文件。';element('download-workspace').hidden=false;element('download-target').textContent='保存位置：'+(info.cloud_download_label||'网盘下载目录');if(typeof CloudSpace!=='undefined')await CloudSpace.mount(()=>{controls();if(!state.stopped&&state.owner&&CloudSpace.state.active)return mountPrivateFiles();},state.privateOnly);controls();if(state.sessionCheck){element('download-session').hidden=false;refreshSession();}if(typeof CloudSpace!=='undefined'&&CloudSpace.state.active)await mountPrivateFiles();else if(!state.privateOnly)await mountLegacyFiles(principal,info,()=>stop());if(state.stopped)return;await refresh();if(state.intent.key){message(state.canSubmit?'发现上次尚待核实的提交，正在查询原操作；不会自动提交。':'上次操作标识已保留，服务恢复后继续核实，不会自动重建任务。');if(state.enabled)await reconcile();}}catch(failure){if(!state.stopped){element('download-gate').textContent=errorText(failure.code);element('download-login').hidden=false;}}})();
