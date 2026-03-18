#!/usr/bin/env pwsh
# Install script for penpal — https://github.com/seastco/penpal
# Usage: irm https://raw.githubusercontent.com/seastco/penpal/master/install.ps1 | iex

$ErrorActionPreference = "Stop"

$Repo = "seastco/penpal"
$InstallDir = Join-Path $env:LOCALAPPDATA "Programs\penpal"

function Info($Message) {
    Write-Host "  $Message"
}

function Warn($Message) {
    Write-Warning $Message
}

function Fail($Message) {
    throw "Error: $Message"
}

function Get-Arch {
    $rawArch = if ($env:PROCESSOR_ARCHITEW6432) {
        $env:PROCESSOR_ARCHITEW6432
    } else {
        $env:PROCESSOR_ARCHITECTURE
    }

    switch ($rawArch.ToUpperInvariant()) {
        "AMD64" { return "amd64" }
        "ARM64" { return "arm64" }
        default { Fail "unsupported architecture: $rawArch" }
    }
}

function Normalize-PathEntry([string]$PathEntry) {
    if ([string]::IsNullOrWhiteSpace($PathEntry)) {
        return ""
    }

    try {
        return [System.IO.Path]::GetFullPath($PathEntry).TrimEnd('\').ToLowerInvariant()
    } catch {
        return $PathEntry.Trim().TrimEnd('\').ToLowerInvariant()
    }
}

try {
    if ($IsLinux -or $IsMacOS) {
        Fail "this installer is for Windows. Use install.sh on macOS/Linux."
    }

    if (-not $env:LOCALAPPDATA) {
        Fail "LOCALAPPDATA is not set"
    }

    $arch = Get-Arch
    $artifact = "penpal-windows-$arch.zip"
    $url = "https://github.com/$Repo/releases/latest/download/$artifact"

    Info "Detected platform: windows/$arch"
    Info "Downloading $artifact..."

    $tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ("penpal-install-" + [Guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Path $tmpDir | Out-Null

    $zipPath = Join-Path $tmpDir $artifact
    $extractDir = Join-Path $tmpDir "extract"
    New-Item -ItemType Directory -Path $extractDir | Out-Null

    try {
        Invoke-WebRequest -Uri $url -OutFile $zipPath
    } catch {
        Fail "download failed - check that a release exists for windows/$arch"
    }

    Expand-Archive -Path $zipPath -DestinationPath $extractDir -Force

    $binaryPath = Join-Path $extractDir "penpal.exe"
    if (-not (Test-Path -LiteralPath $binaryPath)) {
        Fail "binary not found in archive"
    }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $targetBinary = Join-Path $InstallDir "penpal.exe"
    Copy-Item -Path $binaryPath -Destination $targetBinary -Force
    Info "Installed to $targetBinary"

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($null -eq $userPath) {
        $userPath = ""
    }

    $existingEntries = @()
    if ($userPath.Length -gt 0) {
        $existingEntries = $userPath.Split(';', [System.StringSplitOptions]::RemoveEmptyEntries)
    }

    $normalizedInstallDir = Normalize-PathEntry $InstallDir
    $hasInstallDir = $false
    foreach ($entry in $existingEntries) {
        if ((Normalize-PathEntry $entry) -eq $normalizedInstallDir) {
            $hasInstallDir = $true
            break
        }
    }

    if (-not $hasInstallDir) {
        $newUserPath = if ([string]::IsNullOrWhiteSpace($userPath)) {
            $InstallDir
        } else {
            "$userPath;$InstallDir"
        }
        [Environment]::SetEnvironmentVariable("Path", $newUserPath, "User")
        Warn "$InstallDir was added to your User PATH. Open a new terminal to use 'penpal'."
    }

    if (-not (($env:Path -split ';') | Where-Object { (Normalize-PathEntry $_) -eq $normalizedInstallDir })) {
        $env:Path = "$InstallDir;$env:Path"
    }

    Info ""
    Info "penpal installed successfully! Run 'penpal' to get started."
} finally {
    if ($tmpDir -and (Test-Path -LiteralPath $tmpDir)) {
        Remove-Item -LiteralPath $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}
