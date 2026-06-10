App({
  globalData: {
    token: '',
    baseURL: 'http://127.0.0.1:8080/api',
    wsURL: 'ws://127.0.0.1:8080/ws',
    chatSessionID: '',
    interviewSessionID: '',
    skillSessionID: '',
    userId: '',
    currentMode: 'chat'
  },

  onLaunch() {
    // Restore session from storage
    const token = wx.getStorageSync('ia_token')
    if (token) this.globalData.token = token
    this.globalData.chatSessionID = wx.getStorageSync('ia_chat_sid') || ''
    this.globalData.interviewSessionID = wx.getStorageSync('ia_interview_sid') || ''
    this.globalData.skillSessionID = wx.getStorageSync('ia_skill_sid') || ''
  },

  getUserId() {
    try {
      if (this.globalData.token) {
        const payload = this.globalData.token.split('.')[1]
        const decoded = this.base64Decode(payload)
        const json = JSON.parse(decoded)
        return json.username || json.user_id || 'anonymous'
      }
    } catch (e) { }
    return 'anonymous'
  },

  base64Decode(str) {
    // Base64 decode for mini program (no atob available)
    const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/='
    let output = ''
    str = str.replace(/[^A-Za-z0-9\+\/\=]/g, '')
    let i = 0
    while (i < str.length) {
      const idx1 = chars.indexOf(str.charAt(i++))
      const idx2 = chars.indexOf(str.charAt(i++))
      const idx3 = chars.indexOf(str.charAt(i++))
      const idx4 = chars.indexOf(str.charAt(i++))
      const a = (idx1 << 2) | (idx2 >> 4)
      const b = ((idx2 & 15) << 4) | (idx3 >> 2)
      const c = ((idx3 & 3) << 6) | idx4
      output += String.fromCharCode(a)
      if (idx3 !== 64) output += String.fromCharCode(b)
      if (idx4 !== 64) output += String.fromCharCode(c)
    }
    return decodeURIComponent(escape(output))
  }
})

