param([switch]$EnablePersonal, [switch]$EnableGraphExtraction)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$path = Join-Path $root '.env'
if (-not (Test-Path $path)) { Copy-Item (Join-Path $root '.env.example') $path }
$text = [IO.File]::ReadAllText($path)
function New-LocalSecret {
    $bytes = New-Object byte[] 32
    $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
    return [BitConverter]::ToString($bytes).Replace('-', '').ToLowerInvariant()
}
foreach ($key in @('JWT_SECRET', 'NEO4J_PASSWORD')) {
    $pattern = '(?m)^' + $key + '=(.*)$'
    $match = [regex]::Match($text, $pattern)
    $value = if ($match.Success) { $match.Groups[1].Value.Trim().Trim('"').Trim("'") } else { '' }
    if ([string]::IsNullOrWhiteSpace($value) -or $value.StartsWith('change-this')) {
        $line = $key + '=' + (New-LocalSecret)
        if ($match.Success) { $text = [regex]::Replace($text, $pattern, $line) }
        else { $text = $text.TrimEnd() + "`r`n" + $line + "`r`n" }
    }
}
if ($EnablePersonal) {
    $pattern = '(?m)^PERSONAL_KB_ENABLED=.*$'
    if ([regex]::IsMatch($text, $pattern)) { $text = [regex]::Replace($text, $pattern, 'PERSONAL_KB_ENABLED=true') }
    else { $text = $text.TrimEnd() + "`r`nPERSONAL_KB_ENABLED=true`r`n" }
    Write-Host 'Personal libraries enabled; per-member quotas still apply.'
}
if ($EnableGraphExtraction) {
    $pattern = '(?m)^GRAPH_EXTRACTION_ENABLED=.*$'
    if ([regex]::IsMatch($text, $pattern)) { $text = [regex]::Replace($text, $pattern, 'GRAPH_EXTRACTION_ENABLED=true') }
    else { $text = $text.TrimEnd() + "`r`nGRAPH_EXTRACTION_ENABLED=true`r`n" }
    Write-Host 'Model graph extraction enabled for subsequent document processing; existing graph data is preserved. A configured LLM is required.'
}
[IO.File]::WriteAllText($path, $text, (New-Object Text.UTF8Encoding($false)))
Push-Location $root
try {
    docker compose up -d --build
    if ($LASTEXITCODE -ne 0) { throw 'Docker Compose failed. Check the service logs.' }
    docker compose ps
    $expected = '2026.11.03.1'
    $verified = $false
    for ($attempt = 0; $attempt -lt 10; $attempt++) {
        try {
            $health = Invoke-RestMethod -Uri 'http://localhost:8080/healthz' -TimeoutSec 5
            $page = Invoke-WebRequest -Uri 'http://localhost:5173/' -UseBasicParsing -TimeoutSec 5
            if ($health.release -eq $expected -and $page.Content.Contains('name="app-release" content="' + $expected + '"')) {
                $verified = $true
                break
            }
        } catch { }
        Start-Sleep -Seconds 1
    }
    if (-not $verified) { throw 'Build completed, but the running frontend/backend release could not be verified. Check ports 5173/8080 and container logs; do not remove data volumes.' }
    Write-Host ('Frontend and backend release verified: ' + $expected)
    Write-Host 'Open http://localhost:5173 and press Ctrl+F5. Graph readiness is checked separately with scripts/check-graph.ps1.'
} finally { Pop-Location }
