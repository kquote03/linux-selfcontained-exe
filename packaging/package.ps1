<#
.SYNOPSIS
  Assembles the final single-file Linux Lab exe: launcher.exe + a zip
  payload (RVVM binary/DLL, firmware, compressed disk image, version tag)
  + a 64-byte footer the launcher parses to locate the payload.

.DESCRIPTION
  Footer layout (must exactly match launcher/payload.go's footerSize/footerMagic):
    magic          8 bytes  ASCII "LXLABFT1"
    payloadOffset  8 bytes  uint64 LE - byte offset where the zip payload starts
    payloadSize    8 bytes  uint64 LE - size of the zip payload in bytes
    version        4 bytes  uint32 LE - footer format version (currently 1)
    reserved       36 bytes padding

.EXAMPLE
  ./package.ps1 -LauncherExe ..\launcher\launcher.exe `
    -RvvmExe ..\assets\rvvm_x86_64.exe -RvvmDll ..\assets\librvvm.dll `
    -Firmware ..\assets\fw_payload.bin -DiskImageZst ..\assets\disk.img.zst `
    -Version "2026.09.30-1" -OutFile ..\assets\LinuxLab.exe
#>
param(
    [Parameter(Mandatory = $true)][string]$LauncherExe,
    [Parameter(Mandatory = $true)][string]$RvvmExe,
    [Parameter(Mandatory = $true)][string]$RvvmDll,
    [Parameter(Mandatory = $true)][string]$Firmware,
    [Parameter(Mandatory = $true)][string]$DiskImageZst,
    [Parameter(Mandatory = $true)][string]$Version,
    [Parameter(Mandatory = $true)][string]$OutFile
)

$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.IO.Compression
Add-Type -AssemblyName System.IO.Compression.FileSystem

foreach ($f in @($LauncherExe, $RvvmExe, $RvvmDll, $Firmware, $DiskImageZst)) {
    if (-not (Test-Path $f)) { throw "Input file not found: $f" }
}

$tmpZip = [System.IO.Path]::GetTempFileName()
Remove-Item $tmpZip -Force
Write-Host "Building payload zip at $tmpZip ..."

$zip = [System.IO.Compression.ZipFile]::Open($tmpZip, [System.IO.Compression.ZipArchiveMode]::Create)
try {
    [System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, $RvvmExe, "tools/rvvm_x86_64.exe", [System.IO.Compression.CompressionLevel]::Optimal) | Out-Null
    [System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, $RvvmDll, "tools/librvvm.dll", [System.IO.Compression.CompressionLevel]::Optimal) | Out-Null
    [System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, $Firmware, "tools/fw_payload.bin", [System.IO.Compression.CompressionLevel]::Optimal) | Out-Null
    # disk.img.zst is already zstd-compressed - don't ask the zip format to
    # recompress already-compressed bytes, just store it.
    [System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile($zip, $DiskImageZst, "disk.img.zst", [System.IO.Compression.CompressionLevel]::NoCompression) | Out-Null

    $versionEntry = $zip.CreateEntry("VERSION")
    $stream = $versionEntry.Open()
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($Version)
    $stream.Write($bytes, 0, $bytes.Length)
    $stream.Close()
}
finally {
    $zip.Dispose()
}

$launcherBytes = [System.IO.File]::ReadAllBytes($LauncherExe)
$zipBytes = [System.IO.File]::ReadAllBytes($tmpZip)
Remove-Item $tmpZip -Force

Write-Host "Launcher: $($launcherBytes.Length) bytes"
Write-Host "Payload zip: $($zipBytes.Length) bytes"

$payloadOffset = [uint64]$launcherBytes.Length
$payloadSize = [uint64]$zipBytes.Length

$footer = New-Object byte[] 64
[System.Text.Encoding]::ASCII.GetBytes("LXLABFT1").CopyTo($footer, 0)
[System.BitConverter]::GetBytes($payloadOffset).CopyTo($footer, 8)
[System.BitConverter]::GetBytes($payloadSize).CopyTo($footer, 16)
[System.BitConverter]::GetBytes([uint32]1).CopyTo($footer, 24)
# bytes 28-63 stay zero (reserved)

$outDir = Split-Path -Parent $OutFile
if ($outDir -and -not (Test-Path $outDir)) { New-Item -ItemType Directory -Path $outDir -Force | Out-Null }

$outStream = [System.IO.File]::Open($OutFile, [System.IO.FileMode]::Create)
try {
    $outStream.Write($launcherBytes, 0, $launcherBytes.Length)
    $outStream.Write($zipBytes, 0, $zipBytes.Length)
    $outStream.Write($footer, 0, $footer.Length)
}
finally {
    $outStream.Close()
}

$totalSize = $launcherBytes.Length + $zipBytes.Length + $footer.Length
Write-Host "Wrote $OutFile ($totalSize bytes, $([math]::Round($totalSize/1MB, 1)) MB)"
