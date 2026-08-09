-- サークル備品管理システム / SQLite スキーマ
-- 対象: SQLite 3.35+

PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;

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
-- 貸出履歴
-- ============================================================

CREATE TABLE loans (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id         INTEGER NOT NULL REFERENCES items(id),
    user_id         INTEGER NOT NULL REFERENCES users(id),   -- 借用者
    -- 代理登録の場合、登録者と借用者が異なる
    registered_by   INTEGER NOT NULL REFERENCES users(id),
    borrowed_at     TEXT    NOT NULL DEFAULT (datetime('now')),
    -- 借用日+14日をデフォルトで自動入力する。変えたい人だけ変える（摩擦を増やさない）
    due_date        TEXT    NOT NULL,
    returned_at     TEXT,                                    -- NULL = 貸出中
    returned_by     INTEGER REFERENCES users(id),
    note            TEXT,
    created_at      TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- 返却しても行を削除しない。破損・紛失の追跡はこの履歴が唯一の根拠になる。

-- 同一備品が同時に2件貸出中にならないことをDBレベルで保証する
CREATE UNIQUE INDEX idx_loans_one_active_per_item
    ON loans(item_id) WHERE returned_at IS NULL;
CREATE INDEX idx_loans_user_active
    ON loans(user_id) WHERE returned_at IS NULL;
CREATE INDEX idx_loans_due_date
    ON loans(due_date) WHERE returned_at IS NULL;
CREATE INDEX idx_loans_item ON loans(item_id);

-- ============================================================
-- 所在不明報告
-- ============================================================

CREATE TABLE missing_reports (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id       INTEGER NOT NULL REFERENCES items(id),
    -- 報告者を必ず記録する。複数人の独立した報告は、承認者1人より強い証拠になる
    reporter_id   INTEGER NOT NULL REFERENCES users(id),
    reported_at   TEXT    NOT NULL DEFAULT (datetime('now')),
    status        TEXT    NOT NULL DEFAULT '未確認'
                          CHECK (status IN ('未確認', '確定', '発見済み')),
    confirmed_by  INTEGER REFERENCES users(id),
    confirmed_at  TEXT,
    note          TEXT
);
CREATE INDEX idx_missing_item   ON missing_reports(item_id);
CREATE INDEX idx_missing_status ON missing_reports(status);

-- 承認フローは設けない。報告は即時 items.location_status を '所在不明_未確認' にする。
-- 運営は事後に '確定' / '発見済み' へ更新する（承認ではなく追認）。

-- ============================================================
-- 破損報告
-- ============================================================

CREATE TABLE damage_reports (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id       INTEGER NOT NULL REFERENCES items(id),
    reporter_id   INTEGER NOT NULL REFERENCES users(id),
    -- 返却フローから報告された場合、その貸出を紐付ける。
    -- 「いつの貸出中に壊れたか」を後から辿る唯一の手がかりになる
    loan_id       INTEGER REFERENCES loans(id),
    reported_at   TEXT    NOT NULL DEFAULT (datetime('now')),
    description   TEXT    NOT NULL,          -- 破損箇所・状況
    photo_path    TEXT,
    status        TEXT    NOT NULL DEFAULT '未確認'
                          CHECK (status IN ('未確認', '確認済み', '修理済み', '廃棄')),
    confirmed_by  INTEGER REFERENCES users(id),
    confirmed_at  TEXT,
    note          TEXT
);
CREATE INDEX idx_damage_item   ON damage_reports(item_id);
CREATE INDEX idx_damage_status ON damage_reports(status);
CREATE INDEX idx_damage_loan   ON damage_reports(loan_id);

-- 所在不明報告と同じく承認フローは設けない。報告は即時 items.condition を '要修理' にする。

-- ============================================================
-- 通知ログ（二重送信防止・冪等性の担保）
-- ============================================================

CREATE TABLE notification_log (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id),
    kind        TEXT    NOT NULL CHECK (kind IN ('月次', '期限超過', '代理登録')),
    -- 同一種別を同一期間に二度送らないための鍵。例: '2026-09' / 'loan:123'
    dedupe_key  TEXT    NOT NULL,
    sent_at     TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE (user_id, kind, dedupe_key)
);

-- アプリ内スケジューラは再起動で実行タイミングを取りこぼす。
-- 「送ったかどうか」をこのテーブルで判定し、起動時に未送信分を送る設計にすること。

-- ============================================================
-- 棚卸し（学期2回・大掃除に相乗り）
-- ============================================================

CREATE TABLE inventory_checks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL,          -- 例: '2026年度前期 大掃除'
    started_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    finished_at TEXT,
    created_by  INTEGER NOT NULL REFERENCES users(id)
);

CREATE TABLE inventory_check_items (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    check_id    INTEGER NOT NULL REFERENCES inventory_checks(id),
    item_id     INTEGER NOT NULL REFERENCES items(id),
    result      TEXT    NOT NULL CHECK (result IN ('確認済み', '未発見')),
    checked_by  INTEGER NOT NULL REFERENCES users(id),
    checked_at  TEXT    NOT NULL DEFAULT (datetime('now')),
    UNIQUE (check_id, item_id)
);
CREATE INDEX idx_check_items_check ON inventory_check_items(check_id);

-- 棚卸し終了時、その回で '確認済み' が付かなかった備品は
-- location_status を '所在不明_未確認' に一括遷移させる。

-- ============================================================
-- 外部連携（サイネージ等・read only）
-- ============================================================

CREATE TABLE api_tokens (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL,          -- 例: 'サイネージ'
    token_hash  TEXT    NOT NULL UNIQUE,
    scope       TEXT    NOT NULL DEFAULT 'read' CHECK (scope IN ('read')),
    is_active   INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1)),
    last_used_at TEXT,
    created_at  TEXT    NOT NULL DEFAULT (datetime('now'))
);

-- ============================================================
-- ビュー
-- ============================================================

-- 貸出中一覧（全メンバーに公開。可視性は罰則より強く働く）
CREATE VIEW v_active_loans AS
SELECT
    l.id          AS loan_id,
    i.code        AS item_code,
    i.name        AS item_name,
    i.category    AS item_category,
    u.name        AS borrower_name,
    l.borrowed_at,
    l.due_date,
    CASE WHEN date(l.due_date) < date('now') THEN 1 ELSE 0 END AS is_overdue
FROM loans l
JOIN items i ON i.id = l.item_id
JOIN users u ON u.id = l.user_id
WHERE l.returned_at IS NULL;

-- 期限超過のみ（サイネージで実名表示するのはこの範囲だけ）
CREATE VIEW v_overdue_loans AS
SELECT
    i.code      AS item_code,
    i.name      AS item_name,
    u.name      AS borrower_name,
    l.due_date,
    CAST(julianday('now') - julianday(l.due_date) AS INTEGER) AS days_overdue
FROM loans l
JOIN items i ON i.id = l.item_id
JOIN users u ON u.id = l.user_id
WHERE l.returned_at IS NULL
  AND date(l.due_date) < date('now');

-- 備品の現在状態（貸出状態は導出、マスタには持たせない）
CREATE VIEW v_item_status AS
SELECT
    i.id,
    i.code,
    i.name,
    i.category,
    i.location,
    i.condition,
    i.location_status,
    i.is_free_use,
    CASE WHEN l.id IS NULL THEN '在庫' ELSE '貸出中' END AS loan_status,
    u.name AS current_borrower
FROM items i
LEFT JOIN loans l ON l.item_id = i.id AND l.returned_at IS NULL
LEFT JOIN users u ON u.id = l.user_id;
