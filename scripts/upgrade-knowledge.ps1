param([ValidateRange(1,2048)][int]$MaxUploadMB = 200)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $root
$envPath = Join-Path $root '.env'
if (-not (Test-Path -LiteralPath $envPath)) {
    throw 'Missing .env. Complete the initial database/JWT configuration first.'
}
# Backups can contain credentials: keep them under the existing ignored cache.
$backupDir = Join-Path $root '.cache/config-backups'
New-Item -ItemType Directory -Force -Path $backupDir | Out-Null
Copy-Item -LiteralPath $envPath -Destination (Join-Path $backupDir ('.env.' + [guid]::NewGuid().ToString('N')))
$content = [IO.File]::ReadAllText($envPath)
$settings = [ordered]@{
    MAX_UPLOAD_MB = "$MaxUploadMB"
    EMBEDDING_MODE = 'lexical'
    DOCUMENT_PARSER_MODE = 'builtin'
    PERSONAL_KB_ENABLED = 'false'
    AI_DEMO_MODE = 'false'
    GRAPH_EXTRACTION_ENABLED = 'false'
    DOCUMENT_JOB_TIMEOUT = '2h'
}
foreach ($name in $settings.Keys) {
    $pattern = '(?m)^' + [regex]::Escape($name) + '[ \t]*=.*(?:\r?\n|$)'
    $content = [regex]::Replace($content, $pattern, '')
    $content = $content.TrimEnd() + "`r`n$name=$($settings[$name])`r`n"
}
[IO.File]::WriteAllText($envPath, $content, [Text.UTF8Encoding]::new($false))
Write-Host 'Updated knowledge-library settings. Existing JWT, database and account credentials were preserved.'
& docker compose up -d --build
if ($LASTEXITCODE -ne 0) { throw 'Docker rebuild failed. Inspect the preceding build output.' }
Write-Host 'Rebuild complete. Sign in again. Administrators can retry failed documents from Public Resources.'
