-- M2: 貸出・返却。
--
-- missing_reports / inventory_checks / api_tokens は M3・M4 で足す。
-- 使わないテーブルを先に作らないのは、DBを覗いた人が「実装済みだが使われていない」のか
-- 「未実装」のかを区別できるようにするため。
--
-- 完成形の設計は docs/equipment-management-requirements.md §5、
-- このマイルストーンの仕様は docs/m2-implementation-spec.md §8 にある。

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
