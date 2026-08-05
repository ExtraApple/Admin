param(
    [string]$Endpoint = $env:ADMIN_TEST_MINIO_ENDPOINT,
    [string]$AccessKey = $env:ADMIN_TEST_MINIO_ACCESS_KEY,
    [string]$SecretKey = $env:ADMIN_TEST_MINIO_SECRET_KEY,
    [string]$Secure = $env:ADMIN_TEST_MINIO_SECURE
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($Endpoint) -or
    [string]::IsNullOrWhiteSpace($AccessKey) -or
    [string]::IsNullOrWhiteSpace($SecretKey)) {
    throw "ADMIN_TEST_MINIO_ENDPOINT, ADMIN_TEST_MINIO_ACCESS_KEY and ADMIN_TEST_MINIO_SECRET_KEY are required for the mandatory MinIO gate"
}

$env:ADMIN_TEST_MINIO_ENDPOINT = $Endpoint
$env:ADMIN_TEST_MINIO_ACCESS_KEY = $AccessKey
$env:ADMIN_TEST_MINIO_SECRET_KEY = $SecretKey
$env:ADMIN_TEST_MINIO_SECURE = $Secure
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Push-Location $repoRoot
try {
    & go test -tags=minio_integration ./internal/platform/objectstorage -count=1
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
}
finally {
    Pop-Location
}
