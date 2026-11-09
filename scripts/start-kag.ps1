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
    function Show-StartupDiagnostics {
        docker compose ps -a
        docker compose logs --since 10m --tail 100 backend postgres
        $container = docker compose ps -a -q backend
        if ($LASTEXITCODE -eq 0 -and $container) {
            docker inspect --format '{{json .State.Health}}' $container
        }
    }
    Write-Host 'Building images. Existing data volumes are preserved.'
    docker compose build backend worker frontend
    if ($LASTEXITCODE -ne 0) { throw 'Image build failed; startup stopped.' }
    # Backend performs existing-resource checks before it listens. Do not let
    # frontend's dependency check abort the whole update during this stage.
    docker compose up -d postgres redis neo4j backend
    if ($LASTEXITCODE -ne 0) {
        Show-StartupDiagnostics
        throw 'Backend/dependency startup failed. See logs above; do not remove data volumes.'
    }
    $backendReady = $false
    $deadline = [DateTime]::UtcNow.AddMinutes(5)
    $attempt = 0
    while ([DateTime]::UtcNow -lt $deadline) {
        $container = docker compose ps -a -q backend
        if ($LASTEXITCODE -ne 0 -or -not $container) { break }
        $raw = docker inspect --format '{{json .State}}' $container
        if ($LASTEXITCODE -ne 0) { break }
        $state = $raw | ConvertFrom-Json
        if ($state.Health -and $state.Health.Status -eq 'healthy') {
            $backendReady = $true
            break
        }
        if ($state.Status -eq 'exited' -or $state.Status -eq 'dead') { break }
        if ($attempt % 5 -eq 0) {
            Write-Host ('Waiting for backend initialization: container=' + $state.Status + '; health=' + $state.Health.Status + '. Existing-resource checks may take time.')
        }
        $attempt++
        Start-Sleep -Seconds 2
    }
    if (-not $backendReady) {
        Show-StartupDiagnostics
        throw 'Backend did not become healthy within 5 minutes. See logs above; no data volumes were removed. Do not repeatedly rebuild before checking the error.'
    }
    docker compose up -d worker frontend
    if ($LASTEXITCODE -ne 0) {
        Show-StartupDiagnostics
        throw 'Worker/frontend startup failed. Backend readiness was checked; see logs above.'
    }
    docker compose ps
    $expected = '2026.11.09.2'
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
