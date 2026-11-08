import { api } from '../core/api.js';
import { htmlVersionPath, htmlDownloadURL } from './html-preview.js?v=2026110805';

const params = new URLSearchParams(location.search);
const planId = Number(params.get('planId')), studentId = Number(params.get('studentId')), classId = Number(params.get('classId'));
const versionId = params.get('versionId') || '', teacher = params.get('teacher') === '1';
try {
  if (![planId,studentId].every((id)=>Number.isInteger(id)&&id>0) || (!teacher && (!Number.isInteger(classId)||classId<=0)) || !/^[a-f0-9]{32}$/i.test(versionId)) throw new Error('预览链接无效');
  const data = await api(htmlVersionPath(planId, studentId, classId, versionId, teacher));
  if (data.version.id !== versionId) throw new Error('网页版本不匹配');
  document.title = `第${data.version.version}版 · 网页作品`;
  document.querySelector('#previewTitle').textContent = `网页作品 · 第${data.version.version}版`;
  const frame = document.querySelector('#previewFrame'); frame.srcdoc = data.html; frame.classList.remove('hidden');
  const download = document.querySelector('#previewDownload'); download.href = htmlDownloadURL(planId, studentId, classId, versionId, teacher); download.classList.remove('hidden');
  document.querySelector('#previewMessage').classList.add('hidden');
} catch (error) {
  const message = document.querySelector('#previewMessage'); message.textContent = `打开失败：${error.message}`; message.className = 'error-message';
}
