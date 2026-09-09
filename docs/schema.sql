-- サークル備品管理システム / SQLite スキーマ
-- 対象: SQLite 3.35+
--
-- これは「現在DBに適用されている状態」の参照用スナップショット。
-- 実際に適用されるのは internal/db/migrations/ の連番SQL。両方を必ず同時に更新する。
--
-- 現在地: M2（0002_loans.sql まで適用）
--
-- missing_reports / notification_log / inventory_checks /
-- inventory_check_items / api_tokens、および一覧用のビューは、
-- それぞれ M3・M4 で追加する。ここにはまだ無い。
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

-- ============================================================
-- 貸出
-- ============================================================

CREATE TABLE loans (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id       INTEGER NOT NULL REFERENCES items(id),
    -- 借用者。代理登録では registered_by と異なる
    user_id       INTEGER NOT NULL REFERENCES users(id),
    -- 登録者。本人が登録した場合も自分の id を入れる。
    -- NULL可にして「NULLなら本人」とすると、代理かどうかの判定が2通りになる
    registered_by INTEGER NOT NULL REFERENCES users(id),
    -- UTC。datetime('now') と同じ 'YYYY-MM-DD HH:MM:SS' 形式で入れる
    borrowed_at   TEXT    NOT NULL DEFAULT (datetime('now')),
    -- JST の日付。'YYYY-MM-DD'。時刻を持たせない。
    -- UTC の日付で計算すると JST 00:00〜09:00 の借用が1日ずれる
    due_date      TEXT    NOT NULL,
    -- NULL = 貸出中（UTC）
    returned_at   TEXT,
    -- 返却を実行した人。借用者とは限らない。
    -- 棚に戻っているのを見つけた人が処理できないと、記録が永久にズレたままになる
    returned_by   INTEGER REFERENCES users(id),
    -- 誤登録の取り消し。返却とは別の事実として持つ。
    -- 取り消しを返却として記録すると、借りていない人の返却履歴が残り、
    -- 破損の追跡時に誤った経路をたどることになる
    cancelled_at  TEXT,
    cancelled_by  INTEGER REFERENCES users(id),
    cancel_reason TEXT    NOT NULL DEFAULT '',
    note          TEXT    NOT NULL DEFAULT '',
    created_at    TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- 二重貸出をDBで拒否する。
-- アプリ側の検査（SELECT してから INSERT）は競合に負ける。
-- 取り消し済みは対象外。取り消した備品は再度借りられる
CREATE UNIQUE INDEX idx_loans_active_item
    ON loans(item_id) WHERE returned_at IS NULL AND cancelled_at IS NULL;

CREATE INDEX idx_loans_user ON loans(user_id, borrowed_at DESC);
CREATE INDEX idx_loans_item ON loans(item_id, borrowed_at DESC);

-- 注意: loans の行を削除しない。返却しても残す。
--       破損・紛失の追跡はこの履歴が唯一の根拠になる。

-- ============================================================
-- 破損報告
-- ============================================================

CREATE TABLE damage_reports (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id      INTEGER NOT NULL REFERENCES items(id),
    -- 貸出中に報告されたら紐付ける。在庫中の報告では NULL
    loan_id      INTEGER REFERENCES loans(id),
    reporter_id  INTEGER NOT NULL REFERENCES users(id),
    reported_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    description  TEXT    NOT NULL,
    photo_path   TEXT,
    -- 報告は即時反映され、運営は事後に追認する。承認フローを作らない。
    -- 承認待ちの間システムが「良好」と表示し続ける状態は、紙の台帳より悪い
    status       TEXT    NOT NULL DEFAULT '未確認'
                         CHECK (status IN ('未確認', '確認済み', '修理済み', '廃棄')),
    confirmed_by INTEGER REFERENCES users(id),
    confirmed_at TEXT,
    note         TEXT    NOT NULL DEFAULT '',
    created_at   TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at   TEXT    NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_damage_reports_item   ON damage_reports(item_id, reported_at DESC);
CREATE INDEX idx_damage_reports_status ON damage_reports(status);
