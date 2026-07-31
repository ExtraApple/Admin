param(
    [string]$Dsn = $env:ADMIN_TEST_MYSQL_DSN
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($Dsn)) {
    throw "ADMIN_TEST_MYSQL_DSN or -Dsn is required for the mandatory MySQL gate"
}

$env:ADMIN_TEST_MYSQL_DSN = $Dsn
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Push-Location $repoRoot
try {
    & go test -tags=mysql_integration ./... -count=1
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
}
finally {
    Pop-Location
}
