param(
    [Parameter(Mandatory = $true)][string]$Fixture,
    [string]$ApiBaseUrl = 'http://localhost:8080/api/v1',
    [string]$ComposeFile = 'compose.yaml',
    [string]$OutputDirectory = 'tmp/m10-benchmark',
    [string]$DatabaseUser = 'video',
    [string]$DatabaseName = 'video',
    [int]$TimeoutMinutes = 45
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$fixturePath = (Resolve-Path -LiteralPath $Fixture).Path
$fixtureInfo = Get-Item -LiteralPath $fixturePath
if ($fixtureInfo.Extension -ne '.mp4') { throw 'Fixture must be an MP4 file.' }
if ($fixtureInfo.Length -gt 104857600) { throw 'Fixture exceeds the default 100 MiB upload limit.' }

$probe = & ffprobe -v error -show_entries 'stream=width,height:format=duration' -of json $fixturePath | ConvertFrom-Json
if ($LASTEXITCODE -ne 0) { throw 'ffprobe could not inspect the benchmark fixture.' }
$videoStream = @($probe.streams | Where-Object { $_.width -eq 1280 -and $_.height -eq 720 })
$duration = [double]::Parse($probe.format.duration, [Globalization.CultureInfo]::InvariantCulture)
if ($videoStream.Count -eq 0 -or $duration -lt 29 -or $duration -gt 31) {
    throw 'Fixture must be a 1280x720 MP4 of approximately 30 seconds.'
}

$dockerCpu = [int]((& docker info --format '{{.NCPU}}' | Select-Object -First 1).Trim())
if ($LASTEXITCODE -ne 0 -or $dockerCpu -lt 6) { throw 'Docker must be available with at least six CPUs.' }
$dockerMemory = [long]((& docker info --format '{{.MemTotal}}' | Select-Object -First 1).Trim())
if ($LASTEXITCODE -ne 0) { throw 'Docker memory information is unavailable.' }
$dockerVersion = (& docker version --format '{{.Server.Version}}' | Select-Object -First 1).Trim()
if ($LASTEXITCODE -ne 0) { throw 'Docker server version is unavailable.' }

$outputPath = Join-Path $root $OutputDirectory
New-Item -ItemType Directory -Path $outputPath -Force | Out-Null
$machine = @{
    timestamp_utc = [DateTime]::UtcNow.ToString('o')
    logical_cpus = [Environment]::ProcessorCount
    docker_cpus = $dockerCpu
    docker_memory_bytes = $dockerMemory
    docker_server_version = $dockerVersion
    fixture = $fixturePath
    fixture_bytes = $fixtureInfo.Length
    fixture_sha256 = (Get-FileHash -LiteralPath $fixturePath -Algorithm SHA256).Hash
    compose_file = $ComposeFile
}
$machine | ConvertTo-Json -Depth 5 | Set-Content -Encoding utf8 (Join-Path $outputPath 'machine.json')

function Invoke-Sql([string]$Query) {
    $value = & docker compose -f $ComposeFile exec -T postgres psql -U $DatabaseUser -d $DatabaseName -At -F ',' -c $Query
    if ($LASTEXITCODE -ne 0) { throw "PostgreSQL query failed: $Query" }
    return ($value | Select-Object -Last 1).Trim()
}

function Get-Median([double[]]$Values) {
    $sorted = @($Values | Sort-Object)
    $middle = [int][Math]::Floor($sorted.Count / 2)
    if (($sorted.Count % 2) -eq 1) { return [double]$sorted[$middle] }
    return ([double]$sorted[$middle - 1] + [double]$sorted[$middle]) / 2
}

$results = @()
$trialNumber = 0
foreach ($replicas in @(1, 3, 1, 3, 1, 3)) {
    $trialNumber++
    & docker compose -f $ComposeFile up -d --build --wait --wait-timeout 60 --scale "worker=$replicas" worker
    if ($LASTEXITCODE -ne 0) { throw "Could not start $replicas worker replicas." }
    $videoIDs = @()
    for ($i = 1; $i -le 9; $i++) {
        $title = "m10-benchmark-$trialNumber-$i"
        $video = Invoke-RestMethod -Method Post -Uri "$ApiBaseUrl/videos" -ContentType 'application/json' -Body (@{ title = $title } | ConvertTo-Json)
        $videoID = ([guid]::Parse($video.data.id)).ToString()
        $upload = Invoke-RestMethod -Method Post -Uri "$ApiBaseUrl/videos/$videoID/uploads" -ContentType 'application/json' -Body (@{ content_type = 'video/mp4'; size_bytes = $fixtureInfo.Length } | ConvertTo-Json)
        Invoke-WebRequest -UseBasicParsing -Method Put -Uri $upload.data.upload_url -InFile $fixturePath -ContentType 'video/mp4' | Out-Null
        Invoke-RestMethod -Method Post -Uri "$ApiBaseUrl/uploads/$($upload.data.upload_id)/complete" | Out-Null
        $videoIDs += $videoID
    }
    $idList = ($videoIDs | ForEach-Object { "'$_'" }) -join ','
    $deadline = (Get-Date).AddMinutes($TimeoutMinutes)
    while ((Get-Date) -lt $deadline) {
        $counts = (Invoke-Sql "SELECT count(*), count(*) FILTER (WHERE j.status = 'SUCCEEDED' AND v.status = 'READY'), count(*) FILTER (WHERE j.status = 'FAILED' OR v.status = 'FAILED'), count(*) FILTER (WHERE j.status = 'RUNNING') FROM videos v LEFT JOIN processing_jobs j ON j.video_id = v.id WHERE v.id IN ($idList)").Split(',')
        if ([int]$counts[0] -ne 9) { throw "Trial $trialNumber lost video rows." }
        if ([int]$counts[2] -gt 0) { throw "Trial $trialNumber contains a failed video or job." }
        if ([int]$counts[1] -eq 9) { break }
        Start-Sleep -Seconds 5
    }
    if ([int]$counts[1] -ne 9) { throw "Trial $trialNumber timed out before nine videos reached READY." }
    $secondsText = Invoke-Sql "SELECT extract(epoch FROM max(finished_at) - min(started_at)) FROM processing_jobs WHERE video_id IN ($idList) AND status = 'SUCCEEDED' HAVING count(*) = 9"
    $seconds = [double]::Parse($secondsText, [Globalization.CultureInfo]::InvariantCulture)
    if ($seconds -le 0) { throw "Trial $trialNumber has an invalid duration." }
    $duplicateAssets = [int](Invoke-Sql "SELECT count(*) FROM (SELECT video_id, asset_type, variant FROM assets WHERE video_id IN ($idList) GROUP BY video_id, asset_type, variant HAVING count(*) > 1) duplicates")
    if ($duplicateAssets -ne 0) { throw "Trial $trialNumber has duplicate logical assets." }
    $maxRunning = [int](Invoke-Sql "SELECT max((SELECT count(*) FROM processing_jobs b WHERE b.video_id IN ($idList) AND b.started_at <= a.started_at AND b.finished_at > a.started_at)) FROM processing_jobs a WHERE a.video_id IN ($idList)")
    $logPath = Join-Path $outputPath "trial-$trialNumber-worker.log"
    & docker compose -f $ComposeFile logs --no-color worker | Set-Content -Encoding utf8 $logPath
    if ($LASTEXITCODE -ne 0) { throw "Could not save worker logs for trial $trialNumber." }
    $claims = Get-Content -LiteralPath $logPath | Where-Object {
        $line = $_
        $line.Contains('processing job claimed') -and @($videoIDs | Where-Object { $line.Contains($_) }).Count -gt 0
    }
    $workerIDs = @($claims | ForEach-Object {
        if ($_ -match '"worker_id":"([^"]+)"') { $Matches[1] }
    } | Sort-Object -Unique)
    if ($workerIDs.Count -ne $replicas) { throw "Trial $trialNumber was claimed by $($workerIDs.Count) workers, expected $replicas." }
    if ($maxRunning -ne $replicas) { throw "Trial $trialNumber reached $maxRunning concurrent jobs, expected $replicas." }
    $results += @{
        trial = $trialNumber
        replicas = $replicas
        video_ids = $videoIDs
        duration_seconds = $seconds
        ready_videos_per_minute = 9 * 60 / $seconds
        max_running = $maxRunning
        worker_ids = $workerIDs
        worker_log = $logPath
    }
    $results | ConvertTo-Json -Depth 8 | Set-Content -Encoding utf8 (Join-Path $outputPath 'trials.json')
}

$one = Get-Median ([double[]]@($results | Where-Object { $_.replicas -eq 1 } | ForEach-Object { $_.ready_videos_per_minute }))
$three = Get-Median ([double[]]@($results | Where-Object { $_.replicas -eq 3 } | ForEach-Object { $_.ready_videos_per_minute }))
$summary = @{ one_replica_median = $one; three_replica_median = $three; ratio = $three / $one; passes = ($three / $one) -ge 1.5 }
$summary | ConvertTo-Json | Set-Content -Encoding utf8 (Join-Path $outputPath 'summary.json')
Write-Output ("1 replica: {0:N2} READY/min; 3 replicas: {1:N2} READY/min; ratio: {2:N2}x; pass: {3}" -f $one, $three, $summary.ratio, $summary.passes)
if (-not $summary.passes) { throw 'Milestone 10 throughput threshold was not met.' }
