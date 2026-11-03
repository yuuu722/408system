param(
    [string]$Model = 'qwen2.5:3b',
    [string]$BaseUrl = 'http://127.0.0.1:11434'
)
$ErrorActionPreference = 'Stop'
$endpoint = [Uri]$BaseUrl
if ($endpoint.Scheme -ne 'http' -or $endpoint.Host -notin @('localhost', '127.0.0.1', '[::1]', '::1')) {
    throw 'This diagnostic only accepts a local Ollama HTTP endpoint.'
}
$base = $BaseUrl.TrimEnd('/')
function Safe-ModelFailure($failure) {
    $raw = [string]$failure.ErrorDetails.Message
    if ($raw -match '(?i)requires more system memory|out of memory|unable to allocate|not enough memory|insufficient memory') { return '服务报告内存或显存不足；请查看 Ollama 服务日志并调整资源或模型。' }
    if ($raw -match '(?i)model.*not found') { return '模型未找到；请核对已安装模型及完整名称。' }
    if ($raw -match '(?i)runner process has terminated|failed to load model|unable to load model') { return '模型进程启动或运行失败；请查看 Ollama 服务日志。' }
    if ($raw -match '(?i)response_format|json_object') { return '结构化输出请求失败；请核对 Ollama 版本与兼容接口支持。' }
    $status = ''
    if ($failure.Exception.Response) { $status = [string][int]$failure.Exception.Response.StatusCode }
    if ($status) { return "模型接口返回 HTTP $status；具体原因需查看 Ollama 服务日志。" }
    return '连接失败或请求超时；请确认 Ollama 正在运行且监听指定本地端口。'
}
try {
    $tags = Invoke-RestMethod -Uri "$base/api/tags" -TimeoutSec 10
    $installed = @($tags.models | ForEach-Object { $_.name })
    $running = Invoke-RestMethod -Uri "$base/api/ps" -TimeoutSec 10
} catch {
    Write-Output (Safe-ModelFailure $_)
    exit 1
}
$report = [ordered]@{
    Model = $Model
    Installed = ($installed -contains $Model)
    InstalledModels = $installed
    LoadedModels = @($running.models | ForEach-Object { $_.name })
    Note = 'LoadedModels为空表示当时没有驻留模型，不等于未安装。检测将短暂加载所选模型。'
    Checks = @()
}
if (-not $report.Installed) {
    $report.Checks += @{ Name = '模型名称'; Passed = $false; Message = '所选名称不在已安装列表中；请使用上方完整模型名。' }
    $report | ConvertTo-Json -Depth 8
    exit 1
}
foreach ($structured in @($false, $true)) {
    $name = if ($structured) { 'JSON兼容接口' } else { '普通兼容接口' }
    $prompt = if ($structured) { '只返回JSON对象：{"ok":true}' } else { '仅回复：连接正常' }
    $request = @{ model = $Model; stream = $false; temperature = 0; max_tokens = 64; messages = @(@{role = 'user'; content = $prompt}) }
    if ($structured) { $request.response_format = @{ type = 'json_object' } }
    try {
        $response = Invoke-RestMethod -Method Post -Uri "$base/v1/chat/completions" -ContentType 'application/json; charset=utf-8' -Body ($request | ConvertTo-Json -Depth 8) -TimeoutSec 180
        $text = [string]$response.choices[0].message.content
        $valid = -not [string]::IsNullOrWhiteSpace($text)
        if ($structured -and $valid) { $valid = (($text | ConvertFrom-Json).ok -eq $true) }
        if ($response.choices[0].finish_reason -eq 'length') { $valid = $false }
        $message = if ($valid) { '接口返回有效结果。' } else { '接口返回空、截断或不符合要求的结果。' }
        $report.Checks += @{ Name = $name; Passed = $valid; Message = $message }
    } catch {
        $report.Checks += @{ Name = $name; Passed = $false; Message = (Safe-ModelFailure $_) }
    }
}
$report | ConvertTo-Json -Depth 8
if (@($report.Checks | Where-Object { -not $_.Passed }).Count -gt 0) { exit 1 }
