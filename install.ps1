param(
    [string]$InstallDir = (Join-Path $HOME '.ai-toolkit'),
    [string]$RepoUrl = 'https://github.com/innaka-tech/ai-toolkit.git'
)

$ErrorActionPreference = 'Stop'
$SourceDir = Split-Path -Parent $MyInvocation.MyCommand.Path

if (-not (Get-Command git.exe -ErrorAction SilentlyContinue)) {
    throw 'Git for Windows is required. Install it from https://git-scm.com/download/win and run this again.'
}
if (-not (Get-Command bash.exe -ErrorAction SilentlyContinue)) {
    throw 'Git Bash is required. Reinstall Git for Windows with the Git Bash component enabled.'
}

if (([IO.Path]::GetFullPath($SourceDir).TrimEnd('\')) -ne ([IO.Path]::GetFullPath($InstallDir).TrimEnd('\'))) {
    if (Test-Path (Join-Path $InstallDir '.git')) {
        Write-Host "Updating existing toolkit at $InstallDir"
        git.exe -C $InstallDir pull --ff-only
    } elseif (Test-Path $InstallDir) {
        $hasFiles = Get-ChildItem -LiteralPath $InstallDir -Force | Select-Object -First 1
        if ($hasFiles) { throw "Install directory is not empty: $InstallDir" }
        git.exe clone $RepoUrl $InstallDir
    } else {
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $InstallDir) | Out-Null
        git.exe clone $RepoUrl $InstallDir
    }
}

$ScriptsDir = Join-Path $InstallDir 'scripts'
$UserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$PathEntries = @($UserPath -split ';' | Where-Object { $_ })
if ($PathEntries -notcontains $ScriptsDir) {
    [Environment]::SetEnvironmentVariable('Path', (($PathEntries + $ScriptsDir) -join ';'), 'User')
}

Write-Host "AI Toolkit installed at $InstallDir"
Write-Host 'Open a new PowerShell window, then run:'
Write-Host '  ai-toolkit.ps1 --help'
Write-Host "If script execution is blocked: Set-ExecutionPolicy -Scope CurrentUser RemoteSigned"
