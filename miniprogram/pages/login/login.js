const api = require('../../utils/api')

Page({
  data: {
    loading: false,
    errorMsg: ''
  },

  onLoad() {
    const app = getApp()
    if (app.globalData.token) {
      wx.switchTab({ url: '/pages/chat/chat' })
    }
  },

  async handleWeChatLogin() {
    if (this.data.loading) return
    this.setData({ loading: true, errorMsg: '' })

    try {
      const loginRes = await new Promise((resolve, reject) => {
        wx.login({
          success: resolve,
          fail: reject
        })
      })

      if (!loginRes.code) {
        throw new Error('获取登录凭证失败，请重试')
      }

      const res = await api.wechatLogin(loginRes.code)

      if (res.code === 200 && res.data) {
        const { token, user_id, username } = res.data
        const app = getApp()
        app.globalData.token = token
        wx.setStorageSync('ia_token', token)
        wx.setStorageSync('ia_user_id', user_id)
        wx.setStorageSync('ia_username', username)

        wx.switchTab({ url: '/pages/chat/chat' })
      } else {
        throw new Error(res.message || '登录失败，请重试')
      }
    } catch (err) {
      console.error('[Login] error:', err)
      this.setData({
        errorMsg: err.message || '登录失败，请重试',
        loading: false
      })
    }
  }
})
