'use strict';
const node=id=>document.getElementById(id);
const library={session:0,request:0,detail:0,pdfRequest:0,content:false,player:null,catalog:null,bindings:[],category:'all',layout:'cards',busy:false,failed:false,opener:null,controller:new AbortController()};
const cloudFilePage=document.body?.dataset.fileSurface==='downloads';
const categories=['all','software','video','document','other'];
function failureText(error){return ({ACCOUNT_EXISTS:'此账号已注册，请登录或更换账号。',AUTH_REQUIRED:'登录已失效，请重新登录。',AUTH_BUSY:'登录或注册请求过多，请一分钟后再试。',READ_UNAVAILABLE:'暂时无法获取更多文件，已展示的结果可能不完整。',INVALID_REQUEST:'请检查输入后重试。',ORIGIN_REJECTED:'当前入口不可用。'})[error.code]||'连接暂时不可用，请刷新重试。';}
function errorAt(id,error){node(id).textContent=failureText(error);node(id).hidden=false;}
function clearError(id){node(id).textContent='';node(id).hidden=true;}
async function request(path,method='GET',body){
 const response=await fetch(path,{method,credentials:'same-origin',cache:'no-store',signal:library.controller.signal,headers:body?{'Content-Type':'application/json'}:{},body:body?JSON.stringify(body):undefined});
 if(response.status===204)return null;
 const data=await response.json();
 if(!response.ok){const error=new Error('Request refused');error.code=data.code;if(response.status===401&&method==='GET')signOut();throw error;}
 return data;
}
function closeDetail(restore=false){
 if(typeof closeShare==='function')closeShare();library.selected=null;library.downloading=false;node('download-status').textContent='';node('file-share').hidden=true;
 library.pdfRequest++;
 if(library.player){library.player.pause();library.player.removeAttribute('src');library.player.load();library.player=null;}
 library.detail++;node('file-preview').replaceChildren();node('preview-status').textContent='';node('file-download').removeAttribute('href');node('file-download').hidden=true;
 node('file-detail').hidden=true;
 for(const id of ['detail-format','detail-title','detail-type','detail-size','detail-note'])node(id).textContent='';
 if(restore&&library.opener&&library.opener.isConnected)library.opener.focus();
 library.opener=null;
}
function clearFiles(){
 if(library.remoteReader)library.remoteReader.clear();library.remoteReader=null;library.remotePage=null;library.remoteFilter=null;library.remoteCursors=[''];library.remoteIndex=0;
 library.filePage=0;library.autoLoad=false;library.loadingAll=null;node('load-pause').hidden=true;node('file-page-status').textContent='';node('file-page-prev').disabled=true;node('file-page-next').disabled=true;
 library.catalog=null;library.failed=false;library.busy=false;closeDetail();node('file-grid').replaceChildren();node('file-rows').replaceChildren();node('files-empty').hidden=true;node('load-more').hidden=true;
 for(const kind of categories)node('count-'+kind).textContent='';
 node('collection-status').textContent='正在整理你的文件…';node('load-status').textContent='';clearError('library-error');
}
function signOut(clearPending=false){
 if(cloudFilePage){CloudFileSurface.clear();if(library.onUnauthorized)library.onUnauthorized();return;}
	if(clearPending&&typeof CloudDownloadModel!=='undefined'&&library.userID)CloudDownloadModel.clearPending(library.userID);library.userID=null;node('cloud-download-link').hidden=true;
 if(typeof clearSharing==='function')clearSharing();node('my-shares').hidden=true;library.sharing=false;
 library.session++;library.request++;library.controller.abort();library.controller=new AbortController();clearFiles();library.bindings=[];library.content=false;library.category='all';library.layout='cards';
 node('session-error').hidden=true;node('session-error').textContent='';node('login-name').textContent='';node('file-search').value='';node('account-password').value='';node('account').hidden=true;node('library-view').hidden=true;node('login-view').hidden=false;
}
async function signIn(principal){
 const generation=++library.session;
 const info=await request('/api/v1/workspace');if(generation!==library.session)return;
	library.userID=principal.user_id;node('cloud-download-link').hidden=info.cloud_download_enabled!==true&&info.cloud_download_preview_enabled!==true;
 library.remote=info.library_snapshot_enabled===true;node('file-search-label').textContent=library.remote?'搜索已收录的文件名':'搜索已加载的文件名';library.cloudScope=info.cloud_download_files||null;library.bindings=info.bindings;library.wholeRoot=info.whole_root_library===true;node('library-scope').textContent=library.wholeRoot?'展示已接入目录中完成索引的文件，不代表整个网盘。':'当前展示已接入的真实文件，不代表整个网盘。';library.content=info.content_access_enabled===true;library.sharing=info.sharing_enabled===true;node('my-shares').hidden=!library.sharing;node('login-name').textContent=principal.login;node('account').hidden=false;node('session-error').hidden=true;node('session-error').textContent='';node('login-view').hidden=true;node('library-view').hidden=false;clearError('login-error');
 await refreshFiles();
}
function surfaceFiles(){return FileSurface.partition(library.catalog?library.catalog.files:[],library.cloudScope,cloudFilePage?'downloads':'library');}
function visibleFiles(){
 const needle=node('file-search').value.trim().toLocaleLowerCase('zh-CN');
 return surfaceFiles().filter(item=>(library.category==='all'||ResourceDisplay.category(item)===library.category)&&(!needle||ResourceDisplay.name(item).toLocaleLowerCase('zh-CN').includes(needle)));
}
function fileDetail(item,opener){
 closeDetail();
 library.selected=item;node('file-share').hidden=!library.sharing;
 const kind=ResourceDisplay.classify(item);library.opener=opener;
 node('detail-title').textContent=ResourceDisplay.name(item);node('detail-format').textContent=kind.extension.toUpperCase()||'FILE';node('detail-type').textContent=ResourceDisplay.category(item)==='other'?'其他':kind.label;node('detail-size').textContent=ResourceDisplay.size(item.size_bytes);
 node('detail-note').textContent=({software:'安装包不在网页内运行，请下载后自行核实来源与安全性。',video:'播放取决于浏览器对该文件编码的支持；可以拖动进度条跳转。',document:'文档预览只用于阅读，不修改网盘原文件。'})[kind.key]||'文件保留在网盘中，不进行修改。';
 node('file-detail').hidden=false;node('file-detail').focus();node('file-detail').scrollIntoView({block:'nearest'});
 previewFile(item,kind);
}
function fileURL(item,mode,disposition='inline'){const q=new URLSearchParams({binding_id:item.binding_id,resource_id:item.resource_id,path:item.path,disposition});return '/api/v1/files/'+mode+'?'+q;}
async function previewFile(item,kind){
 const generation=library.detail;const session=library.session;
 if(!library.content){node('preview-status').textContent='文件访问尚未启用。';return;}
 const download=node('file-download');download.href=fileURL(item,'content','attachment');download.target='_blank';download.rel='noopener';download.hidden=false;
 const target=node('file-preview');node('preview-status').textContent='';
 if(kind.key==='video'||kind.key==='audio'){
  previewMedia(item,kind,generation,session,target);return;
 }
 if(kind.extension==='pdf'){
  previewPDF(item,generation,session,1);return;
 }
 if(kind.key==='image'&&kind.extension!=='svg'){
  const image=document.createElement('img');image.alt=ResourceDisplay.name(item);image.src=fileURL(item,'content');target.append(image);return;
 }
 if(['txt','md','markdown','csv','docx','pptx','xlsx'].includes(kind.extension)){
  node('preview-status').textContent=['docx','pptx','xlsx'].includes(kind.extension)?'正在生成文本预览（不保留原版式）…':'正在读取文档…';
  try{const result=await request(fileURL(item,'preview'));if(session!==library.session||generation!==library.detail)return;const text=document.createElement('pre');text.textContent=result.text||'文件内容为空。';target.append(text);node('preview-status').textContent=['docx','pptx','xlsx'].includes(kind.extension)?'文本预览 · 不保留原版式，不执行公式或宏。':'文本预览';}
  catch(error){if(session===library.session&&generation===library.detail)node('preview-status').textContent=({PREVIEW_UNSUPPORTED:'该文件暂不支持在线预览，可以下载查看。',PREVIEW_TOO_LARGE:'文件超过在线文本预览大小限制，可以下载查看。',CONTENT_UNAVAILABLE:'暂时无法读取文件内容，请稍后重试。'})[error.code]||failureText(error);}
  return;
 }
 node('preview-status').textContent='此格式支持下载，不在网页内运行或预览。';
}
function previewMedia(item,kind,generation,session,target){
 const player=document.createElement(kind.key==='video'?'video':'audio');library.player=player;player.controls=true;player.preload='metadata';player.setAttribute('aria-label',ResourceDisplay.name(item));
 const toggle=document.createElement('button');toggle.type='button';toggle.className='secondary';toggle.textContent=kind.key==='video'?'播放视频':'播放音频';toggle.setAttribute('aria-label',toggle.textContent);
 const current=()=>session===library.session&&generation===library.detail&&library.player===player;
 const idle=kind.key==='video'?'播放视频':'播放音频';
 function paused(){toggle.textContent=idle;toggle.setAttribute('aria-label',idle);}
 toggle.addEventListener('click',async()=>{if(!current())return;try{if(player.paused){await player.play();}else{player.pause();}}catch(error){if(current())node('preview-status').textContent=error.name==='NotAllowedError'?'浏览器未允许播放，请再次点击播放按钮。':'无法播放：请检查本站连接或下载后使用本地播放器。';}});
 player.addEventListener('loadedmetadata',()=>{if(current())node('preview-status').textContent='媒体已就绪，点击播放按钮开始；可用播放器进度条跳转。';});
 player.addEventListener('playing',()=>{if(current()){toggle.textContent='暂停播放';toggle.setAttribute('aria-label','暂停播放');node('preview-status').textContent='正在播放';}});
 player.addEventListener('pause',()=>{if(current()){paused();node('preview-status').textContent='已暂停';}});
 player.addEventListener('ended',()=>{if(current()){paused();node('preview-status').textContent='播放结束';}});
 player.addEventListener('waiting',()=>{if(current())node('preview-status').textContent='正在缓冲，请稍候…';});
 player.addEventListener('error',()=>{if(current()){paused();node('preview-status').textContent=player.error&&player.error.code===3?'浏览器无法解码该文件，可下载后使用本地播放器。':'无法读取或播放媒体，请检查本站连接；也可能是浏览器不支持该格式，可下载后播放。';}});
 node('preview-status').textContent='正在连接媒体…';player.src=fileURL(item,'content');target.append(toggle,player);
}
async function previewPDF(item,generation,session,page){
 const requestGeneration=++library.pdfRequest;node('file-preview').replaceChildren();node('preview-status').textContent='服务器正在渲染 PDF 第 '+page+' 页…';
 const current=()=>generation===library.detail&&session===library.session&&requestGeneration===library.pdfRequest;
 try{
  const result=await request(fileURL(item,'preview')+'&page='+page);if(!current())return;
  if(result.format!=='pdf'||typeof result.image_base64!=='string'||!result.image_base64.startsWith('iVBORw0KGgo')||!Number.isInteger(result.pages)||result.pages<page)throw new Error('Invalid rendered page');
  const tools=document.createElement('div');tools.className='pdf-controls';const previous=document.createElement('button');previous.type='button';previous.className='secondary';previous.textContent='上一页';previous.disabled=page<=1;previous.addEventListener('click',()=>previewPDF(item,generation,session,page-1));
  const label=document.createElement('span');label.textContent='第 '+page+' / '+result.pages+' 页';const next=document.createElement('button');next.type='button';next.className='secondary';next.textContent='下一页';next.disabled=page>=result.pages;next.addEventListener('click',()=>previewPDF(item,generation,session,page+1));tools.append(previous,label,next);
  const image=document.createElement('img');image.className='pdf-page';image.alt='PDF 第 '+page+' 页';image.src='data:image/png;base64,'+result.image_base64;node('file-preview').append(tools,image);node('preview-status').textContent='服务器渲染预览 · 原文件缓存一小时后自动清理。';
 }catch(error){if(current())node('preview-status').textContent='暂时无法渲染 PDF，可下载原文件查看。';}
}
function drawFiles(preserveDetail=false,preserveRows=false){
 const catalog=library.catalog;if(!catalog)return;
 const remote=library.remote&&library.remotePage;const files=remote?surfaceFiles():visibleFiles();const counts=remote?remote.counts:ResourceDisplay.counts(surfaceFiles());
 for(const kind of categories){node('count-'+kind).textContent=counts?String(counts[kind]):'—';node('category-'+kind).setAttribute('aria-pressed',String(kind===library.category));}
 if(!preserveDetail)closeDetail();if(!preserveRows){node('file-grid').replaceChildren();node('file-rows').replaceChildren();}
 const pageSize=remote?50:120;const total=remote?remote.filtered:files.length;const pages=Math.max(1,Math.ceil(total/pageSize));library.filePage=remote?library.remoteIndex:Math.min(library.filePage||0,pages-1);const offset=remote?remote.offset:library.filePage*pageSize;
 node('file-page-prev').disabled=remote?!remote.ready||library.remoteIndex===0:library.filePage===0;node('file-page-next').disabled=remote?!remote.ready||!remote.next_cursor:library.filePage>=pages-1;node('file-page-status').textContent=remote&&!remote.ready?'目录同步中':files.length?'第 '+(library.filePage+1)+' / '+pages+' 页 · 文件 '+(offset+1)+'–'+Math.min(offset+pageSize,total):'';
 if(!preserveRows)for(const item of (remote?files:files.slice(offset,offset+120))){
  const kind=ResourceDisplay.classify(item);
  if(library.layout==='cards'){const button=document.createElement('button');button.type='button';button.className='file-card '+ResourceDisplay.category(item);
  const format=document.createElement('span');format.className='format';format.textContent=kind.extension.toUpperCase()||'FILE';
  const name=document.createElement('strong');name.textContent=ResourceDisplay.name(item);const meta=document.createElement('small');meta.textContent=(ResourceDisplay.category(item)==='other'?'其他':kind.label)+' · '+ResourceDisplay.size(item.size_bytes);
  button.append(format,name,meta);button.addEventListener('click',()=>fileDetail(item,button));node('file-grid').append(button);}
  else {const row=document.createElement('tr');const title=document.createElement('td');const link=document.createElement('button');link.type='button';link.textContent=ResourceDisplay.name(item);link.addEventListener('click',()=>fileDetail(item,link));title.append(link);row.append(title);
  for(const text of [ResourceDisplay.category(item)==='other'?'其他':kind.label,ResourceDisplay.size(item.size_bytes)]){const cell=document.createElement('td');cell.textContent=text;row.append(cell);}node('file-rows').append(row);}
 }
 node('file-grid').hidden=library.layout!=='cards';node('file-list').hidden=library.layout!=='list';
 for(const layout of ['cards','list'])node('view-'+layout).setAttribute('aria-pressed',String(layout===library.layout));
 node('files-empty').hidden=files.length!==0||library.busy||library.failed||!!(remote&&!remote.ready);
 node('empty-title').textContent=node('file-search').value?'没有匹配的文件':library.category==='all'?'还没有找到文件':'暂时没有这类文件';
 node('empty-description').textContent=catalog.complete?(cloudFilePage?'当前云下载目录中没有匹配结果。':'当前文件库中没有匹配结果。'):'更多文件尚未加载，可继续加载后再找找。';
 node('collection-status').textContent='已加载 '+surfaceFiles().length+' 个文件'+(files.length!==surfaceFiles().length?' · 当前显示 '+files.length+' 个':'');
 node('load-status').textContent=library.busy?'正在加载文件…':library.failed?'加载中断，当前结果可能不完整。':catalog.limited?'已达到本次加载上限，当前结果并非完整文件库。':catalog.complete?(library.wholeRoot?(cloudFilePage?'云下载目录已收录文件加载完成。':'主文件库已收录文件加载完成（不含云下载）。'):'已载入当前接入范围的文件，不是整个网盘。'):'还有文件待加载，搜索与分类仅针对已加载文件。';
 if(remote){node('collection-status').textContent=remote.ready?'已收录 '+remote.counts.all+' 个文件 · 本页 '+files.length+' 个':'正在同步目录 · 已显示 '+files.length+' 个文件';node('load-status').textContent=remote.stale?'后台更新暂时不可用，保留最近成功的目录。':remote.ready?'分类与搜索覆盖已收录文件，目录在后台更新。':'正在后台建立文件目录，统计尚未完成。';}
 if(catalog.unknown)node('load-status').textContent+=' '+catalog.unknown+' 项类型未明确，未当作文件显示。';
 node('load-more').hidden=!!remote||catalog.complete||catalog.limited||library.failed;node('load-more').disabled=library.busy;
 node('load-pause').hidden=!!remote||!library.wholeRoot||!library.autoLoad||catalog.complete||library.failed;
}
async function loadRound(){
 if(library.busy||!library.catalog)return;
 library.busy=true;library.failed=false;clearError('library-error');drawFiles(true);node('refresh').disabled=true;
 const session=library.session;const generation=++library.request;const catalog=library.catalog;
 const current=()=>session===library.session&&generation===library.request&&catalog===library.catalog;
 try{
  await catalog.round(scope=>{const query=new URLSearchParams({binding_id:scope.binding,limit:'50'});if(catalog.wholeRoot)query.set('whole_root','true');if(scope.parent)query.set('parent_resource_id',scope.parent);if(scope.cursor)query.set('cursor',scope.cursor);return request('/api/v1/resources?'+query);},current);
 }catch(error){if(current()){library.failed=true;errorAt('library-error',error);}}
 finally{if(current()){library.busy=false;node('refresh').disabled=false;drawFiles(true);}}
}
async function loadWholeLibrary(){const catalog=library.catalog;if(!catalog||library.loadingAll===catalog)return;library.loadingAll=catalog;library.autoLoad=true;try{while(library.catalog===catalog&&library.autoLoad&&!catalog.complete&&!catalog.limited&&!library.failed){await loadRound();await new Promise(resolve=>setTimeout(resolve,0));}}finally{if(library.catalog===catalog){library.loadingAll=null;library.autoLoad=false;drawFiles(true);}}}
async function refreshFiles(){
 if(library.remote)return loadRemotePage(library.remoteIndex||0,true);
 library.request++;library.controller.abort();library.controller=new AbortController();clearFiles();
 try{library.catalog=new FileCatalog.Catalog(library.bindings,library.wholeRoot);if(library.wholeRoot)await loadWholeLibrary();else await loadRound();}catch(error){library.failed=true;errorAt('library-error',error);node('collection-status').textContent='文件尚未加载';}
}
let registering=false;
function authMode(value){registering=value;node('auth-title').textContent=value?'创建你的账号':'登录文件库';node('login-submit').textContent=value?'注册账号':'进入文件库';node('auth-toggle').textContent=value?'已有账号？返回登录':'没有账号？立即注册';node('account-password').autocomplete=value?'new-password':'current-password';node('confirm-field').hidden=!value;node('account-confirm').required=value;node('auth-help').hidden=!value;node('account-password').value='';node('account-confirm').value='';node('auth-notice').hidden=true;clearError('login-error');}
if(!cloudFilePage){
node('auth-toggle').addEventListener('click',()=>authMode(!registering));
node('login-form').addEventListener('submit',async event=>{
 event.preventDefault();clearError('login-error');node('auth-notice').hidden=true;
 if(registering&&node('account-password').value!==node('account-confirm').value){node('login-error').textContent='两次密码输入不一致。';node('login-error').hidden=false;return;}
 if(Array.from(node('account-password').value).length<6||new TextEncoder().encode(node('account-password').value).length>72){node('login-error').textContent='密码至少 6 个字符，最多 72 字节。';node('login-error').hidden=false;return;}
 node('login-submit').disabled=true;node('auth-toggle').disabled=true;
 try{
  const credentials={login:node('account-name').value,password:node('account-password').value};
  if(registering){await request('/api/v1/auth/register','POST',credentials);authMode(false);node('auth-notice').textContent='账号已创建，请使用刚才的密码登录。';node('auth-notice').hidden=false;node('account-password').focus();}
  else{await signIn(await request('/api/v1/auth/login','POST',credentials));}
  credentials.password='';
 }catch(error){errorAt('login-error',error);if(error.code==='AUTH_REQUIRED')node('login-error').textContent='账号或密码不正确。';}
 finally{node('account-password').value='';node('account-confirm').value='';node('login-submit').disabled=false;node('auth-toggle').disabled=false;}
});
node('logout').addEventListener('click',async()=>{node('logout').disabled=true;try{await request('/api/v1/auth/logout','POST');signOut(true);}catch(error){errorAt('library-error',error);}finally{node('logout').disabled=false;}});
}
node('refresh').addEventListener('click',refreshFiles);node('load-more').addEventListener('click',()=>library.wholeRoot?loadWholeLibrary():loadRound());node('load-pause').addEventListener('click',()=>{library.autoLoad=false;node('load-pause').hidden=true;});
for(const category of categories)node('category-'+category).addEventListener('click',()=>{library.category=category;library.filePage=0;if(library.remote){loadRemotePage(0,true,true);return;}drawFiles();});
for(const layout of ['cards','list'])node('view-'+layout).addEventListener('click',()=>{library.layout=layout;drawFiles();});
let searchTimer;node('file-search').addEventListener('input',()=>{if(library.remote){clearTimeout(searchTimer);searchTimer=setTimeout(()=>loadRemotePage(0,true,true),250);return;}library.filePage=0;drawFiles();});node('detail-close').addEventListener('click',()=>closeDetail(true));
node('file-page-prev').addEventListener('click',()=>{if(library.remote){loadRemotePage(Math.max(0,library.remoteIndex-1));return;}library.filePage=Math.max(0,(library.filePage||0)-1);drawFiles();});node('file-page-next').addEventListener('click',()=>{if(library.remote){loadRemotePage(library.remoteIndex+1);return;}library.filePage=(library.filePage||0)+1;drawFiles();});
node('file-detail').addEventListener('keydown',event=>{if(event.key==='Escape')closeDetail(true);});
if(!cloudFilePage)(async()=>{try{const capabilities=await request('/api/v1/auth/capabilities');node('auth-toggle').hidden=capabilities.registration_enabled!==true;}catch(error){node('auth-toggle').hidden=true;}try{await signIn(await request('/api/v1/auth/me'));}catch(error){if(error.code!=='AUTH_REQUIRED'){node('session-error').textContent='连接暂时不可用，请刷新重试。';node('session-error').hidden=false;}}})();

