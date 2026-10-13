param(
  [Parameter(Mandatory=$true)][string]$GitHubOwner,
  [Parameter(Mandatory=$true)][string]$DatabaseId,
  [Parameter(Mandatory=$true)][string]$KvNamespaceId
)
$path = Join-Path $PSScriptRoot '..\cloudflare\wrangler.jsonc'
$text = Get-Content $path -Raw
$text = $text.Replace('REEMPLAZAR_OWNER', $GitHubOwner).Replace('REEMPLAZAR_DATABASE_ID', $DatabaseId).Replace('REEMPLAZAR_KV_NAMESPACE_ID', $KvNamespaceId)
Set-Content -Path $path -Value $text -Encoding UTF8
Write-Host "wrangler.jsonc configurado (GitHub + D1 + KV)." -ForegroundColor Green
