'use strict';
// Admin-only paged view module. Authentication and HTTP are injected ports;
// consumer controllers and owner-only task handling are independent.
const AdminLists={create({el,read,generation,principal,onError,dateText,cell,badge,ownerSelected,userSelected}){
 const uuid=/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
 const views={};
 for(const kind of ['users','downloads']){
  const prefix=kind==='users'?'users':'admin-downloads';
  const view={cursor:'',next:'',request:0,key:'',loaded:false,owner:'',observed:'',search:'',status:'ALL'};views[kind]=view;
  const rows=()=>el(prefix+'-rows'),footer=()=>el(prefix+'-footer');
  view.clear=()=>{view.loaded=false;view.key='';view.next='';view.observed='';rows().replaceChildren();el(prefix+'-empty').hidden=true;el('more-'+prefix).disabled=true;footer().textContent='等待查询';};
  view.reset=()=>{view.request++;view.cursor='';view.owner='';view.clear();el('refresh-'+prefix).disabled=false;};
  view.invalidate=()=>{view.request++;el('refresh-'+prefix).disabled=false;el('more-'+prefix).disabled=!view.next;};
  view.load=async()=>{
   if(!principal())return;
   const gen=generation(),request=++view.request,key=[view.cursor,view.owner,view.search,view.status].join('|'),previous=view.loaded&&view.key===key;
   if(!previous)view.clear();view.key=key;el(prefix+'-empty').hidden=true;
   el('refresh-'+prefix).disabled=true;el('more-'+prefix).disabled=true;footer().textContent=previous?'正在刷新… · 旧页暂留':'LOADING · 正在查询…';
   const query=new URLSearchParams({limit:'20'});if(view.cursor)query.set('cursor',view.cursor);if(kind==='downloads'&&view.owner)query.set('user_id',view.owner);if(kind==='users'){if(view.search)query.set('q',view.search);if(view.status&&view.status!=='ALL')query.set('status',view.status);}
   try{
    const page=await read('/api/v1/admin/'+kind+'?'+query,gen);
    if(gen!==generation()||request!==view.request)return;
    if(!page||!Array.isArray(page.items)||page.items.length>20||(page.next_cursor&&!uuid.test(page.next_cursor))||(kind==='users'&&page.total!==undefined&&(!Number.isSafeInteger(page.total)||page.total<0||page.total<page.items.length||page.scope!=='FILTERED_PRODUCT_USERS'||!Number.isFinite(new Date(page.read_at).getTime()))))throw Error('Invalid page');
    rows().replaceChildren();
    for(const item of page.items){
     const row=document.createElement('tr');
     if(kind==='users'){
      const name=cell(row,item.login),id=document.createElement('small');id.textContent=item.user_id;name.append(id);
      cell(row).append(badge(item.role,'neutral'));cell(row).append(badge(item.enabled?'启用':'停用',item.enabled?'':'warning'));
      cell(row,dateText(item.created_at));cell(row,dateText(item.updated_at));
      const button=document.createElement('button');button.type='button';button.className='quiet';button.textContent='查看该用户任务';button.addEventListener('click',()=>ownerSelected(item.user_id));const actions=cell(row);actions.append(button);
      if(item.role==='USER'&&userSelected){const edit=document.createElement('button');edit.type='button';edit.className='quiet';edit.textContent='管理此 USER';edit.addEventListener('click',()=>userSelected(item));actions.append(edit);}
     }else{
      cell(row,item.manifest_id);cell(row,item.owner_user_id);cell(row).append(badge(item.manifest_state,item.manifest_state==='RECOVERY_REQUIRED'?'warning':'neutral'));cell(row,item.job_state??'未创建 Job');
      cell(row,String(item.claim_attempts)+' / '+String(item.failure_count));cell(row,dateText(item.updated_at));cell(row,item.binding_id);cell(row,item.job_id??'未创建 Job');cell(row,item.result_copy_id??'未提供');cell(row,dateText(item.created_at));
     }
     rows().append(row);
    }
    view.loaded=true;view.next=page.next_cursor||'';view.observed=new Date().toLocaleString();
    el(prefix+'-empty').hidden=page.items.length!==0;el('more-'+prefix).disabled=!view.next;
    footer().textContent='本页 '+page.items.length+' 项 · GET /api/v1/admin/'+kind+(kind==='users'?(Number.isSafeInteger(page.total)?' · 匹配总数 '+page.total+'（服务端聚合，非本页推算） · 数据库读取于 '+dateText(page.read_at):' · 不代表全站总数（服务端未返回聚合，不据本页推算）'):'')+' · 客户端读取于 '+view.observed+'（非数据库观察时间）'+(kind==='downloads'?' · '+(view.owner?'用户 '+view.owner:'配置 Binding 内全部已归属任务'):' · 过滤条件：'+((view.search?'login~'+view.search:'')+(view.status&&view.status!=='ALL'?' 状态='+view.status:'')));
   }catch(error){if(gen===generation()&&request===view.request){el(prefix+'-empty').hidden=true;footer().textContent=previous?'STALE · 查询失败；保留上次同页结果 · 客户端读取于 '+view.observed:'查询未完成 · 不以空结果代替失败';onError(error);}}
   finally{if(gen===generation()&&request===view.request)el('refresh-'+prefix).disabled=false;}
  };
  view.refresh=()=>{view.cursor='';return view.load();};
  view.more=()=>{if(view.next){view.cursor=view.next;return view.load();}};
 }
 views.downloads.filter=value=>{
  if(value!==''&&!uuid.test(value)){const error=Error('Invalid UUID');error.code='INVALID_REQUEST';onError(error);return;}
  views.downloads.owner=value;views.downloads.cursor='';return views.downloads.load();
 };
 views.users.filter=(search,status)=>{
  const q=(search||'').trim();
  if(!/^[a-z0-9._-]{0,64}$/.test(q)||status&&!['ALL','ENABLED','DISABLED','DELETED'].includes(status)){const error=Error('Invalid user filter');error.code='INVALID_REQUEST';onError(error);return;}
  views.users.search=q;views.users.status=status||'ALL';views.users.cursor='';return views.users.load();
 };
 // Minimal server-aggregated total: one-item page. Never infers from a page sample.
 // Returns a safe integer only when the server provides a validated DirectoryReads total; otherwise null ("未提供").
 views.users.total=async()=>{
  if(!principal())return null;
  const gen=generation();
  try{
   const page=await read('/api/v1/admin/users?limit=1',gen);
   if(gen!==generation())return null;
   if(!page||!Array.isArray(page.items)||typeof page.total!=='number'||!Number.isSafeInteger(page.total)||page.total<0||page.total<page.items.length||page.scope!=='FILTERED_PRODUCT_USERS')return null;
   return page.total;
  }catch{return null;}
 };
 return {...views,reset(){for(const view of Object.values(views))view.reset();view_search_reset();},invalidate(){for(const view of Object.values(views))view.invalidate();}};
 function view_search_reset(){views.users.search='';views.users.status='ALL';el('admin-user-filter').value='';el('users-search').value='';el('users-status').value='ALL';}
}};
