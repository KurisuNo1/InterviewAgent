const api = require('../../utils/api')
const app = getApp()

const PHASE = { JD: 'jd', RESUME: 'resume', INTERVIEW: 'interview', REPORT: 'report' }

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
    // Report
    report: null,
    plan: null,
    // History
    historyList: [],
    pendingSession: null,  // active session that can be resumed
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
    const sid = app.globalData.interviewSessionID
    if (!sid) return
    try {
      const r = await api.restoreSession(sid)
      if (r.code === 200 && r.data) {
        const status = r.data.status || ''
        if (status === 'completed') {
          this.setData({ phase: PHASE.REPORT, pendingSession: null })
          this.loadReport()
        } else if (status === 'interviewing') {
          // Show pending session banner, let user choose to resume
          this.setData({ pendingSession: { id: sid, status: '面试进行中' }, phase: PHASE.JD })
        } else if (status === 'resume_matching') {
          this.setData({ pendingSession: { id: sid, status: '等待上传简历' }, phase: PHASE.JD })
        } else {
          // created, jd_parsing, question_planning
          this.setData({ pendingSession: { id: sid, status: '未完成的面试' }, phase: PHASE.JD })
        }
      }
    } catch (e) { }
  },

  // === JD Analysis ===
  onJDInput(e) { this.setData({ jdText: e.detail.value }) },

  async startJD() {
    const jd = this.data.jdText.trim()
    if (!jd) return wx.showToast({ title: '请输入JD内容', icon: 'none' })
    this.setData({ parsing: true })

    if (!app.globalData.interviewSessionID) {
      const r = await api.createSession()
      app.globalData.interviewSessionID = r.data.id
      wx.setStorageSync('ia_interview_sid', r.data.id)
    }

    try {
      const r = await api.parseJD(app.globalData.interviewSessionID, jd)
      if (r.code === 200) {
        this.setData({ jdResult: r.data, phase: PHASE.RESUME, parsing: false, pendingSession: null })
        wx.showToast({ title: '解析完成', icon: 'success' })
      } else {
        wx.showToast({ title: r.message || '解析失败', icon: 'none' })
      }
    } catch (e) { wx.showToast({ title: '解析失败', icon: 'none' }) }
    this.setData({ parsing: false })
  },

  // === Resume Upload ===
  onResumeInput(e) { this.setData({ resumeText: e.detail.value }) },

  async chooseResumeFile() {
    const that = this
    wx.chooseMessageFile({
      count: 1,
      type: 'file',
      extension: ['pdf', 'txt', 'docx'],
      success(res) {
        const file = res.tempFiles[0]
        const fs = wx.getFileSystemManager()
        const data = fs.readFileSync(file.path, 'base64')
        that.setData({ resumeText: '[文件: ' + file.name + ']', resumeFileData: data, resumeFileName: file.name })
      }
    })
  },

  async uploadResume() {
    const text = this.data.resumeText.trim()
    if (!text) return wx.showToast({ title: '请上传简历或粘贴内容', icon: 'none' })

    let content = ''
    let fileName = 'resume.txt'
    if (this.data.resumeFileData) {
      content = this.data.resumeFileData
      fileName = this.data.resumeFileName
    } else {
      content = this.base64Encode(text)
    }
    this.setData({ uploading: true })

    try {
      const r = await api.uploadResume(app.globalData.interviewSessionID, fileName, content)
      if (r.code === 200) {
        this.setData({ resumeResult: r.data, pendingSession: null })
        wx.showToast({ title: '匹配完成', icon: 'success' })
        await this.startInterview()
      } else {
        wx.showToast({ title: r.message || '匹配失败', icon: 'none' })
      }
    } catch (e) { wx.showToast({ title: '上传失败', icon: 'none' }) }
    this.setData({ uploading: false })
  },

  base64Encode(str) {
    const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/='
    let out = ''; let i = 0
    const utf8 = unescape(encodeURIComponent(str))
    while (i < utf8.length) {
      const a = utf8.charCodeAt(i++)
      const b = utf8.charCodeAt(i++)
      const c = utf8.charCodeAt(i++)
      const i1 = a >> 2
      const i2 = ((a & 3) << 4) | (b >> 4)
      const i3 = isNaN(b) ? 64 : ((b & 15) << 2) | (c >> 6)
      const i4 = isNaN(c) ? 64 : c & 63
      out += chars.charAt(i1) + chars.charAt(i2) + chars.charAt(i3) + chars.charAt(i4)
    }
    return out
  },

  // === Resume pending session ===
  async resumePendingSession() {
    const sid = this.data.pendingSession.id
    const status = this.data.pendingSession.status
    app.globalData.interviewSessionID = sid
    wx.setStorageSync('ia_interview_sid', sid)

    if (status === '面试进行中') {
      this.setData({ phase: PHASE.INTERVIEW, pendingSession: null })
      await this.restoreInterview()
    } else if (status === '等待上传简历') {
      this.setData({ phase: PHASE.RESUME, pendingSession: null })
    } else {
      // Try to figure out what phase we're in
      try {
        const r = await api.restoreSession(sid)
        if (r.code === 200 && r.data) {
          const s = r.data.status || ''
          if (s === 'resume_matching') this.setData({ phase: PHASE.RESUME, pendingSession: null })
          else this.setData({ phase: PHASE.JD, pendingSession: null })
        }
      } catch (e) {
        wx.showToast({ title: '恢复失败', icon: 'none' })
      }
    }
  },

  dismissPending() {
    app.globalData.interviewSessionID = ''
    wx.removeStorageSync('ia_interview_sid')
    this.setData({ pendingSession: null })
  },

  // === Interview ===
  async startInterview() {
    const prevPhase = this.data.phase
    this.setData({ phase: PHASE.INTERVIEW })
    try {
      const r = await api.startInterview(app.globalData.interviewSessionID)
      if (r.code === 200) {
        const data = r.data.data || r.data
        if (r.data.type === 'complete') {
          this.setData({ phase: PHASE.REPORT })
          this.loadReport()
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

  async restoreInterview() {
    const prevPhase = this.data.phase
    this.setData({ phase: PHASE.INTERVIEW, pendingSession: null })

    // Load previous messages from the session
    try {
      const msgR = await api.getMessages(app.globalData.interviewSessionID)
      if (msgR.code === 200 && msgR.data && msgR.data.length > 0) {
        const msgs = msgR.data.map(m => ({ role: m.role === 'user' ? 'user' : 'assistant', content: m.content }))
        this.setData({ messages: msgs })
      }
    } catch (e) { }

    try {
      const r = await api.startInterview(app.globalData.interviewSessionID)
      if (r.code === 200) {
        const data = r.data.data || r.data || ''
        const resumeMsg = data ? '🔄 面试已恢复。\n\n' + data : '🔄 面试已恢复。继续回答当前问题。'
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

  onAnswerInput(e) { this.setData({ answerText: e.detail.value }) },

  onKBChange(e) {
    this.setData({ kbHeight: e.detail.height })
  },

  async submitAnswer() {
    const text = this.data.answerText.trim()
    if (!text || this.data.submitting) return
    this.setData({ answerText: '', submitting: true })
    this.addMessage('user', text)
    this.setData({ typing: true })

    try {
      const r = await api.submitAnswer(app.globalData.interviewSessionID, text)
      this.setData({ typing: false })
      if (r.code === 200) {
        const d = r.data
        this.addMessage('assistant', d.data || d)
        if (d.type === 'complete') {
          wx.showToast({ title: '面试完成！', icon: 'success' })
          setTimeout(() => { this.setData({ phase: PHASE.REPORT }); this.loadReport() }, 800)
        }
      }
    } catch (e) {
      this.setData({ typing: false })
      wx.showToast({ title: '提交失败', icon: 'none' })
    }
    this.setData({ submitting: false })
  },

  async skipQ() {
    try { await api.skipQuestion(app.globalData.interviewSessionID); this.startInterview() } catch (e) { }
  },

  async endInterview() {
    const res = await new Promise(r => wx.showModal({ title: '结束面试', content: '确定要结束并生成报告吗？', success: r }))
    if (!res.confirm) return
    try {
      const r = await api.endInterview(app.globalData.interviewSessionID)
      if (r.code === 200) {
        this.setData({ phase: PHASE.REPORT })
        this.loadReport()
      }
    } catch (e) { wx.showToast({ title: '操作失败', icon: 'none' }) }
  },

  addMessage(role, content) {
    const msgs = [...this.data.messages, { role, content }]
    this.setData({ messages: msgs })
  },

  // === Report ===
  async loadReport() {
    try {
      const [reportR, planR] = await Promise.all([
        api.getReport(app.globalData.interviewSessionID),
        api.getReviewPlan(app.globalData.interviewSessionID)
      ])
      if (reportR.code === 200) this.setData({ report: reportR.data })
      if (planR.code === 200) this.setData({ plan: planR.data })
    } catch (e) { wx.showToast({ title: '加载报告失败', icon: 'none' }) }
  },

  viewPastReport(e) {
    const sid = e.currentTarget.dataset.sid
    wx.navigateTo({ url: '/pages/report/report?sid=' + sid })
  },

  async loadHistory() {
    try {
      const r = await api.listSessions()
      if (r.code === 200 && r.data) {
        const items = r.data.filter(s => s.overall_score > 0).slice(0, 20)
        this.setData({ historyList: items })
      }
    } catch (e) { }
  },

  newInterview() {
    app.globalData.interviewSessionID = ''
    wx.removeStorageSync('ia_interview_sid')
    this.setData({ phase: PHASE.JD, jdResult: null, resumeResult: null, messages: [], report: null, plan: null, pendingSession: null })
  },

  skipResume() {
    this.startInterview()
  }
})
