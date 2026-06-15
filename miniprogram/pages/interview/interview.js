const api = require('../../utils/api')
const app = getApp()

const PHASE = { JD: 'jd', RESUME: 'resume', INTERVIEW: 'interview', REPORT: 'report' }

const dimNames = {
  'technical_accuracy': '基础知识',
  'answer_depth': '回答深度',
  'communication': '沟通表达',
  'project_experience': '项目经验'
}

// Pre-compute display fields for history items so WXML only uses simple property access.
function preprocessHistoryItem(s) {
  var score = Number(s.overall_score) || 0
  var preview = (s.last_message || s.status || '').replace(/\n/g, ' ').substring(0, 35)
  if (!preview) preview = '(无消息)'
  return {
    id: s.id,
    preview: preview,
    scoreText: score.toFixed(1),
    scoreClass: score >= 8 ? 'high' : score >= 5 ? 'mid' : 'low',
    dateText: (s.created_at || '').substring(5, 16)
  }
}

// Pre-compute display fields for report so WXML expressions stay simple.
function preprocessReport(r) {
  var dims = []
  if (r.dimension_score) {
    for (var k in r.dimension_score) {
      var raw = r.dimension_score[k]
      var sc = Math.round(raw * 10)
      dims.push({
        name: dimNames[k] || k,
        score: sc,
        pct: sc,
        cls: sc >= 80 ? 'high' : sc >= 60 ? 'mid' : 'low'
      })
    }
  }
  var reviews = r.question_reviews || []
  if (!reviews.length && r.evaluations && r.evaluations.length) {
    reviews = []
    for (var i = 0; i < r.evaluations.length; i++) {
      var ev = r.evaluations[i]
      var text = '第' + (i+1) + '题 (' + Math.round(ev.total_score*10) + '分)'
      if (ev.praise) text += '\n✅ 亮点：' + ev.praise
      if (ev.issues) text += '\n⚠️ 不足：' + ev.issues
      if (ev.improvement) text += '\n💡 建议：' + ev.improvement
      reviews.push(text)
    }
  }
  var highlights = r.highlights || []
  var weakAreas = r.weak_areas || []
  var score100 = r.score_100 || Math.round((r.overall_score || 0) * 10)
  return {
    report: r,
    dims: dims,
    dimCommentary: r.overall_advice || '',
    reviews: reviews,
    score100: score100,
    scoreClass: score100 >= 80 ? 'high' : score100 >= 60 ? 'mid' : 'low',
    grade: r.grade || '',
    highlights: highlights,
    weakAreas: weakAreas
  }
}

// Pre-compute plan display fields
function preprocessPlan(p) {
  if (!p) return null
  return {
    plan: p,
    hasPlan: !!(p.plan_items && p.plan_items.length),
    hasWeakAreas: !!(p.weak_areas && p.weak_areas.length),
    hasResources: !!(p.resources && p.resources.length),
    planItems: (p.plan_items || []).map(function(it) {
      return {
        topic: it.topic,
        priority: it.priority,
        estimated_hours: it.estimated_hours,
        description: it.description,
        priIcon: it.priority === 'high' ? '▲' : it.priority === 'medium' ? '■' : '●',
        priClass: it.priority === 'high' ? 'high' : it.priority === 'medium' ? 'medium' : 'low'
      }
    })
  }
}

