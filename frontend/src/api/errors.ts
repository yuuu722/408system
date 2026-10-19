const translations: Record<string, string> = {
  'invalid credentials': '用户名、邮箱或密码不正确，请检查后重试',
  'username, email, or password is invalid': '注册信息不符合要求，请检查用户名、邮箱和密码',
  'authentication required': '请先登录后再操作',
  'invalid or expired token': '登录状态已失效，请重新登录',
  'internal server error': '服务器暂时出现问题，请稍后重试',
  'not found': '请求的内容不存在或已被删除',
  'name is required': '请输入名称',
  'question is required': '请输入问题',
  'only TXT, Markdown, and PDF are supported': '仅支持 TXT、Markdown 和 PDF 文件',
  'document is already processing': '资料正在处理中，请等待完成',
  'document is not valid UTF-8': '文件编码不受支持，请另存为 UTF-8 编码后上传',
  'document contains no extractable text': '未能提取文字，请上传可复制文字的资料',
  'Network Error': '无法连接服务器，请检查网络后重试',
}

export function errorMessage(raw: unknown, status?: number): string {
  const english = typeof raw === 'string' ? raw.trim() : ''
  if (translations[english]) return `${translations[english]}（${english}）`
  // Preserve the backend's actionable Chinese model errors and existing bilingual text.
  if (/[\u3400-\u9fff]/u.test(english)) return english
  if (/timeout|timed out/i.test(english)) return '请求超时，请稍后重试（Request timed out）'
  if (/^file size must be between/.test(english)) return `文件大小超出允许范围，请检查后重新上传（${english}）`
  const fallback: Record<number, string> = {
    400: '提交的信息有误，请检查后重试（Invalid request）',
    401: '身份验证失败，请重新登录（Authentication failed）',
    403: '你没有权限执行此操作（Access denied）',
    404: '请求的内容不存在或已被删除（Not found）',
    413: '上传文件过大，请缩小文件后重试（File too large）',
    429: '操作过于频繁，请稍后重试（Too many requests）',
    502: '上游服务暂时不可用，请稍后重试（Bad gateway）',
    503: '服务暂时不可用，请稍后重试（Service unavailable）',
    504: '服务响应超时，请稍后重试（Gateway timeout）',
  }
  return fallback[status ?? 0] || '请求失败，请稍后重试（Request failed）'
}
