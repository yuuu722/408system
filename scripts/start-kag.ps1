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
[IO.File]::WriteAllText($path, $text, (New-Object Text.UTF8Encoding($false)))
Push-Location $root
try {
    docker compose up -d --build
    if ($LASTEXITCODE -ne 0) { throw 'Docker Compose failed. Check the service logs.' }
    docker compose ps
} finally { Pop-Location }
