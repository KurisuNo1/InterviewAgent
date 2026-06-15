const api = require('../../utils/api')
const app = getApp()

function preprocessChatHistory(s) {
  var preview = (s.last_message || '').replace(/\n/g, ' ').substring(0, 40)
  if (!preview) preview = '(无消息)'
  var metaText = (s.created_at || '').substring(5, 16)
  if (s.overall_score > 0) {
    metaText += ' · ' + Number(s.overall_score).toFixed(1) + '分'
  }
  return { id: s.id, preview: preview, metaText: metaText }
}

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

  onLoad: function() {
    this.initChat()
  },

  onShow: function() {
    if (!app.globalData.token) {
      wx.reLaunch({ url: '/pages/login/login' })
      return
    }
    if (this.data.showHistory) this.loadHistory()
  },

  initChat: async function() {
    var sid = app.globalData.chatSessionID
    if (!sid) return
    try {
      var r = await api.getMessages(sid)
      if (r.code === 200 && r.data && r.data.length > 0) {
        var msgs = []
        for (var i = 0; i < r.data.length; i++) {
          msgs.push({
            role: r.data[i].role === 'user' ? 'user' : 'assistant',
            content: r.data[i].content
          })
        }
        this.setData({ messages: msgs, scrollToId: 'msg-' + (msgs.length - 1) })
      }
    } catch (e) { }
  },

  onInput: function(e) {
    this.setData({ inputText: e.detail.value })
  },

  onKBChange: function(e) {
    this.setData({ kbHeight: e.detail.height })
    if (e.detail.height > 0) this.scrollToBottom()
  },

  sendMessage: async function() {
    var text = this.data.inputText.trim()
    if (!text || this.data.sending) return
    this.setData({ inputText: '', sending: true })

    var msgs = this.data.messages.concat([{ role: 'user', content: text }])
    this.setData({ messages: msgs, scrollToId: 'msg-' + (msgs.length - 1) })

    if (!app.globalData.chatSessionID) {
      try {
        var r = await api.createSession()
        app.globalData.chatSessionID = r.data.id
        wx.setStorageSync('ia_chat_sid', r.data.id)
      } catch (e) {
        wx.showToast({ title: '创建会话失败', icon: 'none', duration: 3000 })
        this.setData({ sending: false })
        return
      }
    }

    this.setData({ typing: true, scrollToId: 'msg-' + msgs.length })

    try {
      var r2 = await api.sendMessage(app.globalData.chatSessionID, text)
      this.setData({ typing: false })
      if (r2.code === 200) {
        var reply = r2.data.reply || r2.data.data || ''
        var newMsgs = this.data.messages.concat([{ role: 'assistant', content: reply }])
        this.setData({ messages: newMsgs, scrollToId: 'msg-' + (newMsgs.length - 1) })
      }
    } catch (e) {
      this.setData({ typing: false })
      wx.showToast({ title: '发送失败', icon: 'none' })
    }
    this.setData({ sending: false })
  },

  newChat: function() {
    app.globalData.chatSessionID = ''
    wx.removeStorageSync('ia_chat_sid')
    this.setData({ messages: [] })
  },

  scrollToBottom: function() {
    var len = this.data.messages.length
    this.setData({ scrollToId: len > 0 ? 'msg-' + (len - 1) : 'msg-bottom' })
  },

  showHistoryPanel: async function() {
    this.setData({ showHistory: true })
    await this.loadHistory()
  },

  hideHistoryPanel: function() {
    this.setData({ showHistory: false })
  },

  loadHistory: async function() {
    try {
      var r = await api.listSessions()
      if (r.code === 200 && r.data) {
        var items = []
        for (var i = 0; i < r.data.length; i++) {
          if (r.data[i].last_message || r.data[i].status !== 'created') {
            items.push(preprocessChatHistory(r.data[i]))
          }
        }
        items = items.slice(0, 30)
        this.setData({ historyList: items })
      }
    } catch (e) { }
  },

  restoreChatSession: async function(e) {
    var sid = e.currentTarget.dataset.sid
    try {
      var r = await api.getMessages(sid)
      if (r.code === 200 && r.data && r.data.length > 0) {
        app.globalData.chatSessionID = sid
        wx.setStorageSync('ia_chat_sid', sid)
        var msgs = []
        for (var i = 0; i < r.data.length; i++) {
          msgs.push({
            role: r.data[i].role === 'user' ? 'user' : 'assistant',
            content: r.data[i].content
          })
        }
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
