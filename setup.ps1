param (
    [string]$CustomTargetDir = ""
)

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

# Standard OpenRCT2 user data locations on Windows
$DefaultDirs = @(
    (Join-Path ([Environment]::GetFolderPath("MyDocuments")) "OpenRCT2")
)

if ($CustomTargetDir -ne "") {
    $TargetDirs = @($CustomTargetDir)
} else {
    $TargetDirs = $DefaultDirs
}

function Install-ConfigWithBackup {
    param (
        [string]$Content,
        [string]$Destination
    )

    $DestDir = Split-Path -Parent $Destination
    if (-not (Test-Path -Path $DestDir)) {
        New-Item -ItemType Directory -Path $DestDir -Force | Out-Null
    }

    if (Test-Path -Path $Destination) {
        $ExistingContent = Get-Content -Path $Destination -Raw
        if ($ExistingContent -ne $Content) {
            $Timestamp = Get-Date -Format "yyyyMMddHHmmss"
            $BackupPath = "$Destination.bak.$Timestamp"
            Write-Host "Backing up existing $(Split-Path -Leaf $Destination) to $BackupPath"
            Copy-Item -Path $Destination -Destination $BackupPath -Force
            Write-Host "Installing $(Split-Path -Leaf $Destination)..."
            Set-Content -Path $Destination -Value $Content -Encoding UTF8
        } else {
            Write-Host "$(Split-Path -Leaf $Destination) is already up to date."
        }
    } else {
        Write-Host "Installing $(Split-Path -Leaf $Destination)..."
        Set-Content -Path $Destination -Value $Content -Encoding UTF8
    }
}

Write-Host "Setting up OpenRCT2 configurations for Windows..."

$ConfigSrc = Join-Path $ScriptDir "config\config.ini"
$ShortcutsSrc = Join-Path $ScriptDir "config\shortcuts.json"

$ConfigContent = Get-Content -Path $ConfigSrc -Raw
$ShortcutsContent = Get-Content -Path $ShortcutsSrc -Raw

# Replace placeholders for Windows environment
$EscapedDocs = ([Environment]::GetFolderPath("MyDocuments") -replace '\\', '/')
$ConfigContent = $ConfigContent.Replace("__HOME__/.config/OpenRCT2", "$EscapedDocs/OpenRCT2")
$ConfigContent = $ConfigContent.Replace("__USER__", $env:USERNAME)

foreach ($Dir in $TargetDirs) {
    Write-Host "Target directory: $Dir"
    $TargetConfigFile = Join-Path $Dir "config.ini"
    $TargetShortcutsFile = Join-Path $Dir "shortcuts.json"

    Install-ConfigWithBackup -Content $ConfigContent -Destination $TargetConfigFile
    Install-ConfigWithBackup -Content $ShortcutsContent -Destination $TargetShortcutsFile
}

Write-Host "OpenRCT2 setup complete!"
