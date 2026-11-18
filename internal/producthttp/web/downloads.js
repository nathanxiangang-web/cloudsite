'use strict';
const element=id=>document.getElementById(id);
const state={owner:null,intent:new CloudDownloadModel.Intent(),controller:new AbortController(),preview:null,generation:0,cursor:'',refreshing:false,stopped:false,paginated:false,enabled:false,sessionCheck:false,sessionView:null,sessionLoading:false,canSubmit:false,canPrepare:false,readyFiles:null,privateFiles:null,privateBusy:false,privateContent:false};
const errorText=code=>({PROVIDER_LOGIN_REQUIRED:'115 账号登录已失效，本次新请求未创建下载任务。请管理员恢复网盘登录后继续。',PROVIDER_SESSION_UNKNOWN:'暂时无法确认 115 登录状态，本次新请求未创建下载任务。请稍后检查账号。',AUTH_REQUIRED:'请先登录文件库。',INVALID_REQUEST:'链接或文件名不符合要求，请检查后重试。',ORIGIN_REJECTED:'当前网站入口不可用。',OPERATION_CONFLICT:'同一次提交的内容不一致，请先核实上次结果，不要创建重复任务。',OPERATION_LIMIT:'已达到当前账号的下载次数上限。',CONCURRENCY_LIMIT:'同时进行的任务已满，请等待现有任务完成。',BINDING_NOT_AVAILABLE:'当前下载目标暂不可用。',ADMISSION_UNCERTAIN:'提交结果尚未确认，请核实或使用原内容继续同一次提交。',OPERATION_NOT_FOUND:'暂未查到提交记录。这不能证明之前未提交，请继续核实，或用原内容重试同一次提交。',FROZEN_INTENT:'这次提交的内容已固定，请保持原链接与文件名。',STORAGE_UNAVAILABLE:'浏览器无法保存这次操作标识，请允许本站会话存储后再提交。'})[code]||'连接暂时不可用。若已经点过提交，请先核实上次结果，不要重复新建。';
function message(text){element('download-message').textContent=text;}
function error(code){element('download-error').textContent=errorText(code);element('download-error').hidden=false;}
function clearDownloadError(){element('download-error').hidden=true;element('download-error').textContent='';}
function showPrivateFiles(result){
 if(!result||state.stopped||!state.privateFiles)return;
 library.remote=false;library.cloudScope=result.scope;library.bindings=result.scope?[{binding_id:result.scope.binding_id}]:[];library.wholeRoot=true;
 library.content=state.privateContent;library.sharing=false;library.userID=state.owner;library.onUnauthorized=()=>stop();library.failed=false;
 library.catalog={files:result.items,complete:!result.cursor,limited:!!result.cursor,unknown:0};
 if(library.selected&&!result.items.some(item=>item.binding_id===library.selected.binding_id&&item.resource_id===library.selected.resource_id&&item.path===library.selected.path))closeDetail();
 element('my-shares').hidden=true;drawFiles(true);element('load-more').hidden=true;
 element('private-files-more').hidden=!result.cursor;
 element('collection-status').textContent='私有空间已加载 '+result.items.length+' 个文件';
 element('load-status').textContent=result.cursor?'可继续加载更多文件；搜索与分类仅针对已加载文件。':'当前私有空间的文件已加载完成。';
 element('private-files-status').textContent=result.items.length?'仅显示当前私有空间中的已收录文件。':'当前私有空间还没有已收录文件。';
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
 CloudFileSurface.clear();state.privateFiles=new FileSurface.PrivateFiles();
 element('download-files-scope').textContent='仅显示当前私有空间目录中的直接文件，不包含子文件夹内容；不会混入主文件库。';
 element('download-files').hidden=false;element('refresh').hidden=true;element('private-files-refresh').hidden=false;element('private-files-more').hidden=true;
 await refreshPrivateFiles();
}
function mountLegacyFiles(principal,info,onUnauthorized){
 if(state.privateFiles)state.privateFiles.clear();state.privateFiles=null;state.privateBusy=false;
 element('download-files-scope').textContent='云下载目录中已收录的文件在这里展示，包含子文件夹中的内容；不会混入主文件库。';
 element('refresh').hidden=false;element('private-files-refresh').hidden=true;element('private-files-more').hidden=true;element('private-files-status').textContent='';
 if(typeof CloudFileSurface!=='undefined')return CloudFileSurface.mount(principal,info,onUnauthorized);
}
function stop(clearPending=false){if(typeof CloudSpace!=='undefined')CloudSpace.reset();if(state.privateFiles)state.privateFiles.clear();state.privateFiles=null;state.privateBusy=false;state.privateContent=false;if(typeof CloudFileSurface!=='undefined')CloudFileSurface.clear();element('private-files-refresh').hidden=true;element('private-files-more').hidden=true;element('private-files-status').textContent='';state.stopped=true;state.generation++;state.controller.abort();if(clearPending&&state.owner)CloudDownloadModel.clearPending(state.owner);state.intent.clear();state.preview=null;element('download-source').value='';element('download-name').value='';element('download-execution').textContent='';element('download-task-list').replaceChildren();element('download-workspace').hidden=true;element('download-gate').hidden=false;element('download-gate').textContent='请登录后再使用云下载。';element('download-login').hidden=false;element('download-logout').hidden=true;}
async function api(path,method='GET',body){const response=await fetch(path,{method,credentials:'same-origin',cache:'no-store',signal:state.controller.signal,headers:body?{'Content-Type':'application/json'}:{},body:body?JSON.stringify(body):undefined});if(response.status===204)return null;const data=await response.json();if(!response.ok){if(response.status===401)stop();const failure=new Error('REQUEST_FAILED');failure.code=data.code;throw failure;}return data;}
function controls(){if(state.sessionCheck)state.canSubmit=state.enabled&&state.sessionView?.status==='AUTHENTICATED'&&Date.parse(state.sessionView.valid_until)>Date.now();const busy=state.intent.busy;element('download-source').disabled=busy||state.intent.source!==null;element('download-name').disabled=busy||state.intent.source!==null;element('download-prepare').disabled=busy||!state.canPrepare;element('download-submit').disabled=!state.canSubmit||busy||!state.preview;element('download-submit').textContent=!state.canSubmit?'暂不能提交':busy?'正在提交…':state.intent.key?'继续同一次提交':'提交下载';element('download-reconcile').hidden=!state.intent.key||!state.enabled;}
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
function inputChanged(){if(state.intent.busy)return;state.preview=null;state.generation++;element('download-preview').hidden=true;element('download-execution').textContent='';controls();}
element('download-source').addEventListener('input',inputChanged);element('download-name').addEventListener('input',inputChanged);
element('download-prepare').addEventListener('click',async()=>{
 if(!state.canPrepare||state.stopped||state.intent.busy)return;clearDownloadError();const source=element('download-source').value;const expected=element('download-name').value;const generation=++state.generation;
 if(!source||new TextEncoder().encode(source).length>65536||(expected!==''&&(new TextEncoder().encode(expected).length>512||expected.trim()===''||expected.includes('\0')))){error('INVALID_REQUEST');return;}
 element('download-prepare').disabled=true;
 try{const prepared=await api('/api/v1/cloud-downloads/prepare','POST',{source});if(state.stopped||generation!==state.generation)return;if(typeof prepared.execution_source!=='string')throw new Error('INVALID_PREVIEW');state.preview={source,expected};element('download-execution').textContent=prepared.execution_source;element('download-policy').textContent=prepared.changed?'已移除附加参数，并保留原始输入用于核对。':'链接将按当前内容提交。';element('download-preview').hidden=false;message(!state.canSubmit?'链接检查完成，未启动下载。服务恢复后才可提交。':state.intent.key?'检查完成。将沿用上次的操作标识，不会新建另一组任务。':'检查完成。点击提交后，任务会由网盘执行。');}
 catch(failure){if(!state.stopped&&generation===state.generation)error(failure.code);}
 finally{if(!state.stopped&&generation===state.generation)controls();}
});
function accept(result){const complete=state.intent.accepted(result);if(complete){CloudDownloadModel.clearPending(state.owner);state.preview=null;element('download-source').value='';element('download-name').value='';element('download-execution').textContent='';element('download-preview').hidden=true;message('任务已提交。网盘完成下载并通过收录确认后，可在本页的已下载文件中查看。');}else message('提交已保存，正在等待激活。请保留原链接和操作，不要重新新建任务。');controls();}
element('download-form').addEventListener('submit',async event=>{
 event.preventDefault();controls();clearDownloadError();if(!state.canSubmit||!state.preview||state.intent.busy||state.stopped)return;
 let body;
 try{body=state.intent.begin(state.preview.source,state.preview.expected,()=> 'web-'+crypto.randomUUID());try{CloudDownloadModel.savePending(state.owner,body.operation_key);}catch(failure){state.intent.failed();error('STORAGE_UNAVAILABLE');controls();return;}controls();const result=await api(typeof CloudSpace!=='undefined'?CloudSpace.endpoint():'/api/v1/cloud-downloads','POST',body);if(state.stopped)return;accept(result);await refresh();}
 catch(failure){if(!state.stopped){state.intent.failed();if(state.sessionCheck&&['PROVIDER_LOGIN_REQUIRED','PROVIDER_SESSION_UNKNOWN'].includes(failure.code)){state.sessionView={status:failure.code==='PROVIDER_LOGIN_REQUIRED'?'LOGIN_REQUIRED':'UNKNOWN',valid_until:new Date(Date.now()+15000).toISOString()};sessionMessage();}error(failure.message==='FROZEN_INTENT'?'FROZEN_INTENT':failure.code);message(['PROVIDER_LOGIN_REQUIRED','PROVIDER_SESSION_UNKNOWN'].includes(failure.code)?'链接和操作标识已保留。恢复账号状态后可继续同一次提交。':'没有自动重试。请先核实上次提交，或保持原内容继续同一次操作。');controls();}}
 finally{if(body)body.source='';}
});
async function reconcile(){if(!state.enabled||!state.intent.key||state.stopped)return;clearDownloadError();element('download-reconcile').disabled=true;try{const result=await api('/api/v1/cloud-downloads/operations/'+encodeURIComponent(state.intent.key));if(state.stopped)return;accept(result);await refresh();}catch(failure){if(!state.stopped){error(failure.code);controls();}}finally{element('download-reconcile').disabled=false;}}
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
    else if(typeof CloudFileSurface!=='undefined'&&(ready!==state.readyFiles||CloudFileSurface.usesSnapshots?.()!==true))await CloudFileSurface.refresh();
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
element('download-logout').addEventListener('click',async()=>{try{await api('/api/v1/auth/logout','POST');stop(true);}catch(failure){error(failure.code);}});
document.addEventListener('visibilitychange',()=>{if(!document.hidden&&!state.paginated)refresh();});
window.addEventListener('pagehide',()=>stop());
window.addEventListener('pageshow',event=>{if(event.persisted)location.reload();});
setInterval(()=>{if(!document.hidden){sessionMessage();refreshSession();if(!state.paginated)refresh();}},10000);
(async()=>{try{const principal=await api('/api/v1/auth/me');if(state.stopped)return;const info=await api('/api/v1/workspace');if(state.stopped)return;state.enabled=info.cloud_download_enabled===true;state.sessionCheck=info.cloud_download_session_check===true;state.canSubmit=state.enabled&&!state.sessionCheck;state.canPrepare=state.enabled||state.canSubmit||info.cloud_download_preview_enabled===true;if(!state.canPrepare){element('download-gate').textContent='云下载服务尚未启用，现有文件预览与下载不受影响。';return;}state.owner=principal.user_id;state.privateContent=info.content_access_enabled===true;state.intent=new CloudDownloadModel.Intent(CloudDownloadModel.pending(state.owner));element('download-user').textContent=principal.login;element('download-logout').hidden=false;element('download-gate').hidden=state.enabled;element('download-gate').textContent=info.cloud_download_unavailable_reason==='LOGIN_REQUIRED'?'115 登录已失效。可检查链接和查看文件，恢复登录后才能提交下载。':'云下载尚未接通。可以先检查链接，不会启动下载。';element('download-workspace').hidden=false;element('download-target').textContent='保存位置：'+(info.cloud_download_label||'网盘下载目录');if(typeof CloudSpace!=='undefined')await CloudSpace.mount(()=>{if(!state.stopped&&state.owner&&CloudSpace.state.active)return mountPrivateFiles();});controls();if(state.sessionCheck){element('download-session').hidden=false;refreshSession();}if(typeof CloudSpace!=='undefined'&&CloudSpace.state.active)await mountPrivateFiles();else await mountLegacyFiles(principal,info,()=>stop());if(state.stopped)return;await refresh();if(state.intent.key){message(state.canSubmit?'发现上次尚待核实的提交，正在查询原操作；不会自动提交。':'上次操作标识已保留，服务恢复后继续核实，不会自动重建任务。');if(state.enabled)await reconcile();}}catch(failure){if(!state.stopped){element('download-gate').textContent=errorText(failure.code);element('download-login').hidden=false;}}})();
