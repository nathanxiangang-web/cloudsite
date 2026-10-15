'use strict';
const node=id=>document.getElementById(id);
const library={session:0,request:0,detail:0,pdfRequest:0,content:false,player:null,catalog:null,bindings:[],category:'all',layout:'cards',busy:false,failed:false,opener:null,controller:new AbortController()};
const categories=['all','software','video','document','other'];
function failureText(error){return ({AUTH_REQUIRED:'登录已失效，请重新登录。',AUTH_BUSY:'登录请求过多，请稍候再试。',READ_UNAVAILABLE:'暂时无法获取更多文件，已展示的结果可能不完整。',INVALID_REQUEST:'请检查输入后重试。',ORIGIN_REJECTED:'当前入口不可用。'})[error.code]||'连接暂时不可用，请刷新重试。';}
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
 library.catalog=null;library.failed=false;library.busy=false;closeDetail();node('file-grid').replaceChildren();node('file-rows').replaceChildren();node('files-empty').hidden=true;node('load-more').hidden=true;
 for(const kind of categories)node('count-'+kind).textContent='';
 node('collection-status').textContent='正在整理你的文件…';node('load-status').textContent='';clearError('library-error');
}
function signOut(){
 if(typeof clearSharing==='function')clearSharing();node('my-shares').hidden=true;library.sharing=false;
 library.session++;library.request++;library.controller.abort();library.controller=new AbortController();clearFiles();library.bindings=[];library.content=false;library.category='all';library.layout='cards';
 node('login-name').textContent='';node('file-search').value='';node('account-password').value='';node('account').hidden=true;node('admin-link').hidden=true;node('library-view').hidden=true;node('login-view').hidden=false;
}
async function signIn(principal){
 const generation=++library.session;
 const info=await request('/api/v1/workspace');if(generation!==library.session)return;
 library.bindings=info.bindings;library.content=info.content_access_enabled===true;library.sharing=info.sharing_enabled===true;node('my-shares').hidden=!library.sharing;node('login-name').textContent=principal.login;node('admin-link').hidden=principal.role!=='ADMIN';node('account').hidden=false;node('login-view').hidden=true;node('library-view').hidden=false;clearError('login-error');
 await refreshFiles();
}
function visibleFiles(){
 const needle=node('file-search').value.trim().toLocaleLowerCase('zh-CN');
 return (library.catalog?library.catalog.files:[]).filter(item=>(library.category==='all'||ResourceDisplay.category(item)===library.category)&&(!needle||ResourceDisplay.name(item).toLocaleLowerCase('zh-CN').includes(needle)));
}
function fileDetail(item,opener){
 closeDetail();
 library.selected=item;node('file-share').hidden=!library.sharing;
 const kind=ResourceDisplay.classify(item);library.opener=opener;
 node('detail-title').textContent=ResourceDisplay.name(item);node('detail-format').textContent=kind.extension.toUpperCase()||'FILE';node('detail-type').textContent=kind.label;node('detail-size').textContent=ResourceDisplay.size(item.size_bytes);
 node('detail-note').textContent=({software:'安装包不在网页内运行，请下载后自行核实来源与安全性。',video:'播放取决于浏览器对该文件编码的支持；可以拖动进度条跳转。',document:'文档预览只用于阅读，不修改网盘原文件。'})[kind.key]||'文件保留在网盘中，不进行修改。';
 node('file-detail').hidden=false;node('file-detail').focus();node('file-detail').scrollIntoView({block:'nearest'});
 previewFile(item,kind);
}
function fileURL(item,mode,disposition='inline'){const q=new URLSearchParams({binding_id:item.binding_id,resource_id:item.resource_id,path:item.path,disposition});return '/api/v1/files/'+mode+'?'+q;}
async function previewFile(item,kind){
 const generation=library.detail;const session=library.session;
 if(!library.content){node('preview-status').textContent='文件访问尚未启用。';return;}
 const download=node('file-download');download.href=fileURL(item,'content','attachment');download.hidden=false;
 const target=node('file-preview');node('preview-status').textContent='';
 if(kind.key==='video'||kind.key==='audio'){
  const player=document.createElement(kind.key==='video'?'video':'audio');library.player=player;player.controls=true;player.preload='metadata';player.setAttribute('aria-label',ResourceDisplay.name(item));
  player.src=fileURL(item,'content');player.addEventListener('error',()=>{if(session===library.session&&generation===library.detail)node('preview-status').textContent='当前浏览器无法播放此文件，请尝试下载后播放。';});target.append(player);return;
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
function drawFiles(){
 const catalog=library.catalog;if(!catalog)return;
 const files=visibleFiles();const counts=ResourceDisplay.counts(catalog.files);
 for(const kind of categories){node('count-'+kind).textContent=String(counts[kind]);node('category-'+kind).setAttribute('aria-pressed',String(kind===library.category));}
 closeDetail();node('file-grid').replaceChildren();node('file-rows').replaceChildren();
 for(const item of files){
  const kind=ResourceDisplay.classify(item);
  const button=document.createElement('button');button.type='button';button.className='file-card '+ResourceDisplay.category(item);
  const format=document.createElement('span');format.className='format';format.textContent=kind.extension.toUpperCase()||'FILE';
  const name=document.createElement('strong');name.textContent=ResourceDisplay.name(item);const meta=document.createElement('small');meta.textContent=kind.label+' · '+ResourceDisplay.size(item.size_bytes);
  button.append(format,name,meta);button.addEventListener('click',()=>fileDetail(item,button));node('file-grid').append(button);
  const row=document.createElement('tr');const title=document.createElement('td');const link=document.createElement('button');link.type='button';link.textContent=ResourceDisplay.name(item);link.addEventListener('click',()=>fileDetail(item,link));title.append(link);row.append(title);
  for(const text of [kind.label,ResourceDisplay.size(item.size_bytes)]){const cell=document.createElement('td');cell.textContent=text;row.append(cell);}node('file-rows').append(row);
 }
 node('file-grid').hidden=library.layout!=='cards';node('file-list').hidden=library.layout!=='list';
 for(const layout of ['cards','list'])node('view-'+layout).setAttribute('aria-pressed',String(layout===library.layout));
 node('files-empty').hidden=files.length!==0||library.busy||library.failed;
 node('empty-title').textContent=node('file-search').value?'没有匹配的文件':library.category==='all'?'还没有找到文件':'暂时没有这类文件';
 node('empty-description').textContent=catalog.complete?'当前已接入文件中没有匹配结果。':'更多文件尚未加载，可继续加载后再找找。';
 node('collection-status').textContent='已加载 '+catalog.files.length+' 个文件'+(files.length!==catalog.files.length?' · 当前显示 '+files.length+' 个':'');
 node('load-status').textContent=library.busy?'正在加载文件…':library.failed?'加载中断，当前结果可能不完整。':catalog.limited?'已达到本次加载上限，当前结果并非完整文件库。':catalog.complete?'已载入当前接入范围的文件，不是整个网盘。':'还有文件待加载，搜索与分类仅针对已加载文件。';
 if(catalog.unknown)node('load-status').textContent+=' '+catalog.unknown+' 项类型未明确，未当作文件显示。';
 node('load-more').hidden=catalog.complete||catalog.limited||library.failed;node('load-more').disabled=library.busy;
}
async function loadRound(){
 if(library.busy||!library.catalog)return;
 library.busy=true;library.failed=false;closeDetail();clearError('library-error');drawFiles();node('refresh').disabled=true;
 const session=library.session;const generation=++library.request;const catalog=library.catalog;
 const current=()=>session===library.session&&generation===library.request&&catalog===library.catalog;
 try{
  await catalog.round(scope=>{const query=new URLSearchParams({binding_id:scope.binding,limit:'50'});if(scope.parent)query.set('parent_resource_id',scope.parent);if(scope.cursor)query.set('cursor',scope.cursor);return request('/api/v1/resources?'+query);},current);
 }catch(error){if(current()){library.failed=true;errorAt('library-error',error);}}
 finally{if(current()){library.busy=false;node('refresh').disabled=false;drawFiles();}}
}
async function refreshFiles(){
 library.request++;library.controller.abort();library.controller=new AbortController();clearFiles();
 try{library.catalog=new FileCatalog.Catalog(library.bindings);await loadRound();}catch(error){library.failed=true;errorAt('library-error',error);node('collection-status').textContent='文件尚未加载';}
}
node('login-form').addEventListener('submit',async event=>{event.preventDefault();clearError('login-error');node('login-submit').disabled=true;try{const principal=await request('/api/v1/auth/login','POST',{login:node('account-name').value,password:node('account-password').value});await signIn(principal);}catch(error){errorAt('login-error',error);if(error.code==='AUTH_REQUIRED')node('login-error').textContent='账号或密码不正确。';}finally{node('account-password').value='';node('login-submit').disabled=false;}});
node('logout').addEventListener('click',async()=>{node('logout').disabled=true;try{await request('/api/v1/auth/logout','POST');signOut();}catch(error){errorAt('library-error',error);}finally{node('logout').disabled=false;}});
node('refresh').addEventListener('click',refreshFiles);node('load-more').addEventListener('click',loadRound);
for(const category of categories)node('category-'+category).addEventListener('click',()=>{library.category=category;drawFiles();});
for(const layout of ['cards','list'])node('view-'+layout).addEventListener('click',()=>{library.layout=layout;drawFiles();});
node('file-search').addEventListener('input',drawFiles);node('detail-close').addEventListener('click',()=>closeDetail(true));
node('file-detail').addEventListener('keydown',event=>{if(event.key==='Escape')closeDetail(true);});
(async()=>{try{await signIn(await request('/api/v1/auth/me'));}catch(error){if(error.code!=='AUTH_REQUIRED')errorAt('login-error',error);}})();
