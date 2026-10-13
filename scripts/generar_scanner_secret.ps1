$bytes = New-Object byte[] 32
[System.Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
$secret = [Convert]::ToHexString($bytes).ToLowerInvariant()
Write-Host "SCANNER_SECRET generado:" -ForegroundColor Green
Write-Host $secret
Write-Host ""
Write-Host "Guardalo temporalmente: más adelante lo cargaremos como Secret tanto en Cloudflare como en GitHub. No lo publiques." -ForegroundColor Yellow