Page({
  data: {
    phase: PHASE.JD,
    // JD
    jdText: '',
    parsing: false,
    jdResult: null,
    // Resume
    resumeText: '',
    uploading: false,
    resumeResult: null,
    resumeFileData: '',
    resumeFileName: '',
    // Interview
    messages: [],
    answerText: '',
    submitting: false,
    typing: false,
    progress: { current: 0, total: 5 },
    // Report (pre-processed)
    report: null,
    planData: null,
    dims: [],
    dimCommentary: '',
    reviews: [],
    score100: 0,
    scoreClass: '',
    grade: '',
    highlights: [],
    weakAreas: [],
    // History (pre-processed)
    historyList: [],
    pendingSession: null,
    kbHeight: 0
  },

  onLoad() {
    this.checkResume()
  },

  onShow() {
    if (!app.globalData.token) {
      wx.reLaunch({ url: '/pages/login/login' })
      return
    }
    this.loadHistory()
  },

  async checkResume() {
    var sid = app.globalData.interviewSessionID
    if (!sid) return
    try {
      var r = await api.restoreSession(sid)
      if (r.code === 200 && r.data) {
        var status = r.data.status || ''
        if (status === 'completed') {
          app.globalData.interviewSessionID = ''
          wx.removeStorageSync('ia_interview_sid')
          this.setData({ phase: PHASE.JD, pendingSession: null })
          this.loadHistory()
        } else if (status === 'interviewing') {
          this.setData({ pendingSession: { id: sid, status: '面试进行中' }, phase: PHASE.JD })
        } else if (status === 'resume_matching') {
          this.setData({ pendingSession: { id: sid, status: '等待上传简历' }, phase: PHASE.JD })
        } else {
          this.setData({ pendingSession: { id: sid, status: '未完成的面试' }, phase: PHASE.JD })
        }
      }
    } catch (e) { }
  },

  // === JD Analysis ===
  onJDInput: function(e) {
    this.setData({ jdText: e.detail.value })
  },

  startJD: async function() {
    var jd = this.data.jdText.trim()
    if (!jd) {
      wx.showToast({ title: '请输入JD内容', icon: 'none' })
      return
    }
    this.setData({ parsing: true })

    if (!app.globalData.interviewSessionID) {
      var cr = await api.createSession()
      if (!cr || !cr.data || !cr.data.id) {
        this.setData({ parsing: false })
        wx.showToast({ title: '创建会话失败', icon: 'none' })
        return
      }
      app.globalData.interviewSessionID = cr.data.id
      wx.setStorageSync('ia_interview_sid', cr.data.id)
    }

    try {
      var r = await api.parseJD(app.globalData.interviewSessionID, jd)
      if (r.code === 200) {
        this.setData({ jdResult: r.data, phase: PHASE.RESUME, parsing: false, pendingSession: null })
        wx.showToast({ title: '解析完成', icon: 'success' })
      } else {
        this.setData({ parsing: false })
        wx.showToast({ title: r.message || '解析失败', icon: 'none' })
      }
    } catch (e) {
      this.setData({ parsing: false })
      wx.showToast({ title: '解析失败', icon: 'none' })
    }
  },

  // === Resume Upload ===
  onResumeInput: function(e) {
    this.setData({ resumeText: e.detail.value })
  },

  chooseResumeFile: function() {
    var that = this
    wx.chooseMessageFile({
      count: 1,
      type: 'all',
      success: function(res) {
        var file = res.tempFiles[0]
        var fs = wx.getFileSystemManager()
        var data = fs.readFileSync(file.path, 'base64')
        that.setData({
          resumeText: '[文件: ' + file.name + ']',
          resumeFileData: data,
          resumeFileName: file.name
        })
      }
    })
  },

  uploadResume: async function() {
    var text = this.data.resumeText.trim()
    if (!text) {
      wx.showToast({ title: '请上传简历或粘贴内容', icon: 'none' })
      return
    }

    var content, fileName
    if (this.data.resumeFileData) {
      content = this.data.resumeFileData
      fileName = this.data.resumeFileName
    } else {
      content = this.base64Encode(text)
      fileName = 'resume.txt'
    }
    this.setData({ uploading: true })

    try {
      var r = await api.uploadResume(app.globalData.interviewSessionID, fileName, content)
      if (r.code === 200) {
        this.setData({ resumeResult: r.data, uploading: false, pendingSession: null })
        wx.showToast({ title: '匹配完成', icon: 'success' })
        this.startInterview()
      } else {
        this.setData({ uploading: false })
        wx.showToast({ title: r.message || '匹配失败', icon: 'none' })
      }
    } catch (e) {
      this.setData({ uploading: false })
      wx.showToast({ title: '上传失败', icon: 'none' })
    }
  },

  // Return to JD phase with history list after interview completion.
  // Clears the session so user can start a new interview.
  finishInterview: function(msg) {
    app.globalData.interviewSessionID = ''
    wx.removeStorageSync('ia_interview_sid')
    this.setData({ phase: PHASE.JD, messages: [], report: null, planData: null, pendingSession: null })
    this.loadHistory()
    if (msg) wx.showToast({ title: msg, icon: 'success' })
  },

  base64Encode: function(str) {
    var chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/='
    var out = '', i = 0
    var utf8 = unescape(encodeURIComponent(str))
    while (i < utf8.length) {
      var a = utf8.charCodeAt(i++)
      var b = utf8.charCodeAt(i++)
      var c = utf8.charCodeAt(i++)
      var i1 = a >> 2
      var i2 = ((a & 3) << 4) | (b >> 4)
      var i3 = isNaN(b) ? 64 : ((b & 15) << 2) | (c >> 6)
      var i4 = isNaN(c) ? 64 : c & 63
      out += chars.charAt(i1) + chars.charAt(i2) + chars.charAt(i3) + chars.charAt(i4)
    }
    return out
  },

  // === Resume pending session ===
  resumePendingSession: async function() {
    var sid = this.data.pendingSession.id
    var status = this.data.pendingSession.status
    app.globalData.interviewSessionID = sid
    wx.setStorageSync('ia_interview_sid', sid)

    if (status === '面试进行中') {
      this.setData({ phase: PHASE.INTERVIEW, pendingSession: null })
      this.restoreInterview()
    } else if (status === '等待上传简历') {
      this.setData({ phase: PHASE.RESUME, pendingSession: null })
    } else {
      try {
        var r = await api.restoreSession(sid)
        if (r.code === 200 && r.data) {
          var s = r.data.status || ''
          if (s === 'resume_matching') this.setData({ phase: PHASE.RESUME, pendingSession: null })
          else this.setData({ phase: PHASE.JD, pendingSession: null })
        }
      } catch (e) {
        wx.showToast({ title: '恢复失败', icon: 'none' })
      }
    }
  },

  dismissPending: function() {
    app.globalData.interviewSessionID = ''
    wx.removeStorageSync('ia_interview_sid')
    this.setData({ pendingSession: null })
  },

  // === Interview ===
  startInterview: async function() {
    var prevPhase = this.data.phase
    this.setData({ phase: PHASE.INTERVIEW })
    try {
      var r = await api.startInterview(app.globalData.interviewSessionID)
      if (r.code === 200) {
        var data = r.data.data || r.data
        if (r.data.type === 'complete') {
          this.finishInterview('面试已完成，点击历史记录查看报告')
          return
        }
        this.addMessage('assistant', data)
      } else {
        this.setData({ phase: prevPhase })
        wx.showToast({ title: r.message || '开始面试失败', icon: 'none' })
      }
    } catch (e) {
      this.setData({ phase: prevPhase })
      wx.showToast({ title: '开始面试失败', icon: 'none' })
    }
  },

  restoreInterview: async function() {
    var prevPhase = this.data.phase
    this.setData({ phase: PHASE.INTERVIEW, pendingSession: null })

    try {
      var msgR = await api.getMessages(app.globalData.interviewSessionID)
      if (msgR.code === 200 && msgR.data && msgR.data.length > 0) {
        var msgs = []
        for (var i = 0; i < msgR.data.length; i++) {
          msgs.push({
            role: msgR.data[i].role === 'user' ? 'user' : 'assistant',
            content: msgR.data[i].content
          })
        }
        this.setData({ messages: msgs })
      }
    } catch (e) { }

    try {
      var r = await api.startInterview(app.globalData.interviewSessionID)
      if (r.code === 200) {
        var data = r.data.data || r.data || ''
        var resumeMsg = data ? '🔄 面试已恢复。\n\n' + data : '🔄 面试已恢复。继续回答当前问题。'
        this.addMessage('assistant', resumeMsg)
      } else {
        this.setData({ phase: prevPhase })
        wx.showToast({ title: '恢复失败', icon: 'none' })
      }
    } catch (e) {
      this.setData({ phase: prevPhase })
      wx.showToast({ title: '恢复失败', icon: 'none' })
    }
  },

  onAnswerInput: function(e) {
    this.setData({ answerText: e.detail.value })
  },

  onKBChange: function(e) {
    this.setData({ kbHeight: e.detail.height })
  },

  submitAnswer: async function() {
    var text = this.data.answerText.trim()
    if (!text || this.data.submitting) return
    this.setData({ answerText: '', submitting: true })
    this.addMessage('user', text)
    this.setData({ typing: true })

    try {
      var r = await api.submitAnswer(app.globalData.interviewSessionID, text)
      this.setData({ typing: false })
      if (r.code === 200) {
        var d = r.data
        this.addMessage('assistant', d.data || d)
        if (d.type === 'complete') {
          var that = this
          setTimeout(function() {
            that.finishInterview('面试完成！点击历史记录查看报告')
          }, 800)
        }
      }
    } catch (e) {
      this.setData({ typing: false })
      wx.showToast({ title: '提交失败', icon: 'none' })
    }
    this.setData({ submitting: false })
  },

  skipQ: async function() {
    try {
      await api.skipQuestion(app.globalData.interviewSessionID)
      this.startInterview()
    } catch (e) { }
  },

  endInterview: async function() {
    var that = this
    var res = await new Promise(function(r) {
      wx.showModal({ title: '结束面试', content: '确定要结束并生成报告吗？', success: r })
    })
    if (!res.confirm) return
    try {
      var r = await api.endInterview(app.globalData.interviewSessionID)
      if (r.code === 200) {
        that.finishInterview('报告生成中，请稍后点击历史记录查看')
      }
    } catch (e) {
      wx.showToast({ title: '操作失败', icon: 'none' })
    }
  },

  addMessage: function(role, content) {
    var msgs = this.data.messages.concat([{ role: role, content: content }])
    this.setData({ messages: msgs })
  },

  // === Report ===
  loadReport: async function() {
    try {
      var results = await Promise.all([
        api.getReport(app.globalData.interviewSessionID),
        api.getReviewPlan(app.globalData.interviewSessionID)
      ])
      var reportR = results[0]
      var planR = results[1]
      var setDataObj = {}
      if (reportR.code === 200) {
        var processed = preprocessReport(reportR.data)
        for (var k in processed) {
          setDataObj[k] = processed[k]
        }
      }
      if (planR.code === 200) {
        setDataObj.planData = preprocessPlan(planR.data)
      }
      this.setData(setDataObj)
    } catch (e) {
      wx.showToast({ title: '加载报告失败', icon: 'none' })
    }
  },

  viewPastReport: function(e) {
    var sid = e.currentTarget.dataset.sid
    wx.navigateTo({ url: '/pages/report/report?sid=' + sid })
  },

  loadHistory: async function() {
    try {
      var r = await api.listSessions()
      if (r.code === 200 && r.data) {
        var items = []
        for (var i = 0; i < r.data.length; i++) {
          if (r.data[i].overall_score > 0 || (r.data[i].last_message && r.data[i].status !== 'created')) {
            items.push(preprocessHistoryItem(r.data[i]))
          }
        }
        items = items.slice(0, 20)
        this.setData({ historyList: items })
      }
    } catch (e) { }
  },

  newInterview: function() {
    app.globalData.interviewSessionID = ''
    wx.removeStorageSync('ia_interview_sid')
    this.setData({
      phase: PHASE.JD, jdResult: null, resumeResult: null,
      messages: [], report: null, planData: null, pendingSession: null
    })
  },

  skipResume: function() {
    this.startInterview()
  }
})
