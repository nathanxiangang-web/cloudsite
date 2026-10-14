'use strict';
// Pure presentation hints from Q4/Q5 metadata. No content/access truth inferred.
const ResourceDisplay = (() => {
 const formats = {
  software: ['exe','msi','msix','msixbundle','appx','appxbundle','apk','aab','dmg','pkg','deb','rpm','appimage'],
  video: ['mp4','mkv','webm','mov','avi','m4v','mpeg','mpg','wmv','ts','m2ts','flv','3gp','ogv'],
  document: ['pdf','doc','docx','odt','rtf','md','markdown','txt','ppt','pptx','odp','xls','xlsx','ods','csv','epub'],
  image: ['jpg','jpeg','png','gif','webp','avif','bmp','tif','tiff','svg','heic'],
  archive: ['zip','7z','rar','tar','gz','bz2','xz','zst','iso'],
  audio: ['mp3','m4a','aac','flac','wav','ogg','opus','wma']
 };
 const kinds = {
  software:{label:'软件安装包',glyph:'APP',action:'下载安装包'},
  video:{label:'视频',glyph:'VID',action:'播放视频'},
  document:{label:'文档',glyph:'DOC',action:'预览文档'},
  image:{label:'图片',glyph:'IMG',action:'查看图片'},
  archive:{label:'归档文件',glyph:'ZIP',action:'下载归档'},
  audio:{label:'音频',glyph:'AUD',action:'播放音频'},
  other:{label:'其他文件',glyph:'FILE',action:'打开文件'},
  folder:{label:'目录',glyph:'DIR',action:'打开目录'},
  unknown:{label:'类型未提供',glyph:'?',action:'查看信息'}
 };
 function name(item){return item.name || item.path || item.resource_id || '名称未提供';}
 function classify(item){
  if(item.directory === true)return {key:'folder',extension:'',...kinds.folder};
  if(item.directory !== false)return {key:'unknown',extension:'',...kinds.unknown};
  const filename=(item.name || item.path || '').split('/').pop();
  const dot=filename.lastIndexOf('.');
  const extension=dot>0 && dot<filename.length-1 ? filename.slice(dot+1).toLowerCase() : '';
  const key=Object.keys(formats).find(kind=>formats[kind].includes(extension)) || 'other';
  return {key,extension,...kinds[key]};
 }
 function category(item){const key=classify(item).key;return ['software','video','document'].includes(key)?key:'other';}
 function counts(items){
  const result={all:items.length,software:0,video:0,document:0,other:0};
  for(const item of items)result[category(item)]++;
  return result;
 }
 function filter(items,kind,query){
  const needle=query.trim().toLocaleLowerCase('zh-CN');
  return items.filter(item=>(kind==='all'||category(item)===kind)&&(!needle||(name(item)+'\n'+(item.path||'')).toLocaleLowerCase('zh-CN').includes(needle)));
 }
 function size(value){
  if(value==null || !Number.isFinite(value) || value<0)return '未提供';
  if(value<1024)return value+' B';
  const units=['KB','MB','GB','TB'];let n=value/1024;let unit=0;
  while(n>=1024&&unit<units.length-1){n/=1024;unit++;}
  return n.toFixed(1)+' '+units[unit];
 }
 function accessNote(item){
  const kind=classify(item).key;
  const copy=item.copy_id?'Copy 的可用性不等于文件访问授权。':'当前产品库未投影 Copy；尚不能发放文件访问。';
  const note={software:'安装包只按后缀识别，不推断版本、厂商或安全性，不自动执行。',video:'视频后缀不代表可播放；播放器、字节范围读取和编码验证尚未接入。',document:'文档正文尚未读取；安全正文预览与下载入口尚未接入。',image:'图片内容尚未读取，不使用虚构缩略图。',archive:'归档内容未检查，压缩包不会被自动当作软件。',audio:'音频内容尚未读取，播放与编码验证尚未接入。'}[kind] || '当前只展示规范元数据，文件内容访问尚未接入。';
  return copy+' '+note;
 }
 return Object.freeze({name,classify,category,counts,filter,size,accessNote});
})();
