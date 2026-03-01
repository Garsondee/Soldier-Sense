param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$Overrides
)

$runs = 5
$ticks = 3600
$seedBase = 42
$seedStep = 1

foreach ($pair in $Overrides) {
    if ([string]::IsNullOrWhiteSpace($pair)) {
        continue
    }

    $parts = $pair.Split('=', 2)
    if ($parts.Count -ne 2) {
        continue
    }

    $key = $parts[0].Trim().ToUpperInvariant()
    $value = $parts[1].Trim()

    switch ($key) {
        'RUNS' { $runs = [int]$value }
        'TICKS' { $ticks = [int]$value }
        'SEED_BASE' { $seedBase = [int64]$value }
        'SEED_STEP' { $seedStep = [int64]$value }
    }
}

go run ./cmd/headless-report -runs $runs -ticks $ticks -seed-base $seedBase -seed-step $seedStep
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}
