param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Arguments)

$ErrorActionPreference = 'Stop'
$Bash = Get-Command bash.exe -ErrorAction SilentlyContinue
if (-not $Bash) {
    throw 'Git Bash is required to run ai-toolkit on Windows.'
}

& $Bash.Source (Join-Path $PSScriptRoot 'ai-toolkit') @Arguments
exit $LASTEXITCODE
