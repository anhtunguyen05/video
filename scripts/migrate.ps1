param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('up', 'down')]
    [string]$Direction
)

$composeFile = if ($env:COMPOSE_FILE) { $env:COMPOSE_FILE } else { 'compose.yaml' }
$migrationFiles = @(Get-ChildItem -LiteralPath 'migrations' -Filter "*.$Direction.sql" -File | Sort-Object Name)

if ($Direction -eq 'down') {
    [Array]::Reverse($migrationFiles)
}

foreach ($migrationFile in $migrationFiles) {
    Get-Content -LiteralPath $migrationFile.FullName -Raw |
        docker compose -f $composeFile exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f -'
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
}
