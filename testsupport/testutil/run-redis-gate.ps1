param(
    [string]$Address = $env:ADMIN_TEST_REDIS_ADDR
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($Address)) {
    throw "ADMIN_TEST_REDIS_ADDR or -Address is required for the mandatory Redis gate"
}

$env:ADMIN_TEST_REDIS_ADDR = $Address
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Push-Location $repoRoot
try {
    & go test -tags=redis_integration ./internal/identity/adapters/redis -count=1
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
}
finally {
    Pop-Location
}