// Reuse the exact content/player/download/share controller on the download page.
const CloudFileSurface=Object.freeze({
 usesSnapshots(){return library.remote===true;},
 async mount(principal,info,onUnauthorized){
  if(!cloudFilePage)return;
  CloudFileSurface.clear();
  library.remote=info.library_snapshot_enabled===true;node('file-search-label').textContent=library.remote?'搜索已收录的文件名':'搜索已加载的文件名';library.cloudScope=FileSurface.validScope(info.cloud_download_files)?info.cloud_download_files:null;
  if(!library.cloudScope)return;
  library.onUnauthorized=onUnauthorized;library.userID=principal.user_id;
  library.bindings=[{binding_id:library.cloudScope.binding_id}];library.wholeRoot=true;
  library.content=info.content_access_enabled===true;library.sharing=info.sharing_enabled===true;
  node('my-shares').hidden=!library.sharing;node('download-files').hidden=false;
  await refreshFiles();
 },
 clear(){
  library.session++;library.request++;library.controller.abort();library.controller=new AbortController();
  if(typeof clearSharing==='function')clearSharing();
  clearFiles();library.bindings=[];library.cloudScope=null;library.userID=null;library.content=false;library.sharing=false;
  library.category='all';library.layout='cards';node('file-search').value='';node('my-shares').hidden=true;
  if(cloudFilePage)node('download-files').hidden=true;
 },
 async refresh(){return refreshCanonicalQuietly();}
});
async function refreshCanonicalQuietly(){
 if(library.remote){if(!library.busy&&library.userID)return loadRemotePage(library.remoteIndex||0);return;}
  if(!library.userID||!library.catalog||library.busy||library.quietRefresh||!library.bindings.length)return;
  const session=library.session,generation=library.request,previous=library.catalog;
  const current=()=>session===library.session&&generation===library.request&&previous===library.catalog;
  const next=new FileCatalog.Catalog(library.bindings,library.wholeRoot);library.quietRefresh=true;
  try{
   while(!next.complete&&!next.limited&&current())await next.round(scope=>{const query=new URLSearchParams({binding_id:scope.binding,limit:'50'});if(next.wholeRoot)query.set('whole_root','true');if(scope.parent)query.set('parent_resource_id',scope.parent);if(scope.cursor)query.set('cursor',scope.cursor);return request('/api/v1/resources?'+query);},current);
   if(!current()||!next.complete)return;
   const after=FileSurface.partition(next.files,library.cloudScope,cloudFilePage?'downloads':'library');
   if(library.failed||JSON.stringify(after)!==JSON.stringify(surfaceFiles())){
    library.catalog=next;library.failed=false;
    if(library.selected&&!after.some(item=>item.binding_id===library.selected.binding_id&&item.resource_id===library.selected.resource_id&&item.path===library.selected.path))closeDetail();
    clearError('library-error');drawFiles(true);
   }
  }catch(error){if(current()&&error.name!=='AbortError'){library.failed=true;errorAt('library-error',error);}}
  finally{library.quietRefresh=false;}
}
// GET-only reconciliation, without resetting playback, details or login.
setInterval(()=>{if(!document.hidden&&library.userID&&(library.remote||!cloudFilePage))refreshCanonicalQuietly();},10000);

