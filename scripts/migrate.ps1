param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('up', 'down')]
    [string]$Direction
)

$composeFile = if ($env:COMPOSE_FILE) { $env:COMPOSE_FILE } else { 'compose.yaml' }
$migrationFiles = @(Get-ChildItem -LiteralPath 'migrations' -Filter "*.$Direction.sql" -File | Sort-Object Name)

function Test-MigrationApplied([int64]$Version) {
    if ($Version -eq 1 -and $Direction -eq 'up') {
        return $false
    }

    $query = "SELECT 1 FROM schema_migrations WHERE version = $Version;"
    $result = $query | docker compose -f $composeFile exec -T postgres sh -c 'psql -At -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f -'
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }

    return (($result | Out-String).Trim() -eq '1')
}

if ($Direction -eq 'down') {
    [Array]::Reverse($migrationFiles)
}

foreach ($migrationFile in $migrationFiles) {
    $version = [int64]([regex]::Match($migrationFile.BaseName, '^\d+').Value)
    if (Test-MigrationApplied $version) {
        Write-Host "Skipping migration $version ($Direction): already applied"
        continue
    }

    Write-Host "Applying migration $version ($Direction): $($migrationFile.Name)"
    Get-Content -LiteralPath $migrationFile.FullName -Raw |
        docker compose -f $composeFile exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f -'
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
}
