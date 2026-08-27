-- サークル備品管理システム / SQLite スキーマ
-- 対象: SQLite 3.35+
--
-- これは「現在DBに適用されている状態」の参照用スナップショット。
-- 実際に適用されるのは internal/db/migrations/ の連番SQL。両方を必ず同時に更新する。
--
-- 現在地: M1（0001_init.sql まで適用）
--
-- loans / missing_reports / damage_reports / notification_log /
-- inventory_checks / inventory_check_items / api_tokens、および
-- 一覧用のビューは、それぞれ M2・M3・M4 で追加する。ここにはまだ無い。
-- 完成形の設計は docs/equipment-management-requirements.md §5 データモデル にある。

PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;

-- ============================================================
-- マイグレーション履歴（ランナーが自動で作る）
-- ============================================================

-- internal/db の Migrate が起動時に作成・更新する。連番SQL側では作らない。
CREATE TABLE schema_migrations (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- ユーザー / 認証
-- ============================================================

CREATE TABLE users (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL,
    -- ログインID。admin が発行する。英数字とハイフン・アンダースコアのみ、大文字小文字を区別しない
    -- 運用のため小文字に正規化して格納する（'Yamada' と 'yamada' を別人にしない）
    login_id    TEXT    NOT NULL UNIQUE,
    -- bcrypt ハッシュ。平文は保存しない
    password_hash TEXT  NOT NULL,
    -- 1 = admin が発行した初期パスワードのまま。次回ログイン時に変更を強制する
    must_change_password INTEGER NOT NULL DEFAULT 1
                        CHECK (must_change_password IN (0, 1)),
    -- 通知用。認証には使わないため NULL 可。
    -- SMTP が使えない環境では未設定のまま運用でき、その場合メール通知はスキップする
    email       TEXT    UNIQUE,
    role        TEXT    NOT NULL DEFAULT 'member'
                        CHECK (role IN ('admin', 'member')),
    -- 卒業者は is_active=0 にする。DELETE すると貸出履歴が壊れるため削除しない
    is_active   INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1)),
    created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- 認証は admin 発行の ID + パスワード。マジックリンクは採用しない。
-- 理由: マジックリンクは SMTP が使えることが動作の前提になる。学内SMTPの可否が未確定な段階で
--       ログイン手段をメールに依存させると、SMTP が使えない場合にシステム全体が起動できない。
--       パスワード方式はメールに一切依存しないため、SMTP の可否によらず必ず動く。
-- 「名前選択式では否認を排除できない」という当初の要件は、個人ごとの認証情報がある本方式でも満たされる。

-- パスワード試行の記録。総当たりを鈍らせるために使う（成功したら削除する）
CREATE TABLE login_attempts (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    login_id     TEXT    NOT NULL,   -- 存在しないIDへの試行も記録するため users を参照しない
    attempted_at TEXT    NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_login_attempts ON login_attempts(login_id, attempted_at);

-- 長期セッション（1年）。これがあるため2回目以降はログイン操作が発生しない
CREATE TABLE sessions (
    id          TEXT    PRIMARY KEY,       -- 十分な長さのランダム文字列
    user_id     INTEGER NOT NULL REFERENCES users(id),
    expires_at  TEXT    NOT NULL,
    last_seen_at TEXT,
    created_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_sessions_user ON sessions(user_id);

-- ============================================================
-- 備品マスタ
-- ============================================================

CREATE TABLE items (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    -- QRに埋め込む一意コード。'0001' 形式の4桁ゼロパディング連番。
    -- 分類などの意味を持たせないこと（分類は変わるがラベルは貼り替えられない）
    code            TEXT    NOT NULL UNIQUE,
    name            TEXT    NOT NULL,
    category        TEXT    NOT NULL DEFAULT '未分類',
    model           TEXT,
    owner           TEXT    NOT NULL DEFAULT 'サークル'
                            CHECK (owner IN ('サークル', '学科')),
    -- 1 = 記録不要の自由利用品。貸出フローの対象外。物理的にQRを貼らない運用と対にする
    is_free_use     INTEGER NOT NULL DEFAULT 0 CHECK (is_free_use IN (0, 1)),
    location        TEXT,
    condition       TEXT    NOT NULL DEFAULT '良好'
                            CHECK (condition IN ('良好', '要修理', '廃棄')),
    -- 貸出状態とは独立。「貸出中かつ所在不明」は起こり得る
    location_status TEXT    NOT NULL DEFAULT '在庫'
                            CHECK (location_status IN ('在庫', '所在不明_未確認', '所在不明_確定')),
    photo_path      TEXT,
    note            TEXT,
    created_at      TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT    NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_items_category        ON items(category);
CREATE INDEX idx_items_location_status ON items(location_status);
CREATE INDEX idx_items_free_use        ON items(is_free_use);

-- 注意: 「貸出中かどうか」を items に持たせないこと。
--       loans から導出する（returned_at IS NULL）。二重管理は必ず不整合を起こす。
