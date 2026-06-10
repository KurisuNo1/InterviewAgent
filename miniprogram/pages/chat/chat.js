const api = require('../../utils/api')
const app = getApp()

Page({
  data: {
    messages: [],
    inputText: '',
    sending: false,
    typing: false,
    scrollToId: '',
    kbHeight: 0,
    showHistory: false,
    historyList: []
  },

  onLoad() {
    this.initChat()
  },

  onShow() {
    if (!app.globalData.token) {
      wx.reLaunch({ url: '/pages/login/login' })
      return
    }
    // Refresh history list when tab is shown
    if (this.data.showHistory) this.loadHistory()
  },

  async initChat() {
    const sid = app.globalData.chatSessionID
    if (sid) {
      try {
        const r = await api.getMessages(sid)
        if (r.code === 200 && r.data && r.data.length > 0) {
          const msgs = r.data.map(m => ({ role: m.role === 'user' ? 'user' : 'assistant', content: m.content }))
          this.setData({ messages: msgs, scrollToId: 'msg-' + (msgs.length - 1) })
        }
      } catch (e) { }
    }
  },

  onInput(e) {
    this.setData({ inputText: e.detail.value })
  },

  onKBChange(e) {
    this.setData({ kbHeight: e.detail.height })
    if (e.detail.height > 0) {
      this.scrollToBottom()
    }
  },

  async sendMessage() {
    const text = this.data.inputText.trim()
    if (!text || this.data.sending) return

    this.setData({ inputText: '', sending: true })

    const msgs = [...this.data.messages, { role: 'user', content: text }]
    this.setData({ messages: msgs, scrollToId: 'msg-' + (msgs.length - 1) })

    if (!app.globalData.chatSessionID) {
      try {
        const r = await api.createSession()
        app.globalData.chatSessionID = r.data.id
        wx.setStorageSync('ia_chat_sid', r.data.id)
      } catch (e) {
        wx.showToast({ title: '创建会话失败', icon: 'none', duration: 3000 })
        this.setData({ sending: false })
        return
      }
    }

    this.setData({ typing: true, scrollToId: 'msg-' + (msgs.length) })

    try {
      const r = await api.sendMessage(app.globalData.chatSessionID, text)
      this.setData({ typing: false })
      if (r.code === 200) {
        const reply = r.data.reply || r.data.data || ''
        const newMsgs = [...this.data.messages, { role: 'assistant', content: reply }]
        this.setData({ messages: newMsgs, scrollToId: 'msg-' + (newMsgs.length - 1) })
      }
    } catch (e) {
      this.setData({ typing: false })
      wx.showToast({ title: '发送失败', icon: 'none' })
    }
    this.setData({ sending: false })
  },

  async newChat() {
    app.globalData.chatSessionID = ''
    wx.removeStorageSync('ia_chat_sid')
    this.setData({ messages: [] })
  },

  scrollToBottom() {
    const len = this.data.messages.length
    this.setData({ scrollToId: len > 0 ? 'msg-' + (len - 1) : 'msg-bottom' })
  },

  // === History ===
  async showHistoryPanel() {
    this.setData({ showHistory: true })
    await this.loadHistory()
  },

  hideHistoryPanel() {
    this.setData({ showHistory: false })
  },

  async loadHistory() {
    try {
      const r = await api.listSessions()
      if (r.code === 200 && r.data) {
        const items = r.data.filter(s => s.last_message || s.status !== 'created').slice(0, 30)
        this.setData({ historyList: items })
      }
    } catch (e) { }
  },

  async restoreChatSession(e) {
    const sid = e.currentTarget.dataset.sid
    try {
      const r = await api.getMessages(sid)
      if (r.code === 200 && r.data && r.data.length > 0) {
        app.globalData.chatSessionID = sid
        wx.setStorageSync('ia_chat_sid', sid)
        const msgs = r.data.map(m => ({ role: m.role === 'user' ? 'user' : 'assistant', content: m.content }))
        this.setData({ messages: msgs, scrollToId: 'msg-' + (msgs.length - 1), showHistory: false })
        wx.showToast({ title: '会话已恢复', icon: 'success' })
      } else {
        wx.showToast({ title: '该会话无消息记录（可能已过期）', icon: 'none' })
      }
    } catch (e) {
      wx.showToast({ title: '恢复失败', icon: 'none' })
    }
  }
})
