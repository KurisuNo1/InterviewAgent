/**
 * InterviewAgent — AI 模拟面试官
 * 单页应用 (Vanilla JS)
 */
(function() {
  'use strict';

  const API = '/api';
  let sessionID = null;
  let interviewActive = false;

  // Generate a stable anonymous user ID for history tracking
  function getUserID() {
    let uid = sessionStorage.getItem('interview_user_id');
    if (!uid) {
      uid = 'anon-' + Math.random().toString(36).substring(2, 10);
      sessionStorage.setItem('interview_user_id', uid);
    }
    return uid;
  }

  /* ═══════════════ API 请求封装 ═══════════════ */
  async function api(method, path, body) {
    const opts = {
      method,
      headers: { 'Content-Type': 'application/json' },
    };
    if (body) opts.body = JSON.stringify(body);
    const res = await fetch(API + path, opts);
    const json = await res.json();
    if (!res.ok && json.code !== 200) throw new Error(json.message || '请求失败');
    return json;
  }

  const get  = (p) => api('GET', p);
  const post = (p, b) => api('POST', p, b);

  /* ═══════════════ 提示消息 ═══════════════ */
  function toast(msg, type) {
    const el = document.getElementById('toast');
    el.textContent = msg;
    el.className = 'toast ' + (type || '');
    el.style.display = 'block';
    clearTimeout(el._t);
    el._t = setTimeout(() => { el.style.display = 'none'; }, 4000);
  }

  /* ═══════════════ 屏幕切换 ═══════════════ */
  function showScreen(name) {
    document.querySelectorAll('.screen').forEach(s => s.classList.remove('active'));
    const el = document.getElementById('screen-' + name);
    if (el) el.classList.add('active');
  }

  /* ═══════════════ 步骤指示器 ═══════════════ */
  function setStep(n) {
    document.querySelectorAll('.step').forEach((el, i) => {
      el.classList.remove('active', 'done');
      if (i < n - 1) el.classList.add('done');
      if (i === n - 1) el.classList.add('active');
    });
  }

  /* ═══════════════ 状态标签 ═══════════════ */
  function setStatus(text, cls) {
    const el = document.getElementById('status-badge');
    el.textContent = text;
    el.className = 'status-badge ' + cls;
  }

  function setSessionID(id) {
    sessionID = id;
    const el = document.getElementById('session-id-display');
    el.textContent = id ? '会话: ' + id.substring(0,8) + '...' : '';
    el.style.display = id ? '' : 'none';
  }

  /* ═══════════════ 屏幕 1: 创建会话 ═══════════════ */
  document.getElementById('btn-create-session').addEventListener('click', async () => {
    const jdText = document.getElementById('input-jd').value.trim();
    if (!jdText) { toast('请先粘贴岗位描述（JD）内容。', 'error'); return; }

    showLoading('btn-create-session', true);
    try {
      const res = await post('/sessions', { user_id: getUserID(), jd_text: jdText });
      sessionID = res.data.id;
      setSessionID(sessionID);
      setStatus('会话已创建', 'active');
      setStep(2);

      toast('会话创建成功！正在解析岗位描述...', 'success');
      await parseJD();
    } catch(e) {
      toast(e.message, 'error');
    } finally {
      showLoading('btn-create-session', false);
    }
  });

  async function parseJD() {
    const jdText = document.getElementById('input-jd').value.trim();
    showLoading('btn-parse-jd', true);
    try {
      const res = await post('/sessions/' + sessionID + '/jd', { jd_text: jdText });
      renderJDAnalysis(res.data);
      setStep(2);
      showScreen('jd-result');
      setStatus('JD 已解析', 'active');
    } catch(e) {
      toast('JD 解析失败：' + e.message, 'error');
    } finally {
      showLoading('btn-parse-jd', false);
    }
  }

  function renderJDAnalysis(data) {
    const el = document.getElementById('jd-analysis-content');
    if (!data) { el.innerHTML = '<p style="color:var(--text-muted)">暂无解析结果。</p>'; return; }
    let tags = (data.tech_stack || []).map(t => '<span class="tag">' + escapeHTML(t) + '</span>').join('');
    let coreSkills = (data.core_skills || []).map(s => '<span class="tag">' + escapeHTML(s) + '</span>').join('');
    let bonus = (data.bonus_skills || []).map(s => '<span class="tag">' + escapeHTML(s) + '</span>').join('');
    el.innerHTML = `
      <div class="analysis-grid">
        <div class="analysis-row"><span class="label">目标职位</span><span class="value">${escapeHTML(data.position||'未知')}</span></div>
        <div class="analysis-row"><span class="label">职级要求</span><span class="value">${escapeHTML(data.level||'未知')}</span></div>
        <div class="analysis-row"><span class="label">经验要求</span><span class="value">${escapeHTML(String(data.experience_years||'未知'))} 年</span></div>
        <div class="analysis-row"><span class="label">学历要求</span><span class="value">${escapeHTML(data.degree||'未知')}</span></div>
        <div class="analysis-row"><span class="label">技术栈</span><span class="value">${tags||'未知'}</span></div>
        <div class="analysis-row"><span class="label">核心技能</span><span class="value">${coreSkills||'未知'}</span></div>
        <div class="analysis-row"><span class="label">加分项</span><span class="value">${bonus||'无'}</span></div>
      </div>`;
  }

  document.getElementById('btn-back-to-jd').addEventListener('click', () => showScreen('setup'));

  /* ═══════════════ 屏幕 2: 简历上传 ═══════════════ */
  document.getElementById('btn-go-resume').addEventListener('click', () => {
    showScreen('resume-upload');
    setStep(3);
  });

  document.getElementById('btn-upload-resume').addEventListener('click', async () => {
    const fileInput = document.getElementById('input-resume-file');
    const textInput = document.getElementById('input-resume-text');
    let content = null;
    let fileName = 'resume.txt';

    if (fileInput.files.length > 0) {
      const file = fileInput.files[0];
      fileName = file.name;
      // 直接读取文件为 base64，支持二进制文件（PDF/DOCX）
      content = await readFileAsBase64(file);
    } else if (textInput.value.trim()) {
      content = textToBase64(textInput.value.trim());
    }

    if (!content) { toast('请上传简历文件或粘贴简历文本。', 'error'); return; }

    showLoading('btn-upload-resume', true);
    try {
      const res = await post('/sessions/' + sessionID + '/resume', {
        file_name: fileName,
        content: content
      });
      renderResumeMatch(res.data);
      setStep(3);
      showScreen('resume-result');
      setStatus('简历已匹配', 'active');
    } catch(e) {
      toast('简历上传失败：' + e.message, 'error');
    } finally {
      showLoading('btn-upload-resume', false);
    }
  });

  // 读取文件为 base64 编码字符串（支持 PDF、DOCX 等二进制格式）
  function readFileAsBase64(file) {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => {
        // data:application/pdf;base64,xxxx → 提取 pure base64
        const dataURL = reader.result;
        const base64Idx = dataURL.indexOf(';base64,');
        if (base64Idx !== -1) {
          resolve(dataURL.substring(base64Idx + 8));
        } else {
          resolve(dataURL);
        }
      };
      reader.onerror = () => reject(new Error('文件读取失败'));
      reader.readAsDataURL(file);
    });
  }

  function renderResumeMatch(data) {
    const el = document.getElementById('resume-match-content');
    if (!data) { el.innerHTML = '<p style="color:var(--text-muted)">暂无匹配数据。</p>'; return; }

    let strengths = (data.strengths || []).map(s => '<span class="tag strength">' + escapeHTML(s) + '</span>').join('');
    let gaps = (data.gaps || []).map(g => '<span class="tag gap">' + escapeHTML(g) + '</span>').join('');

    let dimScores = '';
    if (data.dimension_scores) {
      for (const [k, v] of Object.entries(data.dimension_scores)) {
        dimScores += '<div class="analysis-row"><span class="label">' + escapeHTML(k) + '</span><span class="value">' + (Number(v) * 100).toFixed(0) + '/100</span></div>';
      }
    }

    const overallPct = (Number(data.overall_score || 0) * 100);
    const scoreCls = overallPct >= 70 ? 'high' : (overallPct >= 40 ? 'mid' : 'low');

    el.innerHTML = `
      <div class="analysis-grid">
        <div class="analysis-row"><span class="label">综合匹配度</span><span class="value"><span class="score-badge ${scoreCls}">${overallPct.toFixed(0)}%</span></span></div>
        ${dimScores}
        <div class="analysis-row"><span class="label">优势</span><span class="value">${strengths||'无'}</span></div>
        <div class="analysis-row"><span class="label">不足</span><span class="value">${gaps||'无'}</span></div>
        <div class="analysis-row"><span class="label">综合评价</span><span class="value">${escapeHTML(data.resume_summary||'')}</span></div>
      </div>`;
  }

  document.getElementById('btn-back-to-jd2').addEventListener('click', () => showScreen('jd-result'));
  document.getElementById('btn-start-interview').addEventListener('click', startInterview);

  /* ═══════════════ 屏幕 3: 面试房间 ═══════════════ */
  async function startInterview() {
    showScreen('interview-room');
    setStep(4);
    setStatus('面试中', 'active');
    interviewActive = true;

    const chatBox = document.getElementById('chat-box');
    chatBox.innerHTML = '';

    // 获取出题计划
    try {
      const planRes = await get('/sessions/' + sessionID + '/plan');
      const plan = planRes.data;
      const totalQ = plan.total_questions || plan.questions?.length || 0;
      addChatMessage('system', '面试准备就绪，共 ' + totalQ + ' 道题目。现在开始吧！');
      document.getElementById('progress-fill').style.width = '0%';
      document.getElementById('progress-text').textContent = '0 / ' + totalQ;
    } catch(e) {
      addChatMessage('system', '开始面试（未获取到出题计划预览）。');
    }

    // 开始面试
    try {
      const res = await post('/sessions/' + sessionID + '/start');
      const event = res.data;
      addChatMessage('assistant', String(event.data || event));
    } catch(e) {
      toast('开始面试失败：' + e.message, 'error');
    }
  }

  // 输入框快捷键：Enter 发送，Shift+Enter 换行
  document.getElementById('input-answer').addEventListener('keydown', function(e) {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      submitAnswer();
    }
  });

  document.getElementById('btn-send-answer').addEventListener('click', submitAnswer);
  document.getElementById('btn-skip-question').addEventListener('click', skipQuestion);
  document.getElementById('btn-end-interview').addEventListener('click', () => {
    if (confirm('确定要结束面试并查看报告吗？')) {
      interviewActive = false;
      viewReport();
    }
  });

  async function submitAnswer() {
    const input = document.getElementById('input-answer');
    const answer = input.value.trim();
    if (!answer) return;

    addChatMessage('user', answer);
    input.value = '';
    showTyping(true);

    try {
      const res = await post('/sessions/' + sessionID + '/answer', { answer: answer });
      const event = res.data;
      showTyping(false);

      if (event.type === 'complete') {
        addChatMessage('system', '🎉 面试已结束！');
        addChatMessage('assistant', String(event.data || ''));
        interviewActive = false;
        setStatus('已完成', 'complete');
        document.getElementById('btn-view-report').style.display = '';
      } else if (event.type === 'follow_up') {
        addChatMessage('assistant', '🔍 追问：' + String(event.data || ''));
      } else {
        addChatMessage('assistant', String(event.data || ''));
      }
    } catch(e) {
      showTyping(false);
      toast('提交失败：' + e.message, 'error');
    }
  }

  async function skipQuestion() {
    if (!confirm('确定要跳过这道题吗？')) return;
    addChatMessage('system', '⏭ 已跳过此题。');

    try {
      const res = await post('/sessions/' + sessionID + '/skip');
      const event = res.data;
      addChatMessage('assistant', String(event.data || ''));
    } catch(e) {
      toast('跳过失败：' + e.message, 'error');
    }
  }

  /* ═══════════════ 屏幕 4: 面试报告 ═══════════════ */
  async function viewReport() {
    showScreen('report');
    setStep(5);
    setStatus('报告已生成', 'complete');

    const el = document.getElementById('report-content');
    el.innerHTML = '<div class="loading-overlay"><span class="spinner"></span> 正在生成面试报告...</div>';

    try {
      const res = await get('/sessions/' + sessionID + '/report');
      renderReport(res.data);
    } catch(e) {
      el.innerHTML = '<p style="color:var(--danger)">加载报告失败：' + escapeHTML(e.message) + '</p>';
    }
  }

  function renderReport(data) {
    if (!data) { return; }
    const el = document.getElementById('report-content');

    const score = Number(data.overall_score || 0);
    const scoreCls = score >= 7 ? 'high' : (score >= 5 ? 'mid' : 'low');

    let dimHTML = '';
    if (data.dimension_score) {
      const dimNames = {
        'technical_accuracy': '技术准确性',
        'answer_depth': '回答深度',
        'communication': '沟通表达',
        'project_experience': '项目经验',
      };
      for (const [name, val] of Object.entries(data.dimension_score)) {
        const v = Number(val);
        const cls = v >= 7 ? 'high' : (v >= 5 ? 'mid' : 'low');
        const displayName = dimNames[name] || name;
        dimHTML += '<div class="score-item"><div class="dim-name">' + displayName + '</div><div class="dim-value ' + cls + '">' + v.toFixed(1) + '</div></div>';
      }
    }

    let hlHTML = (data.highlights || []).map(h => '<li>' + escapeHTML(h) + '</li>').join('');
    let waHTML = (data.weak_areas || []).map(w => '<span class="tag gap">' + escapeHTML(w) + '</span>').join('');

    let evalHTML = '';
    if (data.evaluations) {
      evalHTML = '<div class="form-group"><label>逐题评估详情</label>';
      evalHTML += data.evaluations.map((ev, i) => `
        <div class="plan-item">
          <div class="plan-topic">第 ${i+1} 题：${escapeHTML(ev.question_id||'')}</div>
          <div class="plan-meta">得分：${Number(ev.total_score||0).toFixed(1)} 分 | ${escapeHTML(ev.feedback||'无评语')}</div>
        </div>
      `).join('');
      evalHTML += '</div>';
    }

    el.innerHTML = `
      <div style="text-align:center;margin-bottom:24px;">
        <div style="font-size:0.9rem;color:var(--text-muted);margin-bottom:8px;">综合评分</div>
        <span class="score-badge ${scoreCls}" style="font-size:2rem;">${score.toFixed(1)} / 10</span>
      </div>
      <div class="score-card">${dimHTML}</div>
      ${data.weak_areas&&data.weak_areas.length ? '<div class="form-group"><label>需要提升的领域</label><div>'+waHTML+'</div></div>' : ''}
      ${data.highlights&&data.highlights.length ? '<div class="form-group"><label>亮点</label><ul style="padding-left:20px;color:var(--text-secondary)">'+hlHTML+'</ul></div>' : ''}
      ${data.summary ? '<div class="form-group"><label>综合评价</label><p style="color:var(--text-secondary)">'+escapeHTML(data.summary)+'</p></div>' : ''}
      ${evalHTML}
    `;
  }

  document.getElementById('btn-view-plan').addEventListener('click', viewReviewPlan);
  document.getElementById('btn-new-session').addEventListener('click', resetApp);

  /* ═══════════════ 屏幕 5: 复习计划 ═══════════════ */
  async function viewReviewPlan() {
    showScreen('review-plan');
    const el = document.getElementById('review-plan-content');
    el.innerHTML = '<div class="loading-overlay"><span class="spinner"></span> 正在加载复习计划...</div>';

    try {
      const res = await get('/sessions/' + sessionID + '/review-plan');
      renderReviewPlan(res.data);
    } catch(e) {
      el.innerHTML = '<p style="color:var(--danger)">加载复习计划失败：' + escapeHTML(e.message) + '</p>';
    }
  }

  function renderReviewPlan(data) {
    if (!data) return;
    const el = document.getElementById('review-plan-content');

    const priorityNames = { high: '高优先级', medium: '中优先级', low: '低优先级' };

    let itemsHTML = (data.plan_items || []).map(item => {
      const p = (item.priority || 'medium').toLowerCase();
      const pName = priorityNames[p] || p;
      return `
      <div class="plan-item priority-${p}">
        <div class="plan-topic">${escapeHTML(item.topic||'')} <span style="font-size:0.75rem;color:var(--text-muted)">[${pName}]</span></div>
        <div class="plan-meta">预计 ${item.estimated_hours||1} 小时 — ${escapeHTML(item.description||'')}</div>
      </div>`;
    }).join('');

    let resHTML = (data.resources || []).map(r => `
      <a class="resource-link" href="${escapeHTML(r.url||'#')}" target="_blank" rel="noopener">📎 ${escapeHTML(r.title||'资源链接')}（来源：${escapeHTML(r.source||r.type||'未知')}）</a>
    `).join('');

    el.innerHTML = `
      ${data.weak_areas&&data.weak_areas.length ? '<div class="form-group"><label>重点复习方向</label><div>' + data.weak_areas.map(w => '<span class="tag gap">'+escapeHTML(w)+'</span>').join('') + '</div></div>' : ''}
      ${itemsHTML ? '<div class="form-group"><label>学习计划</label>'+itemsHTML+'</div>' : '<p style="color:var(--text-muted)">暂无学习计划。</p>'}
      ${resHTML ? '<div class="form-group"><label>推荐学习资源</label><div>'+resHTML+'</div></div>' : ''}
    `;
  }

  document.getElementById('btn-back-to-report').addEventListener('click', () => { showScreen('report'); });
  document.getElementById('btn-new-session2').addEventListener('click', resetApp);

  /* ═══════════════ 重置应用 ═══════════════ */
  function resetApp() {
    sessionID = null;
    interviewActive = false;
    setSessionID(null);
    setStatus('待开始', 'idle');
    setStep(1);
    document.getElementById('input-jd').value = '';
    document.getElementById('input-resume-text').value = '';
    document.getElementById('input-resume-file').value = '';
    document.getElementById('input-answer').value = '';
    document.getElementById('chat-box').innerHTML = '';
    document.getElementById('btn-view-report').style.display = 'none';
    document.getElementById('progress-fill').style.width = '0%';
    document.getElementById('progress-text').textContent = '';
    showScreen('setup');
  }

  /* ═══════════════ 聊天界面辅助函数 ═══════════════ */
  function addChatMessage(role, content) {
    const box = document.getElementById('chat-box');
    const div = document.createElement('div');
    const roleLabels = { assistant: '面试官', user: '候选人', system: '系统' };
    div.className = 'chat-message ' + role;
    div.innerHTML = '<div class="role-label">' + (roleLabels[role] || role) + '</div><div>' + formatContent(String(content)) + '</div>';
    box.appendChild(div);
    box.scrollTop = box.scrollHeight;
  }

  function showTyping(show) {
    const box = document.getElementById('chat-box');
    let el = document.getElementById('typing-indicator');
    if (show) {
      if (!el) {
        el = document.createElement('div');
        el.id = 'typing-indicator';
        el.className = 'typing-indicator';
        el.innerHTML = '<span></span><span></span><span></span>';
        box.appendChild(el);
      }
      el.style.display = '';
      box.scrollTop = box.scrollHeight;
    } else if (el) {
      el.style.display = 'none';
    }
  }

  function formatContent(text) {
    return escapeHTML(text)
      .replace(/NEXT_QUESTION/g, '<span style="color:var(--success)">→ 下一题</span>')
      .replace(/INTERVIEW_COMPLETE/g, '<span style="color:var(--warning)">✓ 面试结束</span>');
  }

  function showLoading(btnId, show) {
    const btn = document.getElementById(btnId);
    if (!btn) return;
    if (show) {
      btn._origText = btn.textContent;
      btn.disabled = true;
      btn.innerHTML = '<span class="spinner"></span> 处理中...';
    } else {
      btn.disabled = false;
      btn.textContent = btn._origText || btn.textContent;
    }
  }

  function escapeHTML(str) {
    const div = document.createElement('div');
    div.appendChild(document.createTextNode(str));
    return div.innerHTML;
  }

  // 分块将文本转为 base64，避免大文件时调用栈溢出
  function textToBase64(text) {
    const encoder = new TextEncoder();
    const bytes = encoder.encode(text);
    const chunkSize = 0x8000; // 32KB chunks
    let binary = '';
    for (let i = 0; i < bytes.length; i += chunkSize) {
      const chunk = bytes.subarray(i, Math.min(i + chunkSize, bytes.length));
      binary += String.fromCharCode.apply(null, chunk);
    }
    return btoa(binary);
  }

  document.getElementById('btn-view-report').addEventListener('click', viewReport);

  /* ═══════════════ 初始化 ═══════════════ */
  showScreen('setup');
  setStatus('待开始', 'idle');
})();
