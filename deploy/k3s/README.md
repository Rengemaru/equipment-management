# k3s デプロイ

既存のCompose運用を残したまま、k3sへ移行するためのマニフェストです。
クラスタ構成、ストレージ、tailnet名、不変のコンテナイメージタグを確認するまで
適用しないでください。

## Gitに保存しない値

次のSecretはクラスタ側で作成します。生成したYAMLはコミットしません。

- `equipment-management/equipment-management-runtime`
  - 必須: `HOST_URL`, `SESSION_SECRET`
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

環境変数ファイルの `HOST_URL` はtailnetのHTTPS URLと一致させます。
`SESSION_SECRET` は32文字以上のランダム値にします。`COOKIE_SECURE=true` は
Deployment側で固定しています。

## 適用前の確認

1. `overlays/tailnet/traefik-ingress.yaml` の `TAILNET_NAME` を置換する。
2. `overlays/tailnet/kustomization.yaml` の `0.0.0-unpublished` を、
   Workflowが発行した不変の `sha-*` タグへ置換する。
3. SQLite PVCが `k3s-server` の `equipment-management-local-retain`、写真PVCが
   `nfs-rwx-retain` で作られることを確認する。
4. Tailscale Kubernetes Operatorと `tailscale` IngressClassを導入する。
   このoverlayはOperatorのインストールや認証を行わない。
5. Tailscale Ingressから既存の `kube-system/traefik` Serviceへ渡しても、
   クラスタ内の既存ルートへ影響しないことを確認する。

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
