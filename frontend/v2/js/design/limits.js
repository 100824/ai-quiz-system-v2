// Keep these limits aligned with backend-go/internal/v2/design_limits.go.
export const designLimits = Object.freeze({title:120,question:1000,questionCount:20,questionsTotal:4000,answer:4000,answersTotal:12000,message:4000});
const invalidText = /[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f-\u009f\u200b\ufeff\ufffd\u202a-\u202e\u2066-\u2069\ud800-\udfff]/u;
export const characterCount = text => Array.from(text).length;
export function designTextError(value,label,limit,required=true) {
  if (value.length>limit*2 || characterCount(value)>limit) return `${label}最多${limit}字`;
  if (invalidText.test(value)) return `${label}包含无效或不可见控制字符，请删除后重试`;
  if (required && !value.replace(/[\p{White_Space}\p{Cf}\p{Mn}\p{Me}]/gu,'')) return `${label}不能为空`;
  return '';
}
export function designAnswersError(answers) {
  if (!answers.length || answers.length>designLimits.questionCount) return `问题数量无效，最多${designLimits.questionCount}道`;
  for (let i=0;i<answers.length;i++) { const error=designTextError(answers[i],`第${i+1}题`,designLimits.answer); if(error)return error; }
  return answers.reduce((n,a)=>n+characterCount(a.trim()),0)>designLimits.answersTotal ? `全部内容合计最多${designLimits.answersTotal}字，请精简后重试` : '';
}
let counterId=0;
export function attachDesignCounters(root,selector,limit) {
  root.querySelectorAll(selector).forEach(input=>{
    // Native maxlength counts UTF-16 units; allow enough for 4000 emoji while
    // validating code points explicitly on input and before every submission.
    input.maxLength=limit*2;
    const counter=document.createElement('span');counter.className='design-character-count';counter.id=`design-character-count-${++counterId}`;
    input.setAttribute('aria-describedby',[input.getAttribute('aria-describedby'),counter.id].filter(Boolean).join(' '));
    input.after(counter);
    const update=()=>{const count=characterCount(input.value);counter.textContent=`${count} / ${limit} 字`;counter.classList.toggle('is-over-limit',count>limit);input.setAttribute('aria-invalid',String(count>limit));};
    input.addEventListener('input',update);update();
  });
}
