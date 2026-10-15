'use strict';
const guestNode=id=>document.getElementById(id);const guestToken=location.hash.slice(1);
if(!/^[A-Za-z0-9_-]{43}$/.test(guestToken)){guestNode('redeem-submit').disabled=true;guestNode('share-message').textContent='分享链接不完整，请向分享者索取新链接。';}
guestNode('redeem-form').addEventListener('submit',async event=>{
 event.preventDefault();if(guestNode('redeem-submit').disabled)return;guestNode('redeem-submit').disabled=true;guestNode('shared-file').hidden=true;guestNode('shared-download').removeAttribute('href');guestNode('share-message').textContent='正在验证分享…';
 try{const response=await fetch('/api/v1/shares/redeem',{method:'POST',credentials:'same-origin',cache:'no-store',headers:{'Content-Type':'application/json'},body:JSON.stringify({token:guestToken,code:guestNode('redeem-code').value})});const result=await response.json();
  if(!response.ok){const error=new Error('Share unavailable');error.code=result.code;throw error;}
  if(!/^\/api\/v1\/shared\/[0-9a-f-]{36}\/content$/.test(result.download_url))throw new Error('Invalid target');
  guestNode('shared-name').textContent=result.name;guestNode('shared-info').textContent=result.size_bytes.toLocaleString()+' 字节 · 有效至 '+new Date(result.expires_at).toLocaleString();guestNode('shared-download').href=result.download_url;guestNode('shared-download').target='_blank';guestNode('shared-download').rel='noopener';guestNode('shared-file').hidden=false;guestNode('share-message').textContent='验证成功。点击下载原文件；不会修改网盘内容。';
 }catch(error){guestNode('share-message').textContent=error.code==='SHARE_BUSY'?'访问频繁，请一分钟后再试。':'链接或提取码无效、已过期或被撤销。';}
 finally{guestNode('redeem-code').value='';guestNode('redeem-submit').disabled=false;}
});
guestNode('shared-download').addEventListener('click',event=>{
 if(guestNode('shared-file').hidden||!guestNode('shared-download').href){event.preventDefault();return;}
 guestNode('share-message').textContent='已请求下载，请在浏览器下载列表查看进度；若出现错误页，分享可能已失效或连接不可用。';
});
