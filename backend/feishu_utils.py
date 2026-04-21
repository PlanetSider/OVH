import base64
import hashlib
import hmac
import json
import time
from typing import Any, Callable, Dict, Optional

import requests


class FeishuClient:
    def __init__(self, get_config: Callable[[], Dict[str, Any]], add_log: Callable[[str, str, str], None]):
        self.get_config = get_config
        self.add_log = add_log
        self._tenant_access_token = ""
        self._token_expire_at = 0.0

    def _cfg(self) -> Dict[str, Any]:
        return self.get_config() or {}

    def is_enabled(self) -> bool:
        cfg = self._cfg()
        return bool(cfg.get("feishuEnabled") and cfg.get("feishuAppId") and cfg.get("feishuAppSecret"))

    def verify_signature(self, timestamp: str, nonce: str, body: bytes, signature: str) -> bool:
        cfg = self._cfg()
        encrypt_key = cfg.get("feishuEncryptKey") or ""
        if not encrypt_key:
            return True
        string_to_sign = timestamp + nonce + encrypt_key + body.decode("utf-8")
        expect = base64.b64encode(hashlib.sha256(string_to_sign.encode("utf-8")).digest()).decode("utf-8")
        return hmac.compare_digest(expect, signature or "")

    def verify_token(self, token: str) -> bool:
        cfg = self._cfg()
        verify_token = cfg.get("feishuVerificationToken") or ""
        if not verify_token:
            return True
        return hmac.compare_digest(verify_token, token or "")

    def _fetch_tenant_access_token(self) -> str:
        cfg = self._cfg()
        url = "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal"
        payload = {
            "app_id": cfg.get("feishuAppId", ""),
            "app_secret": cfg.get("feishuAppSecret", "")
        }
        response = requests.post(url, json=payload, timeout=10)
        response.raise_for_status()
        data = response.json()
        if data.get("code") != 0:
            raise RuntimeError(data.get("msg") or "failed to get tenant access token")
        token = data.get("tenant_access_token") or ""
        expire = int(data.get("expire", 0) or 0)
        self._tenant_access_token = token
        self._token_expire_at = time.time() + max(0, expire - 120)
        return token

    def get_tenant_access_token(self) -> str:
        if self._tenant_access_token and time.time() < self._token_expire_at:
            return self._tenant_access_token
        return self._fetch_tenant_access_token()

    def _request(self, method: str, url: str, *, params: Optional[Dict[str, Any]] = None, json_body: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        token = self.get_tenant_access_token()
        headers = {
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/json; charset=utf-8"
        }
        response = requests.request(method, url, params=params, json=json_body, headers=headers, timeout=10)
        response.raise_for_status()
        data = response.json()
        if data.get("code") != 0:
            raise RuntimeError(data.get("msg") or "feishu api error")
        return data

    def send_text(self, receive_id: str, text: str, receive_id_type: str = "open_id") -> Dict[str, Any]:
        content = json.dumps({"text": text}, ensure_ascii=False)
        return self._request(
            "POST",
            "https://open.feishu.cn/open-apis/im/v1/messages",
            params={"receive_id_type": receive_id_type},
            json_body={
                "receive_id": receive_id,
                "msg_type": "text",
                "content": content
            }
        )

    def send_card(self, receive_id: str, card: Dict[str, Any], receive_id_type: str = "open_id") -> Dict[str, Any]:
        content = json.dumps(card, ensure_ascii=False)
        return self._request(
            "POST",
            "https://open.feishu.cn/open-apis/im/v1/messages",
            params={"receive_id_type": receive_id_type},
            json_body={
                "receive_id": receive_id,
                "msg_type": "interactive",
                "content": content
            }
        )
