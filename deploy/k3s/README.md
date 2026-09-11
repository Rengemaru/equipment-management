# k3s 運用マニュアル

この文書だけで、備品管理アプリをk3sへデプロイし、初回データを用意し、状態を確認し、安全に停止できます。
アプリのソースコードやCompose構成を先に読む必要はありません。

コマンドはリポジトリのルートディレクトリで実行します。
値を置き換える箇所は `<管理者ID>` のように記載します。

## 目的別の入口

- 初めて起動する場合：「[作業前の確認](#作業前の確認)」から順番に進む。
- 管理者と備品を登録する場合：「[初回データを用意する](#初回データを用意する)」へ進む。
- 動作しているか調べる場合：「[ステータスを確認する](#ステータスを確認する)」へ進む。
- 起動しない場合：「[よくあるエラーと処方箋](#よくあるエラーと処方箋)」で画面の表示を探す。
- デモを終了する場合：「[停止する](#停止する)」へ進む。

## マニフェストの構成

2026年9月11日時点で、次の構成を実機確認済みです。

| 対象 | 設定 |
|---|---|
| Namespace | `equipment-management` |
| アクセスURL | `http://equipment-management.home.arpa` |
| Traefikの代表IP | `192.168.13.150` |
| SQLite | `equipment-management-data` PVC、1GiB |
| 写真 | `equipment-management-uploads` PVC、2GiB |
| コンテナ | `ghcr.io/ruwseid/equipment-management:sha-c5d59b4` |
| Pod数 | 1 |
| 配置先 | `k3s-server` |

アプリはTailscale Kubernetes Operatorを使いません。
利用者は各自のTailscale subnet routerからLANへ入り、Traefikを経由してアクセスします。

独自ドメインとCloudflare Tunnelは未導入です。
現在のURLはデモ用であり、永久ラベルへ印刷するURLではありません。

## 作業の全体像

初回デプロイは次の順序で行います。

1. k3sへの接続とストレージを確認する。
2. アクセス名をLAN DNSまたは端末のhostsへ登録する。
3. コンテナイメージのタグを確認する。
4. 実行時Secretを作る。
5. マニフェストを検証して適用する。
6. 最初の管理者を作る。
7. 画面から備品データを登録する。
8. ヘルスチェックと画面操作を確認する。

二回目以降は、SecretとPVCが残っていればマニフェストの再適用だけで起動できます。

## 作業前の確認

### 必要なコマンド

作業端末に次のコマンドが必要です。

- `git`
- `gh`
- `kubectl`
- `openssl`
- `curl`

次のコマンドがすべて成功することを確認します。

```sh
git status --short --branch
gh auth status
kubectl config current-context
kubectl cluster-info
```

`git status` に自分が作っていない変更が表示された場合は、変更を消さずに作業者へ確認します。

### ノードと権限

すべてのノードが `Ready` であることを確認します。

```sh
kubectl get nodes -o wide
```

少なくとも次の権限が必要です。

```sh
kubectl auth can-i create deployments -n equipment-management
kubectl auth can-i create secrets -n equipment-management
kubectl auth can-i create persistentvolumeclaims -n equipment-management
kubectl auth can-i create storageclasses
```

すべて `yes` なら作業を続けます。
`no` がある場合は、権限を持つkubeconfigへ切り替えます。

### TraefikとNFS

TraefikとNFS CSIが動いていることを確認します。

```sh
kubectl -n kube-system get deployment traefik
kubectl -n kube-system get pods -l app.kubernetes.io/instance=csi-driver-nfs -o wide
kubectl get storageclass nfs-rwx-retain
kubectl get csidriver nfs.csi.k8s.io
```

Traefikは `READY 1/1`、NFS CSIのPodは `Running` である必要があります。
`nfs-rwx-retain` が無いクラスタへ、このアプリをそのまま適用することはできません。

NFSの実体は `192.168.13.151` のNFS VMです。
このVMと、そのデータディスクは共有ストレージですが、ストレージHAではありません。

## アクセス名を設定する

TraefikはHost名でアプリを振り分けます。
IPアドレスだけでアクセスすると、既存のOpen WebUIが表示されるのが正常です。

LAN DNSを管理できる場合は、次のAレコードを登録します。

```text
equipment-management.home.arpa  A  192.168.13.150
```

LAN DNSを変更できない間は、アクセスする端末のhostsファイルへ次の1行を追加します。

```text
192.168.13.150 equipment-management.home.arpa
```

hostsファイルの場所は次のとおりです。

- macOSまたはLinux：`/etc/hosts`
- Windows：`C:\Windows\System32\drivers\etc\hosts`

部長側の端末にも同じ設定が必要です。
設定後、名前が同じIPへ解決されることを確認します。

```sh
ping -c 1 equipment-management.home.arpa
```

ICMPを許可していない環境では、後述する `curl` の確認だけで構いません。

## コンテナイメージを確認する

通常のデプロイでは、[オーバーレイ設定](overlays/tailnet/kustomization.yaml)に記載済みのタグを使います。
タグは `latest` ではなく、`sha-` から始まる変更不能な値でなければなりません。

現在の値を確認します。

```sh
rg -n 'newName|newTag' deploy/k3s/overlays/tailnet/kustomization.yaml
```

アプリのコードを更新した場合は、GitHub Actionsの `Container image` が成功してから、そのコミットに対応する `sha-xxxxxxx` へ `newTag` を更新します。

```sh
gh run list --repo ruwseid/equipment-management \
  --workflow container-image.yml --limit 5
```

マニフェストだけを変更した場合はコンテナを再ビルドしません。
その場合は、直前に動作確認済みのイメージタグを継続して使います。

## 実行時Secretを作る

### HTTPデモ用の設定

SecretにはURL、セッション署名鍵、Cookie設定を保存します。
SecretのYAMLや平文の値はGitへ追加しません。

次のコマンドは、権限を所有者だけに限定した一時ファイルを作り、ランダムなセッション鍵を保存します。
生成された鍵は端末へ表示しません。

```sh
RUNTIME_ENV_FILE=$(mktemp)
chmod 600 "$RUNTIME_ENV_FILE"
{
  printf '%s\n' 'HOST_URL=http://equipment-management.home.arpa'
  printf 'SESSION_SECRET='
  openssl rand -base64 48
  printf '%s\n' 'COOKIE_SECURE=false'
} > "$RUNTIME_ENV_FILE"
```

Namespaceを作り、Secretをクラスタへ登録します。

```sh
kubectl apply -f deploy/k3s/base/namespace.yaml
kubectl -n equipment-management create secret generic equipment-management-runtime \
  --from-env-file="$RUNTIME_ENV_FILE" \
  --dry-run=client -o yaml | kubectl apply -f -
rm -f "$RUNTIME_ENV_FILE"
unset RUNTIME_ENV_FILE
```

Secretが存在することだけを確認します。
値を表示する `-o yaml` は付けません。

```sh
kubectl -n equipment-management get secret equipment-management-runtime
```

### SMTPを使う場合

SMTPは任意です。
設定しなくても、ログイン、備品登録、貸出、返却は動作します。

メール通知を使う場合は、一時ファイルへ次の値も追加してからSecretを作ります。

```text
SMTP_HOST=smtp.example.ac.jp
SMTP_PORT=587
SMTP_USER=<SMTPユーザー>
SMTP_PASSWORD=<SMTPパスワード>
SMTP_FROM=備品管理 <noreply@example.ac.jp>
```

`SMTP_USER` と `SMTP_PASSWORD` は両方設定するか、両方省略します。
`SMTP_HOST` を設定した場合は `SMTP_FROM` も必要です。

## デプロイする

### 生成結果を検証する

最初に、Kustomizeが生成するYAMLをローカルで検証します。

```sh
kubectl kustomize deploy/k3s/overlays/tailnet \
  > /tmp/equipment-management.yaml
kubectl create --dry-run=client --validate=strict \
  -f /tmp/equipment-management.yaml -o name
```

次に、実際のAPI Serverで受理できることを確認します。

```sh
kubectl apply --server-side --dry-run=server \
  -k deploy/k3s/overlays/tailnet
```

`serverside-applied (server dry run)` と表示されれば、クラスタは変更されていません。
既存リソースの `last-applied-configuration` を移行できないというWarningだけなら、適用失敗ではありません。

### クラスタへ適用する

検証が成功したら適用します。

```sh
kubectl apply -k deploy/k3s/overlays/tailnet
kubectl -n equipment-management rollout status \
  deployment/equipment-management --timeout=5m
```

`successfully rolled out` と表示されたら起動しています。

## 初回データを用意する

### DBスキーマ

初回起動時に `/data/app.db` が自動作成され、必要なマイグレーションも自動適用されます。
SQLを手動実行する作業はありません。

次のログがあればDBの準備は完了しています。

```sh
kubectl -n equipment-management logs \
  deployment/equipment-management --tail=100
```

```text
db ready: /data/app.db
listening on :8080
```

`SMTP_HOST が未設定` というWarningは、メールを使わないデモでは正常です。

### 最初の管理者

最初の管理者はWeb画面から作れません。
Pod内のサーバーバイナリへ、管理者IDと表示名を渡して作成します。

```sh
kubectl -n equipment-management exec \
  deployment/equipment-management -- \
  /server -create-admin \
  -login-id '<管理者ID>' \
  -name '<管理者名>'
```

通知用メールアドレスも登録する場合は、末尾へ `-email '<メールアドレス>'` を追加します。

初期パスワードは、このコマンドの出力に一度だけ表示されます。
画面を閉じる前に管理者本人へ安全な方法で渡します。
初回ログイン時にはパスワード変更が求められます。

同じログインIDをもう一度指定すると作成に失敗します。
二人目以降の利用者は、管理画面の `/admin/users` から作成します。

### 備品データ

少数の備品は `/admin/items/new` から一件ずつ登録します。
写真はJPEGまたはPNGを使用し、一枚10MB以下にします。

多数の備品は `/admin/items/import` からCSVを取り込みます。
画面でプレビューと登録件数を確認してから確定します。
一行でもエラーがある場合は全件取り込まれないため、途中まで登録されることはありません。

デモ確認だけを行う間は、永久ラベルを印刷しません。
現在の `HOST_URL` をQRへ埋め込むと、Cloudflareの正式URLへ変更した後にそのQRを貼り替える必要があります。

## ステータスを確認する

### 一括確認

```sh
kubectl -n equipment-management get pod,pvc,service,ingress -o wide
```

正常時の条件は次のとおりです。

- Podが `1/1 Running` である。
- Podの `RESTARTS` が増え続けていない。
- 二つのPVCが `Bound` である。
- IngressのHostが `equipment-management.home.arpa` である。
- ServiceにCluster IPが割り当てられている。

### アプリへ直接到達するか確認する

DNSやhostsを設定する前でも、Hostヘッダーを明示すればTraefikを確認できます。

```sh
curl -fsS \
  --resolve equipment-management.home.arpa:80:192.168.13.150 \
  http://equipment-management.home.arpa/healthz
echo
```

正常時は `ok` と表示されます。
このヘルスチェックはHTTPプロセスだけでなく、SQLiteへの問い合わせも確認します。

名前解決を設定した後は、通常のURLでも確認します。

```sh
curl -fsS http://equipment-management.home.arpa/healthz
```

ブラウザでは次のURLを開きます。

```text
http://equipment-management.home.arpa
```

### ServiceとPodの接続を確認する

Traefikが503を返す場合は、ServiceにPodのIPが登録されているか確認します。

```sh
kubectl -n equipment-management get endpointslice \
  -l kubernetes.io/service-name=equipment-management -o wide
```

`ENDPOINTS` に `10.42.x.x` のようなPod IPがあれば接続先は登録されています。

### ログとイベントを確認する

```sh
kubectl -n equipment-management logs \
  deployment/equipment-management --tail=100
kubectl -n equipment-management get events \
  --sort-by=.lastTimestamp | tail -30
```

継続してログを見る場合は `--tail=100` を `--follow` に置き換えます。

## 更新する

アプリコードを更新した場合は、先に新しいコンテナイメージのWorkflowが成功したことを確認します。
その後、オーバーレイの `newTag` を新しい `sha-*` へ変更して適用します。

```sh
kubectl apply --server-side --dry-run=server \
  -k deploy/k3s/overlays/tailnet
kubectl apply -k deploy/k3s/overlays/tailnet
kubectl -n equipment-management rollout status \
  deployment/equipment-management --timeout=5m
```

起動時にDBマイグレーションが自動適用されます。
更新前のDBバックアップが無い状態で、DBスキーマを変更する版へ更新しません。

Secretを更新しただけでは、起動中のPodへ値は反映されません。
Secret更新後はPodを再作成します。

```sh
kubectl -n equipment-management rollout restart \
  deployment/equipment-management
kubectl -n equipment-management rollout status \
  deployment/equipment-management --timeout=5m
```

`SESSION_SECRET` を変更すると、ログイン中の全利用者がログアウトします。

## 停止する

### データを残して一時停止する

Ingressを削除してからPodを0台にします。
この手順ではSecret、Service、PVCを残します。

```sh
kubectl -n equipment-management delete ingress equipment-management
kubectl -n equipment-management scale \
  deployment/equipment-management --replicas=0
kubectl -n equipment-management wait \
  --for=delete pod \
  -l app.kubernetes.io/name=equipment-management \
  --timeout=120s
```

再開するときはマニフェストを再適用します。

```sh
kubectl apply -k deploy/k3s/overlays/tailnet
kubectl -n equipment-management rollout status \
  deployment/equipment-management --timeout=5m
```

### テスト用の実行リソースを片付ける

次の手順はアプリ、公開入口、テスト用Secretを削除しますが、DBと写真のPVCは残します。

```sh
kubectl -n equipment-management delete ingress equipment-management
kubectl -n equipment-management delete service equipment-management
kubectl -n equipment-management delete deployment equipment-management
kubectl -n equipment-management delete secret equipment-management-runtime
kubectl -n equipment-management wait \
  --for=delete pod \
  -l app.kubernetes.io/name=equipment-management \
  --timeout=120s
```

停止後にPodが残っていないことを確認します。

```sh
kubectl -n equipment-management get pods
kubectl -n equipment-management get pvc
```

`kubectl delete -k deploy/k3s/overlays/tailnet` は使用しません。
このコマンドはPVCまで削除対象に含むためです。

## データを守るための制約

SQLiteと写真は別のPVCに保存されます。
Deploymentを削除してもPVCは残ります。

ただし、`Retain` はバックアップではありません。
SQLiteのローカル領域、NFS VM、NFSのデータディスクが失われた場合に備え、別の機器へバックアップする運用が必要です。

アプリの `/server -backup` はSQLiteだけを一貫した状態で複製します。
写真は含まれないため、`/uploads` も別にバックアップします。

備品一覧は `/admin/items` の「全備品をCSVで書き出す」から保存できます。
CSVには利用者、貸出履歴、写真が含まれないため、DBバックアップの代わりにはなりません。

PVCを削除して空の状態へ戻す操作は、この日常手順には含めません。
`Retain` のPVと実データが残るため、PVCだけを削除すると「消えたように見えるがストレージには残っている」状態になるためです。
完全初期化が必要な場合は、バックアップ、対象PVC、PV、NFS上のディレクトリを特定してから別作業として実施します。

## よくあるエラーと処方箋

### `CreateContainerConfigError` と表示される

実行時Secretが無い場合に発生します。

```sh
kubectl -n equipment-management describe pod \
  -l app.kubernetes.io/name=equipment-management
kubectl -n equipment-management get secret equipment-management-runtime
```

Secretが無ければ「実行時Secretを作る」の手順で再作成します。
Secretの名前は `equipment-management-runtime` から変更しません。

### `ImagePullBackOff` または `ErrImagePull` と表示される

イメージタグが存在しないか、GHCRから取得できていません。

```sh
kubectl -n equipment-management describe pod \
  -l app.kubernetes.io/name=equipment-management
rg -n 'newName|newTag' deploy/k3s/overlays/tailnet/kustomization.yaml
gh run list --repo ruwseid/equipment-management \
  --workflow container-image.yml --limit 5
```

Workflowが成功したコミットの `sha-*` を指定します。
現在のGHCRイメージは匿名取得できるため、通常はイメージ取得用Secretを作りません。

### Podが `Pending` のままになる

まずPodのイベントを確認します。

```sh
kubectl -n equipment-management describe pod \
  -l app.kubernetes.io/name=equipment-management
kubectl get node k3s-server
```

`k3s-server` が `NotReady` の場合、アプリは起動しません。
SQLiteがそのノードのローカルPVCを使うため、別ノードへ自動移動させない設定です。

`Insufficient cpu` または `Insufficient memory` がある場合は、同ノードの負荷を確認します。

```sh
kubectl top node k3s-server
kubectl describe node k3s-server
```

### PVCが `Pending` のままになる

どちらのPVCが止まっているかを確認します。

```sh
kubectl -n equipment-management get pvc
kubectl -n equipment-management describe pvc equipment-management-data
kubectl -n equipment-management describe pvc equipment-management-uploads
```

`equipment-management-data` は `WaitForFirstConsumer` のため、Podを作る前の `Pending` は正常です。
Podを作った後も変わらない場合は、`k3s-server` の状態とlocal-path provisionerを確認します。

```sh
kubectl -n kube-system get pods | rg 'local-path'
```

`equipment-management-uploads` が止まる場合は、NFS CSIとNFS VMを確認します。

```sh
kubectl get storageclass nfs-rwx-retain
kubectl -n kube-system get pods \
  -l app.kubernetes.io/instance=csi-driver-nfs -o wide
ping -c 1 192.168.13.151
```

NFS VMが停止している場合は、VMを復旧してからPodを再作成します。

### Podが `CrashLoopBackOff` になる

アプリは設定不備をログへまとめて出して終了します。

```sh
kubectl -n equipment-management logs \
  deployment/equipment-management --previous --tail=150
kubectl -n equipment-management describe pod \
  -l app.kubernetes.io/name=equipment-management
```

よくある原因は次のとおりです。

- `SESSION_SECRET` が32文字未満である。
- `HOST_URL` の末尾に `/` が付いている。
- `HOST_URL` が `http://` または `https://` で始まっていない。
- `COOKIE_SECURE` が `true` または `false` 以外である。
- `SMTP_HOST` を設定したのに `SMTP_FROM` が無い。
- `SMTP_USER` と `SMTP_PASSWORD` の片方だけを設定している。
- `/data` または `/uploads` へ書き込めない。

Secretを修正した後は、Deploymentを再起動します。

### `/healthz` が503になる

再起動直後は、Traefikが新しいPodのEndpointを認識するまで数秒だけ503になることがあります。
10秒ほど待って再確認します。

継続する場合はPod、ログ、EndpointSliceを確認します。

```sh
kubectl -n equipment-management get pods
kubectl -n equipment-management logs \
  deployment/equipment-management --tail=150
kubectl -n equipment-management get endpointslice \
  -l kubernetes.io/service-name=equipment-management -o wide
```

PodがReadyでも `/healthz` が503なら、DBへの問い合わせが失敗しています。
PVCと `/data` に関するログを確認します。

### Open WebUIが表示される

IPアドレスへ直接アクセスした場合は、Open WebUIが表示されるのが正常です。
備品管理アプリは `equipment-management.home.arpa` というHost名で識別されます。

次のコマンドでアプリ自体へ到達できるか確認します。

```sh
curl -fsS \
  --resolve equipment-management.home.arpa:80:192.168.13.150 \
  http://equipment-management.home.arpa/healthz
```

このコマンドが `ok` を返す場合は、LAN DNSまたはhostsの設定を修正します。

### ログイン後にログイン画面へ戻る

HTTPで `COOKIE_SECURE=true` を使うと、ブラウザがセッションCookieを保存しません。
現在のHTTPデモでは `COOKIE_SECURE=false` にします。

CloudflareでHTTPS化した後は `COOKIE_SECURE=true` に戻します。
Secretを更新した後はDeploymentを再起動します。

### `forbidden` または `Unauthorized` と表示される

操作中のkubeconfigに必要な権限がありません。

```sh
kubectl config current-context
kubectl auth can-i create deployments -n equipment-management
kubectl auth can-i create secrets -n equipment-management
```

意図したクラスタと利用者へ切り替えてから再実行します。

### `exec: "sh": executable file not found` と表示される

本番イメージにはシェルが入っていません。
これはイメージを小さく保つための正常な仕様です。

`kubectl exec ... -- sh` は使わず、管理者作成などは `/server` を直接実行します。

```sh
kubectl -n equipment-management exec \
  deployment/equipment-management -- /server -create-admin \
  -login-id '<管理者ID>' -name '<管理者名>'
```

### `SMTP_HOST が未設定` と表示される

メール通知を使わない場合は正常なWarningです。
アプリの起動やログインには影響しません。

メール通知を使う予定なのに表示された場合は、SecretへSMTP設定を追加してDeploymentを再起動します。

### QRが誤ったURLを開く

QRには作成時点の `HOST_URL` が埋め込まれます。
Secretを直しても、印刷済みQRの内容は変わりません。

デモ期間は永久ラベルを印刷せず、Cloudflareの正式URLを決めてから本番ラベルを作成します。

## Tailscale Operatorの判断基準

次の条件を満たす間は、Tailscale Kubernetes Operatorを導入しません。

- 利用者が各自のsubnet routerからクラスタのLAN IPへ到達できる。
- 共通のローカル名をLAN DNSまたは各端末のhostsで解決できる。
- 公開対象がTraefik配下のHTTPまたはHTTPSに限られる。
- Tailscale identityやACLをアプリの認可に使わない。
- 部長のテスト経路を、こちらのtailnetへの招待に依存させない。

次の要件が生じた場合にOperatorを再検討します。

- subnet routeを公開せず、特定Serviceだけをtailnetへ公開する。
- Tailscale identityやACLでService単位のアクセス制御を行う。
- tailnet固有のHTTPS名やMagicDNS名を共有入口にする。
- 複数クラスタ間のService接続やKubernetes APIのtailnet公開を行う。

部長と別々のtailnetを使う間は、片方のtailnetだけで有効なMagicDNS名を共有URLにしません。

## Cloudflareへ切り替えるときの変更点

独自ドメインを取得してCloudflare Tunnelを導入するときは、次の三点を同時に変更します。

1. Ingressの `host` を正式な公開名へ変更する。
2. Secretの `HOST_URL` を `https://<正式な公開名>` へ変更する。
3. Secretの `COOKIE_SECURE` を `true` へ変更する。

Cloudflare側は正式な公開名からTraefikへ転送します。
アプリ、Service、PVCの構成は変更しません。

切り替え後にログイン、備品詳細、画像表示、QR読み取りを確認してから永久ラベルを印刷します。
