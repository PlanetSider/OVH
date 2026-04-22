import json
import os
import sqlite3
import threading
from typing import Any, Callable, Dict, Optional


class SQLiteStorage:
    def __init__(self, db_path: str):
        self.db_path = db_path
        self._lock = threading.RLock()
        parent_dir = os.path.dirname(db_path)
        if parent_dir:
            os.makedirs(parent_dir, exist_ok=True)
        self._init_db()

    def _connect(self):
        conn = sqlite3.connect(self.db_path)
        conn.row_factory = sqlite3.Row
        conn.execute("PRAGMA journal_mode=WAL")
        conn.execute("PRAGMA synchronous=NORMAL")
        return conn

    def _init_db(self):
        with self._connect() as conn:
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS documents (
                    key TEXT PRIMARY KEY,
                    value TEXT NOT NULL,
                    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
                )
                """
            )
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS metadata (
                    key TEXT PRIMARY KEY,
                    value TEXT NOT NULL,
                    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
                )
                """
            )
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS accounts (
                    id TEXT PRIMARY KEY,
                    alias TEXT,
                    app_key TEXT NOT NULL DEFAULT '',
                    app_secret TEXT NOT NULL DEFAULT '',
                    consumer_key TEXT NOT NULL DEFAULT '',
                    endpoint TEXT NOT NULL DEFAULT 'ovh-eu',
                    zone TEXT NOT NULL DEFAULT 'IE',
                    raw_json TEXT NOT NULL,
                    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
                )
                """
            )
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS queue_items (
                    id TEXT PRIMARY KEY,
                    account_id TEXT,
                    plan_code TEXT NOT NULL DEFAULT '',
                    status TEXT NOT NULL DEFAULT 'pending',
                    created_at TEXT,
                    updated_at TEXT,
                    retry_interval INTEGER,
                    retry_count INTEGER,
                    last_check_time INTEGER,
                    next_attempt_at INTEGER,
                    purchased INTEGER,
                    failure_count INTEGER,
                    quantity INTEGER,
                    auto_pay INTEGER,
                    raw_json TEXT NOT NULL,
                    updated_row_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
                )
                """
            )
            conn.execute("CREATE INDEX IF NOT EXISTS idx_queue_items_account_id ON queue_items(account_id)")
            conn.execute("CREATE INDEX IF NOT EXISTS idx_queue_items_status ON queue_items(status)")
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS purchase_history (
                    id TEXT PRIMARY KEY,
                    task_id TEXT,
                    account_id TEXT,
                    plan_code TEXT NOT NULL DEFAULT '',
                    status TEXT NOT NULL DEFAULT '',
                    datacenter TEXT,
                    purchase_time TEXT,
                    raw_json TEXT NOT NULL,
                    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
                )
                """
            )
            conn.execute("CREATE INDEX IF NOT EXISTS idx_purchase_history_account_id ON purchase_history(account_id)")
            conn.execute("CREATE INDEX IF NOT EXISTS idx_purchase_history_task_id ON purchase_history(task_id)")
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS monitor_subscriptions (
                    account_id TEXT,
                    plan_code TEXT,
                    server_name TEXT,
                    datacenters_json TEXT NOT NULL DEFAULT '[]',
                    notify_available INTEGER NOT NULL DEFAULT 1,
                    notify_unavailable INTEGER NOT NULL DEFAULT 0,
                    last_status_json TEXT NOT NULL DEFAULT '{}',
                    history_json TEXT NOT NULL DEFAULT '[]',
                    auto_order INTEGER NOT NULL DEFAULT 0,
                    auto_order_quantity INTEGER NOT NULL DEFAULT 0,
                    created_at TEXT,
                    raw_json TEXT NOT NULL,
                    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
                    PRIMARY KEY (account_id, plan_code)
                )
                """
            )
            conn.execute("CREATE INDEX IF NOT EXISTS idx_monitor_subscriptions_account_id ON monitor_subscriptions(account_id)")
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS monitor_states (
                    account_id TEXT PRIMARY KEY,
                    known_servers_json TEXT NOT NULL DEFAULT '[]',
                    check_interval INTEGER NOT NULL DEFAULT 20,
                    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
                )
                """
            )
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS server_plans (
                    plan_code TEXT PRIMARY KEY,
                    name TEXT,
                    memory TEXT,
                    storage TEXT,
                    raw_json TEXT NOT NULL,
                    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
                )
                """
            )
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS snapshots (
                    snapshot_type TEXT PRIMARY KEY,
                    updated_at_value TEXT,
                    raw_json TEXT NOT NULL,
                    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
                )
                """
            )
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS feishu_user_bindings (
                    account_id TEXT PRIMARY KEY,
                    open_id TEXT NOT NULL DEFAULT '',
                    user_name TEXT NOT NULL DEFAULT '',
                    raw_json TEXT NOT NULL,
                    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
                )
                """
            )
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS bot_account_selections (
                    channel TEXT NOT NULL,
                    user_key TEXT NOT NULL,
                    account_id TEXT NOT NULL,
                    updated_at_value TEXT,
                    raw_json TEXT NOT NULL,
                    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
                    PRIMARY KEY (channel, user_key)
                )
                """
            )
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS server_aliases (
                    account_id TEXT NOT NULL,
                    service_name TEXT NOT NULL,
                    alias TEXT NOT NULL,
                    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
                    PRIMARY KEY (account_id, service_name)
                )
                """
            )
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS bot_pending_actions (
                    channel TEXT NOT NULL,
                    user_key TEXT NOT NULL,
                    raw_json TEXT NOT NULL,
                    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
                    PRIMARY KEY (channel, user_key)
                )
                """
            )
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS config_sniper_tasks (
                    id TEXT PRIMARY KEY,
                    api1_plan_code TEXT,
                    match_status TEXT,
                    enabled INTEGER NOT NULL DEFAULT 1,
                    raw_json TEXT NOT NULL,
                    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
                )
                """
            )
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS vps_subscriptions (
                    id TEXT PRIMARY KEY,
                    plan_code TEXT NOT NULL,
                    ovh_subsidiary TEXT NOT NULL,
                    raw_json TEXT NOT NULL,
                    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
                )
                """
            )
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS vps_monitor_state (
                    key TEXT PRIMARY KEY,
                    value TEXT NOT NULL,
                    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
                )
                """
            )
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS logs (
                    id TEXT PRIMARY KEY,
                    timestamp TEXT NOT NULL,
                    level TEXT NOT NULL,
                    source TEXT NOT NULL,
                    message TEXT NOT NULL,
                    raw_json TEXT NOT NULL,
                    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
                )
                """
            )
            conn.execute("CREATE INDEX IF NOT EXISTS idx_logs_timestamp ON logs(timestamp)")
            conn.execute(
                """
                CREATE TABLE IF NOT EXISTS app_config (
                    key TEXT PRIMARY KEY,
                    raw_json TEXT NOT NULL,
                    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
                )
                """
            )

    def get_json(self, key: str, default: Any = None):
        with self._lock, self._connect() as conn:
            row = conn.execute("SELECT value FROM documents WHERE key = ?", (key,)).fetchone()
        if not row:
            return default
        try:
            return json.loads(row["value"])
        except json.JSONDecodeError:
            return default

    def set_json(self, key: str, value: Any):
        payload = json.dumps(value, ensure_ascii=False)
        with self._lock, self._connect() as conn:
            conn.execute(
                """
                INSERT INTO documents(key, value, updated_at)
                VALUES (?, ?, CURRENT_TIMESTAMP)
                ON CONFLICT(key) DO UPDATE SET
                    value = excluded.value,
                    updated_at = CURRENT_TIMESTAMP
                """,
                (key, payload),
            )

    def has_key(self, key: str) -> bool:
        with self._lock, self._connect() as conn:
            row = conn.execute("SELECT 1 FROM documents WHERE key = ?", (key,)).fetchone()
        return bool(row)

    def get_meta(self, key: str, default: Any = None):
        with self._lock, self._connect() as conn:
            row = conn.execute("SELECT value FROM metadata WHERE key = ?", (key,)).fetchone()
        if not row:
            return default
        try:
            return json.loads(row["value"])
        except json.JSONDecodeError:
            return default

    def set_meta(self, key: str, value: Any):
        payload = json.dumps(value, ensure_ascii=False)
        with self._lock, self._connect() as conn:
            conn.execute(
                """
                INSERT INTO metadata(key, value, updated_at)
                VALUES (?, ?, CURRENT_TIMESTAMP)
                ON CONFLICT(key) DO UPDATE SET
                    value = excluded.value,
                    updated_at = CURRENT_TIMESTAMP
                """,
                (key, payload),
            )

    def replace_logs(self, items, limit=1000):
        trimmed = list(items or [])[-int(limit):]
        with self._lock, self._connect() as conn:
            conn.execute("DELETE FROM logs")
            for item in trimmed:
                self._insert_log(conn, item)

    def append_log(self, item, limit=1000):
        with self._lock, self._connect() as conn:
            self._insert_log(conn, item)
            count_row = conn.execute("SELECT COUNT(*) AS cnt FROM logs").fetchone()
            count = int((count_row["cnt"] if count_row else 0) or 0)
            overflow = max(0, count - int(limit))
            if overflow > 0:
                conn.execute(
                    "DELETE FROM logs WHERE id IN (SELECT id FROM logs ORDER BY timestamp ASC, id ASC LIMIT ?)",
                    (overflow,),
                )

    def _insert_log(self, conn, item):
        if not isinstance(item, dict) or not item.get("id"):
            return
        conn.execute(
            """
            INSERT INTO logs(id, timestamp, level, source, message, raw_json, created_at)
            VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
            ON CONFLICT(id) DO UPDATE SET
                timestamp = excluded.timestamp,
                level = excluded.level,
                source = excluded.source,
                message = excluded.message,
                raw_json = excluded.raw_json
            """,
            (
                item.get("id"),
                item.get("timestamp") or "",
                item.get("level") or "INFO",
                item.get("source") or "system",
                item.get("message") or "",
                json.dumps(item, ensure_ascii=False),
            ),
        )

    def load_logs(self, limit=1000):
        with self._lock, self._connect() as conn:
            rows = conn.execute(
                "SELECT raw_json FROM logs ORDER BY timestamp DESC, id DESC LIMIT ?",
                (int(limit),),
            ).fetchall()
        items = []
        for row in rows:
            try:
                items.append(json.loads(row["raw_json"]))
            except json.JSONDecodeError:
                continue
        items.reverse()
        return items

    def clear_logs(self):
        with self._lock, self._connect() as conn:
            conn.execute("DELETE FROM logs")

    def save_config(self, key, value):
        with self._lock, self._connect() as conn:
            conn.execute(
                """
                INSERT INTO app_config(key, raw_json, updated_at)
                VALUES (?, ?, CURRENT_TIMESTAMP)
                ON CONFLICT(key) DO UPDATE SET
                    raw_json = excluded.raw_json,
                    updated_at = CURRENT_TIMESTAMP
                """,
                (str(key), json.dumps(value, ensure_ascii=False)),
            )

    def load_config(self, key):
        with self._lock, self._connect() as conn:
            row = conn.execute("SELECT raw_json FROM app_config WHERE key = ?", (str(key),)).fetchone()
        if not row:
            return None
        try:
            return json.loads(row["raw_json"])
        except json.JSONDecodeError:
            return None

    def import_json_documents(self, mapping: Dict[str, str], read_json_file: Callable[[str], Optional[Any]]):
        imported = []
        with self._lock:
            already_imported = bool(self.get_meta("legacy_json_imported", False))
            if already_imported:
                return imported
            for key, path in mapping.items():
                if self.has_key(key):
                    continue
                data = read_json_file(path)
                if data is None:
                    continue
                self.set_json(key, data)
                imported.append(key)
            self.set_meta("legacy_json_imported", True)
        return imported

    def replace_accounts(self, accounts):
        with self._lock, self._connect() as conn:
            conn.execute("DELETE FROM accounts")
            for account in accounts:
                if not isinstance(account, dict):
                    continue
                account_id = account.get("id")
                if not account_id:
                    continue
                conn.execute(
                    """
                    INSERT INTO accounts(
                        id, alias, app_key, app_secret, consumer_key, endpoint, zone, raw_json, updated_at
                    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
                    """,
                    (
                        account_id,
                        account.get("alias"),
                        account.get("appKey", ""),
                        account.get("appSecret", ""),
                        account.get("consumerKey", ""),
                        account.get("endpoint", "ovh-eu"),
                        account.get("zone", "IE"),
                        json.dumps(account, ensure_ascii=False),
                    ),
                )

    def load_accounts(self):
        with self._lock, self._connect() as conn:
            rows = conn.execute("SELECT raw_json FROM accounts ORDER BY updated_at, id").fetchall()
        items = []
        for row in rows:
            try:
                items.append(json.loads(row["raw_json"]))
            except json.JSONDecodeError:
                continue
        return items

    def list_accounts(self):
        return self.load_accounts()

    def replace_queue_items(self, items):
        with self._lock, self._connect() as conn:
            conn.execute("DELETE FROM queue_items")
            for item in items:
                self._insert_queue_item(conn, item)

    def upsert_queue_item(self, item):
        with self._lock, self._connect() as conn:
            self._insert_queue_item(conn, item)

    def get_queue_item(self, item_id):
        with self._lock, self._connect() as conn:
            row = conn.execute("SELECT raw_json FROM queue_items WHERE id = ?", (item_id,)).fetchone()
        if not row:
            return None
        try:
            return json.loads(row["raw_json"])
        except json.JSONDecodeError:
            return None

    def _insert_queue_item(self, conn, item):
        if not isinstance(item, dict) or not item.get("id"):
            return
        conn.execute(
            """
            INSERT INTO queue_items(
                id, account_id, plan_code, status, created_at, updated_at,
                retry_interval, retry_count, last_check_time, next_attempt_at,
                purchased, failure_count, quantity, auto_pay, raw_json, updated_row_at
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
            ON CONFLICT(id) DO UPDATE SET
                account_id = excluded.account_id,
                plan_code = excluded.plan_code,
                status = excluded.status,
                created_at = excluded.created_at,
                updated_at = excluded.updated_at,
                retry_interval = excluded.retry_interval,
                retry_count = excluded.retry_count,
                last_check_time = excluded.last_check_time,
                next_attempt_at = excluded.next_attempt_at,
                purchased = excluded.purchased,
                failure_count = excluded.failure_count,
                quantity = excluded.quantity,
                auto_pay = excluded.auto_pay,
                raw_json = excluded.raw_json,
                updated_row_at = CURRENT_TIMESTAMP
            """,
            (
                item.get("id"),
                item.get("accountId"),
                item.get("planCode", ""),
                item.get("status", "pending"),
                item.get("createdAt"),
                item.get("updatedAt"),
                item.get("retryInterval"),
                item.get("retryCount"),
                item.get("lastCheckTime"),
                item.get("nextAttemptAt"),
                item.get("purchased"),
                item.get("failureCount"),
                item.get("quantity"),
                1 if item.get("auto_pay") else 0,
                json.dumps(item, ensure_ascii=False),
            ),
        )

    def load_queue_items(self):
        with self._lock, self._connect() as conn:
            rows = conn.execute("SELECT raw_json FROM queue_items ORDER BY created_at, id").fetchall()
        items = []
        for row in rows:
            try:
                items.append(json.loads(row["raw_json"]))
            except json.JSONDecodeError:
                continue
        return items

    def list_queue_items(self, account_id=None, status=None, limit=None, offset=None):
        clauses = []
        params = []
        if account_id:
            clauses.append("account_id = ?")
            params.append(account_id)
        if status and status != "all":
            clauses.append("LOWER(status) = ?")
            params.append(str(status).lower())
        where_sql = f"WHERE {' AND '.join(clauses)}" if clauses else ""
        sql = f"SELECT raw_json FROM queue_items {where_sql} ORDER BY COALESCE(updated_at, '') DESC, COALESCE(created_at, '') DESC, id DESC"
        if limit is not None:
            sql += " LIMIT ?"
            params.append(int(limit))
        if offset is not None:
            sql += " OFFSET ?"
            params.append(int(offset))
        with self._lock, self._connect() as conn:
            rows = conn.execute(sql, tuple(params)).fetchall()
        items = []
        seen = set()
        for row in rows:
            try:
                item = json.loads(row["raw_json"])
            except json.JSONDecodeError:
                continue
            item_id = item.get("id")
            if item_id and item_id in seen:
                continue
            if item_id:
                seen.add(item_id)
            items.append(item)
        return items

    def list_runnable_queue_items(self, now_ts, processing_ids=None, deleted_ids=None, limit=None):
        processing_set = {str(x) for x in (processing_ids or [])}
        deleted_set = {str(x) for x in (deleted_ids or [])}
        with self._lock, self._connect() as conn:
            rows = conn.execute(
                """
                SELECT raw_json FROM queue_items
                WHERE LOWER(status) = 'running'
                  AND COALESCE(purchased, 0) < COALESCE(quantity, 1)
                  AND (COALESCE(next_attempt_at, 0) = 0 OR ? >= COALESCE(next_attempt_at, 0))
                ORDER BY CASE WHEN json_extract(raw_json, '$.quickOrder') THEN 0 ELSE 1 END,
                         COALESCE(created_at, '') DESC,
                         id DESC
                """,
                (float(now_ts),),
            ).fetchall()
        items = []
        for row in rows:
            try:
                item = json.loads(row["raw_json"])
            except json.JSONDecodeError:
                continue
            item_id = str(item.get("id") or "")
            if not item_id or item_id in processing_set or item_id in deleted_set:
                continue
            items.append(item)
            if limit is not None and len(items) >= int(limit):
                break
        return items

    def count_queue_items(self, account_id=None, status=None):
        clauses = []
        params = []
        if account_id:
            clauses.append("account_id = ?")
            params.append(account_id)
        if status and status != "all":
            clauses.append("LOWER(status) = ?")
            params.append(str(status).lower())
        where_sql = f"WHERE {' AND '.join(clauses)}" if clauses else ""
        with self._lock, self._connect() as conn:
            row = conn.execute(f"SELECT COUNT(*) AS cnt FROM queue_items {where_sql}", tuple(params)).fetchone()
        return int((row["cnt"] if row else 0) or 0)

    def count_queue_items_by_statuses(self, statuses, account_id=None):
        normalized_statuses = [str(status).lower() for status in (statuses or []) if status is not None]
        if not normalized_statuses:
            return 0
        clauses = [f"LOWER(status) IN ({', '.join(['?'] * len(normalized_statuses))})"]
        params = list(normalized_statuses)
        if account_id:
            clauses.append("account_id = ?")
            params.append(account_id)
        where_sql = f"WHERE {' AND '.join(clauses)}"
        with self._lock, self._connect() as conn:
            row = conn.execute(f"SELECT COUNT(*) AS cnt FROM queue_items {where_sql}", tuple(params)).fetchone()
        return int((row["cnt"] if row else 0) or 0)

    def delete_queue_item(self, item_id):
        with self._lock, self._connect() as conn:
            conn.execute("DELETE FROM queue_items WHERE id = ?", (item_id,))

    def delete_queue_items_by_account(self, account_id):
        with self._lock, self._connect() as conn:
            conn.execute("DELETE FROM queue_items WHERE account_id = ?", (account_id,))

    def clear_queue_items(self):
        with self._lock, self._connect() as conn:
            conn.execute("DELETE FROM queue_items")

    def replace_purchase_history(self, items):
        with self._lock, self._connect() as conn:
            conn.execute("DELETE FROM purchase_history")
            for item in items:
                self._insert_purchase_history(conn, item)

    def append_purchase_history(self, item):
        with self._lock, self._connect() as conn:
            self._insert_purchase_history(conn, item)

    def _insert_purchase_history(self, conn, item):
        if not isinstance(item, dict) or not item.get("id"):
            return
        conn.execute(
            """
            INSERT INTO purchase_history(
                id, task_id, account_id, plan_code, status, datacenter, purchase_time, raw_json, updated_at
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
            ON CONFLICT(id) DO UPDATE SET
                task_id = excluded.task_id,
                account_id = excluded.account_id,
                plan_code = excluded.plan_code,
                status = excluded.status,
                datacenter = excluded.datacenter,
                purchase_time = excluded.purchase_time,
                raw_json = excluded.raw_json,
                updated_at = CURRENT_TIMESTAMP
            """,
            (
                item.get("id"),
                item.get("taskId"),
                item.get("accountId"),
                item.get("planCode", ""),
                item.get("status", ""),
                item.get("datacenter"),
                item.get("purchaseTime"),
                json.dumps(item, ensure_ascii=False),
            ),
        )

    def load_purchase_history(self):
        with self._lock, self._connect() as conn:
            rows = conn.execute("SELECT raw_json FROM purchase_history ORDER BY purchase_time, id").fetchall()
        items = []
        for row in rows:
            try:
                items.append(json.loads(row["raw_json"]))
            except json.JSONDecodeError:
                continue
        return items

    def list_purchase_history(self, account_id=None):
        clauses = []
        params = []
        if account_id:
            clauses.append("account_id = ?")
            params.append(account_id)
        where_sql = f"WHERE {' AND '.join(clauses)}" if clauses else ""
        with self._lock, self._connect() as conn:
            rows = conn.execute(
                f"SELECT raw_json FROM purchase_history {where_sql} ORDER BY COALESCE(purchase_time, '') DESC, id DESC",
                tuple(params),
            ).fetchall()
        items = []
        seen = set()
        for row in rows:
            try:
                item = json.loads(row["raw_json"])
            except json.JSONDecodeError:
                continue
            item_id = item.get("id")
            if item_id and item_id in seen:
                continue
            if item_id:
                seen.add(item_id)
            items.append(item)
        return items

    def count_purchase_history_by_status(self, status, account_id=None):
        clauses = ["LOWER(status) = ?"]
        params = [str(status).lower()]
        if account_id:
            clauses.append("account_id = ?")
            params.append(account_id)
        where_sql = f"WHERE {' AND '.join(clauses)}"
        with self._lock, self._connect() as conn:
            row = conn.execute(f"SELECT COUNT(*) AS cnt FROM purchase_history {where_sql}", tuple(params)).fetchone()
        return int((row["cnt"] if row else 0) or 0)

    def delete_purchase_history_by_account(self, account_id):
        with self._lock, self._connect() as conn:
            conn.execute("DELETE FROM purchase_history WHERE account_id = ?", (account_id,))

    def clear_purchase_history(self):
        with self._lock, self._connect() as conn:
            conn.execute("DELETE FROM purchase_history")

    def replace_monitor_subscriptions(self, account_id, subscriptions, known_servers, check_interval):
        normalized_account_id = account_id or "default"
        with self._lock, self._connect() as conn:
            conn.execute("DELETE FROM monitor_subscriptions WHERE account_id = ?", (normalized_account_id,))
            for sub in subscriptions or []:
                if not isinstance(sub, dict):
                    continue
                plan_code = sub.get("planCode")
                if not plan_code:
                    continue
                conn.execute(
                    """
                    INSERT INTO monitor_subscriptions(
                        account_id, plan_code, server_name, datacenters_json,
                        notify_available, notify_unavailable, last_status_json, history_json,
                        auto_order, auto_order_quantity, created_at, raw_json, updated_at
                    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
                    """,
                    (
                        normalized_account_id,
                        plan_code,
                        sub.get("serverName"),
                        json.dumps(sub.get("datacenters") or [], ensure_ascii=False),
                        1 if sub.get("notifyAvailable", True) else 0,
                        1 if sub.get("notifyUnavailable", False) else 0,
                        json.dumps(sub.get("lastStatus") or {}, ensure_ascii=False),
                        json.dumps(sub.get("history") or [], ensure_ascii=False),
                        1 if sub.get("autoOrder", False) else 0,
                        int(sub.get("autoOrderQuantity") or 0),
                        sub.get("createdAt"),
                        json.dumps(sub, ensure_ascii=False),
                    ),
                )
            conn.execute(
                """
                INSERT INTO monitor_states(account_id, known_servers_json, check_interval, updated_at)
                VALUES (?, ?, ?, CURRENT_TIMESTAMP)
                ON CONFLICT(account_id) DO UPDATE SET
                    known_servers_json = excluded.known_servers_json,
                    check_interval = excluded.check_interval,
                    updated_at = CURRENT_TIMESTAMP
                """,
                (
                    normalized_account_id,
                    json.dumps(sorted(list(known_servers or [])), ensure_ascii=False),
                    int(check_interval or 20),
                ),
            )

    def load_monitor_subscriptions(self, account_id):
        normalized_account_id = account_id or "default"
        with self._lock, self._connect() as conn:
            sub_rows = conn.execute(
                "SELECT raw_json FROM monitor_subscriptions WHERE account_id = ? ORDER BY created_at, plan_code",
                (normalized_account_id,),
            ).fetchall()
            state_row = conn.execute(
                "SELECT known_servers_json, check_interval FROM monitor_states WHERE account_id = ?",
                (normalized_account_id,),
            ).fetchone()
        subscriptions = []
        for row in sub_rows:
            try:
                subscriptions.append(json.loads(row["raw_json"]))
            except json.JSONDecodeError:
                continue
        known_servers = []
        check_interval = 20
        if state_row:
            try:
                known_servers = json.loads(state_row["known_servers_json"])
            except json.JSONDecodeError:
                known_servers = []
            check_interval = int(state_row["check_interval"] or 20)
        return {
            "subscriptions": subscriptions,
            "known_servers": known_servers,
            "check_interval": check_interval,
        }

    def delete_monitor_subscriptions(self, account_id):
        normalized_account_id = account_id or "default"
        with self._lock, self._connect() as conn:
            conn.execute("DELETE FROM monitor_subscriptions WHERE account_id = ?", (normalized_account_id,))
            conn.execute("DELETE FROM monitor_states WHERE account_id = ?", (normalized_account_id,))

    def replace_server_plans(self, items):
        with self._lock, self._connect() as conn:
            conn.execute("DELETE FROM server_plans")
            for item in items or []:
                if not isinstance(item, dict):
                    continue
                plan_code = item.get("planCode")
                if not plan_code:
                    continue
                conn.execute(
                    """
                    INSERT INTO server_plans(plan_code, name, memory, storage, raw_json, updated_at)
                    VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
                    """,
                    (
                        plan_code,
                        item.get("name"),
                        item.get("memory"),
                        item.get("storage"),
                        json.dumps(item, ensure_ascii=False),
                    ),
                )

    def load_server_plans(self):
        with self._lock, self._connect() as conn:
            rows = conn.execute("SELECT raw_json FROM server_plans ORDER BY plan_code").fetchall()
        items = []
        for row in rows:
            try:
                items.append(json.loads(row["raw_json"]))
            except json.JSONDecodeError:
                continue
        return items

    def save_snapshot(self, snapshot_type, payload):
        with self._lock, self._connect() as conn:
            conn.execute(
                """
                INSERT INTO snapshots(snapshot_type, updated_at_value, raw_json, updated_at)
                VALUES (?, ?, ?, CURRENT_TIMESTAMP)
                ON CONFLICT(snapshot_type) DO UPDATE SET
                    updated_at_value = excluded.updated_at_value,
                    raw_json = excluded.raw_json,
                    updated_at = CURRENT_TIMESTAMP
                """,
                (
                    snapshot_type,
                    (payload or {}).get("updatedAt") if isinstance(payload, dict) else None,
                    json.dumps(payload, ensure_ascii=False),
                ),
            )

    def load_snapshot(self, snapshot_type):
        with self._lock, self._connect() as conn:
            row = conn.execute(
                "SELECT raw_json FROM snapshots WHERE snapshot_type = ?",
                (snapshot_type,),
            ).fetchone()
        if not row:
            return None
        try:
            return json.loads(row["raw_json"])
        except json.JSONDecodeError:
            return None

    def replace_feishu_user_bindings(self, items):
        with self._lock, self._connect() as conn:
            conn.execute("DELETE FROM feishu_user_bindings")
            for account_id, payload in (items or {}).items():
                if not isinstance(payload, dict):
                    continue
                conn.execute(
                    """
                    INSERT INTO feishu_user_bindings(account_id, open_id, user_name, raw_json, updated_at)
                    VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
                    """,
                    (
                        account_id,
                        payload.get("open_id", ""),
                        payload.get("user_name", ""),
                        json.dumps(payload, ensure_ascii=False),
                    ),
                )

    def load_feishu_user_bindings(self):
        with self._lock, self._connect() as conn:
            rows = conn.execute("SELECT account_id, raw_json FROM feishu_user_bindings ORDER BY account_id").fetchall()
        result = {}
        for row in rows:
            try:
                result[row["account_id"]] = json.loads(row["raw_json"])
            except json.JSONDecodeError:
                continue
        return result

    def replace_bot_account_selections(self, items):
        with self._lock, self._connect() as conn:
            conn.execute("DELETE FROM bot_account_selections")
            for composite_key, payload in (items or {}).items():
                if not isinstance(payload, dict) or ":" not in str(composite_key):
                    continue
                channel, user_key = str(composite_key).split(":", 1)
                conn.execute(
                    """
                    INSERT INTO bot_account_selections(channel, user_key, account_id, updated_at_value, raw_json, updated_at)
                    VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
                    """,
                    (
                        channel,
                        user_key,
                        payload.get("accountId", ""),
                        payload.get("updatedAt"),
                        json.dumps(payload, ensure_ascii=False),
                    ),
                )

    def load_bot_account_selections(self):
        with self._lock, self._connect() as conn:
            rows = conn.execute("SELECT channel, user_key, raw_json FROM bot_account_selections ORDER BY channel, user_key").fetchall()
        result = {}
        for row in rows:
            try:
                result[f"{row['channel']}:{row['user_key']}"] = json.loads(row["raw_json"])
            except json.JSONDecodeError:
                continue
        return result

    def replace_server_aliases(self, items):
        with self._lock, self._connect() as conn:
            conn.execute("DELETE FROM server_aliases")
            for composite_key, alias in (items or {}).items():
                if ":" not in str(composite_key):
                    continue
                account_id, service_name = str(composite_key).split(":", 1)
                conn.execute(
                    """
                    INSERT INTO server_aliases(account_id, service_name, alias, updated_at)
                    VALUES (?, ?, ?, CURRENT_TIMESTAMP)
                    """,
                    (account_id, service_name, str(alias)),
                )

    def load_server_aliases(self):
        with self._lock, self._connect() as conn:
            rows = conn.execute("SELECT account_id, service_name, alias FROM server_aliases ORDER BY account_id, service_name").fetchall()
        return {f"{row['account_id']}:{row['service_name']}": row["alias"] for row in rows}

    def replace_bot_pending_actions(self, items):
        with self._lock, self._connect() as conn:
            conn.execute("DELETE FROM bot_pending_actions")
            for composite_key, payload in (items or {}).items():
                if not isinstance(payload, dict) or ":" not in str(composite_key):
                    continue
                channel, user_key = str(composite_key).split(":", 1)
                conn.execute(
                    """
                    INSERT INTO bot_pending_actions(channel, user_key, raw_json, updated_at)
                    VALUES (?, ?, ?, CURRENT_TIMESTAMP)
                    """,
                    (channel, user_key, json.dumps(payload, ensure_ascii=False)),
                )

    def load_bot_pending_actions(self):
        with self._lock, self._connect() as conn:
            rows = conn.execute("SELECT channel, user_key, raw_json FROM bot_pending_actions ORDER BY channel, user_key").fetchall()
        result = {}
        for row in rows:
            try:
                result[f"{row['channel']}:{row['user_key']}"] = json.loads(row["raw_json"])
            except json.JSONDecodeError:
                continue
        return result

    def replace_config_sniper_tasks(self, items):
        with self._lock, self._connect() as conn:
            conn.execute("DELETE FROM config_sniper_tasks")
            for item in items or []:
                if not isinstance(item, dict) or not item.get("id"):
                    continue
                conn.execute(
                    """
                    INSERT INTO config_sniper_tasks(id, api1_plan_code, match_status, enabled, raw_json, updated_at)
                    VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
                    """,
                    (
                        item.get("id"),
                        item.get("api1_planCode"),
                        item.get("match_status"),
                        1 if item.get("enabled", True) else 0,
                        json.dumps(item, ensure_ascii=False),
                    ),
                )

    def load_config_sniper_tasks(self):
        with self._lock, self._connect() as conn:
            rows = conn.execute("SELECT raw_json FROM config_sniper_tasks ORDER BY updated_at, id").fetchall()
        items = []
        for row in rows:
            try:
                items.append(json.loads(row["raw_json"]))
            except json.JSONDecodeError:
                continue
        return items

    def replace_vps_subscriptions(self, subscriptions, check_interval):
        with self._lock, self._connect() as conn:
            conn.execute("DELETE FROM vps_subscriptions")
            for item in subscriptions or []:
                if not isinstance(item, dict) or not item.get("id"):
                    continue
                conn.execute(
                    """
                    INSERT INTO vps_subscriptions(id, plan_code, ovh_subsidiary, raw_json, updated_at)
                    VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
                    """,
                    (
                        item.get("id"),
                        item.get("planCode", ""),
                        item.get("ovhSubsidiary", ""),
                        json.dumps(item, ensure_ascii=False),
                    ),
                )
            conn.execute(
                """
                INSERT INTO vps_monitor_state(key, value, updated_at)
                VALUES ('check_interval', ?, CURRENT_TIMESTAMP)
                ON CONFLICT(key) DO UPDATE SET
                    value = excluded.value,
                    updated_at = CURRENT_TIMESTAMP
                """,
                (json.dumps(int(check_interval or 60)),),
            )

    def load_vps_subscriptions(self):
        with self._lock, self._connect() as conn:
            rows = conn.execute("SELECT raw_json FROM vps_subscriptions ORDER BY updated_at, id").fetchall()
            row = conn.execute("SELECT value FROM vps_monitor_state WHERE key = 'check_interval'").fetchone()
        subscriptions = []
        for entry in rows:
            try:
                subscriptions.append(json.loads(entry["raw_json"]))
            except json.JSONDecodeError:
                continue
        check_interval = 60
        if row:
            try:
                check_interval = int(json.loads(row["value"]))
            except Exception:
                check_interval = 60
        return {
            "subscriptions": subscriptions,
            "check_interval": check_interval,
        }
