-- M1: 認証とユーザー、備品マスタ。
--
-- loans 以降は各マイルストーンで 0002, 0003... として足す。
-- 使わないテーブルを先に作らないのは、DBを覗いた人が「実装済みだが使われていない」のか
-- 「未実装」のかを区別できるようにするため。設計の全体像は
-- docs/equipment-management-requirements.md §5 にある。
--
-- PRAGMA はここに書かない。foreign_keys / journal_mode は接続ごとの設定であり、
-- journal_mode = WAL はトランザクション内では変更できない（ランナーは1ファイルを
-- 1トランザクションで実行する）。接続時に設定する。

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
