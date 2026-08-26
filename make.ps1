#Requires -Version 5.1
<#
    開発は全てコンテナ内で行う。このスクリプトは docker compose の薄いラッパでしかない。
    macOS / Linux では Makefile が同じタスク名を提供する。タスクを増やすときは両方に追加する。

    使い方: .\make.ps1 <task>
#>

[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [ValidateSet('help', 'up', 'down', 'sh', 'logs', 'fmt', 'test', 'test-web',
        'prod-build', 'prod-up', 'prod-down', 'prod-logs', 'prod-ps',
        'create-admin', 'backup', 'restore')]
    [string]$Task = 'help',

    # create-admin 用。パスワードは受け取らない（生成して一度だけ表示する）
    [string]$LoginId,
    [string]$Name,
    [string]$Email,

    # restore 用。戻す元のバックアップファイル
    [string]$File
)

# $ErrorActionPreference = 'Stop' は設定しない。
# docker compose は進捗を stderr に書く。呼び出し側が 2>&1 で受けると
# PowerShell 5.1 はその1行ごとに ErrorRecord を作り、終了コード0でも失敗扱いになる。
# 成否は最後の exit $LASTEXITCODE で伝える。

# PowerShell 5.1 には && が無いため、コマンドは配列で組み立てて渡す
$ComposeArgs = @('compose', '-f', 'compose.dev.yaml')

# 本番用。対象が compose.yaml であることを prod- で明示する。
# dev と同じ名前にすると、止めるつもりで本番を止める事故が起きる。
$ProdArgs = @('compose', '-f', 'compose.yaml')

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
        Write-Host '  test-web       フロントの型検査・ビルド・テストを実行する'
        Write-Host ''
        Write-Host '  prod-build     本番イメージを作り直す'
        Write-Host '  prod-up        本番を起動する'
        Write-Host '  prod-down      本番を停止する'
        Write-Host '  prod-logs      本番のログを追う'
        Write-Host '  prod-ps        本番の状態とヘルスチェックの結果を見る'
        Write-Host ''
        Write-Host '  create-admin   最初の admin を作る'
        Write-Host '                   .\make.ps1 create-admin -LoginId yamada -Name 山田'
        Write-Host '  backup         バックアップを取り、ホストへ取り出す'
        Write-Host '  restore        バックアップから戻す（サーバを止めて実行する）'
        Write-Host '                   .\make.ps1 restore -File .\backup-2026-08-27.db'
    }
    'up'    { docker @ComposeArgs up -d }
    'down'  { docker @ComposeArgs down }
    'sh'    { docker @ComposeArgs exec dev bash }
    'logs'  { docker @ComposeArgs logs -f }
    'fmt'   { Invoke-InContainer 'gofmt -w . && go vet ./...' }
    'test'  { Invoke-InContainer 'go test ./...' }

    # npm ci を先に流すのは、node_modules が名前付きボリュームにあり、
    # package-lock.json を変えても自動では反映されないため。手元と CI で同じ依存を検査する。
    #
    # テストだけでなく build も通すのは、型検査が npm run build（tsc --noEmit）にしか
    # 無いため。vitest は esbuild で型を落として実行するので、型エラーを見逃す。
    'test-web' { Invoke-InContainer 'cd web && npm ci && npm run build && npm test' }

    # 本番。ソースはマウントされないため、手元の変更は prod-build まで反映されない。
    'prod-build' { docker @ProdArgs build }
    'prod-up'    { docker @ProdArgs up -d }
    'prod-down'  { docker @ProdArgs down }
    'prod-logs'  { docker @ProdArgs logs -f }
    'prod-ps'    { docker @ProdArgs ps }

    # 最初の admin を作る。これが無いと誰もログインできない。
    # パスワードは引数で受け取らない。生成して一度だけ表示する
    # （履歴と ps に平文を残さないため）。
    'create-admin' {
        if (-not $LoginId -or -not $Name) {
            Write-Host '使い方: .\make.ps1 create-admin -LoginId yamada -Name 山田 [-Email a@b.c]'
            exit 1
        }
        $a = @('exec', 'app', '/server', '-create-admin', '-login-id', $LoginId, '-name', $Name)
        if ($Email) { $a += @('-email', $Email) }
        docker @ProdArgs @a
    }

    # 稼働中でも一貫したコピーを1ファイル作る（VACUUM INTO）。上書きはしない。
    # 取り出すところまでやる。ボリュームに置いたままでは、
    # ディスクごと失われた時に一緒に消える。
    'backup' {
        $stamp = Get-Date -Format 'yyyy-MM-dd'
        docker @ProdArgs exec app /server -backup "/data/backup-$stamp.db"
        if ($?) {
            docker @ProdArgs cp "app:/data/backup-$stamp.db" "./backup-$stamp.db"
            if ($?) { Write-Host "取り出しました: .\backup-$stamp.db" }
        }
    }

    # バックアップから戻す。サーバを止めてから実行する。
    # 動いているサーバの足元でファイルを差し替えると壊れる。
    'restore' {
        if (-not $File) {
            Write-Host '使い方: .\make.ps1 restore -File .\backup-2026-08-27.db'
            exit 1
        }
        if (-not (Test-Path $File)) {
            Write-Host "$File が無い"
            exit 1
        }
        docker @ProdArgs stop
        if ($?) { docker @ProdArgs cp $File 'app:/data/restore-src.db' }
        if ($?) { docker @ProdArgs run --rm app -restore /data/restore-src.db }
        if ($?) {
            docker @ProdArgs start
            Write-Host '起動しました。ログインできることを確かめること。'
        }
    }
}

exit $LASTEXITCODE
