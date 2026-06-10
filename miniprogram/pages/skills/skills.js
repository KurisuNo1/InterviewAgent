const api = require('../../utils/api')
const app = getApp()

const skillMeta = {
  'quick_quiz': { name: '快速测验', icon: '🎯', desc: '针对指定技术主题出5道题测试知识水平并评分', category: '面试技能' },
  'knowledge_explain': { name: '知识讲解', icon: '📖', desc: '逐层深入讲解技术概念', category: '面试技能' },
  'project_highlight': { name: '项目亮点提炼', icon: '⭐', desc: '分4阶段提炼面试可用项目亮点', category: '面试技能' },
  'tech_compare': { name: '技术对比', icon: '⚖️', desc: '四维度对比两项技术', category: '面试技能' },
  'algorithm': { name: '算法练习', icon: '💻', desc: 'LeetCode风格算法编程训练', category: '专项训练' },
  'system_design': { name: '系统设计', icon: '🏗️', desc: '系统设计面试模拟训练', category: '专项训练' },
  'behavioral': { name: '行为面试', icon: '🗣️', desc: 'STAR法则行为面试练习', category: '专项训练' },
  'tech_quiz': { name: '技术测验', icon: '📝', desc: '10道技术栈知识问答', category: '专项训练' },
}

Page({
  data: {
    coreSkills: [],
    trainingSkills: [],
    activeSkill: null,
    skillMessages: [],
    skillInput: '',
    skillSending: false,
    kbHeight: 0
  },

  onLoad() { this.loadSkills() },

  onShow() {
    if (!app.globalData.token) {
      wx.reLaunch({ url: '/pages/login/login' })
      return
    }
  },

  async loadSkills() {
    try {
      const r = await api.listSkills()
      if (r.code === 200 && r.data) {
        const core = []; const training = []
        r.data.forEach(s => {
          const meta = skillMeta[s.name] || { name: s.name, icon: '📦', desc: s.description, category: '' }
          const card = { ...s, displayName: meta.name, icon: meta.icon, displayDesc: meta.desc, cat: meta.category }
          if (s.category === 'core' || meta.category === '面试技能') core.push(card)
          else training.push(card)
        })
        this.setData({ coreSkills: core, trainingSkills: training })
      }
    } catch (e) { wx.showToast({ title: '加载失败', icon: 'none' }) }
  },

  async startSkill(e) {
    const name = e.currentTarget.dataset.name
    const meta = skillMeta[name] || { name, icon: '📦' }
    this.setData({ activeSkill: name, skillMessages: [{ role: 'assistant', content: '正在准备练习...' }], skillInput: '' })

    if (!app.globalData.skillSessionID) {
      const r = await api.createSession()
      app.globalData.skillSessionID = r.data.id
      wx.setStorageSync('ia_skill_sid', r.data.id)
    }

    try {
      const r = await api.sendMessage(app.globalData.skillSessionID, 'skill:' + name + ':start')
      if (r.code === 200) {
        const msgs = [{ role: 'assistant', content: r.data.reply }]
        this.setData({ skillMessages: msgs })
      }
    } catch (e) { wx.showToast({ title: '启动失败', icon: 'none' }) }
  },

  onSkillInput(e) { this.setData({ skillInput: e.detail.value }) },

  onKBChange(e) {
    this.setData({ kbHeight: e.detail.height })
  },

  async submitSkillAnswer() {
    const text = this.data.skillInput.trim()
    if (!text) return
    const msgs = [...this.data.skillMessages, { role: 'user', content: text }]
    this.setData({ skillMessages: msgs, skillInput: '', skillSending: true })

    try {
      const r = await api.sendMessage(app.globalData.skillSessionID, 'skill:' + this.data.activeSkill + ':' + text)
      if (r.code === 200) {
        this.setData({ skillMessages: [...this.data.skillMessages, { role: 'assistant', content: r.data.reply }] })
      }
    } catch (e) { wx.showToast({ title: '发送失败', icon: 'none' }) }
    this.setData({ skillSending: false })
  },

  backToList() { this.setData({ activeSkill: null, skillMessages: [] }) }
})
