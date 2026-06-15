const api = require('../../utils/api')

const dimNames = {
  'technical_accuracy': '基础知识',
  'answer_depth': '回答深度',
  'communication': '沟通表达',
  'project_experience': '项目经验'
}

function preprocessReport(r) {
  var dims = []
  if (r.dimension_score) {
    for (var k in r.dimension_score) {
      var raw = r.dimension_score[k]
      var score = Math.round(raw * 10)
      dims.push({
        name: dimNames[k] || k,
        score: score,
        pct: score,
        cls: score >= 80 ? 'high' : score >= 60 ? 'mid' : 'low'
      })
    }
  }
  var reviews = r.question_reviews || []
  if (!reviews.length && r.evaluations && r.evaluations.length) {
    for (var i = 0; i < r.evaluations.length; i++) {
      var ev = r.evaluations[i]
      var text = '第' + (i+1) + '题 (' + Math.round(ev.total_score*10) + '分)'
      if (ev.praise) text += '\n✅ 亮点：' + ev.praise
      if (ev.issues) text += '\n⚠️ 不足：' + ev.issues
      if (ev.improvement) text += '\n💡 建议：' + ev.improvement
      reviews.push(text)
    }
  }
  var score100 = r.score_100 || Math.round((r.overall_score || 0) * 10)
  return {
    report: r,
    dims: dims,
    reviews: reviews,
    score100: score100,
    scoreClass: score100 >= 80 ? 'high' : score100 >= 60 ? 'mid' : 'low',
    grade: r.grade || '',
    highlights: r.highlights || [],
    weakAreas: r.weak_areas || [],
    summary: r.summary || ''
  }
}

function preprocessPlan(p) {
  if (!p) return null
  return {
    plan: p,
    hasPlanItems: !!(p.plan_items && p.plan_items.length),
    hasResources: !!(p.resources && p.resources.length),
    planItems: (p.plan_items || []).map(function(it) {
      return {
        topic: it.topic,
        priority: it.priority,
        estimated_hours: it.estimated_hours,
        description: it.description,
        priIcon: it.priority === 'high' ? '🔴' : it.priority === 'medium' ? '🟡' : '🟢',
        priClass: it.priority
      }
    })
  }
}

Page({
  data: {
    report: null,
    planData: null,
    loading: true,
    error: '',
    dims: [],
    highlights: [],
    weakAreas: [],
    reviews: [],
    score100: 0,
    scoreClass: '',
    grade: '',
    summary: ''
  },

  onLoad: function(options) {
    var sid = options.sid
    if (sid) this.loadReport(sid)
    else this.setData({ loading: false, error: '未指定会话ID' })
  },

  loadReport: async function(sid) {
    try {
      var results = await Promise.all([
        api.getReport(sid),
        api.getReviewPlan(sid)
      ])
      var reportR = results[0]
      var planR = results[1]
      var setDataObj = {}

      if (reportR.code === 200) {
        var proc = preprocessReport(reportR.data)
        for (var k in proc) { setDataObj[k] = proc[k] }
      }

      if (planR.code === 200) {
        setDataObj.planData = preprocessPlan(planR.data)
      }

      if (reportR.code !== 200 && planR.code !== 200) {
        setDataObj.error = reportR.message || '报告加载失败'
      }
      setDataObj.loading = false
      this.setData(setDataObj)
    } catch (e) {
      this.setData({ loading: false, error: '网络错误' })
    }
  }
})
