param(
    [string]$OutputDirectory = ".backups"
)

$ErrorActionPreference = "Stop"
$envFile = Join-Path (Get-Location) ".env"
if (-not (Test-Path -LiteralPath $envFile)) { throw "Copy .env.example to .env first." }
$databaseLine = Get-Content -LiteralPath $envFile | Where-Object { $_ -match '^DATABASE_URL=' } | Select-Object -First 1
if (-not $databaseLine) { throw "DATABASE_URL is missing from .env." }
$databaseUrl = $databaseLine.Substring("DATABASE_URL=".Length).Trim('"', "'")
$databaseUri = [Uri]$databaseUrl
if ($databaseUri.Scheme -notin @("postgres", "postgresql") -or $databaseUri.Host -notin @("localhost", "127.0.0.1", "::1", "postgres")) {
    throw "This backup hook only accepts the local Compose database host."
}
$null = New-Item -ItemType Directory -Force -Path $OutputDirectory
$outputFile = Join-Path $OutputDirectory ("peergit-{0:yyyyMMdd-HHmmss}.dump" -f (Get-Date).ToUniversalTime())
$container = docker compose -f deploy/compose/local.yml ps -q postgres
if ($LASTEXITCODE -ne 0 -or -not $container) { throw "The local PostgreSQL service is not running." }
$containerFile = "/tmp/peergit-backup.dump"
docker exec $container pg_dump --format=custom --no-owner --file=$containerFile --dbname=$databaseUrl
if ($LASTEXITCODE -ne 0) { throw "pg_dump failed." }
try {
    docker cp "${container}:$containerFile" $outputFile
    if ($LASTEXITCODE -ne 0) { throw "Could not copy the backup from the container." }
} finally {
    docker exec $container rm -f $containerFile | Out-Null
}
Get-FileHash -Algorithm SHA256 -LiteralPath $outputFile
