'use strict';
// Admin-only paged view module. Authentication and HTTP are injected ports;
// consumer controllers and owner-only task handling are independent.
const AdminLists={create({el,read,generation,principal,onError,dateText,cell,badge,ownerSelected,userSelected}){
 const uuid=/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
 const views={};
 for(const kind of ['users','downloads']){
  const prefix=kind==='users'?'users':'admin-downloads';
  const view={cursor:'',next:'',request:0,key:'',loaded:false,owner:'',observed:''};views[kind]=view;
  const rows=()=>el(prefix+'-rows'),footer=()=>el(prefix+'-footer');
  view.clear=()=>{view.loaded=false;view.key='';view.next='';view.observed='';rows().replaceChildren();el(prefix+'-empty').hidden=true;el('more-'+prefix).disabled=true;footer().textContent='等待查询';};
  view.reset=()=>{view.request++;view.cursor='';view.owner='';view.clear();el('refresh-'+prefix).disabled=false;};
  view.invalidate=()=>{view.request++;el('refresh-'+prefix).disabled=false;el('more-'+prefix).disabled=!view.next;};
  view.load=async()=>{
   if(!principal())return;
   const gen=generation(),request=++view.request,key=[view.cursor,view.owner].join('|'),previous=view.loaded&&view.key===key;
   if(!previous)view.clear();view.key=key;el(prefix+'-empty').hidden=true;
   el('refresh-'+prefix).disabled=true;el('more-'+prefix).disabled=true;footer().textContent=previous?'正在刷新… · 旧页暂留':'LOADING · 正在查询…';
   const query=new URLSearchParams({limit:'20'});if(view.cursor)query.set('cursor',view.cursor);if(kind==='downloads'&&view.owner)query.set('user_id',view.owner);
   try{
    const page=await read('/api/v1/admin/'+kind+'?'+query,gen);
    if(gen!==generation()||request!==view.request)return;
    if(!page||!Array.isArray(page.items)||page.items.length>20||(page.next_cursor&&!uuid.test(page.next_cursor)))throw Error('Invalid page');
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
    footer().textContent='本页 '+page.items.length+' 项 · GET /api/v1/admin/'+kind+' · 客户端读取于 '+view.observed+'（非数据库观察时间）'+(kind==='downloads'?' · '+(view.owner?'用户 '+view.owner:'配置 Binding 内全部已归属任务'):' · 全站产品用户')+' · 不代表全站总数';
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
 return {...views,reset(){for(const view of Object.values(views))view.reset();el('admin-user-filter').value='';},invalidate(){for(const view of Object.values(views))view.invalidate();}};
}};
