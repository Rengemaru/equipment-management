# k3s デプロイ

既存のCompose運用を残したまま、k3sへ移行するためのマニフェストです。
クラスタ構成、ストレージ、tailnet名、不変のコンテナイメージタグを確認するまで
適用しないでください。

## Gitに保存しない値

次のSecretはクラスタ側で作成します。生成したYAMLはコミットしません。

- `equipment-management/equipment-management-runtime`
  - 必須: `HOST_URL`, `SESSION_SECRET`, `COOKIE_SECURE`
  - 任意: `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM`
- `equipment-management/equipment-management-registry`
  - GHCRのパッケージが非公開の間だけ必要

秘密値をシェル履歴へ残さないよう、権限を600にした環境変数ファイルと
Docker設定ファイルから作成します。

```sh
kubectl apply -f deploy/k3s/base/namespace.yaml
kubectl -n equipment-management create secret generic equipment-management-runtime \
  --from-env-file=/secure/path/equipment-management.env
kubectl -n equipment-management create secret docker-registry equipment-management-registry \
  --from-file=/secure/path/docker-config.json
```

`SESSION_SECRET` は32文字以上のランダム値にします。subnet router経由で
TraefikのHTTP入口を使う検証環境では `COOKIE_SECURE=false`、将来HTTPSへ
切り替えるときは `true` にします。

## 適用前の確認

1. `overlays/tailnet/kustomization.yaml` の `newTag` が、Workflowで発行済みの
   不変な `sha-*` タグを指していることを確認する。
2. SQLite PVCが `k3s-server` の `equipment-management-local-retain`、写真PVCが
   `nfs-rwx-retain` で作られることを確認する。
3. 両方のtailnetから、それぞれのsubnet routerを経由してTraefikのLAN IPへ
   到達できることを確認する。
4. hostを限定しないIngressがLAN内へ公開されることを理解し、テスト期間中の
   利用者認証とネットワーク境界を確認する。

## Tailscale Operatorの判断基準

今回はTailscale Kubernetes Operatorを導入しません。サービスの共有相手である
部長も同じLANサブネットへ到達するsubnet routerを持つため、TraefikのLAN入口を
共通経路にする方が、どちらか一方のtailnetへ依存しないためです。

次の条件を満たす間はOperatorを不要と判断します。

- 利用者が各自のsubnet router経由でクラスタのLAN IPへ到達できる。
- 公開対象がHTTP/HTTPSのTraefik配下に限られる。
- tailnet固有のMagicDNS名やTailscale identityをアプリ認可に使わない。
- 部長のテスト経路をこちらのtailnetへの招待やACL変更に依存させない。

次のいずれかが必要になった場合だけ、Operatorを再検討します。

- subnet routeを公開せず、特定Serviceだけをtailnetへ出したい。
- Tailscale identityやACLでService単位のアクセス制御を行いたい。
- LAN IPや独自DNS設定を利用者へ意識させず、tailnet固有のHTTPS名を使いたい。
- 複数クラスタ間のService接続や、Kubernetes APIのtailnet公開が必要になった。

Operatorを使う場合は、部長が同じtailnetのServiceへ到達できるかを先に確認します。
別tailnetのままなら、Operatorが発行するMagicDNS名を共有入口にせず、現在の
subnet-router経路を残します。

API Serverへ送る前に、生成結果を確認します。

```sh
kubectl kustomize deploy/k3s/overlays/tailnet
```

初回適用前にはserver-side dry-runも行います。

```sh
kubectl apply --server-side --dry-run=server \
  -k deploy/k3s/overlays/tailnet
```

## ストレージと更新の制約

- アプリは意図的に1レプリカ、更新方式は `Recreate` とする。SQLiteと写真は
  ReadWriteOnce PVCとしてマウントする。
- SQLiteは `k3s-server` に固定し、reclaim policyが `Retain` のローカル領域を
  使う。写真は既存の `nfs-rwx-retain` を使う。
- `/data` と `/uploads` は別々に検証済みバックアップを取る。同じPVCに残した
  コピーはバックアップとして扱わない。
- 一時的なtailnet URLを `HOST_URL` にする間は、そのURLをCloudflare移行後も
  残す場合を除き、永久ラベルを印刷しない。
- 空の検証環境への復元試験が完了するまで `compose.yaml` を削除しない。