// A page fetch never clears the last successful publication. Request/session
// fences prevent late responses from crossing a query or account switch.
async function loadRemotePage(index=0,fresh=false,reset=false){
 if(!library.userID)return;
 library.request++;const generation=library.request,session=library.session;
 library.controller.abort();library.controller=new AbortController();
 if(!library.remoteReader)library.remoteReader=new LibraryPageModel.Reader(fetch,library.bindings.map(b=>b.binding_id));
 const filter=library.category+'\0'+node('file-search').value.trim();
 if(reset||library.remoteFilter&&library.remoteFilter!==filter)index=0;
 const cursors=library.remoteCursors||[''];const cursor=cursors[index]||'';
 const query={surface:cloudFilePage?'downloads':'library',category:library.category,q:node('file-search').value.trim(),cursor:index===0&&reset?'':cursor};
 if(!query.cursor&&index>0)query.page=String(index);
 library.busy=true;node('refresh').disabled=true;clearError('library-error');
 const current=()=>session===library.session&&generation===library.request&&library.remote;
 let redraw=false,preserveRows=false,recovered=false;try{
  let result;
  try{result=await library.remoteReader.read(query,library.controller.signal,fresh);}
  catch(error){if(error.code!=='LIBRARY_VERSION_EXPIRED'||!cursor)throw error;query.cursor='';query.page=String(index);recovered=true;result=await library.remoteReader.read(query,library.controller.signal,true);}
  if(!current())return;
  const changed=!library.remotePage||library.remoteFilter!==filter||library.remoteIndex!==Math.floor(result.page.offset/50)||JSON.stringify(library.remotePage)!==JSON.stringify(result.page);
  if(changed&&result.page.ready&&library.selected?.path&&!result.page.items.some(item=>item.binding_id===library.selected.binding_id&&item.resource_id===library.selected.resource_id&&item.path===library.selected.path)){
   const selected=library.selected;
   try{const exact=await request('/api/v1/resources/resolve?'+new URLSearchParams({binding_id:selected.binding_id,path:selected.path}));if(current()&&library.selected===selected&&exact?.binding_id===selected.binding_id&&typeof exact.resource_id==='string'&&exact.resource_id!==selected.resource_id)closeDetail();}
   catch(error){if(current()&&library.selected===selected&&['NOT_FOUND','RESOURCE_AMBIGUOUS'].includes(error.code))closeDetail();}
  }
  if(!current())return;
  index=Math.floor(result.page.offset/50);library.remoteIndex=index;library.remoteFilter=filter;library.remoteCursors=index===0?['']:recovered?Array(index+1).fill(''):cursors.slice(0,index+1);
  library.remoteCursors[index+1]=result.page.next_cursor;
  library.failed=false;
  if(changed){preserveRows=!!library.remotePage&&JSON.stringify(library.remotePage.items)===JSON.stringify(result.page.items);library.remotePage=result.page;library.catalog={files:result.page.items,complete:result.page.ready,limited:false,unknown:result.page.unknown||0};redraw=true;}
 }catch(error){if(current()&&error.name!=='AbortError'){if(error.code==='AUTH_REQUIRED'){signOut();return;}if(error.code==='LIBRARY_SCOPE_CHANGED'){clearFiles();library.failed=true;node('library-error').hidden=false;node('library-error').textContent='文件库范围已变更，请重新加载页面。';return;}library.failed=true;errorAt('library-error',error);node('load-status').textContent='更新失败，保留已显示的文件。';}}
 finally{if(current()){library.busy=false;node('refresh').disabled=false;if(redraw)drawFiles(true,preserveRows);else if(!library.failed&&library.remotePage)node('load-status').textContent=library.remotePage.stale?'后台更新暂时不可用，保留最近成功的目录。':library.remotePage.ready?'分类与搜索覆盖已收录文件，目录在后台更新。':'正在后台建立文件目录，统计尚未完成。';}}
}
