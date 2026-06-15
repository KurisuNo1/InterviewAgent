const api = require('../../utils/api')

const app = getApp()

Page({
  data: { documents: [], loading: false },

  onLoad() { this.loadDocs() },

  onShow() {
    if (!app.globalData.token) {
      wx.reLaunch({ url: '/pages/login/login' })
      return
    }
  },

  async loadDocs() {
    this.setData({ loading: true })
    try {
      const r = await api.listDocuments()
      if (r.code === 200 && r.data) {
        this.setData({ documents: r.data, loading: false })
      }
    } catch (e) { wx.showToast({ title: '加载失败', icon: 'none' }) }
    this.setData({ loading: false })
  },

  async chooseFile() {
    const that = this
    wx.chooseMessageFile({
      count: 3,
      type: 'file',
      success(res) {
        that.uploadFiles(res.tempFiles)
      }
    })
  },

  async uploadFiles(files) {
    wx.showLoading({ title: '上传中...' })
    const uploads = []
    const fs = wx.getFileSystemManager()
    for (const f of files) {
      const data = fs.readFileSync(f.path, 'base64')
      uploads.push({ file_name: f.name, content: data })
    }
    try {
      await api.request('POST', '/documents/upload', { files: uploads }, 120000)
      wx.hideLoading()
      wx.showToast({ title: '上传成功', icon: 'success' })
      this.loadDocs()
    } catch (e) {
      wx.hideLoading()
      wx.showToast({ title: '上传失败', icon: 'none' })
    }
  },

  async deleteDoc(e) {
    const id = e.currentTarget.dataset.id
    const res = await new Promise(r => wx.showModal({ title: '确认删除', content: '确定要删除这个文档吗？', success: r }))
    if (!res.confirm) return
    try {
      await api.deleteDocument(id)
      wx.showToast({ title: '已删除', icon: 'success' })
      this.loadDocs()
    } catch (e) { wx.showToast({ title: '删除失败', icon: 'none' }) }
  }
})
