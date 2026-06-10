const api = require('../../utils/api')

const dimNames = {
  'technical_accuracy': '基础知识',
  'answer_depth': '回答深度',
  'communication': '沟通表达',
  'project_experience': '项目经验'
}

Page({
  data: { report: null, plan: null, loading: true, error: '', dims: [], highlights: [], weakAreas: [], reviews: [], planItems: [] },

  onLoad(options) {
    const sid = options.sid
    if (sid) this.loadReport(sid)
    else this.setData({ loading: false, error: '未指定会话ID' })
  },

  async loadReport(sid) {
    try {
      const [reportR, planR] = await Promise.all([
        api.getReport(sid),
        api.getReviewPlan(sid)
      ])

      if (reportR.code === 200) {
        const r = reportR.data
        // Pre-process dimension scores for WXML
        const dims = []
        if (r.dimension_score) {
          for (const k in r.dimension_score) {
            const raw = r.dimension_score[k]
            const score = Math.round(raw * 10)
            dims.push({
              name: dimNames[k] || k,
              score: score,
              pct: score,
              cls: score >= 80 ? 'high' : score >= 60 ? 'mid' : 'low'
            })
          }
        }
        this.setData({
          report: r,
          dims: dims,
          highlights: r.highlights || [],
          weakAreas: r.weak_areas || [],
          reviews: r.question_reviews || [],
          score100: r.score_100 || Math.round((r.overall_score || 0) * 10),
          grade: r.grade || '',
          summary: r.summary || ''
        })
      }

      if (planR.code === 200) {
        this.setData({ plan: planR.data })
      }

      if (reportR.code !== 200 && planR.code !== 200) {
        this.setData({ error: reportR.message || '报告加载失败' })
      }
    } catch (e) {
      this.setData({ error: '网络错误' })
    }
    this.setData({ loading: false })
  }
})
