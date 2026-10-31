$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
    Write-Host 'Source revision:'
    git log -1 --oneline
    Write-Host 'Neo4j container status:'
    docker compose ps -a neo4j
    Write-Host 'Checking authentication inside Neo4j (does not print credentials):'
    docker compose exec -T neo4j sh -c 'cypher-shell -u neo4j -p "${NEO4J_AUTH#neo4j/}" "RETURN 1 AS ok"'
    if ($LASTEXITCODE -ne 0) { throw 'Neo4j check failed. Keep the data volume. Inspect the error and service logs; an existing database password is not reset by editing .env.' }
    Write-Host 'Checking recent graph synchronization errors:'
    docker compose logs --since 2m --tail 30 backend worker neo4j
} finally { Pop-Location }
