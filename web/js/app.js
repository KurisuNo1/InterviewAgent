(function(){
  const API='/api'; var token=localStorage.getItem('ia_token');
  var currentMode='chat',
    chatSessionID=localStorage.getItem('ia_chat_sid')||null,
    interviewSessionID=localStorage.getItem('ia_interview_sid')||null,
    skillSessionID=localStorage.getItem('ia_skill_sid')||null;

  /* ===== API ===== */
  async function api(m,p,b,timeout){
    var h={'Content-Type':'application/json'}; if(token) h['Authorization']='Bearer '+token;
    var opts={method:m,headers:h};
    if(b) opts.body=JSON.stringify(b);
    if(timeout){
      var ctrl=new AbortController();opts.signal=ctrl.signal;
      setTimeout(function(){ctrl.abort()},timeout);
    }
    var r=await fetch(API+p,opts);
    return r.json();
  }

  /* ===== DOM ===== */
  var $=function(id){return document.getElementById(id)};
  var sidebar=$('sidebar'), chatBox=$('chat-box'), inputMsg=$('input-msg'), btnSend=$('btn-send');

  /* ===== Mode Switching ===== */
  document.querySelectorAll('.nav-item').forEach(function(el){
    el.addEventListener('click',function(){
      var mode=this.getAttribute('data-mode');
      switchMode(mode);
    });
  });

  function switchMode(mode){
    currentMode=mode;
    document.querySelectorAll('.nav-item').forEach(function(e){e.classList.remove('active')});
    document.querySelector('[data-mode="'+mode+'"]').classList.add('active');
    document.querySelectorAll('.mode-content').forEach(function(e){e.classList.remove('active')});
    $('mode-'+mode).classList.add('active');
    if(mode==='chat'){
      initChat();
    }
    if(mode==='interview'){resumeOrStartInterview();loadInterviewHistory();$('interview-sidebar').style.display='flex';$('current-result-panel').style.display='none'}
    if(mode==='skills') loadSkills();
    if(mode==='knowledge') loadDocs();
  }

  /* ===== Sidebar ===== */
  $('btn-toggle-sidebar').addEventListener('click',function(){sidebar.classList.toggle('collapsed')});
  $('btn-toggle-sidebar-2').addEventListener('click',function(){sidebar.classList.toggle('collapsed')});
  $('btn-toggle-sidebar-3').addEventListener('click',function(){sidebar.classList.toggle('collapsed')});
  $('btn-toggle-sidebar-4').addEventListener('click',function(){sidebar.classList.toggle('collapsed')});
  $('btn-new-chat').addEventListener('click',function(){chatSessionID=null;localStorage.removeItem('ia_chat_sid');chatBox.innerHTML='<div class="welcome-msg"><h2>InterviewAgent</h2><p>我是你的AI面试助手。可以闲聊、模拟面试、练习技能。试试问我问题吧！</p></div>';switchMode('chat');});
  $('btn-clear-chat').addEventListener('click',function(){chatBox.innerHTML='';chatSessionID=null;localStorage.removeItem('ia_chat_sid');chatBox.innerHTML='<div class="welcome-msg"><h2>InterviewAgent</h2><p>我是你的AI面试助手。可以闲聊、模拟面试、练习技能。试试问我问题吧！</p></div>';initChat();});

  /* ===== Auth ===== */
  var authToken=null;
  function getUserId(){
    try{
      if(token){var p=JSON.parse(atob(token.split('.')[1]));return p.username||p.user_id||'anonymous';}
    }catch(e){}
    return 'anonymous';
  }
  function updateUserUI(){
    if(token){
      var payload=JSON.parse(atob(token.split('.')[1]));
      $('username-display').textContent=payload.username||'用户';
      $('btn-login').textContent='退出';
    }
  }
  $('btn-login').addEventListener('click',function(){
    if(token){token=null;localStorage.removeItem('ia_token');$('username-display').textContent='未登录';$('btn-login').textContent='登录';
      // Reset sessions on logout so new user gets fresh sessions
      chatSessionID=null;interviewSessionID=null;skillSessionID=null;
      localStorage.removeItem('ia_chat_sid');localStorage.removeItem('ia_interview_sid');localStorage.removeItem('ia_skill_sid');
      switchMode('chat');initChat();
      return;}
    $('login-modal').style.display='flex';
  });
  $('btn-login-cancel').addEventListener('click',function(){$('login-modal').style.display='none'});
  $('btn-login-submit').addEventListener('click',async function(){
    var u=$('login-username').value,p=$('login-password').value;
    var btn=this;btn.disabled=true;btn.textContent='登录中...';
    var r=await api('POST','/auth/login',{username:u,password:p});
    btn.disabled=false;btn.textContent='登录';
    if(r.code===200){token=r.data.token;localStorage.setItem('ia_token',token);$('login-modal').style.display='none';updateUserUI();toast('登录成功','success');
      chatSessionID=null;interviewSessionID=null;skillSessionID=null;
      localStorage.removeItem('ia_chat_sid');localStorage.removeItem('ia_interview_sid');localStorage.removeItem('ia_skill_sid');
      initChat();}
    else{$('login-msg').textContent=r.message||'登录失败'}
  });
  $('btn-register-submit').addEventListener('click',async function(){
    var u=$('login-username').value,p=$('login-password').value;
    if(u.length<3||p.length<6){$('login-msg').textContent='用户名>=3位，密码>=6位';return}
    var btn=this;btn.disabled=true;btn.textContent='注册中...';
    var r=await api('POST','/auth/register',{username:u,password:p});
    btn.disabled=false;btn.textContent='注册';
    if(r.code===200){token=r.data.token;localStorage.setItem('ia_token',token);$('login-modal').style.display='none';updateUserUI();toast('注册成功','success');
      chatSessionID=null;interviewSessionID=null;skillSessionID=null;
      localStorage.removeItem('ia_chat_sid');localStorage.removeItem('ia_interview_sid');localStorage.removeItem('ia_skill_sid');
      initChat();}
    else{$('login-msg').textContent=r.message||'注册失败'}
  });
  updateUserUI();

  /* ===== Toast ===== */
  function toast(msg,type){var t=$('toast');t.textContent=msg;t.className='toast '+(type||'info');t.style.display='block';setTimeout(function(){t.style.display='none'},3000)}
  function toastErr(e){toast(e.message||e,'error')}

  /* ===== Debug Popup ===== */
  var debugPopup=null,debugPopupContent=null,debugBtn=$('btn-debug-panel'),debugOpen=false;

  function getPopup(){
    if(!debugPopup){
      debugPopup=document.createElement('div');
      debugPopup.className='debug-popup';
      debugPopupContent=document.createElement('div');
      debugPopup.appendChild(debugPopupContent);
      document.body.appendChild(debugPopup);
    }
    return debugPopup;
  }

  function positionPopup(){
    if(!debugBtn)return;
    var p=getPopup();
    var rect=debugBtn.getBoundingClientRect();
    p.style.cssText=
      'position:fixed;'+
      'bottom:'+(window.innerHeight-rect.top+8)+'px;'+
      'right:'+Math.max(8, window.innerWidth-rect.right)+'px;'+
      'width:320px;'+
      'max-height:'+Math.min(420, rect.top-16)+'px;'+
      'background:#2f2f2f;'+
      'border:1px solid #333;'+
      'border-radius:12px;'+
      'box-shadow:0 8px 32px rgba(0,0,0,.6);'+
      'overflow-y:auto;'+
      'z-index:99999;'+
      'padding:0;'+
      'display:block;';
  }

  function hidePopup(){
    if(debugPopup){debugPopup.style.display='none'}
    debugOpen=false;
    if(debugBtn)debugBtn.classList.remove('active');
  }

  if(debugBtn){
    debugBtn.addEventListener('click',function(e){
      e.preventDefault();e.stopPropagation();
      debugOpen=!debugOpen;
      if(debugOpen){
        positionPopup();
        showPopupMenu();
      }else{
        hidePopup();
      }
      debugBtn.classList.toggle('active',debugOpen);
    });
  }

  // Close popup when clicking outside
  document.addEventListener('click',function(e){
    if(debugOpen && debugPopup && !debugPopup.contains(e.target) && e.target!==debugBtn){
      hidePopup();
    }
  });

  // Reposition on scroll/resize
  window.addEventListener('scroll',function(){if(debugOpen)positionPopup()},true);
  window.addEventListener('resize',function(){if(debugOpen)positionPopup()});

  function showPopupMenu(){
    var c=debugPopupContent;
    if(!c)return;
    c.innerHTML='';
    var menu=document.createElement('div');
    menu.className='debug-popup-menu';
    var btnStats=document.createElement('button');
    btnStats.className='debug-popup-btn';
    btnStats.innerHTML='<span class="icon">&#x1F4CA;</span> 查看上下文使用量';
    btnStats.addEventListener('click',function(e){e.stopPropagation();loadContextStats()});
    var btnTools=document.createElement('button');
    btnTools.className='debug-popup-btn';
    btnTools.innerHTML='<span class="icon">&#x1F6E0;</span> 查看可用工具';
    btnTools.addEventListener('click',function(e){e.stopPropagation();loadToolsList()});
    var btnUpload=document.createElement('button');
    btnUpload.className='debug-popup-btn';
    btnUpload.innerHTML='<span class="icon">&#x1F4E4;</span> 上传题库';
    btnUpload.addEventListener('click',function(e){e.stopPropagation();showUploadPanel()});
    menu.appendChild(btnStats);
    menu.appendChild(btnTools);
    menu.appendChild(btnUpload);
    c.appendChild(menu);
  }

  function showUploadPanel(){
    var c=debugPopupContent;
    if(!c)return;
    c.innerHTML='';
    var body=document.createElement('div');
    body.className='debug-popup-body';
    body.innerHTML=
      '<div class="debug-popup-header"><span>上传题库</span></div>'+
      '<div class="popup-upload-zone" id="popup-upload-zone">'+
        '<div class="popup-upload-icon">&#x1F4C4;</div>'+
        '<p>拖拽文件到此处或点击选择</p>'+
        '<p style="font-size:.7rem;color:var(--text-muted);margin-top:4px">支持 PDF、TXT、DOCX</p>'+
        '<input type="file" id="popup-file-input" multiple hidden accept=".pdf,.txt,.docx">'+
      '</div>'+
      '<div id="popup-upload-status" style="display:none;text-align:center;padding:12px;color:var(--text-muted);font-size:.8rem"></div>'+
      '<div style="margin-top:12px;text-align:right"></div>';
    c.appendChild(body);

    // Bind events
    var zone=$('popup-upload-zone');
    var fileInput=$('popup-file-input');
    zone.addEventListener('click',function(){fileInput.click()});
    fileInput.addEventListener('change',function(){
      if(this.files.length>0) popupUploadFiles(this.files);
    });
    zone.addEventListener('dragover',function(e){e.preventDefault();zone.classList.add('popup-upload-zone-active')});
    zone.addEventListener('dragleave',function(){zone.classList.remove('popup-upload-zone-active')});
    zone.addEventListener('drop',function(e){e.preventDefault();zone.classList.remove('popup-upload-zone-active');popupUploadFiles(e.dataTransfer.files)});

    // Back button
    var footer=c.querySelector('.debug-popup-body div:last-child');
    if(footer)footer.appendChild(popupBackButton());
  }

  async function popupUploadFiles(files){
    var status=$('popup-upload-status');
    if(status){status.style.display='block';status.textContent='正在上传 '+files.length+' 个文件...'}
    try{
      var uploads=[];for(var i=0;i<files.length;i++){
        var b=await files[i].arrayBuffer();var bytes=new Uint8Array(b);var bin='';
        for(var j=0;j<bytes.length;j++) bin+=String.fromCharCode(bytes[j]);
        uploads.push({file_name:files[i].name,content:btoa(bin)});
      }
      var r=await api('POST','/documents/upload',{files:uploads});
      if(status){
        status.textContent='上传成功：'+files.length+' 个文件';
        status.style.color='var(--accent)';
      }
      // Reset file input
      var fi=$('popup-file-input');if(fi)fi.value='';
      // Refresh knowledge base if visible
      if(currentMode==='knowledge') loadDocs();
    }catch(e){
      if(status){status.textContent='上传失败：'+(e.message||e);status.style.color='var(--danger)'}
    }
  }

  function getActiveSessionID(){
    if(currentMode==='chat')return chatSessionID;
    if(currentMode==='interview')return interviewSessionID;
    if(currentMode==='skills')return skillSessionID;
    return null;
  }

  function popupBackButton(){
    var btn=document.createElement('button');
    btn.className='debug-popup-back';
    btn.innerHTML='&#x2190; 返回';
    btn.addEventListener('click',function(e){e.stopPropagation();showPopupMenu()});
    return btn;
  }

  async function loadContextStats(){
    var c=debugPopupContent;
    if(!c)return;
    c.innerHTML='<div class="debug-popup-body"><span class="debug-loading">加载中...</span></div>';
    var sid=getActiveSessionID();
    var url=sid?API+'/sessions/'+sid+'/context/stats':API+'/context/stats';
    try{
      var d=await api('GET',url.replace(API,''));
      var html='';
      if(d.code===200&&d.data){
        var s=d.data;var pct=s.avg_usage_percent||0;var max=s.max_usage_percent||0;
        var gaugeCls=max>95?'debug-usage-fill-danger':max>80?'debug-usage-fill-warn':'debug-usage-fill-safe';
        html=
          '<div class="debug-popup-body">'+
            '<div class="debug-popup-header"><span>'+(sid?'会话上下文用量':'全局上下文用量')+'</span></div>'+
            '<div class="debug-usage-bar"><div class="debug-usage-bar-fill '+gaugeCls+'" style="width:'+Math.min(max,100)+'%"></div></div>'+
            '<div style="display:flex;justify-content:space-between;font-size:.7rem;color:var(--text-muted);margin-bottom:4px">'+
              '<span>峰值 '+max.toFixed(1)+'%</span>'+
              '<span style="font-size:.65rem">'+(sid?'会话':'全局')+'</span>'+
            '</div>'+
            '<div class="debug-usage-grid">'+
              '<div class="debug-usage-item"><div class="val">'+s.total_calls+'</div><div class="lbl">总调用</div></div>'+
              '<div class="debug-usage-item"><div class="val">'+pct.toFixed(1)+'%</div><div class="lbl">平均用量</div></div>'+
              '<div class="debug-usage-item"><div class="val">'+s.warning_count+'</div><div class="lbl">预警</div></div>'+
              '<div class="debug-usage-item"><div class="val">'+s.critical_count+'</div><div class="lbl">危险</div></div>'+
            '</div>'+
            '<div style="margin-top:10px;text-align:right"></div>'+
          '</div>';
      }else{
        html='<div class="debug-popup-body"><div class="debug-popup-header"><span>上下文使用量</span></div><span class="debug-loading">'+(d.message||'暂无数据')+'</span></div>';
      }
      c.innerHTML=html;
      // Add back button at bottom
      var footer=c.querySelector('.debug-popup-body div:last-child');
      if(footer)footer.appendChild(popupBackButton());
    }catch(e){
      c.innerHTML='<div class="debug-popup-body"><div class="debug-popup-header"><span>上下文使用量</span></div><span style="color:var(--danger);font-size:.8rem">请求失败: '+(e.message||e)+'</span><div style="margin-top:10px;text-align:right"></div></div>';
      var footer=c.querySelector('.debug-popup-body div:last-child');
      if(footer)footer.appendChild(popupBackButton());
    }
  }

  async function loadToolsList(){
    var c=debugPopupContent;
    if(!c)return;
    c.innerHTML='<div class="debug-popup-body"><span class="debug-loading">加载中...</span></div>';
    try{
      var d=await api('GET','/tools');
      var html='';
      if(d.code===200&&d.data&&d.data.length>0){
        html=
          '<div class="debug-popup-body">'+
            '<div class="debug-popup-header"><span>可用工具 ('+d.data.length+')</span></div>'+
            d.data.map(function(t){
              return '<div class="debug-tool-item">&#x2022; <b>'+escHtml(t.name)+'</b> <span style="color:var(--text-muted)">'+escHtml(t.description||'')+'</span></div>';
            }).join('')+
            '<div style="margin-top:10px;text-align:right"></div>'+
          '</div>';
      }else{
        html='<div class="debug-popup-body"><div class="debug-popup-header"><span>可用工具</span></div><span class="debug-loading">'+(d.message||'无可用工具')+'</span></div>';
      }
      c.innerHTML=html;
      var footer=c.querySelector('.debug-popup-body div:last-child');
      if(footer)footer.appendChild(popupBackButton());
    }catch(e){
      c.innerHTML='<div class="debug-popup-body"><div class="debug-popup-header"><span>可用工具</span></div><span style="color:var(--danger);font-size:.8rem">请求失败: '+(e.message||e)+'</span><div style="margin-top:10px;text-align:right"></div></div>';
      var footer=c.querySelector('.debug-popup-body div:last-child');
      if(footer)footer.appendChild(popupBackButton());
    }
  }

  /* ===== Chat ===== */
  async function initChat(){
    loadTools();loadHistory();
  }

  async function ensureSession(){
    if(chatSessionID) return true;
    try{
      var r=await api('POST','/sessions',{user_id:getUserId()});
      chatSessionID=r.data.id;localStorage.setItem('ia_chat_sid',chatSessionID);
      loadTools();loadHistory();
      return true;
    }catch(e){toastErr(e);return false;}
  }

  async function sendMessage(){
    var msg=inputMsg.value.trim(); if(!msg) return;
    if(!chatSessionID){var ok=await ensureSession();if(!ok)return;}
    inputMsg.value=''; inputMsg.style.height='auto'; btnSend.disabled=true;
    appendMessage('user',msg);
    var typingEl=appendTyping();
    try{
      var resp=await fetch(API+'/sessions/'+chatSessionID+'/stream',{
        method:'POST',headers:{'Content-Type':'application/json',Authorization:'Bearer '+(token||'')},
        body:JSON.stringify({message:msg})
      });
      if(!resp.ok) throw new Error('HTTP '+resp.status);
      typingEl.remove();
      var aiEl=appendMessage('assistant','');
      var reader=resp.body.getReader(),decoder=new TextDecoder(),buffer='',fullContent='';
      while(true){
        var{done,value}=await reader.read();
        if(done) break;
        buffer+=decoder.decode(value,{stream:true});
        var lines=buffer.split('\n'); buffer=lines.pop()||'';
        for(var i=0;i<lines.length;i++){
          if(lines[i].startsWith('data:')){
            var data=lines[i].slice(5).trim();
            if(data==='[DONE]') continue;
            try{var d=JSON.parse(data);fullContent+=d.content;aiEl.querySelector('.msg-content').textContent=fullContent;chatBox.scrollTop=chatBox.scrollHeight;}
            catch(e){}
          }
        }
      }
      addToHistory(msg.substring(0,40));
    }catch(e){typingEl&&typingEl.remove();toastErr(e)}
    btnSend.disabled=false;
  }

  function appendMessage(role,content){
    var el=document.createElement('div');el.className='msg '+role;
    el.innerHTML='<div class="msg-avatar">'+(role==='user'?'U':'AI')+'</div><div class="msg-content">'+(content||'')+'</div>';
    chatBox.appendChild(el);chatBox.scrollTop=chatBox.scrollHeight;
    // Remove welcome
    var w=chatBox.querySelector('.welcome-msg'); if(w) w.remove();
    return el;
  }

  function appendTyping(){
    var el=document.createElement('div');el.className='msg assistant typing';
    el.innerHTML='<div class="msg-avatar">AI</div><div class="msg-content"><div class="typing-indicator"><span></span><span></span><span></span></div></div>';
    chatBox.appendChild(el);chatBox.scrollTop=chatBox.scrollHeight;return el;
  }

  inputMsg.addEventListener('keydown',function(e){
    if(e.key==='Enter'&&!e.shiftKey){e.preventDefault();sendMessage()}
  });
  inputMsg.addEventListener('input',function(){this.style.height='auto';this.style.height=Math.min(this.scrollHeight,120)+'px'});
  btnSend.addEventListener('click',sendMessage);

  // Enter key for dynamically-created skill answer input
  document.addEventListener('keydown',function(e){
    if(e.key==='Enter'&&!e.shiftKey&&e.target&&e.target.id==='input-skill-answer'){
      e.preventDefault();
      var btn=$('btn-skill-send');if(btn)btn.click();
    }
  });

  /* ===== History ===== */
  var historyItems=[];
  function loadHistory(){
    api('GET','/sessions?user_id='+encodeURIComponent(getUserId())).then(function(r){
      if(r.code===200&&r.data){
        var sessions=r.data.filter(function(s){return s.status!=='created'||s.overall_score>0});
        var el=$('history-list');
        el.innerHTML=sessions.length?sessions.map(function(s){
          var preview=s.last_message||'';
          preview=preview.replace(/\n/g,' ').substring(0,40);
          if(!preview) preview='(无消息)';
          var date=s.created_at?s.created_at.substring(5,16):'';
          var score=s.overall_score>0?' · '+Number(s.overall_score).toFixed(1)+'分':'';
          return '<div class="history-item" onclick="window._restoreChatSession(\''+s.id+'\')">'+
            '<div class="history-preview">'+escHtml(preview)+'</div>'+
            '<div class="history-meta">'+date+score+'</div></div>';
        }).join(''):'<div class="history-empty">暂无历史对话</div>';
      }
    }).catch(function(){});
  }
  function escHtml(s){var d=document.createElement("div");d.textContent=s;return d.innerHTML}
  function addToHistory(title){
    historyItems.unshift(title);
    var recent=$('sidebar-recent');
    if(recent){recent.style.display='block';}
    var el=$('recent-list');
    if(el){el.innerHTML=historyItems.slice(0,20).map(function(t,i){return '<div class="history-item">'+t+'</div>'}).join('');}
  }

  // Expose non-window functions used in onclick handlers
  window.showJDInput = showJDInput;
  window.showResumeUpload = showResumeUpload;
  window.loadSkills = loadSkills;

  window._restoreChatSession=async function(sid){
    try{
      var r=await api('GET','/sessions/'+sid+'/messages');
      if(r.code===200&&r.data&&r.data.length>0){
        chatSessionID=sid;localStorage.setItem('ia_chat_sid',sid);
        switchMode('chat');
        chatBox.innerHTML='';
        r.data.forEach(function(m){appendMessage(m.role==='user'?'user':'assistant',m.content)});
        toast('会话已恢复','success');
      }else{
        toast('该会话无消息记录（可能已过期）','error');
      }
    }catch(e){toastErr(e)}
  };

  /* ===== Tools ===== */
  async function loadTools(){
    try{
      var r=await fetch(API+'/tools');var d=await r.json();
      if(d.code===200&&d.data&&d.data.length>0){
        $('available-tools-sidebar').style.display='block';
        $('tools-count-sidebar').textContent=d.data.length;
        $('tools-list-sidebar').innerHTML=d.data.map(function(t){return '<div>&#x2022; <b>'+t.name+'</b>: '+t.description+'</div>'}).join('');
        $('tools-toggle-sidebar').onclick=function(){var l=$('tools-list-sidebar');l.style.display=l.style.display==='none'?'block':'none'}
      }
    }catch(e){}
  }

  /* ===== Interview ===== */
  function setProgress(step){
    var steps=document.querySelectorAll('.progress-step');
    var idx={'jd':0,'resume':1,'interviewing':2,'report':3}[step]||0;
    steps.forEach(function(s,i){
      s.classList.remove('active','done');
      if(i<idx) s.classList.add('done');
      else if(i===idx) s.classList.add('active');
    });
    var lines=document.querySelectorAll('.progress-line');
    lines.forEach(function(l,i){l.classList.toggle('done',i<idx)});
  }

  async function resumeOrStartInterview(){
    // Check if there's a saved interview session to resume
    var sid=interviewSessionID;
    if(!sid){showJDInput();return}

    // Try to restore the session state
    try{
      var r=await api('POST','/sessions/'+sid+'/restore');
      if(r.code!==200||!r.data){
        // Session not found — clear and start fresh
        interviewSessionID=null;localStorage.removeItem('ia_interview_sid');
        showJDInput();return
      }
      var phase=r.data.status||'';
      if(phase==='completed'){
        // Already completed — show the report
        interviewSessionID=sid;localStorage.setItem('ia_interview_sid',sid);
        setProgress('report');
        showReport();return
      }
      if(phase==='interviewing'){
        // Mid-interview — restore the interview UI
        interviewSessionID=sid;localStorage.setItem('ia_interview_sid',sid);
        setProgress('interviewing');
        restoreInterviewUI(sid);return
      }
      if(phase==='resume_matching'){
        // JD done, resume pending — show resume upload
        interviewSessionID=sid;localStorage.setItem('ia_interview_sid',sid);
        setProgress('resume');
        showResumeUpload();return
      }
      // Other phases (created, jd_parsing, question_planning) — show JD input
      // but keep the session so JD/resume data can be reused
      interviewSessionID=sid;localStorage.setItem('ia_interview_sid',sid);
      showJDInput();
    }catch(e){
      // Network error or other — start fresh
      showJDInput();
    }
  }

  async function restoreInterviewUI(sid){
    // Fetch the current question from the interview state
    try{
      var r=await api('POST','/sessions/'+sid+'/start');
      var c=$('interview-content');
      if(r.code===200){
        var data=r.data.data||r.data;
        if(r.data.type==='complete'){showReport();return}
        buildInterviewUI();
        appendInterviewMessage('assistant','&#x1F504; 面试已恢复。\n\n'+data);
      } else {
        // Can't restore interview state — the checkpoint may be stale or missing.
        // Try falling back to the report if the interview was actually completed.
        var reportCheck=await api('GET','/sessions/'+sid+'/report');
        if(reportCheck.code===200&&reportCheck.data){
          toast('面试状态已过期，已切换至面试报告','error');
          setProgress('report');
          showReport();
        } else {
          toast('无法恢复面试状态，请开始新的面试','error');
          showJDInput();
        }
      }
    }catch(e){
      toast('恢复面试失败','error');
      showJDInput();
    }
  }

  function showJDInput(){
    var endBtn=$('btn-end-interview-header');if(endBtn)endBtn.style.display='none';
    var c=$('interview-content');
    var resumeBanner='';
    if(interviewSessionID){
      resumeBanner='<div style="background:var(--bg-tertiary);border:1px solid var(--accent);border-radius:8px;padding:10px 14px;margin-bottom:12px;display:flex;align-items:center;justify-content:space-between">'+
        '<span style="font-size:.85rem;color:var(--accent)">&#x1F504; 你有进行中的面试会话</span>'+
        '<button class="btn-sm" onclick="resumeOrStartInterview()" style="background:var(--accent);color:#fff">继续面试</button></div>';
    }
    c.innerHTML='<div class="iv-card"><h3>&#x1F4CB; 岗位描述 (JD)</h3>'+
      '<p style="color:var(--text-muted);margin-bottom:12px">输入目标岗位的 JD 内容，AI 将分析岗位要求并生成面试题。</p>'+
      resumeBanner+
      '<textarea id="input-jd" placeholder="粘贴岗位JD内容..." style="min-height:160px"></textarea>'+
      '<button class="btn btn-primary" id="btn-start-jd" onclick="window._startJD()" style="margin-top:12px">&#x1F50D; 开始解析</button></div>';
    setProgress('jd');
  }

  window._startJD=async function(){
    var jd=$('input-jd').value.trim(); if(!jd) return toast('请输入JD内容','error');
    if(!interviewSessionID){var r=await api('POST','/sessions',{user_id:getUserId()});interviewSessionID=r.data.id;localStorage.setItem('ia_interview_sid',interviewSessionID)}
    var btn=$('btn-start-jd'); if(btn){btn.disabled=true;btn.textContent='解析中...'}
    try{
      var r=await api('POST','/sessions/'+interviewSessionID+'/jd',{jd_text:jd},90000);
      if(btn){btn.disabled=false;btn.textContent='开始解析'}
      if(r.code===200){
        var d=r.data;
        var tags=(d.tech_stack||[]).map(function(t){return '<span class="tag highlight">'+t+'</span>'}).join('');
        var skills=(d.core_skills||[]).map(function(s){return '<span class="tag">'+s+'</span>'}).join('');
        var pos=d.position||'未指定'; var lv=d.level||'未指定'; var exp=d.experience_years;
        var c=$('interview-content');
        c.innerHTML='<div class="iv-card"><h3>&#x2705; JD 分析完成</h3>'+
          '<div class="result-grid">'+
            '<div class="result-item"><div class="val">'+pos+'</div><div class="lbl">岗位</div></div>'+
            '<div class="result-item"><div class="val">'+lv+'</div><div class="lbl">级别</div></div>'+
            '<div class="result-item"><div class="val">'+(exp!=null?exp+'年':'未提及')+'</div><div class="lbl">经验要求</div></div>'+
          '</div>'+
          (tags?'<div class="iv-section"><h4>技术栈</h4><div class="tag-list">'+tags+'</div></div>':'')+
          (skills?'<div class="iv-section"><h4>核心技能</h4><div class="tag-list">'+skills+'</div></div>':'')+
          '<button class="btn btn-primary" id="btn-next-resume" onclick="window.showResumeUpload()" style="margin-top:16px">&#x27A1; 下一步：上传简历</button>'+
          '<button class="btn btn-secondary" onclick="window._startInterview()" style="margin-top:16px;margin-left:8px">跳过，直接面试</button></div>';
        setProgress('resume');
        updateSidePanel(d);
        toast('JD 解析成功','success');
      } else {
        toast(r.message||'JD解析失败','error');
      }
    }catch(e){if(btn){btn.disabled=false;btn.textContent='开始解析'}toastErr(e)}
  };

  function showResumeUpload(){
    var endBtn=$('btn-end-interview-header');if(endBtn)endBtn.style.display='none';
    var c=$('interview-content');
    c.innerHTML='<div class="iv-card"><h3>&#x1F4C4; 上传简历</h3>'+
      '<p style="color:var(--text-muted);margin-bottom:12px">上传简历文件或直接粘贴内容，AI 将对比 JD 分析匹配度。</p>'+
      '<input type="file" id="input-resume-file" accept=".pdf,.txt,.docx" style="margin-bottom:12px">'+
      '<textarea id="input-resume-text" placeholder="或直接粘贴简历内容..." style="min-height:120px"></textarea>'+
      '<button class="btn btn-primary" id="btn-upload-resume" onclick="window._uploadResume()" style="margin-top:12px">&#x1F4CA; 上传并匹配</button></div>';
    setProgress('resume');
  }

  window._uploadResume=async function(){
    var fileInput=$('input-resume-file'); var textInput=$('input-resume-text');
    var content='',fileName='resume.txt';
    if(fileInput.files.length>0){
      var file=fileInput.files[0]; fileName=file.name;
      var buf=await file.arrayBuffer();
      var bytes=new Uint8Array(buf),bin='';
      for(var i=0;i<bytes.length;i++) bin+=String.fromCharCode(bytes[i]);
      content=btoa(bin);
    }else{
      content=btoa(unescape(encodeURIComponent(textInput.value.trim())));
      if(!textInput.value.trim()) return toast('请上传文件或粘贴简历','error');
    }
    var btn=$('btn-upload-resume'); if(btn){btn.disabled=true;btn.textContent='上传分析中...'}
    try{
      var r=await api('POST','/sessions/'+interviewSessionID+'/resume',{file_name:fileName,file_data:content},90000);
      if(btn){btn.disabled=false;btn.textContent='上传并匹配'}
      if(r.code===200){
        var d=r.data;
        var score=Math.round((d.overall_score||0)*100);
        var cls=score>=70?'score-high':score>=40?'score-mid':'score-low';
        var strs=(d.strengths||[]).map(function(s){return '<li>'+s+'</li>'}).join('');
        var gaps=(d.gaps||[]).map(function(s){return '<li>'+s+'</li>'}).join('');
        var c=$('interview-content');
        c.innerHTML='<div class="iv-card"><h3>&#x2705; 简历匹配完成</h3>'+
          '<div style="text-align:center;margin:16px 0"><span class="score-badge '+cls+'">匹配度 '+score+'%</span></div>'+
          (strs?'<div class="iv-section"><h4>&#x1F4AA; 优势</h4><ul style="color:var(--text-secondary);padding-left:20px">'+strs+'</ul></div>':'')+
          (gaps?'<div class="iv-section"><h4>&#x1F3AF; 待提升</h4><ul style="color:var(--text-secondary);padding-left:20px">'+gaps+'</ul></div>':'')+
          '<button class="btn btn-primary" onclick="window._startInterview()" style="margin-top:16px">&#x1F3A4; 开始模拟面试</button></div>';
        setProgress('interviewing');
        updateSidePanel(d);
        toast('匹配完成，开始面试','success');
        await window._startInterview();
      }
    }catch(e){var btn=$('btn-upload-resume');if(btn){btn.disabled=false;btn.textContent='上传并匹配'}toastErr(e)}
  };

  function buildInterviewUI(){
    var c=$('interview-content');
    // Show end button in header
    var endBtn=$('btn-end-interview-header');
    if(endBtn){endBtn.style.display='inline-block';endBtn.onclick=window._endInterview}
    c.innerHTML='<div class="iv-card">'+
      '<div class="iv-header">'+
        '<span class="iv-q-counter">&#x1F3A4; 面试中</span>'+
        '<span class="iv-progress-mini"><div class="iv-progress-mini-fill" id="iv-progress-fill" style="width:0%"></div></span>'+
        '<span class="iv-position" id="iv-position-label">准备中...</span>'+
      '</div>'+
      '<div id="iv-chat-box" class="iv-chat-box">'+
        '<div class="iv-empty"><div class="iv-empty-icon">&#x1F4AC;</div><p>面试即将开始，AI 面试官正在准备问题...</p></div>'+
      '</div>'+
      '<div class="iv-input-area">'+
        '<textarea id="input-interview-answer" placeholder="输入你的回答... (Enter 发送)" rows="1"></textarea>'+
        '<button class="btn-send" id="btn-submit-answer" onclick="window._submitAnswer()">&#x27A4;</button>'+
        '<button class="btn btn-secondary" onclick="window._skipQuestion()">跳过</button>'+
      '</div></div>';
  }

  window._startInterview=async function(){
    setProgress('interviewing');
    buildInterviewUI();
    try{
      var r=await api('POST','/sessions/'+interviewSessionID+'/start',null,60000);
      if(r.code===200){
        var data=r.data.data||r.data;
        if(r.data.type==='complete'){showReport();return}
        appendInterviewMessage('assistant',data);
      }
    }catch(e){toastErr(e)}
  };

  window._submitAnswer=async function(){
    var ans=$('input-interview-answer');if(!ans)return;var v=ans.value.trim();if(!v)return;ans.value='';
    appendInterviewMessage('user',v);
    var btn=$('btn-submit-answer');if(btn){btn.disabled=true;btn.textContent='...'}
    // Show typing indicator while waiting for the interviewer
    var typingEl=appendInterviewTyping();
    try{
      var r=await api('POST','/sessions/'+interviewSessionID+'/answer',{answer:v},120000);
      // Remove typing indicator
      if(typingEl&&typingEl.parentNode)typingEl.remove();
      if(btn){btn.disabled=false;btn.textContent='➤'}
      if(r.code===200){
        var d=r.data;
        appendInterviewMessage('assistant',d.data||d);
        if(d.type==='complete'){toast('面试完成！','success');setTimeout(showReport,500)}
      } else {
        toast(r.message||'提交失败','error');
      }
    }catch(e){
      if(typingEl&&typingEl.parentNode)typingEl.remove();
      if(btn){btn.disabled=false;btn.textContent='➤'}
      toastErr(e);
    }
  };

  // Typing indicator for interview chat
  function appendInterviewTyping(){
    var box=$('iv-chat-box');if(!box)return null;
    var emptyEl=box.querySelector('.iv-empty');if(emptyEl)emptyEl.remove();
    var el=document.createElement('div');el.className='msg assistant';
    el.innerHTML='<div class="msg-avatar">AI</div><div class="msg-content"><div style="font-size:.65rem;color:var(--text-muted);margin-bottom:4px;font-weight:600">面试官</div><div class="typing-indicator"><span></span><span></span><span></span></div></div>';
    box.appendChild(el);box.scrollTop=box.scrollHeight;
    return el;
  }

  window._skipQuestion=async function(){
    try{await api('POST','/sessions/'+interviewSessionID+'/skip');window._startInterview()}catch(e){toastErr(e)}
  };

  window._endInterview=async function(){
    if(!confirm('确定要结束当前面试并生成报告吗？')) return;
    var btns=[$('btn-end-interview'),$('btn-end-interview-header')];
    btns.forEach(function(b){if(b){b.disabled=true;b.textContent='正在生成报告...'}});
    try{
      var r=await api('POST','/sessions/'+interviewSessionID+'/complete');
      if(r.code===200){
        $('btn-end-interview-header').style.display='none';
        toast('面试已结束，正在生成报告','success');
        showReport();
      }else{
        toast(r.message||'结束面试失败','error');
        btns.forEach(function(b){if(b){b.disabled=false;b.textContent='结束面试'}});
      }
    }catch(e){toastErr(e);btns.forEach(function(b){if(b){b.disabled=false;b.textContent='结束面试'}})}
  };

  function appendInterviewMessage(role,content){
    var box=$('iv-chat-box');if(!box)return;
    // Remove empty state on first message
    var emptyEl=box.querySelector('.iv-empty');if(emptyEl)emptyEl.remove();
    var el=document.createElement('div');el.className='msg '+role;
    var avatarText=role==='user'?'U':'AI';
    var roleLabel=role==='assistant'?'<div style="font-size:.65rem;color:var(--text-muted);margin-bottom:4px;font-weight:600">面试官</div>':'<div style="font-size:.65rem;color:var(--text-muted);margin-bottom:4px;text-align:right;font-weight:600">你</div>';
    el.innerHTML='<div class="msg-avatar">'+avatarText+'</div><div class="msg-content">'+roleLabel+content+'</div>';
    box.appendChild(el);box.scrollTop=box.scrollHeight;
  }

  async function showReport(){
    var endBtn=$('btn-end-interview-header');if(endBtn)endBtn.style.display='none';
    setProgress('report');
    var c=$('interview-content');
    c.innerHTML='<div class="iv-card"><h3>&#x1F3C6; 正在生成报告...</h3><p style="color:var(--text-muted)">请稍候，AI 正在分析你的面试表现...</p></div>';
    try{
      var r=await api('GET','/sessions/'+interviewSessionID+'/report');
      var plan=await api('GET','/sessions/'+interviewSessionID+'/review-plan');
      c.innerHTML='';
      if(r.code===200&&r.data){
        var d=r.data; var score100=d.score_100||((d.overall_score||0)*10);
        var grade=d.grade||(score100>=90?'A':score100>=80?'B+':score100>=70?'B':score100>=60?'C':'D');
        var cls=score100>=80?'score-high':score100>=60?'score-mid':'score-low';
        var dimNames={'technical_accuracy':'基础知识','answer_depth':'回答深度','communication':'沟通表达','project_experience':'项目经验'};

        // Header
        var html='<div class="iv-card"><h3>&#x1F3C6; 面试评估报告</h3>';

        // Meta info
        html+='<div style="text-align:center;margin:20px 0">'+
          '<div style="font-size:1.4rem;font-weight:700;margin-bottom:8px"><span class="score-badge '+cls+'">综合得分 '+score100.toFixed(0)+'/100 ('+grade+'级)</span></div>'+
          '<div style="font-size:.8rem;color:var(--text-muted)">评估时间：'+new Date().toISOString().substring(0,16).replace('T',' ')+'</div>'+
          '</div>';

        // Executive summary
        if(d.summary){
          html+='<div class="iv-section"><h4>&#x1F4DD; 综合评价</h4>'+
            '<div style="color:var(--text-secondary);line-height:1.8;white-space:pre-wrap;font-size:.9rem">'+d.summary+'</div></div>';
        }

        // Dimension scores with bars
        var dims='';
        if(d.dimension_score){for(var k in d.dimension_score){
          var dv=d.dimension_score[k]; var pct=dv*10; var dc=pct>=80?'high':pct>=60?'mid':'low';
          var label=dimNames[k]||k;
          dims+='<div class="dim-bar" style="margin:12px 0"><div class="dim-label"><span style="font-weight:600">'+label+'</span><span style="font-weight:700;color:var(--accent)">'+Math.round(pct)+'</span></div>'+
            '<div class="dim-track" style="height:8px;border-radius:4px"><div class="dim-fill '+dc+'" style="width:'+pct+'%;height:100%;border-radius:4px"></div></div></div>';
        }}
        if(dims){
          html+='<div class="iv-section"><h4>&#x1F4CA; 各维度得分</h4>'+dims+'</div>';
        }
        // Dimension commentary
        if(d.overall_advice){
          html+='<div class="iv-section" style="font-size:.85rem;color:var(--text-muted);white-space:pre-wrap;line-height:1.7">'+d.overall_advice+'</div>';
        }

        // Strengths
        if(d.highlights&&d.highlights.length){
          html+='<div class="iv-section"><h4>&#x2B50; 优势</h4>';
          d.highlights.forEach(function(h){html+='<div style="color:var(--text-secondary);padding:6px 0;line-height:1.6">• '+h+'</div>';});
          html+='</div>';
        }

        // Areas to improve
        if(d.weak_areas&&d.weak_areas.length){
          html+='<div class="iv-section"><h4>&#x1F3AF; 待提升</h4>';
          d.weak_areas.forEach(function(w){html+='<div style="color:var(--text-secondary);padding:6px 0;line-height:1.6">• '+w+'</div>';});
          html+='</div>';
        }

        // Per-question reviews
        var qReviews='';
        if(d.question_reviews){d.question_reviews.forEach(function(qr,i){
          qReviews+='<div class="hist-entry" style="padding:16px;margin-bottom:14px;white-space:pre-wrap;line-height:1.7;font-size:.88rem;color:var(--text-secondary)">'+qr+'</div>';
        })}
        // Fallback to evaluations
        if(!qReviews&&d.evaluations){d.evaluations.forEach(function(ev,i){
          qReviews+='<div class="hist-entry" style="padding:16px;margin-bottom:14px"><b>第'+(i+1)+'题 ('+Math.round(ev.total_score*10)+'分)</b><br>'+
            (ev.praise?'<div style="margin-top:8px"><span style="color:var(--accent)">&#x1F44D; 亮点：</span>'+ev.praise+'</div>':'')+
            (ev.issues?'<div style="margin-top:6px"><span style="color:var(--danger)">&#x26A0; 不足：</span>'+ev.issues+'</div>':'')+
            (ev.improvement?'<div style="margin-top:6px"><span style="color:#eab308">&#x1F4A1; 建议：</span>'+ev.improvement+'</div>':'')+
            '</div>';
        })}
        if(qReviews){
          html+='<div class="iv-section"><h4>&#x1F4AC; 逐题点评</h4>'+qReviews+'</div>';
        }

        html+='</div>'; // close iv-card
        c.innerHTML+=html;

      } else {
        c.innerHTML+='<div class="iv-card"><h3>&#x26A0; 报告加载失败</h3>'+
          '<p style="color:var(--text-muted)">'+(r.message||'该面试报告暂不可用，可能数据已过期或面试未完成')+'</p></div>';
      }
      // Review plan
      if(plan.code===200&&plan.data){
        var p=plan.data; var items='';
        (p.plan_items||[]).forEach(function(it){
          items+='<div class="hist-entry" style="padding:16px;margin-bottom:12px"><b>'+(it.priority==='high'?'&#x1F534;':it.priority==='medium'?'&#x1F7E1;':'&#x1F7E2;')+' '+it.topic+'</b> <span class="tag">'+it.estimated_hours+'h</span><br><small style="color:var(--text-secondary);line-height:1.7;display:block;margin-top:8px">'+it.description+'</small></div>';
        });
        var res=''; (p.resources||[]).forEach(function(r){
          res+='<div class="hist-entry"><a href="'+r.url+'" target="_blank" style="color:var(--accent)">&#x1F517; '+r.title+'</a><br><small style="color:var(--text-muted)">'+r.type+' · '+r.source+(r.description?'<br>'+r.description:'')+'</small></div>';
        });
        c.innerHTML+='<div class="iv-card" style="margin-top:16px"><h3>&#x1F4DA; 复习计划</h3>'+
          (p.weak_areas&&p.weak_areas.length?'<div style="margin-bottom:12px"><span style="color:var(--text-muted)">重点提升：</span>'+(p.weak_areas||[]).map(function(w){return '<span class="tag" style="border-color:var(--danger);color:var(--danger)">'+w+'</span>'}).join(' ')+'</div>':'')+
          '<div class="iv-section"><h4>&#x1F4CB; 学习计划</h4>'+items+'</div>'+
          (res?'<div class="iv-section"><h4>&#x1F517; 推荐资源</h4>'+res+'</div>':'')+'</div>';
      }
      c.innerHTML+='<button class="btn btn-primary" onclick="window.showJDInput()" style="margin-top:16px">&#x1F504; 开始新的面试</button>';
    }catch(e){toastErr(e)}
  }

  window._loadReviewPlan=async function(){showReport()};

  function updateSidePanel(data){
    var panel=$('current-result-panel'); var content=$('current-result-content');
    if(!panel||!content) return;
    panel.style.display='block';
    if(data.position){
      content.innerHTML='<div style="font-size:.85rem"><b>'+data.position+'</b><br>'+
        '<span style="color:var(--text-muted)">级别: '+data.level+'</span></div>';
    }
    if(data.overall_score!==undefined){
      var s=Math.round(data.overall_score*100);
      content.innerHTML+='<div style="margin-top:8px;font-size:1.2rem;font-weight:700;color:var(--accent)">匹配 '+s+'%</div>';
    }
    $('interview-sidebar').style.display='flex';
  }

  /* ===== Interview History ===== */
  async function loadInterviewHistory(){
    var list=$('interview-history-list');
    if(!list) return;
    list.innerHTML='<div style="color:var(--text-muted);font-size:.8rem;padding:8px">加载中...</div>';
    try{
      var r=await api('GET','/sessions?user_id='+encodeURIComponent(getUserId()));
      if(r.code===200&&r.data){
        var interviewStatuses=['jd_parsing','resume_matching','question_planning','interviewing','completed'];
        var items=r.data.filter(function(s){return interviewStatuses.indexOf(s.status)!==-1||s.overall_score>0}).slice(0,15);
        list.innerHTML=items.map(function(s){
          var preview=s.last_message||'';
          preview=preview.replace(/\n/g,' ').substring(0,35);
          if(!preview) preview='(无消息)';
          var score=s.overall_score>0?'<span class="hist-score">'+s.overall_score.toFixed(1)+'</span>':'<span class="hist-score" style="opacity:0.4">-</span>';
          var date=(s.created_at||'').substring(5,16);
          return '<div class="hist-entry" onclick="window._viewPastResult(\''+s.id+'\')"><div>'+score+'</div><div class="hist-preview">'+escHtml(preview)+'</div><div class="hist-date">'+date+'</div></div>';
        }).join('')||'<div style="color:var(--text-muted);font-size:.8rem">暂无历史记录</div>';
        if(items.length>0){$('interview-sidebar').style.display='flex'}
      }
    }catch(e){}
  }

  window._viewPastResult=async function(sid){
    // Switch to interview mode so the report is visible
    if(currentMode!=='interview'){switchMode('interview')}
    var c=$('interview-content');
    c.innerHTML='<div class="iv-card"><h3>&#x1F4CA; 加载中...</h3><p style="color:var(--text-muted)">正在获取面试报告...</p></div>';
    try{
      var r=await api('GET','/sessions/'+sid+'/report');
      var plan=await api('GET','/sessions/'+sid+'/review-plan');
      c.innerHTML='';
      if(r.code===200&&r.data){
        var d=r.data; var score100=d.score_100||((d.overall_score||0)*10);
        var grade=d.grade||(score100>=90?'A':score100>=80?'B+':score100>=70?'B':score100>=60?'C':'D');
        var cls=score100>=80?'score-high':score100>=60?'score-mid':'score-low';
        var dimNames={'technical_accuracy':'基础知识','answer_depth':'回答深度','communication':'沟通表达','project_experience':'项目经验'};

        var html='<div class="iv-card"><h3>&#x1F4CA; 历史面试报告</h3>';

        html+='<div style="text-align:center;margin:20px 0">'+
          '<div style="font-size:1.4rem;font-weight:700;margin-bottom:8px"><span class="score-badge '+cls+'">综合得分 '+score100.toFixed(0)+'/100 ('+grade+'级)</span></div>'+
          '</div>';

        if(d.summary){
          html+='<div class="iv-section"><h4>&#x1F4DD; 综合评价</h4>'+
            '<div style="color:var(--text-secondary);line-height:1.8;white-space:pre-wrap;font-size:.9rem">'+d.summary+'</div></div>';
        }

        var dims='';
        if(d.dimension_score){for(var k in d.dimension_score){
          var dv=d.dimension_score[k]; var pct=dv*10; var dc=pct>=80?'high':pct>=60?'mid':'low';
          var label=dimNames[k]||k;
          dims+='<div class="dim-bar" style="margin:12px 0"><div class="dim-label"><span style="font-weight:600">'+label+'</span><span style="font-weight:700;color:var(--accent)">'+Math.round(pct)+'</span></div>'+
            '<div class="dim-track" style="height:8px;border-radius:4px"><div class="dim-fill '+dc+'" style="width:'+pct+'%;height:100%;border-radius:4px"></div></div></div>';
        }}
        if(dims){
          html+='<div class="iv-section"><h4>&#x1F4CA; 各维度得分</h4>'+dims+'</div>';
        }
        if(d.overall_advice){
          html+='<div class="iv-section" style="font-size:.85rem;color:var(--text-muted);white-space:pre-wrap;line-height:1.7">'+d.overall_advice+'</div>';
        }

        if(d.highlights&&d.highlights.length){
          html+='<div class="iv-section"><h4>&#x2B50; 优势</h4>';
          d.highlights.forEach(function(h){html+='<div style="color:var(--text-secondary);padding:6px 0;line-height:1.6">• '+h+'</div>';});
          html+='</div>';
        }

        if(d.weak_areas&&d.weak_areas.length){
          html+='<div class="iv-section"><h4>&#x1F3AF; 待提升</h4>';
          d.weak_areas.forEach(function(w){html+='<div style="color:var(--text-secondary);padding:6px 0;line-height:1.6">• '+w+'</div>';});
          html+='</div>';
        }

        var qReviews='';
        if(d.question_reviews){d.question_reviews.forEach(function(qr,i){
          qReviews+='<div class="hist-entry" style="padding:16px;margin-bottom:14px;white-space:pre-wrap;line-height:1.7;font-size:.88rem;color:var(--text-secondary)">'+qr+'</div>';
        })}
        if(!qReviews&&d.evaluations){d.evaluations.forEach(function(ev,i){
          qReviews+='<div class="hist-entry" style="padding:16px;margin-bottom:14px"><b>第'+(i+1)+'题 ('+Math.round(ev.total_score*10)+'分)</b><br>'+
            (ev.praise?'<div style="margin-top:8px"><span style="color:var(--accent)">&#x1F44D; 亮点：</span>'+ev.praise+'</div>':'')+
            (ev.issues?'<div style="margin-top:6px"><span style="color:var(--danger)">&#x26A0; 不足：</span>'+ev.issues+'</div>':'')+
            (ev.improvement?'<div style="margin-top:6px"><span style="color:#eab308">&#x1F4A1; 建议：</span>'+ev.improvement+'</div>':'')+
            '</div>';
        })}
        if(qReviews){
          html+='<div class="iv-section"><h4>&#x1F4AC; 逐题点评</h4>'+qReviews+'</div>';
        }

        html+='</div>'; // close iv-card
        c.innerHTML+=html;

        // Review plan
        if(plan.code===200&&plan.data){
          var p=plan.data; var items='';
          (p.plan_items||[]).forEach(function(it){
            items+='<div class="hist-entry" style="padding:16px;margin-bottom:12px"><b>'+(it.priority==='high'?'&#x1F534;':it.priority==='medium'?'&#x1F7E1;':'&#x1F7E2;')+' '+it.topic+'</b> <span class="tag">'+it.estimated_hours+'h</span><br><small style="color:var(--text-secondary);line-height:1.7;display:block;margin-top:8px">'+it.description+'</small></div>';
          });
          var res=''; (p.resources||[]).forEach(function(rr){
            res+='<div class="hist-entry"><a href="'+rr.url+'" target="_blank" style="color:var(--accent)">&#x1F517; '+rr.title+'</a><br><small style="color:var(--text-muted)">'+rr.type+' · '+rr.source+(rr.description?'<br>'+rr.description:'')+'</small></div>';
          });
          c.innerHTML+='<div class="iv-card" style="margin-top:16px"><h3>&#x1F4DA; 复习计划</h3>'+
            (p.weak_areas&&p.weak_areas.length?'<div style="margin-bottom:12px"><span style="color:var(--text-muted)">重点提升：</span>'+(p.weak_areas||[]).map(function(w){return '<span class="tag" style="border-color:var(--danger);color:var(--danger)">'+w+'</span>'}).join(' ')+'</div>':'')+
            '<div class="iv-section"><h4>&#x1F4CB; 学习计划</h4>'+items+'</div>'+
            (res?'<div class="iv-section"><h4>&#x1F517; 推荐资源</h4>'+res+'</div>':'')+'</div>';
        }
      } else {
        c.innerHTML='<div class="iv-card"><h3>&#x26A0; 无法加载报告</h3>'+
          '<p style="color:var(--text-muted)">'+(r.message||'该面试报告暂不可用，可能数据已过期')+'</p>';
      }
      c.innerHTML+='<button class="btn btn-primary" onclick="window.showJDInput()" style="margin-top:16px">&#x1F504; 开始新的面试</button>';
      setProgress('report');
    }catch(e){toastErr(e)}
  };

  // History modal
  var btnHistory=$('btn-interview-history');if(btnHistory)btnHistory.addEventListener('click',async function(){
    $('history-modal').style.display='flex';
    var list=$('history-modal-list');
    list.innerHTML='<div style="text-align:center;color:var(--text-muted);padding:20px">加载中...</div>';
    try{
      var r=await api('GET','/sessions?user_id='+encodeURIComponent(getUserId()));
      if(r.code===200&&r.data){
        var interviewStatuses=['jd_parsing','resume_matching','question_planning','interviewing','completed'];
        var items=r.data.filter(function(s){return interviewStatuses.indexOf(s.status)!==-1||s.overall_score>0});
        list.innerHTML=items.length?items.map(function(s){
          var cls=s.overall_score>=7?'score-high':s.overall_score>=4?'score-mid':'score-low';
          var date=(s.created_at||'').substring(0,16);
          var preview=s.last_message||'';
          preview=preview.replace(/\n/g,' ').substring(0,50);
          if(!preview) preview='(无消息)';
          return '<div class="hist-card" onclick="window._viewPastResult(\''+s.id+'\');$(\'history-modal\').style.display=\'none\'">'+
            '<div class="hist-meta"><span class="hist-score-big '+cls+'">'+s.overall_score.toFixed(1)+'</span><span>'+date+'</span></div>'+
            '<div class="hist-preview-text">'+escHtml(preview)+'</div></div>';
        }).join(''):'<div style="color:var(--text-muted);text-align:center;padding:20px">暂无完成的面试</div>';
      }
    }catch(e){list.innerHTML='<div style="color:var(--danger)">加载失败</div>'}
  });
  $('btn-history-close').addEventListener('click',function(){$('history-modal').style.display='none'});
  $('history-modal').addEventListener('click',function(e){if(e.target===this) this.style.display='none'});

  /* ===== Skills ===== */

  // Chinese display metadata for each skill
  var skillMeta={
    'quick_quiz':       {name:'快速测验',   icon:'🎯', desc:'针对指定技术主题，出5道题测试你的知识水平并评分', category:'面试技能'},
    'knowledge_explain':{name:'知识讲解',   icon:'📖', desc:'逐层深入讲解技术概念：入门概述→核心原理→进阶优化→前沿对比', category:'面试技能'},
    'project_highlight':{name:'项目亮点提炼', icon:'⭐', desc:'分4阶段提炼项目面试亮点，生成STAR格式的面试故事', category:'面试技能'},
    'tech_compare':     {name:'技术对比',   icon:'⚖️', desc:'从性能、生态、学习曲线、适用场景四个维度对比两项技术', category:'面试技能'},
    'algorithm':        {name:'算法练习',   icon:'💻', desc:'LeetCode 风格算法编程训练，逐步提升解题能力', category:'专项训练'},
    'system_design':    {name:'系统设计',   icon:'🏗️', desc:'系统设计面试模拟，涵盖需求分析、容量估算、架构设计', category:'专项训练'},
    'behavioral':       {name:'行为面试',   icon:'🗣️', desc:'STAR 法则行为面试练习，提升软技能与沟通表达', category:'专项训练'},
    'tech_quiz':        {name:'技术测验',   icon:'📝', desc:'技术栈知识快速问答，10道题目循序渐进覆盖多领域', category:'专项训练'},
  };

  async function loadSkills(){
    $('skills-content').innerHTML='<div style="text-align:center;padding:48px;color:var(--text-muted)"><div class="spin" style="width:32px;height:32px;border:3px solid var(--border);border-top-color:var(--accent);border-radius:50%;margin:0 auto 12px"></div>加载技能列表中...</div>';
    try{
      var r=await api('GET','/skills');
      if(r.code===200&&r.data){
        var coreSkills=[];var trainingSkills=[];
        r.data.forEach(function(s){
          var meta=skillMeta[s.name]||{name:s.name,icon:'📦',desc:s.description,category:''};
          var card='<div class="skill-card" onclick="window._startSkill(\''+s.name+'\')">'+
            '<span class="skill-icon">'+meta.icon+'</span>'+
            '<div class="skill-body"><div class="skill-name">'+meta.name+'</div>'+
            '<div class="skill-desc">'+meta.desc+'</div></div>'+
            '<span class="skill-arrow">→</span></div>';
          if(s.category==='core'||meta.category==='面试技能'){coreSkills.push(card)}
          else{trainingSkills.push(card)}
        });
        var html='<div class="skills-hero"><h2>技能练习中心</h2>'+
          '<p>选择一项技能开始多轮交互练习。区别于无状态的工具调用，技能是有状态的多轮对话模块，AI 会记住上下文并逐步深入。</p></div>';
        if(coreSkills.length){
          html+='<div class="skills-section"><div class="skills-section-title">🎤 面试技能</div>'+
            '<div class="skills-grid">'+coreSkills.join('')+'</div></div>';
        }
        if(trainingSkills.length){
          html+='<div class="skills-section"><div class="skills-section-title">🏋️ 专项训练</div>'+
            '<div class="skills-grid">'+trainingSkills.join('')+'</div></div>';
        }
        $('skills-content').innerHTML=html;
      }
    }catch(e){}
  }

  window._startSkill=async function(name){
    if(!skillSessionID){var r=await api('POST','/sessions',{user_id:getUserId()});skillSessionID=r.data.id;localStorage.setItem('ia_skill_sid',skillSessionID)}
    // Build the chat UI first
    var meta=skillMeta[name];var displayName=meta?meta.icon+' '+meta.name:name;
    var backBtn='<button class="btn-sm" onclick="window.loadSkills()">返回</button>';
    $('skills-content').innerHTML='<div class="chat-header"><span class="chat-title">'+displayName+'</span>'+backBtn+'</div><div id="skill-chat-box" class="chat-box" style="height:300px;overflow-y:auto"><div class="msg assistant"><div class="msg-avatar">AI</div><div class="msg-content" style="color:var(--text-muted)">正在准备练习...</div></div></div><div class="chat-input-area"><textarea id="input-skill-answer" placeholder="输入你的回答..." rows="2" disabled></textarea><button class="btn-send" id="btn-skill-send" onclick="window._submitSkillAnswer(\''+name+'\')" disabled>&#x27A4;</button></div>';
    // Send initial request to get welcome message
    try{
      var r=await api('POST','/sessions/'+skillSessionID+'/message',{message:'skill:'+name+':start'});
      // Enable input after welcome arrives
      var ta=$('input-skill-answer');var sb=$('btn-skill-send');
      if(ta)ta.disabled=false;if(sb)sb.disabled=false;
      if(r.code===200){
        var box=$('skill-chat-box');if(!box)return;
        var el=document.createElement('div');el.className='msg assistant';
        el.innerHTML='<div class="msg-avatar">AI</div><div class="msg-content">'+r.data.reply+'</div>';
        box.appendChild(el);box.scrollTop=box.scrollHeight;
      }
    }catch(e){var ta=$('input-skill-answer');var sb=$('btn-skill-send');if(ta)ta.disabled=false;if(sb)sb.disabled=false;toastErr(e)}
  };

  window._submitSkillAnswer=async function(name){
    var ans=$('input-skill-answer');if(!ans)return;var v=ans.value.trim();if(!v)return;ans.value='';
    var box=$('skill-chat-box');if(!box)return;
    var btn=$('btn-skill-send');if(btn){btn.disabled=true;btn.textContent='...'}
    // User message
    var el=document.createElement('div');el.className='msg user';el.innerHTML='<div class="msg-avatar">U</div><div class="msg-content">'+v+'</div>';box.appendChild(el);
    box.scrollTop=box.scrollHeight;
    try{
      var r=await api('POST','/sessions/'+skillSessionID+'/message',{message:'skill:'+name+':'+v});
      if(btn){btn.disabled=false;btn.textContent='➤'}
      if(r.code===200){
        var el2=document.createElement('div');el2.className='msg assistant';el2.innerHTML='<div class="msg-avatar">AI</div><div class="msg-content">'+r.data.reply+'</div>';box.appendChild(el2);
        box.scrollTop=box.scrollHeight;
      }
    }catch(e){if(btn){btn.disabled=false;btn.textContent='➤'}toastErr(e)}
    // Refocus input
    if(ans)ans.focus();
  };

  /* ===== Knowledge ===== */
  async function loadDocs(){
    $('knowledge-content').innerHTML='<div style="text-align:center;padding:48px;color:var(--text-muted)"><div class="spin" style="width:32px;height:32px;border:3px solid var(--border);border-top-color:var(--accent);border-radius:50%;margin:0 auto 12px"></div>加载知识库...</div>';
    try{
      var r=await fetch(API+'/documents');var d=await r.json();
      var html='<div class="knowledge-toolbar"><h3 style="margin:0">已上传文档</h3><button class="btn-sm" onclick="window._refreshDocs()">刷新</button></div><div id="doc-list"></div>';
      if(d.code===200&&d.data&&d.data.length===0){
        html+='<div style="text-align:center;padding:60px 20px;color:var(--text-muted)">暂无文档，请通过 AI 闲聊工具栏上传题库</div>';
      }
      $('knowledge-content').innerHTML=html;
      if(d.code===200&&d.data&&d.data.length>0){
        var list=$('doc-list');
        d.data.forEach(function(doc){list.innerHTML+='<div class="doc-item"><span class="doc-name">'+escHtml(doc.source_file)+'</span><button class="btn-sm btn-danger-sm" onclick="window._deleteDoc(\''+doc.id+'\')">删除</button></div>'});
      }
    }catch(e){}
  }

  window._refreshDocs=function(){loadDocs()};

  async function uploadFiles(files){
    var uploads=[];for(var i=0;i<files.length;i++){
      var b=await files[i].arrayBuffer();var bytes=new Uint8Array(b);var bin='';
      for(var j=0;j<bytes.length;j++) bin+=String.fromCharCode(bytes[j]);
      uploads.push({file_name:files[i].name,content:btoa(bin)});
    }
    try{await api('POST','/documents/upload',{files:uploads});toast('上传成功','success');loadDocs()}catch(e){toastErr(e)}
  }

  window._deleteDoc=async function(id){try{await api('DELETE','/documents/'+id);toast('删除成功','success');loadDocs()}catch(e){toastErr(e)}};

  /* ===== Startup ===== */
  async function restoreOnLoad(){
    if(chatSessionID){
      try{
        var r=await api('GET','/sessions/'+chatSessionID+'/messages');
        if(r.code===200&&r.data&&r.data.length>0){
          var w=chatBox.querySelector('.welcome-msg'); if(w) w.remove();
          r.data.forEach(function(m){appendMessage(m.role==='user'?'user':'assistant',m.content)});
        }
      }catch(e){}
    }
    switchMode('chat');
  }
  restoreOnLoad();
})();
