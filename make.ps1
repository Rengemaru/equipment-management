#Requires -Version 5.1
<#
    開発は全てコンテナ内で行う。このスクリプトは docker compose の薄いラッパでしかない。
    macOS / Linux では Makefile が同じタスク名を提供する。タスクを増やすときは両方に追加する。

    使い方: .\make.ps1 <task>
#>

[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('help', 'up', 'down', 'sh', 'logs', 'fmt', 'test')]
    [string]$Task = 'help'
)

$ErrorActionPreference = 'Stop'

# PowerShell 5.1 には && が無いため、コマンドは配列で組み立てて渡す
$ComposeArgs = @('compose', '-f', 'compose.dev.yaml')

function Invoke-InContainer {
    param([string]$Command)
    # sh -lc は使わない。ログインシェルが PATH を上書きして go が見つからなくなる。
    # $Command にダブルクォートを含めないこと（PowerShell 5.1 が引数のクォートを壊す）。
    docker @ComposeArgs exec dev bash -c $Command
}

switch ($Task) {
    'help' {
        Write-Host '  up             開発コンテナを起動する'
        Write-Host '  down           開発コンテナを停止する'
        Write-Host '  sh             コンテナ内のシェルに入る'
        Write-Host '  logs           コンテナのログを追う'
        Write-Host '  fmt            gofmt と go vet をかける'
        Write-Host '  test           Go のテストを実行する'
    }
    'up'    { docker @ComposeArgs up -d }
    'down'  { docker @ComposeArgs down }
    'sh'    { docker @ComposeArgs exec dev bash }
    'logs'  { docker @ComposeArgs logs -f }
    'fmt'   { Invoke-InContainer 'gofmt -w . && go vet ./...' }
    'test'  { Invoke-InContainer 'go test ./...' }
}

exit $LASTEXITCODE
