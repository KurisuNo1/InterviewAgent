const app = getApp()

function request(method, path, data, timeout = 30000) {
  return new Promise((resolve, reject) => {
    const headers = { 'Content-Type': 'application/json' }
    if (app.globalData.token) {
      headers['Authorization'] = 'Bearer ' + app.globalData.token
    }
    const url = app.globalData.baseURL + path
    console.log('[API]', method, url)
    wx.request({
      url: url,
      method: method,
      header: headers,
      data: data,
      timeout: timeout,
      success(res) {
        console.log('[API]', method, url, '->', res.statusCode)
        resolve(res.data)
      },
      fail(err) {
        console.error('[API]', method, url, 'FAILED:', err.errMsg || err)
        reject(err)
      }
    })
  })
}

// 登录
function login(username, password) {
  return request('POST', '/auth/login', { username, password })
}

function register(username, password) {
  return request('POST', '/auth/register', { username, password })
}

function wechatLogin(code) {
  return request('POST', '/auth/wechat-login', { code })
}

// Session
function createSession() {
  return request('POST', '/sessions', { user_id: app.getUserId() })
}

function restoreSession(sid) {
  return request('POST', '/sessions/' + sid + '/restore')
}

// 聊天（非流式回退）
function sendMessage(sid, message) {
  return request('POST', '/sessions/' + sid + '/message', { message }, 60000)
}

// 面试
function parseJD(sid, jdText) {
  return request('POST', '/sessions/' + sid + '/jd', { jd_text: jdText }, 90000)
}

function uploadResume(sid, fileName, fileData) {
  return request('POST', '/sessions/' + sid + '/resume', { file_name: fileName, file_data: fileData }, 90000)
}

function startInterview(sid) {
  return request('POST', '/sessions/' + sid + '/start', null, 60000)
}

function submitAnswer(sid, answer) {
  return request('POST', '/sessions/' + sid + '/answer', { answer }, 120000)
}

function skipQuestion(sid) {
  return request('POST', '/sessions/' + sid + '/skip')
}

function endInterview(sid) {
  return request('POST', '/sessions/' + sid + '/complete', null, 60000)
}

function getReport(sid) {
  return request('GET', '/sessions/' + sid + '/report')
}

function getReviewPlan(sid) {
  return request('GET', '/sessions/' + sid + '/review-plan')
}

function listSessions() {
  return request('GET', '/sessions?user_id=' + encodeURIComponent(app.getUserId()))
}

function getMessages(sid) {
  return request('GET', '/sessions/' + sid + '/messages')
}

// 技能
function listSkills() {
  return request('GET', '/skills')
}

// 知识库
function listDocuments() {
  return request('GET', '/documents')
}

function deleteDocument(id) {
  return request('DELETE', '/documents/' + id)
}

// WebSocket 连接
let wsTask = null
let wsCallbacks = {}

function connectWebSocket() {
  wsTask = wx.connectSocket({
    url: app.globalData.wsURL,
    header: app.globalData.token ? { 'Authorization': 'Bearer ' + app.globalData.token } : {},
    success() { console.log('[WS] connected') },
    fail(err) { console.error('[WS] connect failed', err) }
  })

  wsTask.onMessage((res) => {
    const data = JSON.parse(res.data)
    if (wsCallbacks.onMessage) wsCallbacks.onMessage(data)
  })

  wsTask.onClose(() => {
    if (wsCallbacks.onClose) wsCallbacks.onClose()
  })

  wsTask.onError((err) => {
    console.error('[WS] error', err)
  })
}

function onWSMessage(cb) { wsCallbacks.onMessage = cb }
function onWSClose(cb) { wsCallbacks.onClose = cb }
function sendWS(data) {
  if (wsTask) wsTask.send({ data: JSON.stringify(data) })
}
function closeWS() {
  if (wsTask) wsTask.close()
}

module.exports = {
  request, login, register, wechatLogin,
  createSession, restoreSession, sendMessage,
  parseJD, uploadResume, startInterview, submitAnswer,
  skipQuestion, endInterview, getReport, getReviewPlan,
  listSessions, getMessages, listSkills, listDocuments, deleteDocument,
  connectWebSocket, onWSMessage, onWSClose, sendWS, closeWS
}
